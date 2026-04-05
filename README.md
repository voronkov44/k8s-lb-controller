# k8s-lb-controller

`k8s-lb-controller` is a Kubernetes controller built with Kubebuilder and `controller-runtime`.
Проект развивается поэтапно как контроллер для `Service` типа `LoadBalancer` без CRD и пока использует только built-in ресурс `core/v1 Service`.

## Текущий статус

Сейчас реализована Phase 3 baseline:

- фильтрация managed `Service` по:
  - `spec.type == LoadBalancer`
  - `spec.loadBalancerClass != nil`
  - `spec.loadBalancerClass == K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS`
- статический IPAM поверх `K8S_LB_CONTROLLER_IP_POOL`
- обновление `.status.loadBalancer.ingress`
- идемпотентный reconcile без лишних status update
- mock provider с in-memory state
- finalizer на managed `Service`
- cleanup provider state при удалении `Service`

## Что именно умеет контроллер

Для matching `Service` контроллер:

1. добавляет finalizer `iedge.local/service-lb-finalizer`
2. выделяет или переиспользует внешний IP из configured static pool
3. записывает IP в `.status.loadBalancer.ingress`
4. синхронизирует desired state в mock provider
5. повторно не переписывает status без необходимости

Для deleting managed `Service` контроллер:

1. видит `DeletionTimestamp`
2. вызывает cleanup в mock provider
3. удаляет finalizer
4. не оставляет объект зависшим в `Terminating`

Важно: provider state сейчас только mock/in-memory и нужен как подготовка к будущему реальному HAProxy provider.

## Пока сознательно не реализовано

- реальный HAProxy provider
- EndpointSlice logic
- backend discovery
- multi-port backend behavior
- CRD
- Helm chart
- persistent provider state
- cleanup `Service.status` при удалении

## Структура

- `cmd/main.go` — entrypoint и wiring зависимостей
- `internal/config` — env-конфиг
- `internal/controller` — reconcile, filtering, finalizer flow
- `internal/ipam` — stateless-ish IP allocation
- `internal/status` — helper для `Service.status.loadBalancer.ingress`
- `internal/provider` — provider interface и mock provider

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
```

## Локальный запуск

```sh
cp .env.example .env
make run
```

Или без `.env`:

```sh
make run
```

## Пример matching Service

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

Проверить `EXTERNAL-IP` и finalizer:

```sh
kubectl get svc demo -n demo
kubectl get svc demo -n demo -o jsonpath='{.status.loadBalancer.ingress[0].ip}'; echo
kubectl get svc demo -n demo -o jsonpath='{.metadata.finalizers}'; echo
```

Ожидаемо первым будет назначен IP `203.0.113.10`.

Удалить `Service` и убедиться, что объект не зависает:

```sh
kubectl delete svc demo -n demo
kubectl get svc demo -n demo
```

Контроллер перед удалением выполнит cleanup mock provider state и затем снимет finalizer.

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

## Примечания

- scaffold Kubebuilder сохранён
- `K8S_LB_CONTROLLER_IP_POOL` должен содержать уникальные IPv4-адреса через запятую
- mock provider не является source of truth для IP allocation
