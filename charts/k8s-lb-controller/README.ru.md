# Helm Chart `k8s-lb-controller`

Этот chart разворачивает `k8s-lb-controller`, лёгкий Kubernetes LoadBalancer-контроллер, который выделяет внешние IPv4-адреса из статического пула и синхронизирует состояние HAProxy для подходящих `Service`.

[English version](README.md)

Подробности об архитектуре контроллера, логике reconcile и runtime-конфигурации описаны в основном [README.md](../../README.md).

## Что устанавливает Chart

- `Deployment` контроллера
- `ServiceAccount`
- RBAC для чтения и обновления управляемых `Service`
- `Role` и `RoleBinding` для leader election
- Опциональный metrics `Service`
- Опциональный `ServiceMonitor`

## OCI-установка

Chart публикуется как OCI chart:

`oci://ghcr.io/voronkov44/charts/k8s-lb-controller`

```bash
helm install k8s-lb-controller oci://ghcr.io/voronkov44/charts/k8s-lb-controller \
  --version 0.1.0 \
<<<<<<< HEAD
  -n k8s-lb-controller-system --create-namespace
=======
  -n k8s-lb-controller-system \
  --create-namespace
>>>>>>> main
```

Если вы работаете из checkout этого репозитория, можно заменить OCI-ссылку на `./charts/k8s-lb-controller`.

## Пример override-файла

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
<<<<<<< HEAD
  -n k8s-lb-controller-system --create-namespace \
=======
  -n k8s-lb-controller-system \
  --create-namespace \
>>>>>>> main
  -f values-local.yaml
```

## Важные параметры

| Параметр | Описание |
| --- | --- |
| `image.repository`, `image.tag`, `image.pullPolicy` | Настройки образа контроллера. |
| `controller.loadBalancerClass` | Контроллер управляет только теми `Service`, у которых `spec.loadBalancerClass` совпадает с этим значением. |
| `controller.ipPool` | Статический IPv4-пул для выделения внешних адресов. Перед использованием вне локальных тестов замените примерные адреса на свои. |
| `controller.gracefulShutdownTimeout` | Таймаут graceful shutdown для controller manager. Держите `terminationGracePeriodSeconds` больше или равным этому значению. |
| `metrics.port` | Единое значение для metrics bind port, container port и metrics `Service`. |
| `health.port` | Единое значение для health/readiness bind port и probe port. |
| `metrics.service.enabled` | Включает metrics `Service`. |
| `metrics.serviceMonitor.enabled` | Запрашивает `ServiceMonitor`, если в кластере доступны CRD от Prometheus Operator. |
| `resources`, `nodeSelector`, `tolerations`, `affinity` | Стандартные параметры ресурсов и размещения pod. |

## Примечания

- Контроллер управляет только теми `Service`, у которых `spec.loadBalancerClass` совпадает со значением `controller.loadBalancerClass`.
- Внешние адреса выделяются из статического IPv4-пула, заданного в `controller.ipPool`.
- Поддержка `ServiceMonitor` опциональна. Chart рендерит его только если `metrics.service.enabled=true`, `metrics.serviceMonitor.enabled=true` и кластер поддерживает `monitoring.coreos.com/v1`.

## Текущие Рамки И Ограничения

- Chart намеренно соответствует текущему поведению контроллера: статический IPv4-пул и синхронизация состояния HAProxy-провайдера.
- Он не добавляет интеграции с облачными провайдерами или функции контроллера, которых нет в этом репозитории.
- Текущий фокус контроллера — IPv4 и TCP-трафик сервисов.

## Удаление

```bash
helm uninstall k8s-lb-controller -n k8s-lb-controller-system
```
