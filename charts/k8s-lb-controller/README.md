# k8s-lb-controller Helm Chart

This chart installs `k8s-lb-controller` and supports both deployment modes introduced by the repository:

- `local-haproxy`: controller-only mode, still the default
- `dataplane-api`: controller + standalone dataplane server mode

[Русская версия](README.ru.md)

Repository-level architecture and rollout context are documented in [README.md](../../README.md).

## What The Chart Installs

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
- an HAProxy sidecar in the dataplane pod

## Installation

OCI chart reference:

`oci://ghcr.io/voronkov44/charts/k8s-lb-controller`

### Local Mode

```bash
helm install k8s-lb-controller oci://ghcr.io/voronkov44/charts/k8s-lb-controller \
  --version 0.1.0 \
  -n k8s-lb-controller-system \
  --create-namespace
```

### Dataplane Mode

```bash
helm install k8s-lb-controller oci://ghcr.io/voronkov44/charts/k8s-lb-controller \
  --version 0.1.0 \
  -n k8s-lb-controller-system \
  --create-namespace \
  --set controller.providerMode=dataplane-api \
  --set dataplane.enabled=true
```

For a checkout of this repository, replace the OCI reference with `./charts/k8s-lb-controller`.

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
| `dataplane.enabled` | Enables the dataplane Deployment and Service. |
| `dataplane.hostNetwork`, `dataplane.shareProcessNamespace` | Pod-level runtime wiring for the real dataplane listener. |
| `dataplane.image.repository`, `dataplane.image.tag`, `dataplane.image.pullPolicy` | Dataplane image settings. |
| `dataplane.interface`, `dataplane.ipAttach.*` | Host interface selection and command-based external IP attachment settings for controlled environments. |
| `dataplane.http.port` | ClusterIP Service port and container port for the dataplane API. |
| `dataplane.http.addr` | Optional explicit `K8S_LB_DATAPLANE_HTTP_ADDR`; when empty the chart derives it from `dataplane.http.port`. |
| `dataplane.haproxy.image.*` | Sidecar image settings for the HAProxy runtime container. |
| `dataplane.haproxy.configPath`, `dataplane.haproxy.pidFile` | Shared runtime file paths used by the API container and the HAProxy sidecar. |
| `dataplane.haproxy.validateCommand` | HAProxy validation command run by the dataplane API container before replacing the active config. |
| `dataplane.haproxy.reloadCommand` | HAProxy reload command run by the dataplane API container after a successful config update. |
| `dataplane.logLevel` | Dataplane log verbosity. |
| `dataplane.resources`, `dataplane.nodeSelector`, `dataplane.tolerations`, `dataplane.affinity` | Standard pod scheduling and resource settings for the dataplane API container. |

## URL Wiring

When:

- `controller.providerMode=dataplane-api`
- `dataplane.enabled=true`
- `controller.dataplane.apiURL` is empty

the chart generates the controller dataplane URL automatically as:

`http://<release>-k8s-lb-controller-dataplane.<namespace>.svc:<dataplane.http.port>`

If `controller.dataplane.apiURL` is set explicitly, that value overrides the generated in-cluster service URL.

## Example Values

### Local Mode

```yaml
controller:
  providerMode: local-haproxy
  loadBalancerClass: lab.local/service-lb
  ipPool:
    - 10.0.0.240
    - 10.0.0.241
    - 10.0.0.242
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
  http:
    port: 8090
  haproxy:
    configPath: /var/run/k8s-lb-dataplane/haproxy.cfg
    pidFile: /var/run/k8s-lb-dataplane/haproxy.pid
```

## Notes

- Local mode remains available and is still the chart default.
- Dataplane mode now runs the dataplane API server plus an HAProxy sidecar in one pod.
- The stage-4 dataplane runtime is intended for controlled single-node and lab environments.
- Dataplane mode enables host networking and command-based interface IP attachment, so it needs elevated networking permissions in the dataplane pod.
- Netlink and broader production networking semantics are still deferred.
- `ServiceMonitor` support remains optional and is rendered only when the existing metrics settings enable it.

## Verification

```bash
helm lint ./charts/k8s-lb-controller
helm lint ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
helm template k8s-lb-controller ./charts/k8s-lb-controller
helm template k8s-lb-controller ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
```

## Uninstall

```bash
helm uninstall k8s-lb-controller -n k8s-lb-controller-system
```
