# k8s-lb-controller

`k8s-lb-controller` is a Kubernetes controller built with Kubebuilder and `controller-runtime`.
Проект развивается инкрементально как MVP-контроллер для `Service` типа `LoadBalancer` без CRD и пока работает только с built-in ресурсами Kubernetes.

## Текущий статус

Сейчас реализована Phase 4 baseline:

- scaffold Kubebuilder сохранён
- built-in managed object остаётся `core/v1 Service`
- фильтрация managed `Service` по:
  - `spec.type == LoadBalancer`
  - `spec.loadBalancerClass != nil`
  - `spec.loadBalancerClass == K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS`
- статический IPAM поверх `K8S_LB_CONTROLLER_IP_POOL`
- идемпотентное обновление `.status.loadBalancer.ingress`
- finalizer на managed `Service`
- cleanup provider state при удалении `Service`
- backend discovery через `discovery.k8s.io/v1 EndpointSlice`
- real file-based HAProxy provider
- детерминированный render полного HAProxy config
- atomic write итогового конфига
- optional validate/reload commands

## Что именно делает контроллер

Для matching `Service` контроллер:

1. добавляет finalizer `iedge.local/service-lb-finalizer`
2. выделяет или переиспользует внешний IP из configured static pool
3. записывает IP в `.status.loadBalancer.ingress`
4. читает связанные `EndpointSlice`
5. извлекает только ready IPv4 backend endpoints
6. строит desired provider model: namespace, service name, class, external IP, service ports, backends
7. передаёт desired state в HAProxy provider

Для deleting managed `Service` контроллер:

1. видит `DeletionTimestamp`
2. вызывает cleanup в provider
3. пересобирает HAProxy config без удаляемого `Service`
4. снимает finalizer только после успешного cleanup

## EndpointSlice discovery

Phase 4 использует `EndpointSlice` как единственный источник backend discovery.

Текущий baseline:

- `EndpointSlice` ищутся по namespace и label `kubernetes.io/service-name=<service-name>`
- watcher на `EndpointSlice` маппит изменения обратно в конкретный owning `Service`
- Pod watch отдельно не используется
- берутся только ready endpoints
- используется только IPv4
- backend ordering детерминированный
- если backend'ов нет, provider всё равно получает корректное desired state

Порты матчятся простым и понятным образом:

- по имени `ServicePort.Name`
- либо по `targetPort`, если он задан именем
- либо по numeric `targetPort`
- если `targetPort` не задан, используется service port number

## HAProxy provider

Провайдер расположен в `internal/provider/haproxy`.

Текущая модель:

- controller собирает desired state для всех managed `Service`
- provider хранит aggregate state in-memory внутри процесса
- на каждом `Ensure` и `Delete` provider пересобирает полный итоговый config
- config рендерится детерминированно
- candidate config сначала пишется во временный файл
- затем файл атомарно заменяет active config
- после этого optionally выполняется reload command

Провайдер потокобезопасный, идемпотентный и не использует `panic`.

### Формат baseline config

Для каждого managed service port создаются:

- один `frontend`
- один `backend`

Baseline сейчас такой:

- `bind <externalIP>:<servicePort>`
- `mode tcp`
- `default_backend <derived-name>`
- `server <derived-name> <backend-ip>:<backend-port>`

Если backend'ов нет, provider рендерит disabled placeholder server, чтобы config оставался валидным.

Имена `frontend`/`backend`/`server` санитизируются и воспроизводимы.

## Переменные окружения

Приложение автоматически пытается загрузить `.env` при старте.
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

Пример `.env`:

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

- если `VALIDATE_COMMAND` пустой, config просто атомарно записывается
- если `RELOAD_COMMAND` пустой, это считается нормальным dev/demo режимом
- placeholder `{{config}}` заменяется на путь к config file
  - для validate это candidate path
  - для reload это active path

Пример validate-only:

```dotenv
K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND=haproxy -c -f {{config}}
```

Пример validate + reload через helper binary:

```dotenv
K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND=haproxy -c -f {{config}}
K8S_LB_CONTROLLER_HAPROXY_RELOAD_COMMAND=/usr/local/bin/haproxy-reload {{config}}
```

Важно:

- команды разбиваются по пробелам без shell parsing
- для сложной reload-логики лучше использовать wrapper binary/script в собственном image
- default manager image distroless, поэтому shell utilities внутри контейнера по умолчанию отсутствуют

## Локальный запуск

```sh
cp .env.example .env
make run
```

Или без `.env`:

```sh
make run
```

В dev-mode без reload command можно просто смотреть сгенерированный config локально:

```sh
cat /tmp/k8s-lb-controller-haproxy.cfg
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

## Demo Flow В kind

Создать кластер и задеплоить контроллер:

```sh
kind create cluster --name k8s-lb-controller
make docker-build IMG=k8s-lb-controller:dev
kind load docker-image k8s-lb-controller:dev --name k8s-lb-controller
make deploy IMG=k8s-lb-controller:dev
```

Создать namespace и demo workload:

```sh
kubectl create namespace demo
kubectl apply -n demo -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
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
EOF
```

Создать matching `Service`:

```sh
kubectl apply -n demo -f - <<'EOF'
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
EOF
```

Проверить `EXTERNAL-IP`, finalizer и связанные `EndpointSlice`:

```sh
kubectl get svc demo -n demo
kubectl get svc demo -n demo -o jsonpath='{.status.loadBalancer.ingress[0].ip}'; echo
kubectl get svc demo -n demo -o jsonpath='{.metadata.finalizers}'; echo
kubectl get endpointslice -n demo -l kubernetes.io/service-name=demo
```

Ожидаемо первым будет назначен IP `203.0.113.10`.

Если контроллер запущен локально через `make run`, посмотреть generated config можно так:

```sh
cat /tmp/k8s-lb-controller-haproxy.cfg
```

Если контроллер запущен как default distroless Deployment в kind, удобнее смотреть controller logs:

```sh
kubectl logs -n k8s-lb-controller-system deployment/k8s-lb-controller-controller-manager -c manager -f
```

При scale workload provider state должен обновляться после изменения `EndpointSlice`:

```sh
kubectl scale deployment demo -n demo --replicas=2
kubectl get endpointslice -n demo -l kubernetes.io/service-name=demo -o wide
```

Удалить `Service` и убедиться, что cleanup выполнен:

```sh
kubectl delete svc demo -n demo
kubectl get svc demo -n demo
kubectl logs -n k8s-lb-controller-system deployment/k8s-lb-controller-controller-manager -c manager --tail=100
```

## Локальная проверка

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

## Деплой в кластер

```sh
make docker-build docker-push IMG=<registry>/k8s-lb-controller:<tag>
make deploy IMG=<registry>/k8s-lb-controller:<tag>
```

Удаление:

```sh
make undeploy
```

## Что сознательно ещё не реализовано

- advanced HAProxy HTTP routing features
- TLS termination
- advanced health checks
- UDP support
- multiple providers
- CRD для IP pool
- persistent provider state вне процесса
- Helm chart
- Prometheus / ServiceMonitor expansion
- external API-based HAProxy management
- отдельные контроллеры под split reconciliation
