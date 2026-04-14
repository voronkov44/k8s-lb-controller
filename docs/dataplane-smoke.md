# Dataplane Smoke Validation

This guide validates the current controller plus dataplane path end to end in a controlled single-node Kind or lab environment.

Russian version: [dataplane-smoke.ru.md](dataplane-smoke.ru.md)

See also:

- [../README.md](../README.md)
- [release-checklist.md](release-checklist.md)
- [release-checklist.ru.md](release-checklist.ru.md)

## What This Flow Proves

The smoke flow checks the implemented dataplane path, including:

- controller plus dataplane deployment wiring
- external IP allocation for a managed `LoadBalancer` `Service`
- external IP attachment on the dataplane host interface
- HAProxy listener readiness on the assigned external IP
- rendered HAProxy configuration contents
- end-to-end HTTP reachability to the demo backend

It does not validate multi-node dataplane placement, HA coordination, BGP, ARP, NDP, cloud-provider-style publication, or broad production hardening.

## Prerequisites

- `docker`, `kind`, and `kubectl` must be available locally
- the environment must allow the dataplane pod to use host networking and `NET_ADMIN`
- `make verify-dataplane` is recommended before running the live smoke flow

The default smoke path expects the current repository defaults:

- smoke Kind cluster name `k8s-lb-controller-dataplane-smoke`
- dataplane interface `eth0`
- attached external IP width `/32`
- first allocated external IP `203.0.113.10`

If you change the IP pool, dataplane interface, or related manifests, adjust the manual checks accordingly.

## Automated Kind Smoke Flow

Run the full automated smoke validation:

```sh
make smoke-dataplane-kind
```

`make smoke-dataplane` is an alias for the same flow.

The target:

- creates or reuses a Kind cluster
- builds controller and dataplane images
- loads both images into Kind
- deploys controller plus dataplane mode
- deploys the demo backend and demo client from [`hack/dataplane-smoke.yaml`](../hack/dataplane-smoke.yaml)
- waits for external IP allocation
- checks dataplane logs, attached IP state, HAProxy listener state, rendered config, and end-to-end HTTP traffic
- gathers diagnostics automatically if the flow fails

Useful options:

```sh
KEEP_CLUSTER=true make smoke-dataplane-kind
KEEP_DEPLOYMENT=true make smoke-dataplane-kind
DATAPLANE_KIND_CLUSTER=my-lab make smoke-dataplane-kind
```

## Manual Validation Flow

Set reusable variables first:

```sh
CLUSTER_NAME="${DATAPLANE_KIND_CLUSTER:-k8s-lb-controller-dataplane-smoke}"
MANAGER_IMAGE="example.com/k8s-lb-controller:e2e"
DATAPLANE_IMAGE="example.com/k8s-lb-controller-dataplane:e2e"
```

### 1. Create or Reuse the Kind Cluster

```sh
DATAPLANE_KIND_CLUSTER="${CLUSTER_NAME}" make kind-up-dataplane
```

### 2. Build the Images

```sh
make docker-build IMG="${MANAGER_IMAGE}"
make docker-build-dataplane DATAPLANE_IMG="${DATAPLANE_IMAGE}"
```

### 3. Load the Images into Kind

```sh
kind load docker-image "${MANAGER_IMAGE}" --name "${CLUSTER_NAME}"
kind load docker-image "${DATAPLANE_IMAGE}" --name "${CLUSTER_NAME}"
```

### 4. Deploy Controller Plus Dataplane Mode

```sh
make deploy-dataplane \
  IMG="${MANAGER_IMAGE}" \
  DATAPLANE_IMG="${DATAPLANE_IMAGE}"
```

Wait for both deployments:

```sh
kubectl rollout status deployment/k8s-lb-controller-controller-manager -n k8s-lb-controller-system --timeout=180s
kubectl rollout status deployment/k8s-lb-controller-dataplane -n k8s-lb-controller-system --timeout=180s
```

### 5. Deploy the Demo Workload

```sh
kubectl apply -f hack/dataplane-smoke.yaml
kubectl rollout status deployment/demo -n k8s-lb-controller-smoke --timeout=180s
kubectl rollout status deployment/demo-client -n k8s-lb-controller-smoke --timeout=180s
```

### 6. Check External IP Assignment

```sh
kubectl get svc demo-lb -n k8s-lb-controller-smoke -o wide
kubectl get svc demo-lb -n k8s-lb-controller-smoke -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

With the current default IP pool, the first managed `Service` should receive `203.0.113.10`.

### 7. Check Dataplane Logs and Attached IP State

```sh
kubectl logs -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane
kubectl exec -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane -- \
  sh -ec 'ip -4 addr show dev eth0'
```

You should see the assigned external IP attached as `/32` on the dataplane host interface.

### 8. Check HAProxy Listener and Rendered Config

```sh
kubectl exec -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane -- \
  sh -ec 'ss -ltnp'
kubectl exec -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane -- \
  cat /var/run/k8s-lb-dataplane/haproxy.cfg
```

You should see HAProxy listening on the assigned external IP and port `80`, and the rendered configuration should include a matching `bind <external-ip>:80` line plus the expected backend entries.

### 9. Verify End-to-End HTTP Reachability

```sh
EXTERNAL_IP="$(kubectl get svc demo-lb -n k8s-lb-controller-smoke -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"
kubectl exec -n k8s-lb-controller-smoke deploy/demo-client -c toolbox -- \
  wget -qO- "http://${EXTERNAL_IP}/"
```

The response should contain `Welcome to nginx!`.

## Failure Diagnostics

If the automated smoke flow fails, it gathers:

- `kubectl get pods -A -o wide`
- `kubectl get svc -A`
- `kubectl get endpointslices -A`
- `kubectl describe` output for the controller, dataplane, and smoke `Service`
- controller logs
- dataplane API logs
- HAProxy sidecar logs
- dataplane `ip addr`
- dataplane `ss -ltnp`
- rendered HAProxy configuration

If you keep the cluster or deployment, the most useful follow-up commands are:

```sh
kubectl get pods -A -o wide
kubectl get svc -A
kubectl logs -n k8s-lb-controller-system deploy/k8s-lb-controller-controller-manager
kubectl logs -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c dataplane
kubectl logs -n k8s-lb-controller-system deploy/k8s-lb-controller-dataplane -c haproxy
```

## Cleanup

Remove the demo workload and controller plus dataplane deployment:

```sh
kubectl delete -f hack/dataplane-smoke.yaml --ignore-not-found
make undeploy-dataplane ignore-not-found=true
```

Delete the Kind cluster used by the smoke flow:

```sh
DATAPLANE_KIND_CLUSTER="${DATAPLANE_KIND_CLUSTER:-k8s-lb-controller-dataplane-smoke}" make kind-down-dataplane
```
