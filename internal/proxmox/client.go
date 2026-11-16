package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type VM struct {
	ID       int
	Name     string
	Node     string
	Tags     []string
	GuestIPs []string
	GuestIPAvailable bool
}

type API interface {
	ListVMs(context.Context) ([]VM, error)
}

type Client struct {
	baseURL string
	tokenID string
	tokenSecret string
	httpClient *http.Client
	cacheMu sync.Mutex
	cachedVMs []VM
	cacheUntil time.Time
}

func NewClient(baseURL, tokenID, tokenSecret string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), tokenID: tokenID, tokenSecret: tokenSecret, httpClient: httpClient}
}

type resource struct {
	Type string `json:"type"`
	VMID int `json:"vmid"`
	Name string `json:"name"`
	Node string `json:"node"`
	Status string `json:"status"`
}

type vmConfig struct {
	Name string `json:"name"`
	Tags string `json:"tags"`
}

type agentResult struct {
	Result []struct {
		Name string `json:"name"`
		IPAddresses []struct {
			IP string `json:"ip-address"`
			Type string `json:"ip-address-type"`
		} `json:"ip-addresses"`
	} `json:"result"`
}

func (c *Client) ListVMs(ctx context.Context) ([]VM, error) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if time.Now().Before(c.cacheUntil) {
		return append([]VM(nil), c.cachedVMs...), nil
	}
	vms, err := c.listVMs(ctx)
	if err != nil {
		return nil, err
	}
	c.cachedVMs = append([]VM(nil), vms...)
	c.cacheUntil = time.Now().Add(30 * time.Second)
	return vms, nil
}

func (c *Client) listVMs(ctx context.Context) ([]VM, error) {
	var resources []resource
	if err := c.get(ctx, "/cluster/resources?type=vm", &resources); err != nil {
		return nil, fmt.Errorf("list Proxmox VMs: %w", err)
	}
	vms := make([]VM, 0, len(resources))
	for _, item := range resources {
		if item.Type != "qemu" || item.VMID == 0 || item.Name == "" || item.Node == "" {
			continue
		}
		if item.Status != "running" {
			continue
		}
		vm := VM{ID: item.VMID, Name: item.Name, Node: item.Node}
		var config vmConfig
		path := fmt.Sprintf("/nodes/%s/qemu/%d/config", url.PathEscape(item.Node), item.VMID)
		if err := c.get(ctx, path, &config); err != nil {
			return nil, fmt.Errorf("read configuration for VM %d: %w", item.VMID, err)
		}
		if config.Name != "" {
			vm.Name = config.Name
		}
		vm.Tags = splitTags(config.Tags)
		ips, err := c.guestIPs(ctx, item.Node, item.VMID)
		if err == nil {
			vm.GuestIPs = ips
			vm.GuestIPAvailable = len(ips) > 0
		}
		// Guest-agent errors are non-fatal: caller treats absent IP data as unavailable.
		vms = append(vms, vm)
	}
	return vms, nil
}

func (c *Client) guestIPs(ctx context.Context, node string, vmid int) ([]string, error) {
	var result agentResult
	path := fmt.Sprintf("/nodes/%s/qemu/%d/agent/network-get-interfaces", url.PathEscape(node), vmid)
	if err := c.get(ctx, path, &result); err != nil {
		return nil, err
	}
	var ips []string
	for _, iface := range result.Result {
		for _, address := range iface.IPAddresses {
			if address.Type == "ipv4" || address.Type == "ipv6" {
				ip := net.ParseIP(address.IP)
				if ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() {
					ips = append(ips, ip.String())
				}
			}
		}
	}
	return ips, nil
}

func (c *Client) get(ctx context.Context, path string, output any) error {
	u, err := url.Parse(c.baseURL + "/api2/json" + path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "PVEAPIToken="+c.tokenID+"="+c.tokenSecret)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Proxmox API returned %s", resp.Status)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("decode Proxmox API response: %w", err)
	}
	return nil
}

func splitTags(tags string) []string {
	var result []string
	for _, tag := range strings.Split(tags, ";") {
		if tag = strings.TrimSpace(tag); tag != "" {
			result = append(result, tag)
		}
	}
	return result
}
