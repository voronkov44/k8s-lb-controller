# k8s-lb-controller

`k8s-lb-controller` is a Kubernetes controller built with Kubebuilder and `controller-runtime`.
Проект решает узкую, понятную задачу: обрабатывает `Service` типа `LoadBalancer` с `loadBalancerClass: iedge.local/service-lb`, назначает им внешний IP из статического пула и синхронизирует file-based конфигурацию HAProxy по данным из `EndpointSlice`.

Текущая цель репозитория: defendable MVP без CRD, без Helm chart и без сложной сетевой логики. Контроллер работает только с built-in ресурсами Kubernetes:

- `core/v1 Service`
- `discovery.k8s.io/v1 EndpointSlice`

## Что делает проект

Для matching `Service` контроллер:

1. фильтрует ресурс по `spec.type == LoadBalancer`
2. проверяет `spec.loadBalancerClass == iedge.local/service-lb`
3. добавляет finalizer `iedge.local/service-lb-finalizer`
4. выделяет или переиспользует IP из `K8S_LB_CONTROLLER_IP_POOL`
5. идемпотентно обновляет `.status.loadBalancer.ingress`
6. читает связанные `EndpointSlice`
7. выбирает только ready IPv4 backend endpoints
8. рендерит aggregate HAProxy config через provider

При удалении managed `Service` контроллер:

1. вызывает provider cleanup
2. пересобирает aggregate HAProxy config без удаляемого сервиса
3. снимает finalizer только после успешного cleanup

## Архитектура MVP

Базовый поток выглядит так:

`Service -> Reconcile -> IPAM -> Status -> EndpointSlice discovery -> HAProxy provider -> haproxy.cfg`

Ключевые элементы:

- `Service` остаётся единственной управляемой сущностью, отдельные CRD не используются
- `loadBalancerClass` фиксирован по умолчанию: `iedge.local/service-lb`
- IPAM простой и статический, пул задаётся через env
- backend discovery опирается только на `EndpointSlice`
- provider file-based: хранит aggregate state in-memory и на каждом `Ensure/Delete` перерисовывает полный config
- итоговый HAProxy config пишется атомарно через временный файл и `rename`
- validate/reload команды optional и подходят для локального режима и для будущего containerized режима
- `/metrics` отдаёт как стандартные controller-runtime метрики, так и несколько кастомных MVP-метрик контроллера

## Что реализовано

Phase 1:

- scaffold Kubebuilder сохранён
- загрузка `.env` через `godotenv`
- manager и wiring controller-runtime
- Service reconciler
- фильтрация по `LoadBalancer` и `loadBalancerClass`

Phase 2:

- static IP pool
- deterministic IP allocation
- reuse уже назначенного валидного IP
- идемпотентное обновление `.status.loadBalancer.ingress`

Phase 3:

- provider interface
- in-memory mock provider
- finalizer flow
- provider cleanup при удалении `Service`

Phase 4:

- EndpointSlice-based backend discovery
- только ready IPv4 backends
- deterministic backend ordering
- file-based HAProxy provider
- deterministic aggregate HAProxy config
- cleanup provider state из итогового config при удалении `Service`

Phase 5a:

- unit tests доведены до MVP-level coverage по ключевым сценариям reconcile/IPAM/backends/provider
- e2e сценарий усилен для demo use-case
- добавлены базовые кастомные Prometheus metrics
- README доведён до состояния финального MVP
- оформлен короткий и воспроизводимый demo scenario

## Что сознательно не реализовано

Следующие возможности специально оставлены вне текущего MVP:

- Helm chart
- chart publishing
- новый provider
- CRD для IP pool
- multiple providers
- advanced HAProxy HTTP routing
- TLS termination
- advanced health checks
- UDP support
- persistent provider state
- external API-based HAProxy management
- split reconciliation into multiple controllers
- web UI

Helm chart intentionally отложен на следующий отдельный этап после стабилизации и финальной проверки кода.

## Конфигурация

Приложение при старте пытается загрузить `.env`.
Если файла нет, используются обычные env и default values.
Если переменная уже задана в окружении ОС или CI, она имеет приоритет над `.env`.

`Makefile` тоже автоматически подхватывает `.env`, если файл существует.

| Variable | Default |
| --- | --- |
| `K8S_LB_CONTROLLER_METRICS_ADDR` | `:8080` |
| `K8S_LB_CONTROLLER_HEALTH_ADDR` | `:8081` |
| `K8S_LB_CONTROLLER_LEADER_ELECT` | `false` |
| `K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS` | `iedge.local/service-lb` |
| `K8S_LB_CONTROLLER_IP_POOL` | `203.0.113.10,203.0.113.11,203.0.113.12` |
| `K8S_LB_CONTROLLER_REQUEUE_AFTER` | `30s` |
| `K8S_LB_CONTROLLER_LOG_LEVEL` | `info` |
| `K8S_LB_CONTROLLER_HAPROXY_CONFIG_PATH` | `/tmp/k8s-lb-controller-haproxy.cfg` |
| `K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND` | empty |
| `K8S_LB_CONTROLLER_HAPROXY_RELOAD_COMMAND` | empty |

Поддерживаемые уровни логирования: `debug`, `info`, `warn`, `error`.

В репозитории есть готовый [.env.example](.env.example):

```dotenv
K8S_LB_CONTROLLER_METRICS_ADDR=:8080
K8S_LB_CONTROLLER_HEALTH_ADDR=:8081
K8S_LB_CONTROLLER_LEADER_ELECT=false
K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS=iedge.local/service-lb
K8S_LB_CONTROLLER_IP_POOL=203.0.113.10,203.0.113.11,203.0.113.12
K8S_LB_CONTROLLER_REQUEUE_AFTER=30s
K8S_LB_CONTROLLER_LOG_LEVEL=info
K8S_LB_CONTROLLER_HAPROXY_CONFIG_PATH=/tmp/k8s-lb-controller-haproxy.cfg
K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND=
K8S_LB_CONTROLLER_HAPROXY_RELOAD_COMMAND=
```

### Validate / reload commands

Обе команды optional.

- если validate command пустой, config просто атомарно записывается
- если reload command пустой, это считается нормальным dev/demo режимом
- placeholder `{{config}}` заменяется на путь к config file
- для validate используется candidate file path
- для reload используется active config path

Пример validate-only режима:

```dotenv
K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND=haproxy -c -f {{config}}
```

Пример validate + reload:

```dotenv
K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND=haproxy -c -f {{config}}
K8S_LB_CONTROLLER_HAPROXY_RELOAD_COMMAND=/usr/local/bin/haproxy-reload {{config}}
```

## Пример managed Service

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

## Локальный запуск

Рекомендуемый путь для demo и ручной проверки: поднять `kind` или `k3d`, а сам controller запустить локально через `make run`.
Так проще всего показать и `EXTERNAL-IP`, и generated HAProxy config, и `/metrics` на хосте.

### Вариант 1. kind + локальный controller

```sh
kind create cluster --name k8s-lb-controller
cp .env.example .env
make run
```

Контроллер будет использовать текущий kubeconfig context.

### Вариант 2. k3d + локальный controller

```sh
k3d cluster create k8s-lb-controller
cp .env.example .env
make run
```

### Вариант 3. controller внутри кластера

Если нужен вариант ближе к CI/e2e:

```sh
make docker-build IMG=k8s-lb-controller:dev
kind load docker-image k8s-lb-controller:dev --name k8s-lb-controller
make deploy IMG=k8s-lb-controller:dev
```

Для `k3d` вместо `kind load` обычно используется:

```sh
k3d image import k8s-lb-controller:dev -c k8s-lb-controller
make deploy IMG=k8s-lb-controller:dev
```

## Demo workload

Для проверки контроллера достаточно простого `Deployment`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: demo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: demo
  template:
    metadata:
      labels:
        app: demo
    spec:
      containers:
        - name: nginx
          image: nginx:stable
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: demo
  namespace: demo
spec:
  type: LoadBalancer
  loadBalancerClass: iedge.local/service-lb
  selector:
    app: demo
  ports:
    - port: 80
      targetPort: 80
```

Применение:

```sh
kubectl create namespace demo
kubectl apply -f demo.yaml
```

Или через stdin:

```sh
kubectl create namespace demo
kubectl apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: demo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: demo
  template:
    metadata:
      labels:
        app: demo
    spec:
      containers:
        - name: nginx
          image: nginx:stable
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: demo
  namespace: demo
spec:
  type: LoadBalancer
  loadBalancerClass: iedge.local/service-lb
  selector:
    app: demo
  ports:
    - port: 80
      targetPort: 80
EOF
```

## Как проверить результат

Показать назначенный внешний IP:

```sh
kubectl get svc demo -n demo
kubectl get svc demo -n demo -o jsonpath='{.status.loadBalancer.ingress[0].ip}'; echo
```

Ожидаемо первым будет назначен IP `203.0.113.10`.

Проверить finalizer:

```sh
kubectl get svc demo -n demo -o jsonpath='{.metadata.finalizers}'; echo
```

Проверить связанные `EndpointSlice`:

```sh
kubectl get endpointslice -n demo -l kubernetes.io/service-name=demo -o wide
```

Проверить generated HAProxy config при локальном запуске контроллера:

```sh
cat /tmp/k8s-lb-controller-haproxy.cfg
```

Если контроллер запущен как Deployment внутри кластера, смотреть файл менее удобно, поэтому для защиты обычно проще использовать локальный `make run`.

## Metrics

Метрики доступны на стандартном endpoint `/metrics`.

При локальном запуске:

```sh
curl -s http://127.0.0.1:8080/metrics | grep '^k8s_lb_controller_'
```

При in-cluster deployment:

```sh
kubectl port-forward -n k8s-lb-controller-system \
  svc/k8s-lb-controller-controller-manager-metrics-service 8080:8080
curl -s http://127.0.0.1:8080/metrics | grep '^k8s_lb_controller_'
```

Добавленные MVP-метрики:

- `k8s_lb_controller_service_reconcile_total`
- `k8s_lb_controller_service_reconcile_errors_total`
- `k8s_lb_controller_service_reconcile_duration_seconds`
- `k8s_lb_controller_ip_allocations_total{result=...}`
- `k8s_lb_controller_provider_operations_total{operation=...,result=...}`
- `k8s_lb_controller_provider_managed_services`

## Удаление и cleanup

Удалить managed `Service`:

```sh
kubectl delete svc demo -n demo
```

Проверить, что `Service` не завис в `Terminating`:

```sh
kubectl wait --for=delete svc/demo -n demo --timeout=120s
```

Если контроллер запущен локально, после удаления можно снова открыть config и убедиться, что блоки для `demo` исчезли:

```sh
cat /tmp/k8s-lb-controller-haproxy.cfg
```

## Demo Scenario Для Защиты

Ниже самый простой и воспроизводимый сценарий. Он рассчитан на локальный запуск контроллера против `kind` или `k3d`, потому что так удобно одновременно показать логику контроллера, generated config и метрики.

1. Поднять кластер:

```sh
kind create cluster --name k8s-lb-controller
```

2. Запустить controller локально:

```sh
cp .env.example .env
make run
```

3. Развернуть demo app и managed `Service`:

```sh
kubectl create namespace demo
kubectl apply -f demo.yaml
```

4. Показать `EXTERNAL-IP`:

```sh
kubectl get svc demo -n demo
kubectl get svc demo -n demo -o jsonpath='{.status.loadBalancer.ingress[0].ip}'; echo
```

5. Показать finalizer:

```sh
kubectl get svc demo -n demo -o jsonpath='{.metadata.finalizers}'; echo
```

6. Показать `EndpointSlice`:

```sh
kubectl get endpointslice -n demo -l kubernetes.io/service-name=demo -o wide
```

7. Показать generated HAProxy config:

```sh
cat /tmp/k8s-lb-controller-haproxy.cfg
```

8. Увеличить число backend pod:

```sh
kubectl scale deployment demo -n demo --replicas=2
kubectl rollout status deployment/demo -n demo
```

9. Повторно показать `EndpointSlice` и HAProxy config:

```sh
kubectl get endpointslice -n demo -l kubernetes.io/service-name=demo -o wide
cat /tmp/k8s-lb-controller-haproxy.cfg
```

10. Показать метрики:

```sh
curl -s http://127.0.0.1:8080/metrics | grep '^k8s_lb_controller_'
```

11. Удалить `Service`:

```sh
kubectl delete svc demo -n demo
kubectl wait --for=delete svc/demo -n demo --timeout=120s
```

12. Показать cleanup:

```sh
cat /tmp/k8s-lb-controller-haproxy.cfg
```

Что удобно проговаривать на защите:

- контроллер не требует CRD и работает поверх built-in `Service`
- выбор ресурсов делается через `loadBalancerClass`
- IP назначается из статического пула детерминированно
- backend discovery идёт через `EndpointSlice`
- provider синхронизирует aggregate HAProxy config
- удаление безопасное благодаря finalizer
- базовая observability есть через `/metrics`

## Команды для разработки

```sh
make manifests
make lint
make test
make run
```

E2E:

```sh
make test-e2e
```

## Следующий этап

После стабилизации текущего MVP следующим отдельным этапом планируется packaging:

- Helm chart
- упаковка и публикация chart
- при необходимости дополнительная polish-интеграция для deployment workflow

На текущем этапе Helm chart intentionally не входит в готовую часть проекта.
