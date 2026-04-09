# k8s-lb-controller

`k8s-lb-controller` — Kubernetes-контроллер на базе `controller-runtime`, который управляет выбранными `Service` типа `LoadBalancer`.
Он выделяет внешние IPv4-адреса из статического пула, находит ready backend-адреса через `EndpointSlice` и синхронизирует требуемое состояние через provider abstraction.

English version: [README.md](README.md)

## Текущий Объём Работ

Сейчас репозиторий поддерживает два runtime-режима провайдера для контроллера:

- `local-haproxy`: исходный локальный файловый HAProxy provider, он по-прежнему используется по умолчанию
- `dataplane-api`: контроллер отправляет desired state в отдельный dataplane HTTP API

Отдельный dataplane-компонент реализован в `cmd/dataplane`.
Он хранит полное desired state в памяти, рендерит один детерминированный HAProxy config для всех управляемых сервисов и применяет его атомарно.

Важное ограничение текущей стадии:

- Stage 4 добавляет реальный HAProxy runtime для dataplane в controlled single-node и lab environment.
- На этой стадии появляется command-based host integration для attach/detach внешних IP-адресов.
- Netlink, multi-node/HA dataplane placement и более широкая production networking semantics всё ещё не реализованы.

## Режимы Развёртывания

### Local Mode

Local mode сохраняет исходное поведение:

- запускается только контроллер
- provider mode равен `local-haproxy`
- HAProxy config пишет сам контроллер

Этот режим остаётся режимом по умолчанию в коде, Kustomize и Helm.

### Dataplane Mode

Dataplane mode разворачивает два компонента:

- контроллер как control-plane компонент
- отдельный dataplane pod, в котором работают:
  - dataplane API server
  - HAProxy sidecar, который реально слушает трафик

В этом режиме:

- контроллер использует `K8S_LB_CONTROLLER_PROVIDER_MODE=dataplane-api`
- контроллер отправляет `PUT /services/{namespace}/{name}` и `DELETE /services/{namespace}/{name}` в dataplane service
- dataplane хранит все сервисы в памяти и рендерит/применяет один aggregate HAProxy config
- dataplane pod использует host networking и command-based `ip addr add` / `ip addr del` интеграцию, чтобы attach/detach внешние IPv4 на одном настроенном host interface

Этот режим специально ориентирован на demo, local lab и controlled single-node environment.

## Структура Репозитория

```text
cmd/main.go                      Бинарь контроллера
cmd/dataplane/main.go            Бинарь dataplane
internal/config/                 Runtime-конфигурация контроллера
internal/dataplane/              Reusable dataplane engine, HTTP handler, render/apply logic
internal/controller/             Логика reconcile для Service
internal/ipam/                   Выделение адресов из статического IPv4-пула
internal/backends/               Backend discovery по EndpointSlice
internal/provider/               Provider interface и provider implementations
internal/provider/haproxy/       Локальный файловый HAProxy provider
config/default/                  Kustomize entrypoint для controller-only local mode
config/dataplane/                Manifest Deployment и Service для dataplane
config/default-dataplane/        Kustomize entrypoint для controller + dataplane mode
charts/k8s-lb-controller/        Helm chart
```

## Build Targets

В репозитории теперь есть отдельные build path для обоих бинарей и обоих образов:

```sh
make build
make build-dataplane
make docker-build
make docker-build-dataplane
```

Полезные deployment-oriented target:

```sh
make deploy
make deploy-dataplane
make build-installer
make build-installer-dataplane
```

## Kustomize

Старый controller-only Kustomize entrypoint сохранён без изменений:

- local mode: `config/default`

Также добавлен отдельный additive entrypoint для controller + dataplane mode:

- dataplane mode: `config/default-dataplane`

### Рендер Local Mode

```sh
./bin/kustomize build config/default
```

### Рендер Dataplane Mode

```sh
./bin/kustomize build config/default-dataplane
```

### Деплой Local Mode

```sh
make deploy IMG=ghcr.io/voronkov44/k8s-lb-controller:dev
```

### Деплой Dataplane Mode

```sh
make deploy-dataplane \
  IMG=ghcr.io/voronkov44/k8s-lb-controller:dev \
  DATAPLANE_IMG=ghcr.io/voronkov44/k8s-lb-controller-dataplane:dev
```

В dataplane Kustomize entrypoint контроллер привязан к in-cluster service URL:

`http://k8s-lb-controller-dataplane.k8s-lb-controller-system.svc:8090`

Stage 4 dataplane-манифесты также включают host networking, `shareProcessNamespace`, реальный HAProxy sidecar и command-based IP attachment на интерфейс `eth0`.

## Helm

Helm chart поддерживает оба режима и не удаляет local mode.

Подробная документация по chart:

- [charts/k8s-lb-controller/README.md](charts/k8s-lb-controller/README.md)
- [charts/k8s-lb-controller/README.ru.md](charts/k8s-lb-controller/README.ru.md)

### Template Local Mode

```sh
helm template k8s-lb-controller ./charts/k8s-lb-controller
```

### Template Dataplane Mode

```sh
helm template k8s-lb-controller ./charts/k8s-lb-controller \
  --set controller.providerMode=dataplane-api \
  --set dataplane.enabled=true
```

Для dataplane mode chart теперь использует значения:

- `controller.providerMode`
- `controller.dataplane.apiURL`
- `controller.dataplane.apiTimeout`
- `dataplane.enabled`
- `dataplane.hostNetwork`
- `dataplane.shareProcessNamespace`
- `dataplane.image.*`
- `dataplane.interface`
- `dataplane.ipAttach.*`
- `dataplane.http.port`
- `dataplane.http.addr`
- `dataplane.haproxy.*`
- `dataplane.haproxy.image.*`
- `dataplane.logLevel`
- `dataplane.gracefulShutdownTimeout`
- `dataplane.resources`
- `dataplane.nodeSelector`
- `dataplane.tolerations`
- `dataplane.affinity`

Если `controller.dataplane.apiURL` не задан и `dataplane.enabled=true`, chart автоматически генерирует in-cluster URL dataplane service.
Stage 4 chart defaults также включают реальный dataplane runtime path с host networking, shared-pid HAProxy sidecar и command-based IP attachment через `dataplane.interface`.

## Runtime-Конфигурация

Контроллер по-прежнему использует исходные переменные окружения для IP allocation и локального HAProxy режима.
Stage 1 добавил:

- `K8S_LB_CONTROLLER_PROVIDER_MODE`
- `K8S_LB_CONTROLLER_DATAPLANE_API_URL`
- `K8S_LB_CONTROLLER_DATAPLANE_API_TIMEOUT`

Dataplane server использует:

- `K8S_LB_DATAPLANE_HTTP_ADDR`
- `K8S_LB_DATAPLANE_HAPROXY_CONFIG_PATH`
- `K8S_LB_DATAPLANE_HAPROXY_VALIDATE_COMMAND`
- `K8S_LB_DATAPLANE_HAPROXY_RELOAD_COMMAND`
- `K8S_LB_DATAPLANE_HAPROXY_PID_FILE`
- `K8S_LB_DATAPLANE_LOG_LEVEL`
- `K8S_LB_DATAPLANE_GRACEFUL_SHUTDOWN_TIMEOUT`
- `K8S_LB_DATAPLANE_IP_ATTACH_ENABLED`
- `K8S_LB_DATAPLANE_INTERFACE`
- `K8S_LB_DATAPLANE_IP_COMMAND`
- `K8S_LB_DATAPLANE_IP_CIDR_SUFFIX`

## Проверка

Проверка репозитория:

```sh
go test ./...
make lint
```

Рендер манифестов и chart:

```sh
./bin/kustomize build config/default
./bin/kustomize build config/default-dataplane
helm template k8s-lb-controller ./charts/k8s-lb-controller
helm template k8s-lb-controller ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
```

## Что Отложено До Stage 5

Stage 4 делает dataplane mode пригодным для controlled environment, но некоторые production-oriented части всё ещё осознанно отложены:

- netlink-based IP management
- multi-node или HA dataplane placement и coordination
- BGP, ARP/NDP, cloud-provider и другие advanced network publication semantics
- более широкое production hardening вокруг host integration
- live end-to-end traffic validation против развернутого dataplane
