package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/edorid/proxmox-host-labeler/internal/proxmox"
)

const reconcileInterval = 5 * time.Minute

type NodeReconciler struct {
	client.Client
	API      proxmox.API
	LabelKey string
	Recorder record.EventRecorder
}

func (r *NodeReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: request.Name}, node); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	vms, err := r.API.ListVMs(ctx)
	if err != nil {
		logger.Error(err, "could not list Proxmox VMs; retaining existing host label")
		r.Recorder.Event(node, corev1.EventTypeWarning, "ProxmoxUnavailable", "Could not confirm Proxmox host placement; retaining existing label")
		return ctrl.Result{RequeueAfter: reconcileInterval}, nil
	}
	vm, matched, reason := MatchNode(node, vms)
	if !matched {
		logger.Info("Node placement not updated", "reason", reason)
		r.Recorder.Event(node, corev1.EventTypeWarning, "PlacementUnconfirmed", reason+"; retaining existing host label")
		return ctrl.Result{RequeueAfter: reconcileInterval}, nil
	}
	if !vm.GuestIPAvailable {
		logger.Info("guest-agent IP data unavailable; accepting unique name and cluster-tag match", "vm", vm.Name)
		r.Recorder.Event(node, corev1.EventTypeWarning, "GuestIPUnavailable", "Guest-agent IP data unavailable; placement matched by VM name and cluster tag")
	}
	if vm.Node == "" {
		return ctrl.Result{RequeueAfter: reconcileInterval}, nil
	}
	if node.Labels[r.LabelKey] != vm.Node {
		original := node.DeepCopy()
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		node.Labels[r.LabelKey] = vm.Node
		if err := r.Patch(ctx, node, client.MergeFrom(original)); err != nil {
			return ctrl.Result{RequeueAfter: reconcileInterval}, err
		}
		r.Recorder.Event(node, corev1.EventTypeNormal, "HostPlacementUpdated", "Updated physical Proxmox host placement label")
	}
	return ctrl.Result{RequeueAfter: reconcileInterval}, nil
}

func (r *NodeReconciler) SetupWithManager(manager ctrl.Manager) error {
	if problems := validation.IsQualifiedName(r.LabelKey); len(problems) > 0 {
		return fmt.Errorf("invalid host label key %q: %s", r.LabelKey, strings.Join(problems, "; "))
	}
	return ctrl.NewControllerManagedBy(manager).
		For(&corev1.Node{}).
		Complete(r)
}
