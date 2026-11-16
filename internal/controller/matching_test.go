package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/edorid/proxmox-host-labeler/internal/proxmox"
)

func TestMatchNode(t *testing.T) {
	tests := []struct {
		name       string
		node       *corev1.Node
		vms        []proxmox.VM
		wantMatch  bool
		wantReason string
	}{
		{
			name: "matches exact name cluster tag and guest IP",
			node: testNode("worker-1", "blue", "10.0.0.12"),
			vms: []proxmox.VM{{Name: "worker-1", Node: "pve-2", Tags: []string{"rancher-cluster-blue"}, GuestIPs: []string{"10.0.0.12"}, GuestIPAvailable: true}},
			wantMatch: true,
		},
		{
			name: "permits unavailable guest IP data",
			node: testNode("worker-1", "blue", "10.0.0.12"),
			vms: []proxmox.VM{{Name: "worker-1", Node: "pve-2", Tags: []string{"rancher-cluster-blue"}}},
			wantMatch: true,
		},
		{
			name: "rejects confirmed IP mismatch",
			node: testNode("worker-1", "blue", "10.0.0.12"),
			vms: []proxmox.VM{{Name: "worker-1", Node: "pve-2", Tags: []string{"rancher-cluster-blue"}, GuestIPs: []string{"10.0.0.99"}, GuestIPAvailable: true}},
			wantReason: "Node InternalIP does not match Proxmox guest-agent IPs",
		},
		{
			name: "rejects ambiguous VM names",
			node: testNode("worker-1", "blue", "10.0.0.12"),
			vms: []proxmox.VM{
				{Name: "worker-1", Node: "pve-1", Tags: []string{"rancher-cluster-blue"}},
				{Name: "worker-1", Node: "pve-2", Tags: []string{"rancher-cluster-blue"}},
			},
			wantReason: "multiple VMs matched Node name and Rancher cluster tag",
		},
		{
			name: "rejects absent required cluster tag",
			node: testNode("worker-1", "blue", "10.0.0.12"),
			vms: []proxmox.VM{{Name: "worker-1", Node: "pve-2", Tags: []string{"other"}}},
			wantReason: "no VM matched Node name and Rancher cluster tag",
		},
		{
			name: "rejects missing Node InternalIP",
			node: testNode("worker-1", "blue", ""),
			vms: []proxmox.VM{{Name: "worker-1", Node: "pve-2", Tags: []string{"rancher-cluster-blue"}}},
			wantReason: "Node has no InternalIP",
		},
		{
			name: "rejects conflicting cluster metadata",
			node: func() *corev1.Node {
				n := testNode("worker-1", "blue", "10.0.0.12")
				n.Annotations = map[string]string{clusterNameKey: "green"}
				return n
			}(),
			wantReason: "Node cluster-name label and annotation conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, matched, reason := MatchNode(tt.node, tt.vms)
			if matched != tt.wantMatch {
				t.Fatalf("MatchNode() matched = %v, want %v", matched, tt.wantMatch)
			}
			if reason != tt.wantReason {
				t.Fatalf("MatchNode() reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func testNode(name, clusterName, internalIP string) *corev1.Node {
	node := &corev1.Node{}
	node.Name = name
	node.Labels = map[string]string{clusterNameKey: clusterName}
	if internalIP != "" {
		node.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: internalIP}}
	}
	return node
}
