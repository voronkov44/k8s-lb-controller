# k8s-lb-controller

`k8s-lb-controller` is a Kubernetes controller built with Kubebuilder and `controller-runtime`.
Проект предназначен для последующей интеграции `Service` типа `LoadBalancer` с внешним балансировщиком.

## Текущий статус

Сейчас реализована только Phase 1 MVP:

- используется built-in ресурс `core/v1 Service`
- CRD пока не используются
- контроллер отслеживает `Service`
- в обработку попадают только сервисы:
  - `spec.type == LoadBalancer`
  - `spec.loadBalancerClass != nil`
  - `spec.loadBalancerClass == K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS`
- подходящие сервисы только логируются и ставятся на повторную обработку через `RequeueAfter`
- IPAM, provider, HAProxy, EndpointSlice logic, finalizer и status updates пока не реализованы

## Структура Phase 1

- `cmd/main.go` — entrypoint, manager setup, logger, healthz/readyz
- `internal/config/config.go` — загрузка env-конфига
- `internal/controller/service_controller.go` — `ServiceReconciler`
- `internal/controller/service_filter.go` — фильтрация `Service`
- `internal/controller/setup.go` — регистрация контроллеров

## Требования

- Go `1.26`
- Docker
- kubectl
- kind или k3d для локальной проверки в кластере

## Переменные окружения

См. [`.env.example`](/Users/f1lzz/GolandProjects/k8s-controller/k8s-lb-controller/.env.example).

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
| `K8S_LB_CONTROLLER_REQUEUE_AFTER` | `30s` |
| `K8S_LB_CONTROLLER_LOG_LEVEL` | `info` |

Поддерживаемые уровни логирования: `debug`, `info`, `warn`, `error`.

## Локальный запуск

```sh
cp .env.example .env
make run
```

Можно и без `.env`:

```sh
make run
```

Или переопределить конкретную переменную из shell:

```sh
K8S_LB_CONTROLLER_LOG_LEVEL=debug make run
```

## Что сейчас умеет контроллер

Во время reconcile контроллер:

1. читает `Service` по `namespace/name`
2. игнорирует отсутствующие объекты без ошибки
3. игнорирует все сервисы, которые не подходят под фильтр
4. логирует подходящий `Service`
5. возвращает `ctrl.Result{RequeueAfter: ...}`

## Пример Service, который попадет в обработку

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
      targetPort: 8080
```

## Запуск в kind

```sh
kind create cluster --name k8s-lb-controller
make docker-build IMG=k8s-lb-controller:dev
kind load docker-image k8s-lb-controller:dev --name k8s-lb-controller
make deploy IMG=k8s-lb-controller:dev
```

Проверка:

```sh
kubectl get pods -n k8s-lb-controller-system
kubectl logs -n k8s-lb-controller-system deployment/k8s-lb-controller-controller-manager -c manager -f
kubectl apply -f ./service.yaml
```

Удаление:

```sh
make undeploy
kind delete cluster --name k8s-lb-controller
```

## Запуск в k3d

```sh
k3d cluster create k8s-lb-controller
make docker-build IMG=k8s-lb-controller:dev
k3d image import k8s-lb-controller:dev -c k8s-lb-controller
make deploy IMG=k8s-lb-controller:dev
```

Проверка:

```sh
kubectl get pods -n k8s-lb-controller-system
kubectl logs -n k8s-lb-controller-system deployment/k8s-lb-controller-controller-manager -c manager -f
```

Удаление:

```sh
make undeploy
k3d cluster delete k8s-lb-controller
```

## Локальная проверка lint/test

```sh
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

- `make install` и `make uninstall` сейчас ничего не устанавливают, потому что в Phase 1 нет CRD
- scaffold Kubebuilder сохранен; проект не пересоздавался
- Helm предполагается как следующий шаг, но в Phase 1 deployment остается на Kustomize-манифестах Kubebuilder
