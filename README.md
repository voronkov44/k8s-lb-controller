# k8s-lb-controller

`k8s-lb-controller` is a `controller-runtime` based Kubernetes controller for selected `Service` objects of type `LoadBalancer`. It allocates external IPv4 addresses from a static pool, discovers ready backends from `EndpointSlice`, and syncs load-balancer state through a provider abstraction.

Russian version: [README.ru.md](README.ru.md)

## Current Project State

The controller currently supports two provider modes:

- `local-haproxy`: the original controller-only mode. It remains available, backward-compatible, and is still the default.
- `dataplane-api`: the controller sends desired state to a separate dataplane component over HTTP.

The standalone dataplane lives in `cmd/dataplane/main.go`. In the current implementation it provides:

- an HTTP API for controller-to-dataplane synchronization
- an in-memory desired state store for managed services
- deterministic HAProxy configuration rendering
- an atomic apply flow with HAProxy validation and reload support
- a Kubernetes rollout that pairs the dataplane API container with a real HAProxy sidecar
- external IP attachment backends for `netlink` and `exec`, with `netlink` as the default and `exec` as a fallback

## Deployment Modes

### Local Mode

Local mode preserves the original behavior:

- only the controller is deployed
- `controller.providerMode` is `local-haproxy`
- the controller renders and applies the HAProxy configuration itself

This mode remains the default in the codebase, Kustomize, and Helm.

### Dataplane Mode

Dataplane mode deploys the controller and a separate dataplane rollout:

- the controller acts as the control-plane component
- the dataplane API stores desired state in memory and renders the shared HAProxy configuration
- the dataplane rollout runs a real HAProxy sidecar
- the dataplane pod uses host networking and attaches the assigned external IP on one configured host interface

This mode is intended for controlled single-node and lab environments.

## Build, Run, and Deploy

Build targets:

```sh
make build
make build-dataplane
make docker-build
make docker-build-dataplane
```

Local development run targets:

```sh
make run
make run-dataplane
```

Deployment-oriented targets:

```sh
make deploy
make deploy-dataplane
make build-installer
make build-installer-dataplane
```

Validation-oriented targets:

```sh
make verify-dataplane
make smoke-dataplane-kind
make test-e2e-dataplane
make kind-up-dataplane
make kind-down-dataplane
```

## Kustomize

The repository keeps two Kustomize entrypoints:

- `config/default`: controller-only local mode
- `config/default-dataplane`: controller plus separate dataplane mode

Render them with the repo-managed `kustomize` binary:

```sh
make kustomize
./bin/kustomize build config/default
./bin/kustomize build config/default-dataplane
```

Deploy local mode:

```sh
make deploy IMG=ghcr.io/voronkov44/k8s-lb-controller:dev
```

Deploy dataplane mode:

```sh
make deploy-dataplane \
  IMG=ghcr.io/voronkov44/k8s-lb-controller:dev \
  DATAPLANE_IMG=ghcr.io/voronkov44/k8s-lb-controller-dataplane:dev
```

In the dataplane overlay, the controller is wired to the in-cluster dataplane URL `http://k8s-lb-controller-dataplane.k8s-lb-controller-system.svc:8090`. The dataplane deployment enables host networking, `shareProcessNamespace`, and host-side IP attachment on `eth0` by default.

## Helm

The Helm chart also supports both modes.

Install local mode from this checkout:

```sh
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system \
  --create-namespace
```

Install dataplane mode from this checkout:

```sh
helm install k8s-lb-controller ./charts/k8s-lb-controller \
  -n k8s-lb-controller-system \
  --create-namespace \
  --set controller.providerMode=dataplane-api \
  --set dataplane.enabled=true
```

If `controller.dataplane.apiURL` is left empty and `dataplane.enabled=true`, the chart generates the in-cluster dataplane service URL automatically. In dataplane mode, the chart defaults to `dataplane.ipAttach.mode=netlink`; `exec` remains available as a fallback.

Detailed chart usage lives in:

- [charts/k8s-lb-controller/README.md](charts/k8s-lb-controller/README.md)
- [charts/k8s-lb-controller/README.ru.md](charts/k8s-lb-controller/README.ru.md)

## Validation and Smoke Flows

Use `make verify-dataplane` for release-readiness checks that do not require a live dataplane run. It covers:

- `go test ./...`
- `make lint`
- controller and dataplane builds
- Kustomize rendering for local and dataplane modes
- Helm lint and template rendering for local and dataplane modes

Use `make smoke-dataplane-kind` for the automated Kind smoke flow that builds images, deploys controller plus dataplane mode, deploys the demo workload, and checks external IP allocation, IP attachment, HAProxy listener readiness, rendered configuration, and end-to-end HTTP reachability.

Use `make test-e2e-dataplane` for the dataplane-mode e2e test suite on Kind. Use `make kind-up-dataplane` and `make kind-down-dataplane` when you want to manage the smoke-test Kind cluster manually.

Detailed validation guides:

- Smoke validation: [docs/dataplane-smoke.md](docs/dataplane-smoke.md), [docs/dataplane-smoke.ru.md](docs/dataplane-smoke.ru.md)
- Release-readiness checklist: [docs/release-checklist.md](docs/release-checklist.md), [docs/release-checklist.ru.md](docs/release-checklist.ru.md)