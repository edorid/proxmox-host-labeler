# Proxmox Host Labeler

A Kubernetes operator that labels Nodes with the physical Proxmox host currently running their VM. It publishes placement only; workload affinity and replica spreading remain the responsibility of each workload.

## How matching works

For each Node, the operator requires:

1. A `cluster.x-k8s.io/cluster-name` value on either the Node label or annotation. If both exist, they must agree.
2. A unique Proxmox QEMU VM whose configured name exactly equals the Kubernetes Node name and whose tags include `rancher-cluster-<cluster-name>`.
3. At least one Node `InternalIP`.

When Proxmox guest-agent IPs are available, at least one must overlap the Node's InternalIP addresses. If guest-agent data is temporarily unavailable or has no usable addresses, the unique name and cluster-tag match is accepted with a warning. A confirmed IP mismatch is not accepted.

The physical host is the Proxmox node reported for the VM by the Proxmox cluster resource API. The default Node label is `edorid.github.io/proxmox-host`; configure a domain you control before production use. Placement is reconciled at startup, on Node events, and at five-minute intervals. Proxmox failures and unconfirmed matches retain the previous label and emit a Kubernetes Event and log message. Labels are not removed on uninstall.

## Proxmox API permissions

Create a Proxmox API token with read-only access sufficient to list cluster resources, read VM configuration, and query the QEMU guest agent. A commonly used minimal role for the Proxmox CCM is `Sys.Audit`, `VM.Audit`, and `VM.GuestAgent.Audit`; verify permissions against your Proxmox version and token privilege-separation settings.

## Install

Create a Secret in the namespace where the chart will be installed. The token ID has the Proxmox form `user@realm!token-name`.

```sh
kubectl create namespace host-labeler
kubectl -n host-labeler create secret generic proxmox-host-labeler \
  --from-literal=token-id='kubernetes@pve!host-labeler' \
  --from-literal=token-secret='<token-secret>'
```

Install the chart from the repository checkout:

```sh
helm upgrade --install proxmox-host-labeler ./deploy/helm/proxmox-host-labeler \
  --namespace host-labeler \
  --set proxmox.url='https://proxmox.example.com:8006' \
  --set hostLabelKey='edorid.github.io/proxmox-host'
```

TLS certificate verification is enabled by default. Configure Proxmox with a certificate trusted by the operator's system trust store. `proxmox.insecureTLS=true` is available for testing only.

## Scheduling example

Prefer spreading replicas across physical hosts while still allowing scheduling when necessary:

```yaml
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                topologyKey: edorid.github.io/proxmox-host
                labelSelector:
                  matchLabels:
                    app.kubernetes.io/name: my-service
```

Kubernetes uses the Node label for future scheduling decisions; changing a Node's host label does not move Pods already running on it. A retained label can be stale during a Proxmox outage, so required anti-affinity may leave Pods pending if topology information is incomplete or no alternative host is available.

## Development

Requires Go 1.23+ and Docker for image builds. The repository pins the Go toolchain line in `mise.toml`.

```sh
make test
make build
make docker-build
```
