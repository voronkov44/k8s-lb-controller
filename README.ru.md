# k8s-lb-controller

Kubernetes-контроллер на базе `controller-runtime`, который управляет выбранными `Service` типа `LoadBalancer`: выделяет внешний IP из статического пула, находит backend-адреса через `EndpointSlice` и синхронизирует требуемое состояние через слой провайдера.

English version: [README.md](README.md)

## Обзор

`k8s-lb-controller` — намеренно ограниченный по объёму контроллерный проект, построенный вокруг встроенных Kubernetes-ресурсов.
В качестве основного ресурса он отслеживает `Service`, а изменения связанных `EndpointSlice` использует для повторного reconcile соответствующего сервиса.

Для подходящих `Service` контроллер:

- обрабатывает только объекты с `spec.type: LoadBalancer`
- фильтрует их по настроенному `spec.loadBalancerClass`
- выделяет или переиспользует внешний IPv4-адрес из статического пула
- публикует этот адрес в `.status.loadBalancer.ingress`
- находит ready IPv4 backend-адреса через `EndpointSlice`
- формирует целевое состояние для провайдера и применяет его через абстракцию провайдера

В текущем MVP runtime-провайдер файловый и рендерит детерминированный HAProxy config.
Такой подход позволяет показать реальную control-plane логику, не пряча её за интеграцией с облачным провайдером или собственным API.

## Зачем Нужен Этот Проект

Kubernetes задаёт модель `LoadBalancer`-сервиса, но конкретная реализация всегда зависит от окружения.
В managed-cloud среде эта логика обычно скрыта внутри платформы. В локальных кластерах, bare-metal лабораториях, demo-сценариях и контролируемых окружениях гораздо полезнее иметь компактный контроллер, в котором это поведение явно видно.

Этот репозиторий сделан как намеренно ограниченный проект Kubernetes-контроллера: достаточно компактный, чтобы его можно было быстро прочитать, и при этом достаточно серьёзный, чтобы показать проектирование reconcile-циклов, работу с finalizer, публикацию статуса, backend discovery, интеграцию с провайдером, тестирование и deployment workflow.
Это лёгкий контроллерный проект и production-style MVP для обучения, demo и контролируемых окружений, а не полная замена системам уровня MetalLB или облачных load balancer-реализаций.

## Установка

Канонический публичный репозиторий: [github.com/voronkov44/k8s-lb-controller](https://github.com/voronkov44/k8s-lb-controller)

Для пакетной установки поддерживается OCI chart для Helm:

```sh
helm install k8s-lb-controller oci://ghcr.io/voronkov44/charts/k8s-lb-controller \
  --version 0.1.0 \
  -n k8s-lb-controller-system --create-namespace
```

Подробная документация по Helm, значения chart и связанные примечания описаны в:

- [charts/k8s-lb-controller/README.md](charts/k8s-lb-controller/README.md)
- [charts/k8s-lb-controller/README.ru.md](charts/k8s-lb-controller/README.ru.md)

Kustomize-манифесты остаются доступным вариантом для разработки, отладки и установки через манифесты.
Публикация chart и релизные артефакты отслеживаются через GitHub Releases.

## Что Входит В Текущий MVP

Репозиторий завершён по функциональности для текущей фазы и включает следующее:

- `Service` используется как основной объект reconcile.
- Связанные `EndpointSlice` отслеживаются и приводят к повторному reconcile владельца.
- Обрабатываются только `Service` с `type: LoadBalancer`.
- Управляемые объекты фильтруются по настроенному `loadBalancerClass`.
- Внешние IPv4-адреса выделяются из статического конфигурируемого пула.
- Уже назначенный валидный адрес переиспользуется, если это возможно.
- Выбранный адрес публикуется в статус `Service`.
- Backend-адреса определяются по ready IPv4 endpoints из `EndpointSlice`.
- Целевое состояние синхронизируется через абстракцию провайдера.
- В runtime binary используется файловый HAProxy provider.
- Поддержаны finalizer, deletion flow и cleanup на случай, когда `Service` удаляется или перестаёт соответствовать критериям контроллера.
- Экспортируются metrics, health и readiness endpoints.
- Есть unit-, regression- и end-to-end тесты.
- Есть Helm chart, OCI-публикация, локальный сценарий разработки и Kustomize-манифесты для деплоя.

Важные замечания о текущем состоянии:

- runtime-провайдер ориентирован на IPv4 и TCP-трафик
- контроллер сфокусирован на control-plane поведении и provider synchronization
- Helm — рекомендуемый путь пакетной установки, а Kustomize-манифесты остаются полезными для разработки и работы с манифестами

## Текущие Ограничения

Это намеренно ограниченный MVP-контроллер, и у текущего репозитория есть несколько важных границ:

- Выделение адресов основано на заранее настроенном статическом IPv4-пуле.
- Выбор управляемых `Service` идёт по одному настроенному `loadBalancerClass`.
- Runtime-провайдер по умолчанию рендерит файловый HAProxy config.
- Контроллер сфокусирован на IPv4 и TCP-трафике.

## Как Использовать

Этот контроллер лучше всего подходит для сценариев, где нужна небольшая и прозрачная реализация поведения `LoadBalancer`:

- локальные кластеры для разработки и demo
- bare-metal или лабораторные окружения со статическим пулом внешних адресов
- учебные и исследовательские сценарии, где важно быстро понять логику контроллера

На практике использование выглядит так:

1. Установить контроллер через OCI Helm chart или Kustomize-манифесты из этого репозитория.
2. Настроить `loadBalancerClass` и статический пул IP-адресов, которыми контроллер должен управлять.
3. Создать `Service` с `spec.type: LoadBalancer`.
4. Указать в `spec.loadBalancerClass` то же значение, которое настроено у контроллера.
5. Дать контроллеру выделить адрес из пула, найти backend-адреса через `EndpointSlice`, синхронизировать состояние провайдера и опубликовать выбранный адрес в `.status.loadBalancer.ingress`.

Для пакетной установки используйте Helm-команду выше.
Для установки через манифесты и сценариев разработки используйте Kustomize-путь ниже.

## Архитектура

Репозиторий намеренно остаётся компактным.
Основные части проекта выглядят так:

```text
cmd/main.go                    Запуск manager и wiring
internal/config/               Загрузка runtime-конфигурации
internal/controller/           Логика reconcile и watches
internal/ipam/                 Парсинг и выделение адресов из статического пула
internal/backends/             Backend discovery по EndpointSlice
internal/provider/             Provider interface и in-memory mock для тестов
internal/provider/haproxy/     Файловый HAProxy provider
internal/status/               Хелперы для публикации Service status
internal/metrics/              Custom Prometheus metrics
config/default/                Основной Kustomize entrypoint
config/manager/                Manifest Deployment контроллера
config/rbac/                   Service account и RBAC
config/prometheus/             Optional ServiceMonitor
charts/k8s-lb-controller/      Helm chart и схема values
test/e2e/                      End-to-end тесты на базе Kind
```

С архитектурной точки зрения проект разделяет обязанности control plane Kubernetes и детали реализации провайдера:

- `internal/controller` определяет, управляется ли `Service`, и ведёт reconcile.
- `internal/ipam` отвечает за детерминированное выделение адресов из статического пула.
- `internal/backends` преобразует данные `EndpointSlice` в backend-адреса для слоя провайдера.
- `internal/provider` задаёт границу абстракции.
- `internal/provider/haproxy` материализует целевое состояние в виде HAProxy config file.
- `internal/status` обновляет `Service` status только при реальных изменениях.

Такое разделение делает reconcile loop читаемым и при этом оставляет в проекте реальные обязанности контроллера.

## Как Проходит Reconcile

Для подходящего `Service` цикл reconcile выполняет следующие шаги:

1. Считывает `Service`.
2. Если объект находится в deletion flow, очищает provider state и снимает finalizer.
3. Если объект больше не соответствует критериям контроллера, очищает ранее управляемое состояние, при необходимости очищает status и снимает finalizer.
4. Гарантирует наличие finalizer у контроллера.
5. Читает релевантные `Service` и выделяет или переиспользует IP из настроенного пула.
6. Читает связанные `EndpointSlice` и находит ready IPv4 backends.
7. Строит provider model и вызывает `provider.Ensure`.
8. Публикует выбранный внешний IP в `.status.loadBalancer.ingress`.
9. Возвращает requeue только в случае, когда управляемый `Service` временно не может получить свободный IP из пула.

Порядок здесь принципиален: provider synchronization выполняется до публикации статуса, а cleanup path явно разделён для deletion и для случая, когда объект больше не является managed.

## Конфигурация

Приложение настраивается через переменные окружения.
Если рядом есть `.env`, он загружается без перезаписи уже установленных переменных окружения.
Это удобно для локальной разработки и при этом не мешает явной конфигурации в CI или deployment manifests.

Helm chart отображает эти настройки через values; подробности по Helm вынесены в [charts/k8s-lb-controller/README.ru.md](charts/k8s-lb-controller/README.ru.md).

| Variable | Default | Назначение |
| --- | --- | --- |
| `K8S_LB_CONTROLLER_METRICS_ADDR` | `:8080` | Адрес metrics server |
| `K8S_LB_CONTROLLER_HEALTH_ADDR` | `:8081` | Адрес health/readiness probes |
| `K8S_LB_CONTROLLER_LEADER_ELECT` | `false` | Включение leader election |
| `K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS` | `iedge.local/service-lb` | Управляемый `loadBalancerClass` |
| `K8S_LB_CONTROLLER_IP_POOL` | `203.0.113.10,203.0.113.11,203.0.113.12` | Статический пул внешних IPv4-адресов |
| `K8S_LB_CONTROLLER_REQUEUE_AFTER` | `30s` | Задержка перед повторным reconcile при отсутствии свободного IP |
| `K8S_LB_CONTROLLER_GRACEFUL_SHUTDOWN_TIMEOUT` | `15s` | Таймаут graceful shutdown для controller manager |
| `K8S_LB_CONTROLLER_LOG_LEVEL` | `info` | Уровень логирования: `debug`, `info`, `warn`, `error` |
| `K8S_LB_CONTROLLER_HAPROXY_CONFIG_PATH` | `/tmp/k8s-lb-controller-haproxy.cfg` | Путь к рендеримому HAProxy config |
| `K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND` | empty | Необязательная команда для валидации candidate config |
| `K8S_LB_CONTROLLER_HAPROXY_RELOAD_COMMAND` | empty | Необязательная команда после успешного обновления config |

Важно:

- `K8S_LB_CONTROLLER_IP_POOL` должен содержать валидные уникальные IPv4-адреса.
- `K8S_LB_CONTROLLER_REQUEUE_AFTER` используется только для управляемых сервисов, которые ждут свободный IP.
- `K8S_LB_CONTROLLER_GRACEFUL_SHUTDOWN_TIMEOUT` должен быть положительным.
- Если заданы команды валидации или перезагрузки HAProxy, токен `{{config}}` будет заменён на соответствующий путь к config file.
- В Kustomize-манифестах для in-cluster запуска leader election включён явно.

Для локального старта можно использовать готовый шаблон:

```sh
cp .env.example .env
```

## Локальный Запуск

### Что Понадобится

- Go 1.26.x
- Docker
- `kubectl`
- `kind` для автоматизированного e2e-сценария

### Основные Команды Для Разработки

```sh
make build
make lint
make test
make test-e2e
```

Дополнительно:

```sh
make manifests
make build-installer
```

### Рекомендуемый Локальный Сценарий

Для интерактивной разработки и demo-сценариев самый простой путь — запускать контроллер локально против текущего kubeconfig context:

```sh
kind create cluster --name k8s-lb-controller
cp .env.example .env
make run
```

Что важно:

- `make run` запускает контроллер на хосте.
- При локальном запуске бинарь делает предварительную проверку kubeconfig и заранее сообщает о проблемах вроде отсутствующего kubeconfig или пустого `current-context`.
- При конфигурации по умолчанию HAProxy config будет записываться в `/tmp/k8s-lb-controller-haproxy.cfg` на хосте.

### Запуск Внутри Кластера

Если нужно запустить контроллер как `Deployment` внутри кластера:

```sh
make docker-build IMG=k8s-lb-controller:dev
kind load docker-image k8s-lb-controller:dev --name k8s-lb-controller
make deploy IMG=k8s-lb-controller:dev
```

Этот путь близок к тому, что реально проверяется в e2e-тестах.

## Манифесты И Deployment

Репозиторий поддерживает два пути установки:

- Helm chart для пакетной установки и распространения
- Kustomize-манифесты для разработки, отладки и установки через манифесты

Подробную Helm-документацию, значения chart и связанные примечания смотрите в [charts/k8s-lb-controller/README.ru.md](charts/k8s-lb-controller/README.ru.md).

Kustomize-ресурсы в этом репозитории организованы так:

- `config/default` — основной deployment entrypoint
- `config/manager` — `Deployment` контроллера
- `config/rbac` — service account и минимальный RBAC для `Service` и `EndpointSlice`
- `config/default/metrics_service.yaml` — внутренний metrics `Service`
- `config/prometheus/monitor.yaml` — optional `ServiceMonitor` для кластеров, где уже есть Prometheus Operator CRDs

`ServiceMonitor` остаётся optional, потому что Prometheus Operator CRDs есть не в каждом кластере.

Отрендерить набор манифестов с помощью локально установленного в репозитории Kustomize:

```sh
make kustomize
bin/kustomize build config/default
```

Задеплоить:

```sh
make deploy IMG=ghcr.io/voronkov44/k8s-lb-controller:latest
```

Удалить:

```sh
make undeploy
```

Сгенерировать единый install bundle:

```sh
make build-installer
```

Команда создаёт `dist/install.yaml`.

### Минимальный Пример Managed Service

Контроллер управляет только теми сервисами, которые соответствуют настроенному классу:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: demo
spec:
  type: LoadBalancer
  loadBalancerClass: iedge.local/service-lb
  selector:
    app: demo
  ports:
    - port: 80
      targetPort: 80
```

## Тестирование

В текущем состоянии репозиторий включает unit-, regression- и end-to-end тесты.

- `make lint` запускает `golangci-lint`
- `make test` запускает основной Go test suite без e2e-сценариев, используя envtest assets, и пишет `cover.out`
- `make test-e2e` поднимает кластер Kind, собирает и загружает образ контроллера, деплоит манифесты, прогоняет Ginkgo e2e-тесты и затем удаляет кластер

Сейчас тестами покрыто:

- селекцию объектов, lifecycle, finalizer и cleanup behavior
- логику выделения и переиспользования IP
- backend discovery по `EndpointSlice`
- порядок обновления status и идемпотентность
- рендеринг и apply semantics HAProxy provider
- проверку manifest configuration для graceful shutdown alignment
- end-to-end сценарии для metrics, managed vs ignored services, backend updates и deletion cleanup

В репозитории также есть GitHub Actions workflows для:

- lint
- unit и regression tests
- end-to-end tests

## Лицензия

Проект распространяется под Apache License, Version 2.0.
