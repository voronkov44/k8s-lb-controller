# Helm Chart `k8s-lb-controller`

Этот chart разворачивает `k8s-lb-controller` и поддерживает оба deployment mode, которые теперь есть в репозитории:

- `local-haproxy`: controller-only mode, он по-прежнему используется по умолчанию
- `dataplane-api`: режим controller + standalone dataplane server

[English version](README.md)

Общий контекст по архитектуре и rollout см. в [README.md](../../README.md).

## Что Устанавливает Chart

Всегда:

- `Deployment` контроллера
- `ServiceAccount`
- RBAC для `Service`, `Service/status` и `EndpointSlice`
- `Role` и `RoleBinding` для leader election
- optional metrics `Service`
- optional `ServiceMonitor`

Когда `dataplane.enabled=true`:

- `Deployment` dataplane
- `Service` dataplane

## Установка

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

Если вы работаете из checkout этого репозитория, можно заменить OCI reference на `./charts/k8s-lb-controller`.

## Важные Values

### Controller Values

| Value | Описание |
| --- | --- |
| `image.repository`, `image.tag`, `image.pullPolicy` | Настройки образа контроллера. |
| `controller.providerMode` | `local-haproxy` или `dataplane-api`. |
| `controller.loadBalancerClass` | Управляемый `spec.loadBalancerClass`. |
| `controller.ipPool` | Статический IPv4-пул для выделения внешних адресов. |
| `controller.dataplane.apiURL` | Необязательный явный override для dataplane API URL. |
| `controller.dataplane.apiTimeout` | Таймаут запросов от контроллера к dataplane API. |
| `controller.haproxy.*` | Настройки локального HAProxy provider для режима `local-haproxy`. |

### Dataplane Values

| Value | Описание |
| --- | --- |
| `dataplane.enabled` | Включает dataplane Deployment и Service. |
| `dataplane.image.repository`, `dataplane.image.tag`, `dataplane.image.pullPolicy` | Настройки образа dataplane. |
| `dataplane.http.port` | ClusterIP Service port и container port для dataplane API. |
| `dataplane.http.addr` | Необязательный явный `K8S_LB_DATAPLANE_HTTP_ADDR`; если пусто, chart выводит его из `dataplane.http.port`. |
| `dataplane.haproxy.configPath` | Путь к HAProxy config внутри dataplane. |
| `dataplane.haproxy.validateCommand` | Необязательная команда валидации HAProxy config. |
| `dataplane.haproxy.reloadCommand` | Необязательная команда reload HAProxy. |
| `dataplane.logLevel` | Уровень логирования dataplane. |
| `dataplane.resources`, `dataplane.nodeSelector`, `dataplane.tolerations`, `dataplane.affinity` | Стандартные настройки ресурсов и размещения pod dataplane. |

## Как Формируется URL

Когда:

- `controller.providerMode=dataplane-api`
- `dataplane.enabled=true`
- `controller.dataplane.apiURL` пустой

chart автоматически формирует controller-side dataplane URL:

`http://<release>-k8s-lb-controller-dataplane.<namespace>.svc:<dataplane.http.port>`

Если `controller.dataplane.apiURL` задан явно, он имеет приоритет над автоматически сгенерированным in-cluster service URL.

## Примеры Values

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
  http:
    port: 8090
  haproxy:
    configPath: /var/run/k8s-lb-dataplane/haproxy.cfg
```

## Примечания

- Local mode никуда не делся и остаётся режимом по умолчанию для chart.
- Dataplane mode на этой стадии разворачивает только dataplane API server process.
- Chart пока не добавляет host networking, interface IP attachment, netlink integration или real external traffic publication.
- Поддержка `ServiceMonitor` остаётся опциональной и рендерится только при включённых существующих metrics settings.

## Проверка

```bash
helm lint ./charts/k8s-lb-controller
helm lint ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
helm template k8s-lb-controller ./charts/k8s-lb-controller
helm template k8s-lb-controller ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
```

## Удаление

```bash
helm uninstall k8s-lb-controller -n k8s-lb-controller-system
```
