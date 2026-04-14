#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KIND_BIN="${KIND:-kind}"
KUBECTL_BIN="${KUBECTL:-kubectl}"
KIND_CLUSTER="${KIND_CLUSTER:-k8s-lb-controller-dataplane-smoke}"
SYSTEM_NAMESPACE="${SYSTEM_NAMESPACE:-k8s-lb-controller-system}"
SMOKE_NAMESPACE="${SMOKE_NAMESPACE:-k8s-lb-controller-smoke}"
SMOKE_MANIFEST="${SMOKE_MANIFEST:-${ROOT_DIR}/hack/dataplane-smoke.yaml}"
MANAGER_IMAGE="${E2E_MANAGER_IMAGE:-example.com/k8s-lb-controller:e2e}"
DATAPLANE_IMAGE="${E2E_DATAPLANE_IMAGE:-example.com/k8s-lb-controller-dataplane:e2e}"
EXPECTED_EXTERNAL_IP="${EXPECTED_EXTERNAL_IP:-203.0.113.10}"
DATAPLANE_INTERFACE="${DATAPLANE_INTERFACE:-eth0}"
DATAPLANE_CIDR_SUFFIX="${DATAPLANE_CIDR_SUFFIX:-32}"
KEEP_CLUSTER="${KEEP_CLUSTER:-false}"
KEEP_DEPLOYMENT="${KEEP_DEPLOYMENT:-false}"

controller_selector="control-plane=controller-manager"
dataplane_selector="control-plane=dataplane"
controller_deployment="k8s-lb-controller-controller-manager"
dataplane_deployment="k8s-lb-controller-dataplane"
dataplane_runtime_dir="/var/run/k8s-lb-dataplane"
cluster_created="false"

log() {
  printf '[smoke-dataplane] %s\n' "$*"
}

run() {
  log "running: $*"
  "$@"
}

kind_cluster_exists() {
  "${KIND_BIN}" get clusters | grep -Fxq "${KIND_CLUSTER}"
}

active_pod_name() {
  local namespace="$1"
  local selector="$2"

  "${KUBECTL_BIN}" get pods \
    -n "${namespace}" \
    -l "${selector}" \
    -o go-template='{{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ "\n" }}{{ end }}{{ end }}' \
    | awk 'NF { print; exit }'
}

wait_for_rollout() {
  local namespace="$1"
  local deployment="$2"
  run "${KUBECTL_BIN}" rollout status "deployment/${deployment}" -n "${namespace}" --timeout=180s
}

wait_for_service_ip() {
  local deadline
  deadline=$((SECONDS + 180))

  while (( SECONDS < deadline )); do
    local external_ip
    external_ip="$("${KUBECTL_BIN}" get svc demo-lb -n "${SMOKE_NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
    if [[ -n "${external_ip}" ]]; then
      printf '%s\n' "${external_ip}"
      return 0
    fi

    sleep 2
  done

  return 1
}

wait_for_log_pattern() {
  local namespace="$1"
  local container="$2"
  local pod_name="$3"
  local pattern="$4"
  local deadline
  deadline=$((SECONDS + 120))

  while (( SECONDS < deadline )); do
    local log_args=("${KUBECTL_BIN}" logs "${pod_name}" -n "${namespace}")
    if [[ -n "${container}" ]]; then
      log_args+=(-c "${container}")
    fi

    if "${log_args[@]}" 2>/dev/null | grep -Fq "${pattern}"; then
      return 0
    fi

    sleep 2
  done

  return 1
}

collect_diagnostics() {
  log "collecting failure diagnostics"

  "${KUBECTL_BIN}" get pods -A -o wide || true
  "${KUBECTL_BIN}" get svc -A || true
  "${KUBECTL_BIN}" get endpointslices -A || true
  "${KUBECTL_BIN}" describe deployment "${controller_deployment}" -n "${SYSTEM_NAMESPACE}" || true
  "${KUBECTL_BIN}" describe deployment "${dataplane_deployment}" -n "${SYSTEM_NAMESPACE}" || true
  if "${KUBECTL_BIN}" get namespace "${SMOKE_NAMESPACE}" >/dev/null 2>&1; then
    "${KUBECTL_BIN}" describe svc demo-lb -n "${SMOKE_NAMESPACE}" || true
  else
    log "smoke namespace ${SMOKE_NAMESPACE} does not exist yet; skipping smoke service diagnostics"
  fi

  local controller_pod
  controller_pod="$(active_pod_name "${SYSTEM_NAMESPACE}" "${controller_selector}" || true)"
  if [[ -n "${controller_pod}" ]]; then
    "${KUBECTL_BIN}" logs "${controller_pod}" -n "${SYSTEM_NAMESPACE}" || true
  fi

  local dataplane_pod
  dataplane_pod="$(active_pod_name "${SYSTEM_NAMESPACE}" "${dataplane_selector}" || true)"
  if [[ -n "${dataplane_pod}" ]]; then
    "${KUBECTL_BIN}" logs "${dataplane_pod}" -n "${SYSTEM_NAMESPACE}" -c dataplane || true
    "${KUBECTL_BIN}" logs "${dataplane_pod}" -n "${SYSTEM_NAMESPACE}" -c haproxy || true
    "${KUBECTL_BIN}" exec "${dataplane_pod}" -n "${SYSTEM_NAMESPACE}" -c dataplane -- sh -ec "ip -4 addr show dev ${DATAPLANE_INTERFACE}" || true
    "${KUBECTL_BIN}" exec "${dataplane_pod}" -n "${SYSTEM_NAMESPACE}" -c dataplane -- sh -ec 'ss -ltnp' || true
    "${KUBECTL_BIN}" exec "${dataplane_pod}" -n "${SYSTEM_NAMESPACE}" -c dataplane -- cat "${dataplane_runtime_dir}/haproxy.cfg" || true
  fi
}

cleanup() {
  local exit_code=$?

  if [[ "${exit_code}" -ne 0 ]]; then
    collect_diagnostics
  fi

  if [[ "${KEEP_DEPLOYMENT}" != "true" ]]; then
    run make undeploy-dataplane ignore-not-found=true || true
    "${KUBECTL_BIN}" delete namespace "${SMOKE_NAMESPACE}" --ignore-not-found --wait=false || true
  fi

  if [[ "${KEEP_CLUSTER}" != "true" && "${cluster_created}" == "true" ]]; then
    run "${KIND_BIN}" delete cluster --name "${KIND_CLUSTER}" || true
  fi

  exit "${exit_code}"
}
trap cleanup EXIT

if ! command -v "${KIND_BIN}" >/dev/null 2>&1; then
  echo "kind is required but not found at ${KIND_BIN}" >&2
  exit 1
fi

if ! command -v "${KUBECTL_BIN}" >/dev/null 2>&1; then
  echo "kubectl is required but not found at ${KUBECTL_BIN}" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for the Kind smoke flow" >&2
  exit 1
fi

cd "${ROOT_DIR}"

if kind_cluster_exists; then
  log "reusing Kind cluster ${KIND_CLUSTER}"
else
  run "${KIND_BIN}" create cluster --name "${KIND_CLUSTER}"
  cluster_created="true"
fi

run make docker-build "IMG=${MANAGER_IMAGE}"
run make docker-build-dataplane "DATAPLANE_IMG=${DATAPLANE_IMAGE}"
run "${KIND_BIN}" load docker-image "${MANAGER_IMAGE}" --name "${KIND_CLUSTER}"
run "${KIND_BIN}" load docker-image "${DATAPLANE_IMAGE}" --name "${KIND_CLUSTER}"

run make deploy-dataplane "IMG=${MANAGER_IMAGE}" "DATAPLANE_IMG=${DATAPLANE_IMAGE}"

wait_for_rollout "${SYSTEM_NAMESPACE}" "${controller_deployment}"
wait_for_rollout "${SYSTEM_NAMESPACE}" "${dataplane_deployment}"

run "${KUBECTL_BIN}" apply -f "${SMOKE_MANIFEST}"

wait_for_rollout "${SMOKE_NAMESPACE}" "demo"
wait_for_rollout "${SMOKE_NAMESPACE}" "demo-client"

external_ip="$(wait_for_service_ip)"
log "service received external IP ${external_ip}"
if [[ "${external_ip}" != "${EXPECTED_EXTERNAL_IP}" ]]; then
  echo "expected external IP ${EXPECTED_EXTERNAL_IP}, got ${external_ip}" >&2
  exit 1
fi

controller_pod="$(active_pod_name "${SYSTEM_NAMESPACE}" "${controller_selector}")"
dataplane_pod="$(active_pod_name "${SYSTEM_NAMESPACE}" "${dataplane_selector}")"
client_pod="$(active_pod_name "${SMOKE_NAMESPACE}" "app=demo-client")"

wait_for_log_pattern "${SYSTEM_NAMESPACE}" "dataplane" "${dataplane_pod}" "dataplane service ensured"
wait_for_log_pattern "${SYSTEM_NAMESPACE}" "" "${controller_pod}" "ensured provider state"

attached_addresses="$("${KUBECTL_BIN}" exec "${dataplane_pod}" -n "${SYSTEM_NAMESPACE}" -c dataplane -- sh -ec "ip -4 addr show dev ${DATAPLANE_INTERFACE}")"
printf '%s\n' "${attached_addresses}" | grep -Fq "${external_ip}/${DATAPLANE_CIDR_SUFFIX}"

listener_output="$("${KUBECTL_BIN}" exec "${dataplane_pod}" -n "${SYSTEM_NAMESPACE}" -c dataplane -- sh -ec 'ss -ltnp')"
printf '%s\n' "${listener_output}" | grep -Fq "${external_ip}:80"

haproxy_config="$("${KUBECTL_BIN}" exec "${dataplane_pod}" -n "${SYSTEM_NAMESPACE}" -c dataplane -- cat "${dataplane_runtime_dir}/haproxy.cfg")"
printf '%s\n' "${haproxy_config}" | grep -Fq "bind ${external_ip}:80"

client_response="$("${KUBECTL_BIN}" exec "${client_pod}" -n "${SMOKE_NAMESPACE}" -c toolbox -- wget -qO- "http://${external_ip}/")"
printf '%s\n' "${client_response}" | grep -Fq "Welcome to nginx!"

controller_logs="$("${KUBECTL_BIN}" logs "${controller_pod}" -n "${SYSTEM_NAMESPACE}")"
printf '%s\n' "${controller_logs}" | grep -Fq "ensured provider state"

log "dataplane smoke validation completed successfully"
