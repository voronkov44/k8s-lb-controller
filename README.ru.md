# k8s-lb-controller

`k8s-lb-controller` — Kubernetes-контроллер на базе `controller-runtime`, который управляет выбранными `Service` типа `LoadBalancer`. Он выделяет внешние IPv4-адреса из статического пула, находит готовые адреса backend через `EndpointSlice` и синхронизирует состояние балансировщика через абстракцию провайдера.
## Режимы Развёртывания

### Локальный Режим

Локальный режим сохраняет исходное поведение:

- разворачивается только контроллер
- `controller.providerMode` равен `local-haproxy`
- контроллер сам формирует и применяет конфигурацию HAProxy

Этот режим остаётся режимом по умолчанию в кодовой базе, Kustomize и Helm.

### Режим Dataplane

Режим dataplane разворачивает контроллер и отдельный dataplane:

- контроллер работает как управляющий компонент
- dataplane API хранит требуемое состояние в памяти и формирует общую конфигурацию HAProxy
- в dataplane-развёртывании запускается отдельный sidecar-контейнер HAProxy
- pod dataplane использует сетевой режим хоста и привязывает выделенный внешний IP к одному настроенному сетевому интерфейсу

Этот режим предназначен для контролируемых одновузловых и лабораторных окружений.

## Сборка, Локальный Запуск и Развёртывание

Цели сборки:

```sh
make build
make build-dataplane
make docker-build
make docker-build-dataplane
```

Цели для локального запуска:

```sh
make run
make run-dataplane
```

Цели для развёртывания:

```sh
make deploy
make deploy-dataplane
make build-installer
make build-installer-dataplane
```

Цели для проверки:

```sh
make verify-dataplane
make smoke-dataplane-kind
make test-e2e-dataplane
make kind-up-dataplane
make kind-down-dataplane
```

## Kustomize

В репозитории есть две точки входа Kustomize:

- `config/default`: локальный режим только с контроллером
- `config/default-dataplane`: режим с контроллером и отдельным dataplane

Рендеринг через `kustomize`, который управляется из репозитория:

```sh
make kustomize
./bin/kustomize build config/default
./bin/kustomize build config/default-dataplane
```

Развёртывание локального режима:

```sh
make deploy IMG=ghcr.io/voronkov44/k8s-lb-controller:dev
```

Развёртывание режима dataplane:

```sh
make deploy-dataplane \
  IMG=ghcr.io/voronkov44/k8s-lb-controller:dev \
  DATAPLANE_IMG=ghcr.io/voronkov44/k8s-lb-controller-dataplane:dev
```

В dataplane-оверлее контроллер подключён к внутрикластерному URL dataplane `http://k8s-lb-controller-dataplane.k8s-lb-controller-system.svc:8090`. Развёртывание dataplane по умолчанию включает host networking, `shareProcessNamespace` и привязку внешнего IP к интерфейсу `eth0`.

## Helm

Helm chart также поддерживает оба режима.

Установка локального режима из локальной копии репозитория:

```sh
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system \
  --create-namespace
```

Установка режима dataplane из локальной копии репозитория:

```sh
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system \
  --create-namespace \
  --set controller.providerMode=dataplane-api \
  --set dataplane.enabled=true
```

Если `controller.dataplane.apiURL` не задан и `dataplane.enabled=true`, chart автоматически формирует внутрикластерный URL сервиса dataplane. В режиме dataplane по умолчанию используется `dataplane.ipAttach.mode=netlink`; `exec` остаётся запасным вариантом.

Подробная документация по chart:

- [charts/k8s-lb-controller/README.md](charts/k8s-lb-controller/README.md)
- [charts/k8s-lb-controller/README.ru.md](charts/k8s-lb-controller/README.ru.md)

## Проверка и Smoke-Сценарии

`make verify-dataplane` выполняет проверки готовности без запуска живого dataplane. В него входят:

- `go test ./...`
- `make lint`
- сборка контроллера и dataplane
- рендеринг Kustomize для локального режима и режима dataplane
- `helm lint` и `helm template` для локального режима и режима dataplane

`make smoke-dataplane-kind` запускает автоматизированный Kind smoke-сценарий: собирает образы, разворачивает контроллер и dataplane, поднимает демонстрационную нагрузку и проверяет выделение внешнего IP, его привязку к интерфейсу, готовность HAProxy listener, содержимое сгенерированной конфигурации и сквозную HTTP-доступность.

`make test-e2e-dataplane` запускает e2e-набор тестов для режима dataplane в Kind. `make kind-up-dataplane` и `make kind-down-dataplane` удобны, когда кластер для smoke-проверки нужно поднять и разобрать вручную.

Подробные инструкции по проверке:

- smoke-проверка: [docs/dataplane-smoke.md](docs/dataplane-smoke.md), [docs/dataplane-smoke.ru.md](docs/dataplane-smoke.ru.md)
- чеклист готовности: [docs/release-checklist.md](docs/release-checklist.md), [docs/release-checklist.ru.md](docs/release-checklist.ru.md)
