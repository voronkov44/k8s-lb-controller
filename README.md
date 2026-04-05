# k8s-lb-controller

`k8s-lb-controller` is a Kubernetes controller built with Kubebuilder and `controller-runtime`.
Проект предназначен для интеграции `Service` типа `LoadBalancer` с внешним балансировщиком, но сейчас реализован только baseline без provider и без CRD.

## Текущий статус

Сейчас реализована Phase 2 MVP:

- используется built-in ресурс `core/v1 Service`
- CRD не используются
- контроллер отслеживает только `Service`
- в обработку попадают только сервисы:
  - `spec.type == LoadBalancer`
  - `spec.loadBalancerClass != nil`
  - `spec.loadBalancerClass == K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS`
- matching `Service` получает внешний IP из настроенного статического пула
- IP записывается в `.status.loadBalancer.ingress`
- при повторных reconcile статус не переписывается без необходимости
- allocator опирается только на текущее состояние живых managed `Service`

Пока сознательно не реализованы:

- provider
- HAProxy integration
- EndpointSlice logic
- finalizer и cleanup
- persistent state для аллокаций
- CRD и Helm chart

## Структура

- `cmd/main.go` — entrypoint, manager setup, logger, healthz/readyz
- `internal/config/config.go` — env-конфиг и валидация
- `internal/controller/service_controller.go` — reconcile для `Service`
- `internal/controller/service_filter.go` — фильтрация managed `Service`
- `internal/ipam` — статический IPAM поверх configured pool
- `internal/status` — helper для `Service.status.loadBalancer.ingress`

## Требования

- Go `1.26`
- Docker
- kubectl
- kind или k3d для локальной проверки в кластере

## Переменные окружения

Приложение автоматически пытается загрузить `.env` при старте.
Если файла нет, используются обычные переменные окружения и default values.
Если переменная уже задана в окружении ОС или CI, она имеет приоритет над значением из `.env`.

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

Можно и без `.env`:

```sh
make run
```

Переопределение из shell по-прежнему работает:

```sh
K8S_LB_CONTROLLER_LOG_LEVEL=debug make run
```

## Что сейчас умеет контроллер

Во время reconcile контроллер:

1. читает `Service` по `namespace/name`
2. игнорирует отсутствующие объекты без ошибки
3. игнорирует все сервисы, которые не подходят под фильтр
4. пропускает `Service`, который уже удаляется
5. вычисляет занятые IP по текущим managed `Service`
6. сохраняет уже назначенный корректный IP, если он не конфликтует
7. иначе выбирает первый свободный IP из пула
8. обновляет `.status.loadBalancer.ingress` только при реальном изменении
9. возвращает `ctrl.Result{RequeueAfter: ...}`

После назначения IP `kubectl get svc` начинает показывать `EXTERNAL-IP`.

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

Применить matching `Service`:

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

Проверить, что появился `EXTERNAL-IP`:

```sh
kubectl get svc demo -n demo
kubectl get svc demo -n demo -o jsonpath='{.status.loadBalancer.ingress[0].ip}'; echo
kubectl logs -n k8s-lb-controller-system deployment/k8s-lb-controller-controller-manager -c manager -f
```

Ожидаемо первым будет назначен IP `203.0.113.10`.

## Локальная проверка

```sh
make manifests
make lint
make test
```

E2E:

```sh
make test-e2e
```

## Деплой в кластер

Сборка и публикация образа:

```sh
make docker-build docker-push IMG=<registry>/k8s-lb-controller:<tag>
```

Деплой:

```sh
make deploy IMG=<registry>/k8s-lb-controller:<tag>
```

Удаление:

```sh
make undeploy
```

## Примечания

- `make install` и `make uninstall` сейчас ничего не делают, потому что в проекте нет CRD
- scaffold Kubebuilder сохранён
- `K8S_LB_CONTROLLER_IP_POOL` должен содержать уникальные IPv4-адреса через запятую
