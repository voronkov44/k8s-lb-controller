# Helm Chart `k8s-lb-controller`

Этот chart устанавливает `k8s-lb-controller` и поддерживает оба режима провайдера контроллера, которые есть в репозитории сейчас:

- `local-haproxy`: режим по умолчанию только с контроллером
- `dataplane-api`: контроллер плюс отдельные `Deployment` и `Service` для dataplane

English version: [README.md](README.md)

Общий обзор репозитория: [README.ru.md](../../README.ru.md)

## Что Устанавливает Chart

Всегда:

- `Deployment` контроллера
- `ServiceAccount`
- RBAC для `Service`, `Service/status` и `EndpointSlice`
- `Role` и `RoleBinding` для leader election
- необязательный metrics `Service`
- необязательный `ServiceMonitor`

Когда `dataplane.enabled=true`:

- `Deployment` dataplane
- `Service` dataplane
- sidecar-контейнер HAProxy внутри pod dataplane

По умолчанию chart разворачивает только контроллер. Ресурсы dataplane добавляются только при включении режима dataplane.

## Установка

OCI-адрес chart:

`oci://ghcr.io/voronkov44/charts/k8s-lb-controller`

Установка локального режима из локальной копии репозитория:

```bash
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system \
  --create-namespace
```

Установка режима dataplane из локальной копии репозитория:

```bash
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system \
  --create-namespace \
  --set controller.providerMode=dataplane-api \
  --set dataplane.enabled=true
```

Если нужен опубликованный OCI chart, замените `./charts/k8s-lb-controller` на OCI reference выше и добавьте `--version 0.1.1`.

## Важные Values

### Значения Контроллера

| Value | Описание |
| --- | --- |
| `image.repository`, `image.tag`, `image.pullPolicy` | Настройки образа контроллера. |
| `controller.providerMode` | `local-haproxy` или `dataplane-api`. |
| `controller.loadBalancerClass` | Управляемый `spec.loadBalancerClass`. |
| `controller.ipPool` | Статический IPv4-пул для выделения внешних адресов. |
| `controller.dataplane.apiURL` | Необязательный явный URL dataplane API. |
| `controller.dataplane.apiTimeout` | Таймаут запросов контроллера к dataplane API. |
| `controller.haproxy.*` | Настройки локального HAProxy-провайдера для режима `local-haproxy`. |

### Значения Dataplane

| Value | Описание |
| --- | --- |
| `dataplane.enabled` | Включает `Deployment` и `Service` dataplane. |
| `dataplane.image.repository`, `dataplane.image.tag`, `dataplane.image.pullPolicy` | Настройки образа dataplane API. |
| `dataplane.http.port`, `dataplane.http.addr` | Адрес прослушивания dataplane API и порт сервиса. |
| `dataplane.hostNetwork`, `dataplane.shareProcessNamespace` | Настройки runtime для внутрикластерного развёртывания dataplane. |
| `dataplane.interface` | Интерфейс хоста, к которому привязывается внешний IP в режиме dataplane. |
| `dataplane.ipAttach.enabled` | Включает привязку внешнего IP на стороне хоста. |
| `dataplane.ipAttach.mode` | Backend привязки внешнего IP: `netlink` или `exec`. |
| `dataplane.ipAttach.command`, `dataplane.ipAttach.cidrSuffix` | Команда для exec-backend и ширина прикрепляемого CIDR. |
| `dataplane.haproxy.image.*` | Настройки образа sidecar-контейнера HAProxy. |
| `dataplane.haproxy.configPath`, `dataplane.haproxy.pidFile` | Общие runtime-файлы, которые используют контейнер dataplane API и sidecar-контейнер HAProxy. |
| `dataplane.haproxy.validateCommand`, `dataplane.haproxy.reloadCommand` | Команды для проверки конфигурации и атомарного reload. |
| `dataplane.logLevel`, `dataplane.gracefulShutdownTimeout` | Поведение runtime dataplane. |
| `dataplane.resources`, `dataplane.nodeSelector`, `dataplane.tolerations`, `dataplane.affinity` | Настройки ресурсов и размещения для pod dataplane API. |

### Значения Метрик и Мониторинга

| Value | Описание |
| --- | --- |
| `metrics.service.enabled` | Создаёт metrics `Service`. |
| `metrics.serviceMonitor.enabled` | Создаёт `ServiceMonitor`, если нужна интеграция с Prometheus Operator. |

## Как Формируется URL Dataplane

Когда одновременно выполняются условия:

- `controller.providerMode=dataplane-api`
- `dataplane.enabled=true`
- `controller.dataplane.apiURL` пустой

chart автоматически формирует URL dataplane для контроллера:

`http://<release>-k8s-lb-controller-dataplane.<namespace>.svc:<dataplane.http.port>`

Если `controller.dataplane.apiURL` задан явно, он имеет приоритет над автоматически сформированным внутрикластерным URL сервиса.

Chart также проверяет, что `controller.providerMode=dataplane-api` должен сопровождаться либо `dataplane.enabled=true`, либо непустым `controller.dataplane.apiURL`.

## Примеры Values

### Локальный Режим

```yaml
controller:
  providerMode: local-haproxy
  loadBalancerClass: lab.local/service-lb
  ipPool:
    - 203.0.113.10
    - 203.0.113.11
    - 203.0.113.12
```

### Режим Dataplane

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

## Область Применения и Ограничения

- Локальный режим по-прежнему доступен, обратно совместим и остаётся режимом chart по умолчанию.
- Режим dataplane разворачивает отдельный dataplane API и sidecar-контейнер HAProxy. Это не распределённая многовузловая dataplane-система.
- В текущих значениях chart режим dataplane использует host networking, `shareProcessNamespace` и привязку внешнего IP на стороне хоста.
- `dataplane.ipAttach.mode` по умолчанию равен `netlink`; `exec` остаётся запасным вариантом.
- Текущее развёртывание dataplane предназначено для контролируемых одновузловых и лабораторных окружений.
- Координация dataplane между несколькими узлами, HA-сценарии, BGP, ARP, NDP, публикация по модели cloud provider и более широкое производственное усиление надёжности намеренно остаются вне текущей области применения.

## Проверка

```bash
helm lint ./charts/k8s-lb-controller
helm lint ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
helm template k8s-lb-controller ./charts/k8s-lb-controller
helm template k8s-lb-controller ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
make verify-dataplane
make smoke-dataplane-kind
```

Подробные инструкции по проверке в контролируемом окружении и по проверке готовности:

- smoke-проверка: [../../docs/dataplane-smoke.md](../../docs/dataplane-smoke.md), [../../docs/dataplane-smoke.ru.md](../../docs/dataplane-smoke.ru.md)
- чеклист готовности: [../../docs/release-checklist.md](../../docs/release-checklist.md), [../../docs/release-checklist.ru.md](../../docs/release-checklist.ru.md)

## Удаление

```bash
helm uninstall k8s-lb-controller -n k8s-lb-controller-system
```
