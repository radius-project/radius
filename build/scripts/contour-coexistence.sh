#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0

# Exercise the checkout's rad installer and external HTTP, not just Gateway status.
# Requires Docker, k3d, kubectl, Helm, jq and curl; see running-functional-tests.md.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly FIXTURES="${SCRIPT_DIR}/../../test/infra/contour-coexistence"
# shellcheck source=build/scripts/retry-curl.sh
source "${SCRIPT_DIR}/retry-curl.sh"
# shellcheck source=build/scripts/retry-helm-pull.sh
source "${SCRIPT_DIR}/retry-helm-pull.sh"
readonly K3S_IMAGE="rancher/k3s:v1.35.5-k3s1"
readonly RADIUS_VERSION="0.62.0"
readonly CONTOUR_VERSION="0.1.0"

if [[ $# != 1 || ! -x "$1" ]]; then
    echo "Usage: $0 /path/to/branch-built/rad (or make test-contour-coexistence)" >&2
    exit 2
fi
readonly RAD="$1"
for tool in docker k3d kubectl helm jq curl; do
    command -v "${tool}" >/dev/null || {
        echo "Missing required tool: ${tool}" >&2
        exit 1
    }
done
docker info >/dev/null

state="$(mktemp -d "${TMPDIR:-/tmp}/radius-coexistence.XXXXXXXX")"
readonly state
cluster="rad-coexist-$(printf '%s' "${state##*.}" | tr '[:upper:]' '[:lower:]')"
readonly cluster
cluster_owned=false
export KUBECONFIG="${state}/kubeconfig"
export HELM_CONFIG_HOME="${state}/helm/config"
export HELM_CACHE_HOME="${state}/helm/cache"
export HELM_DATA_HOME="${state}/helm/data"
export HELM_REPOSITORY_CONFIG="${HELM_CONFIG_HOME}/repositories.yaml"
export HELM_REPOSITORY_CACHE="${HELM_CACHE_HOME}/repository"
export HELM_REGISTRY_CONFIG="${HELM_CONFIG_HOME}/registry.json"

kube() {
    kubectl --kubeconfig "${KUBECONFIG}" --request-timeout=10s "$@"
}

diagnostics() {
    local status=0
    kube get pods,services -A -o wide || status=1
    kube get events -A --sort-by=.metadata.creationTimestamp || status=1
    kube get gateway,httproute -A -o yaml || status=1
    kube logs -n radius-system deployment/contour-contour --all-containers \
        --tail=100 || status=1
    return "${status}"
}

cleanup() {
    local status=$?
    trap - EXIT
    if ((status != 0)) && [[ -s "${KUBECONFIG}" ]]; then
        diagnostics || echo "Some failure diagnostics were unavailable." >&2
    fi
    if [[ "${cluster_owned}" == true ]]; then
        if ! k3d cluster delete "${cluster}"; then
            echo "Cleanup failed: delete only cluster ${cluster} manually." >&2
            status=1
        fi
    fi
    rm -rf "${state}" || status=1
    exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "${state}/home" "${HELM_CONFIG_HOME}" "${HELM_CACHE_HOME}" \
    "${HELM_DATA_HOME}"

wait_for() {
    local description="$1"
    shift
    for attempt in {1..30}; do
        if "$@"; then
            return 0
        fi
        echo "Waiting for ${description} (${attempt}/30)..."
        sleep 3
    done
    echo "Timed out waiting for ${description} (30 attempts)." >&2
    return 1
}

http_ok() {
    local host="$1" port="$2"
    curl --noproxy '*' --fail --silent --show-error --connect-timeout 2 \
        --max-time 5 -H "Host: ${host}" "http://127.0.0.1:${port}/" \
        >"${state}/response" &&
        grep -Fxq 'Hostname: coexistence-backend' "${state}/response"
}

published_port() {
    docker inspect "k3d-${cluster}-serverlb" |
        jq -er --arg port "$1/tcp" '
            .[0].NetworkSettings.Ports[$port][] |
            select(.HostIp == "127.0.0.1") | .HostPort |
            select(test("^[1-9][0-9]*$"))'
}

crd_ownership() {
    kube get crd -o json | jq -S '
        [.items[] |
            select(.spec.group == "gateway.networking.k8s.io" or
                .spec.group == "gateway.networking.x-k8s.io") |
            {name: .metadata.name, uid: .metadata.uid,
             owner: .metadata.annotations["meta.helm.sh/release-name"],
             namespace: .metadata.annotations["meta.helm.sh/release-namespace"],
             manager: .metadata.labels["app.kubernetes.io/managed-by"]}] |
        sort_by(.name)'
}

traefik_ready() {
    kube -n kube-system rollout status deployment/traefik --timeout=10s
}

envoy_ready() {
    kube -n radius-system get service contour-envoy -o json |
        jq -e '
            .spec.type == "LoadBalancer" and
            ([.spec.ports[] | select(.name == "http").port] == [8080]) and
            ([.spec.ports[] | select(.name == "https").port] == [8443]) and
            any(.status.loadBalancer.ingress[]?;
                ((.ip // .hostname // "") | length) > 0)
        ' >/dev/null
}

route_ready() {
    kube -n ingress-probe get httproute radius-route -o json |
        jq -e '
            .metadata.generation as $generation |
            any(.status.parents[]?;
                .conditions as $conditions |
                .controllerName == "projectcontour.io/gateway-controller" and
                .parentRef.name == "radius" and
                .parentRef.namespace == "radius-system" and
                .parentRef.sectionName == "http" and
                all("Accepted", "ResolvedRefs";
                    . as $type |
                    any($conditions[]?;
                        .type == $type and .status == "True" and
                        .observedGeneration == $generation)))
        ' >/dev/null
}

# Refuse a collision before taking responsibility for even a partial creation.
k3d cluster list -o json >"${state}/clusters.json"
if ! jq -e --arg name "${cluster}" \
    'all(.[]; .name != $name)' "${state}/clusters.json" >/dev/null; then
    echo "Refusing to reuse existing cluster ${cluster}." >&2
    exit 1
fi

retry_helm_pull oci://ghcr.io/radius-project/helm-chart/radius \
    --version "${RADIUS_VERSION}" --destination "${state}"
retry_curl --output "${state}/contour-${CONTOUR_VERSION}.tgz" \
    "https://github.com/projectcontour/helm-charts/releases/download/contour-${CONTOUR_VERSION}/contour-${CONTOUR_VERSION}.tgz"

echo "Creating ${cluster}; temporary configuration: ${state}"
cluster_owned=true
k3d cluster create "${cluster}" --image "${K3S_IMAGE}" \
    --kubeconfig-update-default=false --kubeconfig-switch-context=false \
    --api-port 127.0.0.1:0 \
    --port '127.0.0.1:0:80@loadbalancer' \
    --port '127.0.0.1:0:8080@loadbalancer' \
    --wait --timeout 180s
k3d kubeconfig get "${cluster}" >"${KUBECONFIG}"
# k3d 5.7 records API port 0 rather than Docker's assigned port in kubeconfig.
api_port="$(published_port 6443)"
kube config set-cluster "k3d-${cluster}" \
    --server="https://127.0.0.1:${api_port}"
traefik_port="$(published_port 80)"
contour_port="$(published_port 8080)"
echo "HTTP host ports: Traefik=${traefik_port}, Contour=${contour_port}"

wait_for "bundled Traefik" traefik_ready
kube apply -f "${FIXTURES}/backend.yaml"
kube -n ingress-probe rollout status deployment/whoami --timeout=180s
wait_for "Traefik HTTP before Radius" http_ok traefik.test "${traefik_port}"
crd_ownership >"${state}/crds-before.json"
jq -e 'length > 0 and all(.[]; .owner == "traefik-crd" and
    .namespace == "kube-system" and .manager == "Helm")' \
    "${state}/crds-before.json" >/dev/null

HOME="${state}/home" "${RAD}" install kubernetes \
    --kubecontext "k3d-${cluster}" \
    --chart "${state}/radius-${RADIUS_VERSION}.tgz" \
    --contour-chart "${state}/contour-${CONTOUR_VERSION}.tgz" \
    --contour-set \
    'contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}' \
    --contour-set \
    envoy.hostNetwork=false,envoy.service.ports.http=8080,envoy.service.ports.https=8443 \
    --timeout 15m

kube -n radius-system wait --for=condition=Programmed gateway/radius \
    --timeout=180s
kube get gatewayclass contour -o json | jq -e \
    '.spec.controllerName == "projectcontour.io/gateway-controller"' >/dev/null
kube -n radius-system get daemonset contour-envoy -o json |
    jq -e '.spec.template.spec.hostNetwork != true and
        all(.spec.template.spec.containers[].ports[]?; (.hostPort // 0) == 0)' \
        >/dev/null
wait_for "Envoy ports 8080/8443 and LoadBalancer address" envoy_ready
kube -n radius-system get service contour-envoy -o wide
kube apply -f "${FIXTURES}/route.yaml"
wait_for "accepted Contour HTTPRoute attachment" route_ready
wait_for "Contour external HTTP" http_ok radius.test "${contour_port}"
wait_for "Traefik HTTP after Radius" http_ok traefik.test "${traefik_port}"
crd_ownership >"${state}/crds-after.json"
diff -u "${state}/crds-before.json" "${state}/crds-after.json"
echo "PASS: external Traefik and Contour HTTP; shared CRD ownership unchanged."
