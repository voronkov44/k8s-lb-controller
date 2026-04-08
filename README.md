# k8s-lb-controller

A `controller-runtime`-based Kubernetes controller that manages selected `Service` objects of type `LoadBalancer` by allocating external IPs from a static pool, discovering backends from `EndpointSlice`, and syncing provider state.

Russian version: [README.ru.md](README.ru.md)

## Overview

`k8s-lb-controller` is an intentionally scoped controller project built around Kubernetes built-in resources.
It watches `Service` objects as the primary resource and reacts to related `EndpointSlice` updates.

For matching `Service` objects, the controller:

- processes only `spec.type: LoadBalancer`
- filters by a configured `spec.loadBalancerClass`
- allocates or reuses an external IPv4 address from a configured static pool
- publishes that address into `.status.loadBalancer.ingress`
- discovers ready IPv4 backends from `EndpointSlice`
- builds desired provider state and applies it through a provider abstraction

In the current MVP, the runtime provider is file-based and renders a deterministic HAProxy configuration.
This keeps the project grounded in real controller behavior without hiding the control-plane logic behind a cloud provider or a custom API.

## Why This Project Exists

Kubernetes defines the `LoadBalancer` service model, but the implementation is always environment-specific.
In managed clouds, that logic is usually provided by the platform. In local clusters, bare-metal labs, demos, and controlled setups, a smaller controller that makes the behavior explicit can be easier to understand and easier to experiment with.

This repository exists as an intentionally scoped load balancer controller project: compact enough to read end to end, but realistic enough to demonstrate reconcile design, finalizers, status publication, backend discovery, provider integration, testing, and deployment workflow.
It is a lightweight controller project and a production-style MVP for learning, demos, and controlled environments, not a replacement for systems such as MetalLB or cloud-provider load balancers.

## Installation

Canonical repository: [github.com/voronkov44/k8s-lb-controller](https://github.com/voronkov44/k8s-lb-controller)

Packaged installation is available as an OCI Helm chart:

```sh
helm install k8s-lb-controller oci://ghcr.io/voronkov44/charts/k8s-lb-controller \
  --version 0.1.0 \
<<<<<<< HEAD
  -n k8s-lb-controller-system --create-namespace
=======
  -n k8s-lb-controller-system \
  --create-namespace
>>>>>>> main
```

Detailed Helm usage, values, and chart-specific notes are documented in:

- [charts/k8s-lb-controller/README.md](charts/k8s-lb-controller/README.md)
- [charts/k8s-lb-controller/README.ru.md](charts/k8s-lb-controller/README.ru.md)

Kustomize manifests remain available for development, debugging, and manifest-based installation.
<<<<<<< HEAD
Release artifacts and chart publication are tracked through GitHub Releases.
=======
Release notes and published artifacts are available in GitHub Releases: [github.com/voronkov44/k8s-lb-controller/releases](https://github.com/voronkov44/k8s-lb-controller/releases).
>>>>>>> main

## Current MVP Scope

The repository is feature-complete for the current phase and covers the following implemented scope:

- Watches `Service` as the primary reconciled object.
- Watches related `EndpointSlice` objects and requeues the owning `Service`.
- Processes only `Service` objects with `type: LoadBalancer`.
- Filters managed objects by configured `loadBalancerClass`.
- Allocates external IPv4 addresses from a static configured pool.
- Reuses an already assigned valid address when possible.
- Publishes the selected address into `Service` status.
- Tracks backends through ready IPv4 endpoints discovered from `EndpointSlice`.
- Syncs desired load balancer state through a provider abstraction.
- Uses a file-based HAProxy provider in the runtime binary.
- Handles finalizers, deletion, and cleanup when a `Service` is removed or stops matching controller selection.
- Exposes metrics, health, and readiness endpoints.
- Includes unit, regression, and end-to-end test coverage.
- Includes Helm chart support, OCI chart distribution, local development flow, and Kustomize-based deployment manifests.

Important current-state notes:

- The default runtime provider is aimed at IPv4 and TCP service traffic.
- The controller focuses on control-plane behavior and provider synchronization.
- Helm is the recommended packaged installation path, while Kustomize manifests remain useful for development and manifest-oriented workflows.

## Current Limitations

This is an intentionally scoped controller MVP, and the current repository has a few important boundaries:

- Address allocation is based on a configured static IPv4 pool.
- Service selection is driven by a single configured `loadBalancerClass`.
- The default runtime provider renders a file-based HAProxy configuration.
- The controller is focused on IPv4 and TCP service traffic.

## How to Use It

This controller fits best in environments where you want a small, explicit implementation of `LoadBalancer` behavior:

- local clusters used for development or demos
- bare-metal or lab environments with a controlled external IP pool
- educational setups where the controller logic should remain easy to inspect

At a high level, the usage pattern is:

1. Install the controller with the OCI Helm chart or the Kustomize manifests in this repository.
2. Configure the `loadBalancerClass` and static IP pool you want it to manage.
3. Create a `Service` with `spec.type: LoadBalancer`.
4. Set `spec.loadBalancerClass` to the same value the controller is configured to watch.
5. Let the controller allocate an address from the configured pool, discover backends from `EndpointSlice`, sync provider state, and publish the selected address into `.status.loadBalancer.ingress`.

For packaged installation, use the Helm command shown above.
For manifest-based and development workflows, use the Kustomize path described below.

## Architecture

The repository is intentionally compact.
The main moving parts are:

```text
cmd/main.go                    Manager startup and wiring
internal/config/               Runtime configuration loading
internal/controller/           Service reconcile logic and watches
internal/ipam/                 Static IP pool parsing and allocation
internal/backends/             EndpointSlice-based backend discovery
internal/provider/             Provider interface and in-memory mock used in tests
internal/provider/haproxy/     File-based HAProxy provider
internal/status/               Service status publication helpers
internal/metrics/              Custom Prometheus metrics
config/default/                Kustomize deployment entrypoint
config/manager/                Controller Deployment manifest
config/rbac/                   Service account and RBAC
config/prometheus/             Optional ServiceMonitor manifest
charts/k8s-lb-controller/      Helm chart and values schema
test/e2e/                      Kind-based end-to-end tests
```

The controller keeps Kubernetes control-plane concerns separate from provider implementation details:

- `internal/controller` decides whether a `Service` is managed and drives reconciliation.
- `internal/ipam` owns deterministic static pool allocation.
- `internal/backends` turns `EndpointSlice` data into provider-ready backend endpoints.
- `internal/provider` defines the abstraction boundary.
- `internal/provider/haproxy` materializes desired state as a rendered HAProxy config file.
- `internal/status` updates `Service` status only when needed.

This separation keeps the reconcile loop readable while still exercising real controller responsibilities.

## Reconcile Flow

For a matching `Service`, the reconcile loop follows this sequence:

1. Read the `Service`.
2. If the object is deleting, clean up provider state and remove the finalizer.
3. If the object no longer matches controller selection, clean up previously managed state, clear status when owned by this controller, and remove the finalizer.
4. Ensure the controller finalizer is present.
5. List relevant `Service` objects and allocate or reuse an IP from the configured pool.
6. List related `EndpointSlice` objects and discover ready IPv4 backends.
7. Build the provider model and call `provider.Ensure`.
8. Publish the selected external IP into `.status.loadBalancer.ingress`.
9. Requeue only when a managed `Service` cannot currently obtain a free IP from the pool.

This ordering matters: provider synchronization happens before status publication, and cleanup paths are explicit for both deletion and "no longer managed" transitions.

## Configuration

The binary is configured through environment variables.
When `.env` exists, it is loaded without overriding values that are already set in the environment.
This keeps local development convenient while leaving CI and deployment configuration explicit.

The Helm chart exposes these settings through chart values; see [charts/k8s-lb-controller/README.md](charts/k8s-lb-controller/README.md) for Helm-specific guidance.

| Variable | Default | Purpose |
| --- | --- | --- |
| `K8S_LB_CONTROLLER_METRICS_ADDR` | `:8080` | Metrics server bind address |
| `K8S_LB_CONTROLLER_HEALTH_ADDR` | `:8081` | Health and readiness probe bind address |
| `K8S_LB_CONTROLLER_LEADER_ELECT` | `false` | Enable leader election |
| `K8S_LB_CONTROLLER_LOAD_BALANCER_CLASS` | `iedge.local/service-lb` | Managed `loadBalancerClass` |
| `K8S_LB_CONTROLLER_IP_POOL` | `203.0.113.10,203.0.113.11,203.0.113.12` | Static external IPv4 pool |
| `K8S_LB_CONTROLLER_REQUEUE_AFTER` | `30s` | Requeue delay when no IP is available |
| `K8S_LB_CONTROLLER_GRACEFUL_SHUTDOWN_TIMEOUT` | `15s` | Controller manager graceful shutdown timeout |
| `K8S_LB_CONTROLLER_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `K8S_LB_CONTROLLER_HAPROXY_CONFIG_PATH` | `/tmp/k8s-lb-controller-haproxy.cfg` | Rendered HAProxy config path |
| `K8S_LB_CONTROLLER_HAPROXY_VALIDATE_COMMAND` | empty | Optional command used to validate a candidate config |
| `K8S_LB_CONTROLLER_HAPROXY_RELOAD_COMMAND` | empty | Optional command used after a successful config update |

Notes:

- `K8S_LB_CONTROLLER_IP_POOL` must contain valid, unique IPv4 addresses.
- `K8S_LB_CONTROLLER_REQUEUE_AFTER` is used only for managed services waiting for a free IP.
- `K8S_LB_CONTROLLER_GRACEFUL_SHUTDOWN_TIMEOUT` must be positive.
- When HAProxy validate or reload commands are configured, `{{config}}` is replaced with the relevant config file path.
- The Kustomize deployment manifest enables leader election explicitly for in-cluster operation.

For local development, `.env.example` provides a ready-to-copy starting point:

```sh
cp .env.example .env
```

## Running Locally

### Prerequisites

- Go 1.26.x
- Docker
- `kubectl`
- `kind` for the automated e2e flow

### Common Development Commands

```sh
make build
make lint
make test
make test-e2e
```

Useful additional commands:

```sh
make manifests
make build-installer
```

### Recommended Local Controller Flow

For interactive development and demos, the simplest path is to run the controller locally against your current kubeconfig context:

```sh
kind create cluster --name k8s-lb-controller
cp .env.example .env
make run
```

Notes:

- `make run` starts the controller on your host.
- The binary performs a small local kubeconfig preflight check to catch a missing kubeconfig or missing `current-context`.
- With the default configuration, the rendered HAProxy config is written to `/tmp/k8s-lb-controller-haproxy.cfg` on the host.

### In-Cluster Development Flow

If you want to run the controller as a `Deployment` inside the cluster:

```sh
make docker-build IMG=k8s-lb-controller:dev
kind load docker-image k8s-lb-controller:dev --name k8s-lb-controller
make deploy IMG=k8s-lb-controller:dev
```

This path is close to what the e2e tests exercise.

## Deployment / Manifests

The repository supports two installation paths:

- Helm chart for packaged installation and distribution
- Kustomize manifests for development, debugging, and manifest-based installs

For detailed Helm usage, values, and chart-specific notes, see [charts/k8s-lb-controller/README.md](charts/k8s-lb-controller/README.md).

The Kustomize resources in this repository are organized as follows:

- `config/default` is the main deployment entrypoint.
- `config/manager` contains the controller `Deployment`.
- `config/rbac` contains the service account and minimal RBAC for `Service` and `EndpointSlice`.
- `config/default/metrics_service.yaml` exposes the metrics endpoint internally.
- `config/prometheus/monitor.yaml` contains an optional `ServiceMonitor` for clusters that already have Prometheus Operator CRDs.

The `ServiceMonitor` stays optional because not every cluster has Prometheus Operator CRDs installed.

Render the default install set with the repo-local Kustomize tool:

```sh
make kustomize
bin/kustomize build config/default
```

Deploy it:

```sh
make deploy IMG=ghcr.io/voronkov44/k8s-lb-controller:latest
```

Remove it:

```sh
make undeploy
```

Generate a consolidated install bundle:

```sh
make build-installer
```

This writes `dist/install.yaml`.

### Minimal Managed Service Example

The controller only manages services that match the configured class:

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

## Testing

The current repository state includes unit, regression, and end-to-end tests.

- `make lint` runs `golangci-lint`.
- `make test` runs the non-e2e Go test suite with envtest assets and writes `cover.out`.
- `make test-e2e` creates a Kind cluster, builds and loads the controller image, deploys the manifests, runs Ginkgo end-to-end tests, and tears the cluster down.

The current test coverage includes:

- controller selection, lifecycle, finalizer, and cleanup behavior
- static IP allocation and reuse logic
- `EndpointSlice` backend discovery behavior
- status update ordering and idempotency
- HAProxy provider rendering and apply semantics
- manifest validation for graceful shutdown alignment
- end-to-end verification of metrics, managed vs ignored services, backend updates, and deletion cleanup

The repository also includes GitHub Actions workflows for:

- lint
- unit and regression tests
- end-to-end tests

## License

Licensed under the Apache License, Version 2.0.
