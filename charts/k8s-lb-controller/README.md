# k8s-lb-controller Helm Chart

This chart installs `k8s-lb-controller` and supports both controller provider modes available in the repository today:

- `local-haproxy`: the default controller-only mode
- `dataplane-api`: the controller plus a separate dataplane deployment and service

Russian version: [README.ru.md](README.ru.md)

Repository overview: [../../README.md](../../README.md)

## What The Chart Deploys

Always:

- controller `Deployment`
- `ServiceAccount`
- RBAC for `Service`, `Service/status`, and `EndpointSlice`
- leader-election `Role` and `RoleBinding`
- optional metrics `Service`
- optional `ServiceMonitor`

When `dataplane.enabled=true`:

- dataplane `Deployment`
- dataplane `Service`
- HAProxy sidecar in the dataplane pod

By default, the chart deploys only the controller. Dataplane resources are added only when dataplane mode is enabled.

## Installation

OCI chart reference:

`oci://ghcr.io/voronkov44/charts/k8s-lb-controller`

Install local mode from this repository checkout:

```bash
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system \
  --create-namespace
```

Install dataplane mode from this repository checkout:

```bash
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system \
  --create-namespace \
  --set controller.providerMode=dataplane-api \
  --set dataplane.enabled=true
```

If you prefer the published OCI chart, replace `./charts/k8s-lb-controller` with the OCI reference above and add `--version 0.1.0`.

## Important Values

### Controller Values

| Value | Description |
| --- | --- |
| `image.repository`, `image.tag`, `image.pullPolicy` | Controller image settings. |
| `controller.providerMode` | `local-haproxy` or `dataplane-api`. |
| `controller.loadBalancerClass` | Managed `spec.loadBalancerClass`. |
| `controller.ipPool` | Static IPv4 pool used for external address allocation. |
| `controller.dataplane.apiURL` | Optional explicit dataplane API URL override. |
| `controller.dataplane.apiTimeout` | Controller-side timeout for dataplane API requests. |
| `controller.haproxy.*` | Local HAProxy provider settings used in `local-haproxy` mode. |

### Dataplane Values

| Value | Description |
| --- | --- |
| `dataplane.enabled` | Enables the dataplane `Deployment` and `Service`. |
| `dataplane.image.repository`, `dataplane.image.tag`, `dataplane.image.pullPolicy` | Dataplane API image settings. |
| `dataplane.http.port`, `dataplane.http.addr` | Dataplane API listener address and service port. |
| `dataplane.hostNetwork`, `dataplane.shareProcessNamespace` | Runtime wiring for the in-cluster dataplane rollout. |
| `dataplane.interface` | Host interface used for external IP attachment in dataplane mode. |
| `dataplane.ipAttach.enabled` | Enables host-side IP attachment in the dataplane rollout. |
| `dataplane.ipAttach.mode` | External IP attachment backend: `netlink` or `exec`. |
| `dataplane.ipAttach.command`, `dataplane.ipAttach.cidrSuffix` | Exec-backend command and attached CIDR width. |
| `dataplane.haproxy.image.*` | HAProxy sidecar image settings. |
| `dataplane.haproxy.configPath`, `dataplane.haproxy.pidFile` | Shared runtime files used by the dataplane API container and HAProxy sidecar. |
| `dataplane.haproxy.validateCommand`, `dataplane.haproxy.reloadCommand` | Commands used during config validation and atomic reload. |
| `dataplane.logLevel`, `dataplane.gracefulShutdownTimeout` | Dataplane runtime behavior. |
| `dataplane.resources`, `dataplane.nodeSelector`, `dataplane.tolerations`, `dataplane.affinity` | Resource and scheduling settings for the dataplane API pod. |

### Metrics and Monitoring Values

| Value | Description |
| --- | --- |
| `metrics.service.enabled` | Creates the metrics `Service`. |
| `metrics.serviceMonitor.enabled` | Creates a `ServiceMonitor` when Prometheus Operator integration is wanted. |

## Dataplane URL Wiring

When all of the following are true:

- `controller.providerMode=dataplane-api`
- `dataplane.enabled=true`
- `controller.dataplane.apiURL` is empty

the chart generates the controller-side dataplane URL automatically as:

`http://<release>-k8s-lb-controller-dataplane.<namespace>.svc:<dataplane.http.port>`

If `controller.dataplane.apiURL` is set explicitly, that value overrides the generated in-cluster service URL.

The chart also validates that `controller.providerMode=dataplane-api` must be paired with either `dataplane.enabled=true` or a non-empty `controller.dataplane.apiURL`.

## Example Values

### Local Mode

```yaml
controller:
  providerMode: local-haproxy
  loadBalancerClass: lab.local/service-lb
  ipPool:
    - 203.0.113.10
    - 203.0.113.11
    - 203.0.113.12
```

### Dataplane Mode

```yaml
controller:
  providerMode: dataplane-api
  dataplane:
    apiTimeout: 10s

dataplane:
  enabled: true
  interface: eth0
  ipAttach:
    enabled: true
    mode: netlink
  http:
    port: 8090
  haproxy:
    configPath: /var/run/k8s-lb-dataplane/haproxy.cfg
    pidFile: /var/run/k8s-lb-dataplane/haproxy.pid
```

## Scope and Limitations

- Local mode remains available, backward-compatible, and is still the chart default.
- Dataplane mode deploys a standalone dataplane API plus an HAProxy sidecar. It does not create a distributed multi-node dataplane system.
- In the current chart defaults, dataplane mode uses host networking, `shareProcessNamespace`, and host-side external IP attachment.
- `dataplane.ipAttach.mode` defaults to `netlink`; `exec` remains available as a fallback.
- The current dataplane rollout is intended for controlled single-node and lab environments.
- Multi-node or HA dataplane coordination, BGP, ARP, NDP, cloud-provider-style publication, and broader production hardening are intentionally outside the current scope.

## Verification

```bash
helm lint ./charts/k8s-lb-controller
helm lint ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
helm template k8s-lb-controller ./charts/k8s-lb-controller
helm template k8s-lb-controller ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
make verify-dataplane
make smoke-dataplane-kind
```

For detailed controlled-environment validation and release-readiness guidance, see:

- Smoke validation: [../../docs/dataplane-smoke.md](../../docs/dataplane-smoke.md), [../../docs/dataplane-smoke.ru.md](../../docs/dataplane-smoke.ru.md)
- Release-readiness checklist: [../../docs/release-checklist.md](../../docs/release-checklist.md), [../../docs/release-checklist.ru.md](../../docs/release-checklist.ru.md)

## Uninstall

```bash
helm uninstall k8s-lb-controller -n k8s-lb-controller-system
```
