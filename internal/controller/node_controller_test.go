package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/edorid/proxmox-host-labeler/internal/proxmox"
)

type apiStub struct {
	vms []proxmox.VM
	err error
}

func (a apiStub) ListVMs(context.Context) ([]proxmox.VM, error) {
	return a.vms, a.err
}

func TestReconcileUpdatesPlacementAndRetainsUnconfirmedValue(t *testing.T) {
	tests := []struct {
		name string
		api apiStub
		initialHost string
		wantHost string
	}{
		{
			name: "updates label on confirmed placement",
			api: apiStub{vms: []proxmox.VM{{Name: "worker-1", Node: "pve-2", Tags: []string{"rancher-cluster-blue"}}}},
			wantHost: "pve-2",
		},
		{
			name: "retains label when placement is unconfirmed",
			api: apiStub{vms: []proxmox.VM{{Name: "worker-1", Node: "pve-2", Tags: []string{"rancher-cluster-blue"}, GuestIPs: []string{"10.0.0.99"}, GuestIPAvailable: true}}},
			initialHost: "pve-1",
			wantHost: "pve-1",
		},
		{
			name: "retains label during Proxmox failure",
			api: apiStub{err: errors.New("unavailable")},
			initialHost: "pve-1",
			wantHost: "pve-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			node := testNode("worker-1", "blue", "10.0.0.12")
			if tt.initialHost != "" {
				node.Labels["edorid.github.io/proxmox-host"] = tt.initialHost
			}
			kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
			reconciler := &NodeReconciler{
				Client: kubeClient,
				API: tt.api,
				LabelKey: "edorid.github.io/proxmox-host",
				Recorder: record.NewFakeRecorder(10),
			}

			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if result.RequeueAfter != 5*time.Minute {
				t.Fatalf("Reconcile() requeue = %s, want 5m", result.RequeueAfter)
			}
			updated := &corev1.Node{}
			if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: node.Name}, updated); err != nil {
				t.Fatal(err)
			}
			if got := updated.Labels["edorid.github.io/proxmox-host"]; got != tt.wantHost {
				t.Fatalf("host label = %q, want %q", got, tt.wantHost)
			}
		})
	}
}
