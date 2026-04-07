# k8s-lb-controller Helm Chart

This chart installs `k8s-lb-controller`, a lightweight Kubernetes LoadBalancer controller that assigns external IPv4 addresses from a static pool and syncs HAProxy state for matching `Service` objects.

[Русская версия](README.ru.md)

For controller architecture, reconcile behavior, and runtime configuration details, see the repository [README.md](../../README.md).

## What this chart installs

- A controller `Deployment`
- A `ServiceAccount`
- RBAC for watching and updating managed `Service` objects
- Leader-election `Role` and `RoleBinding`
- An optional metrics `Service`
- An optional `ServiceMonitor`

## OCI installation

The chart is published as an OCI chart:

`oci://ghcr.io/voronkov44/charts/k8s-lb-controller`

```bash
helm install k8s-lb-controller oci://ghcr.io/voronkov44/charts/k8s-lb-controller \
  --version 0.1.0 \
  -n k8s-lb-controller-system --create-namespace
```

For a checkout of this repository, replace the OCI reference with `./charts/k8s-lb-controller`.

## Example override values

```yaml
controller:
  loadBalancerClass: lab.local/service-lb
  ipPool:
    - 10.0.0.240
    - 10.0.0.241
    - 10.0.0.242
  gracefulShutdownTimeout: 20s

metrics:
  port: 9090
  serviceMonitor:
    enabled: true

terminationGracePeriodSeconds: 30
```

```bash
helm install k8s-lb-controller oci://ghcr.io/voronkov44/charts/k8s-lb-controller \
  --version 0.1.0 \
  -n k8s-lb-controller-system --create-namespace \
  -f values-local.yaml
```

## Important values

| Value | Description |
| --- | --- |
| `image.repository`, `image.tag`, `image.pullPolicy` | Controller image settings. |
| `controller.loadBalancerClass` | Only `Service` objects with a matching `spec.loadBalancerClass` are managed. |
| `controller.ipPool` | Static IPv4 pool used for external address allocation. Replace the example addresses before use outside local testing or controlled environments. |
| `controller.gracefulShutdownTimeout` | Controller manager shutdown timeout. Keep `terminationGracePeriodSeconds` greater than or equal to this value. |
| `metrics.port` | Single source of truth for the metrics bind port, container port, and metrics `Service` port. |
| `health.port` | Single source of truth for the health and readiness bind port and probe port. |
| `metrics.service.enabled` | Enables the metrics `Service`. |
| `metrics.serviceMonitor.enabled` | Requests a `ServiceMonitor` when Prometheus Operator CRDs are available. |
| `resources`, `nodeSelector`, `tolerations`, `affinity` | Standard pod scheduling and resource settings. |

## Notes

- The controller manages only `Service` objects whose `spec.loadBalancerClass` matches `controller.loadBalancerClass`.
- The controller allocates external addresses from the static IPv4 pool configured in `controller.ipPool`.
- `ServiceMonitor` support is optional. The chart renders it only when `metrics.service.enabled=true`, `metrics.serviceMonitor.enabled=true`, and the target cluster advertises `monitoring.coreos.com/v1`.

## Scope and limitations

- The chart is intentionally aligned with the current controller behavior: static IPv4 pool allocation plus HAProxy provider synchronization.
- It does not add cloud-provider integrations or controller features that are not implemented in this repository.
- The current controller focus is IPv4 and TCP service traffic.

## Uninstall

```bash
helm uninstall k8s-lb-controller -n k8s-lb-controller-system
```
