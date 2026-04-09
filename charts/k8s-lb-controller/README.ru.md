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
- HAProxy sidecar внутри dataplane pod

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
| `dataplane.hostNetwork`, `dataplane.shareProcessNamespace` | Pod-level wiring для реального dataplane listener. |
| `dataplane.image.repository`, `dataplane.image.tag`, `dataplane.image.pullPolicy` | Настройки образа dataplane. |
| `dataplane.interface`, `dataplane.ipAttach.*` | Выбор host interface и command-based attach/detach внешних IP для controlled environment. |
| `dataplane.http.port` | ClusterIP Service port и container port для dataplane API. |
| `dataplane.http.addr` | Необязательный явный `K8S_LB_DATAPLANE_HTTP_ADDR`; если пусто, chart выводит его из `dataplane.http.port`. |
| `dataplane.haproxy.image.*` | Настройки образа sidecar-контейнера с HAProxy runtime. |
| `dataplane.haproxy.configPath`, `dataplane.haproxy.pidFile` | Shared runtime paths, которые используют и dataplane API container, и HAProxy sidecar. |
| `dataplane.haproxy.validateCommand` | Команда валидации HAProxy config перед заменой active config. |
| `dataplane.haproxy.reloadCommand` | Команда reload HAProxy после успешного обновления config. |
| `dataplane.logLevel` | Уровень логирования dataplane. |
| `dataplane.resources`, `dataplane.nodeSelector`, `dataplane.tolerations`, `dataplane.affinity` | Стандартные настройки ресурсов и размещения для dataplane API container. |

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
  interface: eth0
  ipAttach:
    enabled: true
  http:
    port: 8090
  haproxy:
    configPath: /var/run/k8s-lb-dataplane/haproxy.cfg
    pidFile: /var/run/k8s-lb-dataplane/haproxy.pid
```

## Примечания

- Local mode никуда не делся и остаётся режимом по умолчанию для chart.
- Dataplane mode теперь запускает dataplane API server и HAProxy sidecar в одном pod.
- Stage 4 ориентирован на controlled single-node и lab environment.
- Dataplane mode включает host networking и command-based interface IP attachment, поэтому dataplane pod требует повышенных networking permissions.
- Netlink и более широкие production networking semantics пока остаются отложенными.
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
