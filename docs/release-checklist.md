# Release-Readiness Checklist

This checklist validates the current repository state before a future release or publication step. It does not create a release, publish artifacts, or change chart or application versions.

Russian version: [release-checklist.ru.md](release-checklist.ru.md)

See also:

- [../README.md](../README.md)
- [dataplane-smoke.md](dataplane-smoke.md)
- [dataplane-smoke.ru.md](dataplane-smoke.ru.md)

## 1. Static and Rendering Checks

Run the main release-readiness target:

```sh
make verify-dataplane
```

That target covers:

- `go test ./...`
- `make lint`
- controller build
- dataplane build
- Kustomize rendering for local and dataplane modes
- Helm lint for local and dataplane modes
- Helm template rendering for local and dataplane modes

## 2. Optional Dataplane e2e Suite

If you want an additional automated Kind-backed test run, execute:

```sh
make test-e2e-dataplane
```

This runs the dataplane-mode e2e suite and cleans up the Kind cluster it created for that test flow.

## 3. Controlled Kind Smoke Validation

Run the end-to-end dataplane smoke flow:

```sh
make smoke-dataplane-kind
```

Useful variants:

```sh
KEEP_CLUSTER=true make smoke-dataplane-kind
KEEP_DEPLOYMENT=true make smoke-dataplane-kind
DATAPLANE_KIND_CLUSTER=my-lab make smoke-dataplane-kind
```

The smoke flow validates:

- controller plus dataplane deployment wiring
- reconciliation of a managed `LoadBalancer` `Service`
- desired-state delivery to the dataplane API
- external IP attachment on the dataplane host interface
- HAProxy listener readiness on the assigned IP
- rendered HAProxy configuration contents
- end-to-end HTTP reachability through the dataplane path

For step-by-step manual validation, diagnostics, and cleanup, follow [dataplane-smoke.md](dataplane-smoke.md) or [dataplane-smoke.ru.md](dataplane-smoke.ru.md).

## 4. Explicit Rendering Commands

If you want to rerun the render checks directly, use:

```sh
./bin/kustomize build config/default
./bin/kustomize build config/default-dataplane
helm lint ./charts/k8s-lb-controller
helm lint ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
helm template k8s-lb-controller ./charts/k8s-lb-controller
helm template k8s-lb-controller ./charts/k8s-lb-controller --set controller.providerMode=dataplane-api --set dataplane.enabled=true
```

## 5. What This Checklist Does Not Claim

This checklist validates the currently implemented, controlled dataplane path. It does not claim readiness for:

- chart or application version changes
- GitHub release creation
- multi-node dataplane placement or HA coordination
- BGP, ARP, NDP, or cloud-provider-style external IP publication
- broader production hardening beyond the current controlled single-node dataplane model
