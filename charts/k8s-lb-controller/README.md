# k8s-lb-controller Helm Chart

This chart installs `k8s-lb-controller`, a lightweight Kubernetes LoadBalancer controller that assigns external IPv4 addresses from a static pool and syncs HAProxy state for matching `Service` objects.

[Русская версия](README.ru.md)

## What this chart installs

- A controller `Deployment`
- A `ServiceAccount`
- RBAC for watching and updating managed `Service` objects
- Leader-election `Role` and `RoleBinding`
- An optional metrics `Service`
- An optional `ServiceMonitor`

## Quick install

For a checkout of this repository:

```bash
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system --create-namespace
```

Override `controller.loadBalancerClass` and `controller.ipPool` before using the chart outside local testing.

## Example with overridden values

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
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system --create-namespace \
  -f values-local.yaml
```

## Important values

| Value | Description |
| --- | --- |
| `image.repository`, `image.tag`, `image.pullPolicy` | Controller image settings. |
| `controller.loadBalancerClass` | Only `Service` objects with this `spec.loadBalancerClass` are managed. |
| `controller.ipPool` | Static IPv4 pool used for external address allocation. |
| `controller.gracefulShutdownTimeout` | Controller manager shutdown timeout. Keep `terminationGracePeriodSeconds` greater than or equal to this value. |
| `metrics.port` | Single source of truth for the metrics bind port, container port, and metrics `Service` port. |
| `health.port` | Single source of truth for the health and readiness bind port and probe port. |
| `metrics.service.enabled` | Enables the metrics `Service`. |
| `metrics.serviceMonitor.enabled` | Requests a `ServiceMonitor` when Prometheus Operator CRDs are available. |
| `resources`, `nodeSelector`, `tolerations`, `affinity` | Standard pod scheduling and resource settings. |

## Scope and limitations

- The controller manages only `Service` objects whose `spec.loadBalancerClass` matches `controller.loadBalancerClass`.
- The current scope is a static IPv4 pool plus HAProxy provider synchronization. The chart does not add cloud-provider integrations or other controller features that the application does not implement.
- Review the default example IP pool before use outside local testing or controlled environments.

## ServiceMonitor

`ServiceMonitor` support is optional. The chart renders it only when all of the following are true:

- `metrics.service.enabled=true`
- `metrics.serviceMonitor.enabled=true`
- The target cluster advertises `monitoring.coreos.com/v1`
