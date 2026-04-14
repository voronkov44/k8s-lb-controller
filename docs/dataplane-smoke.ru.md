# Smoke-Проверка Dataplane

Это руководство проверяет текущий путь контроллера и dataplane по полному сценарию в контролируемом одновузловом окружении Kind или на лабораторном стенде.

English version: [dataplane-smoke.md](dataplane-smoke.md)

См. также:

- [../README.ru.md](../README.ru.md)
- [release-checklist.md](release-checklist.md)
- [release-checklist.ru.md](release-checklist.ru.md)

## Что Показывает Эта Проверка

Smoke-сценарий проверяет реализованный dataplane-путь, включая:

- связность развёртываний контроллера и dataplane
- выделение внешнего IP для управляемого `Service` типа `LoadBalancer`
- привязку внешнего IP к сетевому интерфейсу хоста dataplane
- готовность HAProxy listener на выделенном внешнем IP
- содержимое сгенерированной конфигурации HAProxy
- сквозную HTTP-доступность до демонстрационного backend

Он не проверяет размещение dataplane на нескольких узлах, HA-координацию, BGP, ARP, NDP, публикацию по модели cloud provider и широкое производственное усиление надёжности.

## Предварительные Условия

- локально должны быть доступны `docker`, `kind` и `kubectl`
- окружение должно разрешать pod dataplane использовать host networking и capability `NET_ADMIN`
- перед живой smoke-проверкой рекомендуется выполнить `make verify-dataplane`

Путь smoke-проверки по умолчанию рассчитан на текущие значения репозитория:

- имя Kind-кластера `k8s-lb-controller-dataplane-smoke`
- интерфейс dataplane `eth0`
- ширина прикрепляемого внешнего IP `/32`
- первый выделяемый внешний IP `203.0.113.10`

Если вы меняете IP-пул, интерфейс dataplane или связанные манифесты, скорректируйте ручные проверки.

## Автоматизированный Smoke-Сценарий в Kind

Запустите полную автоматизированную smoke-проверку:

```sh
make smoke-dataplane-kind
```

`make smoke-dataplane` — это алиас для того же сценария.

Эта цель:

- создаёт или переиспользует Kind-кластер
- собирает образы контроллера и dataplane
- загружает оба образа в Kind
- разворачивает режим с контроллером и dataplane
- разворачивает демонстрационный backend и демонстрационный клиент из [`hack/dataplane-smoke.yaml`](../hack/dataplane-smoke.yaml)
- ждёт выделения внешнего IP
- проверяет логи dataplane, состояние привязанного IP, состояние HAProxy listener, сгенерированную конфигурацию и сквозной HTTP-трафик
- автоматически собирает диагностику при ошибке

Полезные опции:

```sh
KEEP_CLUSTER=true make smoke-dataplane-kind
KEEP_DEPLOYMENT=true make smoke-dataplane-kind
DATAPLANE_KIND_CLUSTER=my-lab make smoke-dataplane-kind
```

## Ручной Сценарий Проверки

Сначала задайте повторно используемые переменные:

```sh
CLUSTER_NAME="${DATAPLANE_KIND_CLUSTER:-k8s-lb-controller-dataplane-smoke}"
MANAGER_IMAGE="example.com/k8s-lb-controller:e2e"
DATAPLANE_IMAGE="example.com/k8s-lb-controller-dataplane:e2e"
```

### 1. Создайте или Переиспользуйте Kind-Кластер

```sh
DATAPLANE_KIND_CLUSTER="${CLUSTER_NAME}" make kind-up-dataplane
```

### 2. Соберите Образы

```sh
make docker-build IMG="${MANAGER_IMAGE}"
make docker-build-dataplane DATAPLANE_IMG="${DATAPLANE_IMAGE}"
```

### 3. Загрузите Образы в Kind

```sh
kind load docker-image "${MANAGER_IMAGE}" --name "${CLUSTER_NAME}"
kind load docker-image "${DATAPLANE_IMAGE}" --name "${CLUSTER_NAME}"
```

### 4. Разверните Режим с Контроллером и Dataplane

```sh
make deploy-dataplane \
  IMG="${MANAGER_IMAGE}" \
  DATAPLANE_IMG="${DATAPLANE_IMAGE}"
```

Дождитесь готовности обоих развёртываний:

```sh
kubectl rollout status deployment/k8s-lb-controller-controller-manager -n k8s-lb-controller-system --timeout=180s
kubectl rollout status deployment/k8s-lb-controller-dataplane -n k8s-lb-controller-system --timeout=180s
```

### 5. Разверните Демонстрационную Нагрузку

```sh
kubectl apply -f hack/dataplane-smoke.yaml
kubectl rollout status deployment/demo -n k8s-lb-controller-smoke --timeout=180s
kubectl rollout status deployment/demo-client -n k8s-lb-controller-smoke --timeout=180s
```

### 6. Проверьте Выделение Внешнего IP

```sh
kubectl get svc demo-lb -n k8s-lb-controller-smoke -o wide
kubectl get svc demo-lb -n k8s-lb-controller-smoke -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

С текущим IP-пулом по умолчанию первый управляемый `Service` должен получить `203.0.113.10`.

### 7. Проверьте Логи Dataplane и Состояние Привязанного IP

```sh
kubectl logs -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane
kubectl exec -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane -- \
  sh -ec 'ip -4 addr show dev eth0'
```

Вы должны увидеть, что выделенный внешний IP привязан к интерфейсу хоста dataplane как `/32`.

### 8. Проверьте HAProxy Listener и Сгенерированную Конфигурацию

```sh
kubectl exec -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane -- \
  sh -ec 'ss -ltnp'
kubectl exec -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane -- \
  cat /var/run/k8s-lb-dataplane/haproxy.cfg
```

Вы должны увидеть, что HAProxy слушает на выделенном внешнем IP и порту `80`, а в конфигурации есть строка `bind <external-ip>:80` и ожидаемые backend-записи.

### 9. Проверьте Сквозную HTTP-Доступность

```sh
EXTERNAL_IP="$(kubectl get svc demo-lb -n k8s-lb-controller-smoke -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"
kubectl exec -n k8s-lb-controller-smoke deploy/demo-client -c toolbox -- \
  wget -qO- "http://${EXTERNAL_IP}/"
```

Ответ должен содержать `Welcome to nginx!`.

## Диагностика Сбоев

Если автоматизированный smoke-сценарий завершается с ошибкой, он собирает:

- `kubectl get pods -A -o wide`
- `kubectl get svc -A`
- `kubectl get endpointslices -A`
- вывод `kubectl describe` для контроллера, dataplane и smoke `Service`
- логи контроллера
- логи dataplane API
- логи sidecar HAProxy
- вывод dataplane `ip addr`
- вывод dataplane `ss -ltnp`
- сгенерированную конфигурацию HAProxy

Если кластер или развёртывание сохранены, наиболее полезны такие команды:

```sh
kubectl get pods -A -o wide
kubectl get svc -A
kubectl logs -n k8s-lb-controller-system deploy/k8s-lb-controller-controller-manager
kubectl logs -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane
kubectl logs -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c haproxy
```

## Очистка

Удалите демонстрационную нагрузку и развёртывание контроллера с dataplane:

```sh
kubectl delete -f hack/dataplane-smoke.yaml --ignore-not-found
make undeploy-dataplane ignore-not-found=true
```

Удалите Kind-кластер, использовавшийся для smoke-проверки:

```sh
DATAPLANE_KIND_CLUSTER="${DATAPLANE_KIND_CLUSTER:-k8s-lb-controller-dataplane-smoke}" make kind-down-dataplane
```
