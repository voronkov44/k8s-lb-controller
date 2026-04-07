# Helm Chart `k8s-lb-controller`

Этот chart разворачивает `k8s-lb-controller` в кластере Kubernetes.

[English version](README.md)

## Что устанавливается

- Deployment
- ServiceAccount
- ClusterRole и ClusterRoleBinding
- Role и RoleBinding для leader election
- Service для метрик
- Опциональный ServiceMonitor

## Установка

```bash
helm install k8s-lb-controller . -n k8s-lb-controller-system --create-namespace
```

## Пример файла с overrides

```yaml
image:
  repository: ghcr.io/f1lzz/k8s-lb-controller
  tag: "0.1.0"

controller:
  loadBalancerClass: iedge.local/service-lb
  ipPool:
    - 10.0.0.240
    - 10.0.0.241
    - 10.0.0.242

metrics:
  serviceMonitor:
    enabled: false
```

Установка с пользовательскими значениями:

```bash
helm install k8s-lb-controller . \
  -n k8s-lb-controller-system --create-namespace \
  -f values-production.yaml
```

## Обновление

```bash
helm upgrade k8s-lb-controller . \
  -n k8s-lb-controller-system \
  -f values-production.yaml
```

## Удаление

```bash
helm uninstall k8s-lb-controller -n k8s-lb-controller-system
```

## Важные параметры

- `image.repository`, `image.tag`, `image.pullPolicy`
- `controller.loadBalancerClass`
- `controller.ipPool`
- `controller.requeueAfter`
- `controller.gracefulShutdownTimeout`
- `controller.logLevel`
- `controller.haproxy.configPath`
- `metrics.service.enabled`
- `metrics.serviceMonitor.enabled`
- `resources`
- `nodeSelector`, `tolerations`, `affinity`

## Примечания

- Команды выше предполагают, что `helm` запускается из каталога этого chart.
- Шаблоны chart не фиксируют namespace жёстко и устанавливаются в namespace Helm release.
- `metrics.serviceMonitor.enabled` стоит включать только в кластерах, где уже установлены CRD от Prometheus Operator.
- Контроллер управляет только теми объектами `Service`, у которых `spec.loadBalancerClass` совпадает со значением в настройках chart.
