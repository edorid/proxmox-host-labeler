package controller

import (
	"net"

	corev1 "k8s.io/api/core/v1"
	"github.com/edorid/proxmox-host-labeler/internal/proxmox"
)

const clusterNameKey = "cluster.x-k8s.io/cluster-name"

// MatchNode returns the uniquely identified VM for a Node, or an explanation
// suitable for an operator-facing warning. Guest IP data is additional
// verification when available, not a prerequisite for matching.
func MatchNode(node *corev1.Node, vms []proxmox.VM) (proxmox.VM, bool, string) {
	clusterName, reason := nodeClusterName(node)
	if reason != "" {
		return proxmox.VM{}, false, reason
	}
	var matches []proxmox.VM
	for _, vm := range vms {
		if vm.Name == node.Name && hasTag(vm.Tags, "rancher-cluster-"+clusterName) {
			matches = append(matches, vm)
		}
	}
	if len(matches) == 0 {
		return proxmox.VM{}, false, "no VM matched Node name and Rancher cluster tag"
	}
	if len(matches) > 1 {
		return proxmox.VM{}, false, "multiple VMs matched Node name and Rancher cluster tag"
	}
	var nodeIPs []string
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP && address.Address != "" {
			if ip := net.ParseIP(address.Address); ip != nil {
				nodeIPs = append(nodeIPs, ip.String())
			}
		}
	}
	if len(nodeIPs) == 0 {
		return proxmox.VM{}, false, "Node has no InternalIP"
	}
	vm := matches[0]
	if vm.GuestIPAvailable && !overlaps(nodeIPs, vm.GuestIPs) {
		return proxmox.VM{}, false, "Node InternalIP does not match Proxmox guest-agent IPs"
	}
	return vm, true, ""
}

func nodeClusterName(node *corev1.Node) (string, string) {
	label := node.Labels[clusterNameKey]
	annotation := node.Annotations[clusterNameKey]
	if label != "" && annotation != "" && label != annotation {
		return "", "Node cluster-name label and annotation conflict"
	}
	if label == "" {
		label = annotation
	}
	if label == "" {
		return "", "Node has no cluster.x-k8s.io/cluster-name label or annotation"
	}
	return label, ""
}

func hasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}

func overlaps(left, right []string) bool {
	set := make(map[string]struct{}, len(left))
	for _, item := range left {
		set[item] = struct{}{}
	}
	for _, item := range right {
		if _, ok := set[item]; ok {
			return true
		}
	}
	return false
}
