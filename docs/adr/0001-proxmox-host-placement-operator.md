# ADR 0001: Track Proxmox physical host placement on Kubernetes Nodes

## Status

Accepted

## Context

Kubernetes Nodes run as virtual machines managed by Rancher on Proxmox. Workloads need to make their own topology-aware scheduling decisions using the VM's current physical Proxmox host. Physical placement can change due to VM migration, so Node labels need periodic reconciliation.

The operator must identify a VM without assuming Rancher places a Proxmox VM ID on each Node. Node and VM names alone are not sufficient identity when names are missing or ambiguous. Stale labels can mislead scheduling, but removing them during a transient Proxmox outage also discards useful last-known state.

## Decision

Build a Go Kubernetes operator, installed by Helm, with one active replica per Kubernetes cluster. It connects to one Proxmox cluster using credentials from an existing Kubernetes Secret and verifies TLS by default.

At startup, when Nodes appear, and every five minutes, it reconciles Node placement. A Node is eligible only when its Node name exactly matches one Proxmox VM name, that VM has the tag `rancher-cluster-<cluster-name>`, and the cluster name is consistently available as the Node's `cluster.x-k8s.io/cluster-name` label or annotation. Missing or conflicting cluster metadata, absent cluster tags, and ambiguous VM names are not eligible for labeling.

The operator requires a Node `InternalIP`. If Proxmox guest-agent IPs are available, at least one must match a Node InternalIP. If guest-agent IP data is temporarily unavailable or contains no usable addresses, a unique name-and-cluster-tag match may still be used, with a warning. A confirmed IP mismatch prevents an update.

On a successful match, the operator sets a configurable, operator-owned Node label containing the physical host name. The default key is `edorid.github.io/proxmox-host`. It may overwrite a different value under that configured key. On API errors or unconfirmed matches, it preserves any existing value and emits logs and Kubernetes Events. Uninstalling the operator does not remove labels.

The operator only publishes placement. Workloads independently choose preferred or required scheduling policies; documentation will include examples and explain that label updates do not move already-running Pods.

## Consequences

- Periodic reconciliation bounds normal migration detection delay to approximately five minutes, plus API and reconciliation time.
- Retaining labels during outages avoids removing known placement but can leave stale topology data. Consumers must account for this trade-off.
- A unique VM name plus cluster tag and optional guest-agent IP comparison provides the agreed identity checks without requiring a VMID on the Node.
- Nodes missing required metadata or Node InternalIP remain unlabeled or retain their previous label, and the reason is observable through logs and Events.
- The label key must be configurable; the default domain should be verified as controlled by the project owner before production use.
- A single replica keeps initial deployment simple but has no active-active availability during operator downtime; reconciliation resumes after restart.
- The operator does not modify workload affinity, placement, or existing Pod locations.
