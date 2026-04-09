# k8s-lb-controller

`k8s-lb-controller` is a `controller-runtime` based Kubernetes controller for selected `Service` objects of type `LoadBalancer`.
It allocates external IPv4 addresses from a static pool, discovers ready backends from `EndpointSlice`, and syncs desired load balancer state through a provider abstraction.

Russian version: [README.ru.md](README.ru.md)

## Current Scope

The repository now supports two runtime provider modes for the controller:

- `local-haproxy`: the original local file-based HAProxy provider, still the default
- `dataplane-api`: the controller sends desired state to a separate dataplane HTTP API

The standalone dataplane component is implemented in `cmd/dataplane`.
It keeps full in-memory desired state, renders one deterministic HAProxy config for all managed services, and applies that config atomically.

Important current limitation:

- Stage 5 keeps the real HAProxy dataplane runtime for controlled single-node and lab environments.
- It now supports netlink-based host-side IP attachment by default, with the stage-4 exec path kept as a fallback.
- It still does not implement multi-node/HA dataplane placement or broader production networking semantics.

## Deployment Modes

### Local Mode

Local mode keeps the original behavior:

- the controller runs alone
- provider mode is `local-haproxy`
- the controller writes the HAProxy config itself

This mode remains the default in code, Kustomize, and Helm.

### Dataplane Mode

Dataplane mode deploys two components:

- the controller as the control-plane component
- the dataplane pod as a separate component with:
  - the dataplane API server
  - an HAProxy sidecar process that actually listens for traffic

In this mode:

- the controller uses `K8S_LB_CONTROLLER_PROVIDER_MODE=dataplane-api`
- the controller sends `PUT /services/{namespace}/{name}` and `DELETE /services/{namespace}/{name}` requests to the dataplane service
- the dataplane process stores all desired services in memory and renders/applies one aggregate HAProxy config
- the dataplane pod uses host networking plus host-side IP attachment on one configured interface
- stage 5 defaults to a netlink-based attachment backend and still supports the stage-4 exec backend as a fallback

This mode is intentionally aimed at demos, local labs, and single-node controlled environments.

## Repository Layout

```text
cmd/main.go                      Controller binary
cmd/dataplane/main.go            Dataplane binary
internal/config/                 Controller runtime configuration
internal/dataplane/              Reusable dataplane engine, HTTP handler, render/apply logic
internal/controller/             Service reconcile logic
internal/ipam/                   Static IPv4 pool allocation
internal/backends/               EndpointSlice-based backend discovery
internal/provider/               Provider interface and provider implementations
internal/provider/haproxy/       Local file-based HAProxy provider
config/default/                  Kustomize entrypoint for controller-only local mode
config/dataplane/                Dataplane Deployment and Service manifests
config/default-dataplane/        Kustomize entrypoint for controller + dataplane mode
charts/k8s-lb-controller/        Helm chart
```

## Build Targets

The repository now has dedicated build paths for both binaries and both images:

```sh
make build
make build-dataplane
make docker-build
make docker-build-dataplane
```

Useful deployment-oriented targets:

```sh
make deploy
make deploy-dataplane
make build-installer
make build-installer-dataplane
```

## Kustomize

The repository keeps the existing controller-only Kustomize entrypoint unchanged:

- local mode: `config/default`

It also adds a separate additive entrypoint for controller + dataplane mode:

- dataplane mode: `config/default-dataplane`

### Render Local Mode

```sh
./bin/kustomize build config/default
```

### Render Dataplane Mode

```sh
./bin/kustomize build config/default-dataplane
```

### Deploy Local Mode

```sh
make deploy IMG=ghcr.io/voronkov44/k8s-lb-controller:dev
```

### Deploy Dataplane Mode

```sh
make deploy-dataplane \
  IMG=ghcr.io/voronkov44/k8s-lb-controller:dev \
  DATAPLANE_IMG=ghcr.io/voronkov44/k8s-lb-controller-dataplane:dev
```

In the dataplane Kustomize entrypoint, the controller is wired to the in-cluster service URL:

`http://k8s-lb-controller-dataplane.k8s-lb-controller-system.svc:8090`

The dataplane manifests also enable host networking, `shareProcessNamespace`, a real HAProxy sidecar, and netlink-based IP attachment on interface `eth0` by default. Exec mode remains available through the dataplane IP attach mode setting.

## Helm

The Helm chart supports both modes without removing local mode.

Detailed chart usage is documented in:

- [charts/k8s-lb-controller/README.md](charts/k8s-lb-controller/README.md)
- [charts/k8s-lb-controller/README.ru.md](charts/k8s-lb-controller/README.ru.md)

### Template Local Mode

```sh
helm template k8s-lb-controller ./charts/k8s-lb-controller
```

### Template Dataplane Mode

```sh
helm template k8s-lb-controller ./charts/k8s-lb-controller \
  --set controller.providerMode=dataplane-api \
  --set dataplane.enabled=true
```

Helm values for dataplane mode include:

- `controller.providerMode`
- `controller.dataplane.apiURL`
- `controller.dataplane.apiTimeout`
- `dataplane.enabled`
- `dataplane.hostNetwork`
- `dataplane.shareProcessNamespace`
- `dataplane.image.*`
- `dataplane.interface`
- `dataplane.ipAttach.*`
- `dataplane.http.port`
- `dataplane.http.addr`
- `dataplane.haproxy.*`
- `dataplane.haproxy.image.*`
- `dataplane.logLevel`
- `dataplane.gracefulShutdownTimeout`
- `dataplane.resources`
- `dataplane.nodeSelector`
- `dataplane.tolerations`
- `dataplane.affinity`

If `controller.dataplane.apiURL` is not set and `dataplane.enabled=true`, the chart generates the in-cluster dataplane service URL automatically.
The chart defaults also enable the real dataplane runtime path with host networking, a shared-pid HAProxy sidecar, and `dataplane.ipAttach.mode=netlink`. Exec mode remains available as a fallback.

## Runtime Configuration

The controller still uses the original environment variables for IP allocation and local HAProxy settings.
Stage 1 added:

- `K8S_LB_CONTROLLER_PROVIDER_MODE`
- `K8S_LB_CONTROLLER_DATAPLANE_API_URL`
- `K8S_LB_CONTROLLER_DATAPLANE_API_TIMEOUT`

The dataplane server uses:

- `K8S_LB_DATAPLANE_HTTP_ADDR`
- `K8S_LB_DATAPLANE_HAPROXY_CONFIG_PATH`
- `K8S_LB_DATAPLANE_HAPROXY_VALIDATE_COMMAND`
- `K8S_LB_DATAPLANE_HAPROXY_RELOAD_COMMAND`
- `K8S_LB_DATAPLANE_HAPROXY_PID_FILE`
- `K8S_LB_DATAPLANE_LOG_LEVEL`
- `K8S_LB_DATAPLANE_GRACEFUL_SHUTDOWN_TIMEOUT`
- `K8S_LB_DATAPLANE_IP_ATTACH_ENABLED`
- `K8S_LB_DATAPLANE_IP_ATTACH_MODE`
- `K8S_LB_DATAPLANE_INTERFACE`
- `K8S_LB_DATAPLANE_IP_COMMAND`
- `K8S_LB_DATAPLANE_IP_CIDR_SUFFIX`

## Verification

Repository-level verification:

```sh
go test ./...
make lint
```

Manifest and chart rendering:

```sh
./bin/kustomize build config/default
./bin/kustomize build config/default-dataplane
helm template k8s-lb-controller ./charts/k8s-lb-controller
helm template k8s-lb-controller ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
```

## What Is Deferred To Stage 6

Stage 5 improves the host-side IP management quality, but several production-oriented items are still intentionally deferred:

- multi-node or HA dataplane placement and coordination
- BGP, ARP/NDP, cloud-provider, or other advanced network publication semantics
- broader production hardening around host integration
- live end-to-end traffic validation against the deployed dataplane
