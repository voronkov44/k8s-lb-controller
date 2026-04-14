# Чеклист Готовности

Этот чеклист проверяет текущее состояние репозитория перед будущим шагом публикации или релиза. Он не создаёт релиз, не публикует артефакты и не меняет версии chart или приложения.

English version: [release-checklist.md](release-checklist.md)

См. также:

- [../README.ru.md](../README.ru.md)
- [dataplane-smoke.md](dataplane-smoke.md)
- [dataplane-smoke.ru.md](dataplane-smoke.ru.md)

## 1. Статические Проверки и Рендеринг

Запустите основную цель проверки готовности:

```sh
make verify-dataplane
```

Эта цель включает:

- `go test ./...`
- `make lint`
- сборку контроллера
- сборку dataplane
- рендеринг Kustomize для локального режима и режима dataplane
- `helm lint` для локального режима и режима dataplane
- рендеринг `helm template` для локального режима и режима dataplane

## 2. Необязательный e2e-Набор Для Dataplane

Если нужен дополнительный автоматизированный прогон в Kind, выполните:

```sh
make test-e2e-dataplane
```

Эта цель запускает e2e-набор для режима dataplane и затем очищает Kind-кластер, созданный для этого сценария.

## 3. Контролируемая Smoke-Проверка в Kind

Запустите end-to-end smoke-сценарий dataplane:

```sh
make smoke-dataplane-kind
```

Полезные варианты:

```sh
KEEP_CLUSTER=true make smoke-dataplane-kind
KEEP_DEPLOYMENT=true make smoke-dataplane-kind
DATAPLANE_KIND_CLUSTER=my-lab make smoke-dataplane-kind
```

Smoke-сценарий проверяет:

- связность развёртываний контроллера и dataplane
- согласование состояния управляемого `Service` типа `LoadBalancer`
- доставку требуемого состояния в dataplane API
- привязку внешнего IP к сетевому интерфейсу хоста dataplane
- готовность HAProxy listener на выделенном IP
- содержимое сгенерированной конфигурации HAProxy
- сквозную HTTP-доступность через dataplane-путь

Для пошаговой ручной проверки, диагностики и очистки используйте [dataplane-smoke.md](dataplane-smoke.md) или [dataplane-smoke.ru.md](dataplane-smoke.ru.md).

## 4. Явные Команды Для Рендеринга

Если нужно повторно выполнить проверки рендеринга напрямую, используйте:

```sh
./bin/kustomize build config/default
./bin/kustomize build config/default-dataplane
helm lint ./charts/k8s-lb-controller
helm lint ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
helm template k8s-lb-controller ./charts/k8s-lb-controller
helm template k8s-lb-controller ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
```

## 5. Что Этот Чеклист Не Утверждает

Этот чеклист проверяет текущий реализованный dataplane-путь в контролируемом окружении. Он не означает готовность к следующим вещам:

- изменению версий chart или приложения
- созданию GitHub-релиза
- размещению dataplane на нескольких узлах и HA-координации
- публикации внешних IP через BGP, ARP, NDP или по модели cloud provider
- более широкому производственному усилению надёжности за пределами текущей контролируемой одновузловой dataplane-модели
