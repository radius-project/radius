#!/usr/bin/env bash
# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.
# Build and exercise user RBAC on a dedicated, disposable kind cluster.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLUSTER="${RBAC_POC_CLUSTER:-radius-user-rbac-poc}"
STATE="${ROOT}/dist/user-rbac-poc"
if [[ "${CLUSTER}" != radius-user-rbac-poc ]]; then
    STATE="${ROOT}/dist/${CLUSTER}"
fi
export KUBECONFIG="${STATE}/kubeconfig"
CONTEXT="kind-${CLUSTER}"
export GOMAXPROCS="${GOMAXPROCS:-4}"

usage() {
    cat <<'EOF'
Usage: bash hack/user-rbac/poc.sh up|test|logs|down

up    Build local images, create a dedicated kind cluster and install Radius.
test  Run real aggregated-API scenarios (requires the dedicated cluster).
logs  Save UCP JSON audit logs and workload image IDs in this cluster's state.
down  Delete only the cluster created by this kit; retain logs and credentials.

Environment:
  RBAC_POC_CLUSTER  Dedicated name, radius-user-rbac-poc[-suffix].
  RBAC_POC_RUNTIME docker or podman (auto-detected).
  RBAC_POC_SKIP_BUILD=true Reuse previously built and loaded POC images.
  RBAC_POC_ENGINE_TAG Deployment engine tag (default: latest).
  RADIUS_USER_RBAC_RESUME=true Repeat workload tests on existing kit policy.

State: dist/user-rbac-poc for the default cluster, dist/$RBAC_POC_CLUSTER otherwise.
EOF
}

die() {
    echo "[ERROR] $*" >&2
    exit 1
}

require_tools() {
    local tool
    for tool in go make kind kubectl helm jq curl; do
        command -v "${tool}" >/dev/null || die "missing ${tool}"
    done
}

runtime() {
    if [[ -n "${RBAC_POC_RUNTIME:-}" ]]; then
        RUNTIME="${RBAC_POC_RUNTIME}"
    elif command -v docker >/dev/null; then
        RUNTIME=docker
    else
        RUNTIME=podman
    fi
    [[ "${RUNTIME}" == docker || "${RUNTIME}" == podman ]] ||
        die "runtime must be docker or podman"
    command -v "${RUNTIME}" >/dev/null || die "missing ${RUNTIME}"
    "${RUNTIME}" info >/dev/null
    export KIND_EXPERIMENTAL_PROVIDER="${RUNTIME}"
}

owned() {
    [[ -f "${STATE}/owner" ]] || die "no ownership marker; refusing shared cluster"
    [[ "$(cat "${STATE}/owner")" == "${CLUSTER}" ]] ||
        die "ownership marker names another cluster"
}

up() {
    require_tools
    runtime
    mkdir -p "${STATE}"
    chmod 700 "${STATE}"
    local containers
    containers="$("${RUNTIME}" ps -a --format '{{.Names}}')"
    if grep -Fxq "${CLUSTER}-control-plane" <<<"${containers}"; then
        owned
        [[ "$("${RUNTIME}" inspect --format \
            '{{index .Config.Labels "io.x-k8s.kind.cluster"}}' \
            "${CLUSTER}-control-plane")" == "${CLUSTER}" ]] ||
            die "existing container is not this kit's kind cluster"
    else
        printf '%s\n' "${CLUSTER}" >"${STATE}/owner"
        kind create cluster --name "${CLUSTER}" \
            --kubeconfig "${KUBECONFIG}" --wait 180s
    fi
    kubectl --context "${CONTEXT}" wait --for=condition=Ready nodes \
        --all --timeout=180s
    local tag image arch archive
    tag="user-rbac-poc"
    arch="$("${RUNTIME}" exec "${CLUSTER}-control-plane" uname -m)"
    case "${arch}" in
        aarch64 | arm64) arch=arm64 ;;
        x86_64 | amd64) arch=amd64 ;;
        *) die "unsupported node architecture ${arch}" ;;
    esac
    if [[ "${RBAC_POC_SKIP_BUILD:-false}" != true ]]; then
        make -C "${ROOT}" build-rad copy-manifests
        for image in ucpd applications-rp dynamic-rp controller pre-upgrade; do
            make -C "${ROOT}" "build-${image}-linux-${arch}"
            "${RUNTIME}" build --platform "linux/${arch}" \
                --build-arg "TARGETARCH=${arch}" \
                --tag "localhost/user-rbac-poc/${image}:${tag}" \
                --file "${ROOT}/deploy/images/${image}/Dockerfile" \
                "${ROOT}/dist"
            archive="${STATE}/${image}.tar"
            "${RUNTIME}" save --output "${archive}" \
                "localhost/user-rbac-poc/${image}:${tag}"
            kind load image-archive --name "${CLUSTER}" "${archive}"
            rm "${archive}"
        done
    fi
    local issuer rad
    issuer="kubernetes:$(kubectl --context "${CONTEXT}" get ns kube-system \
        -o jsonpath='{.metadata.uid}')"
    printf '%s\n' "${issuer}" >"${STATE}/issuer"
    rad="${ROOT}/dist/$(go env GOOS)_$(go env GOARCH)/release/rad"
    [[ -x "${rad}" ]] || die "rad not built; rerun without RBAC_POC_SKIP_BUILD"
    # rad's Kubernetes helper reads HOME/.kube/config rather than KUBECONFIG.
    mkdir -p "${STATE}/home/.kube"
    ln -sf "${KUBECONFIG}" "${STATE}/home/.kube/config"
    local overrides=(
        --set global.imageRegistry=localhost/user-rbac-poc \
        --set "global.imageTag=${tag}" \
        --set global.rbac.enabled=true --set global.rbac.dryRun=false \
        --set controller.imagePullPolicy=IfNotPresent \
        --set controlPlaneReadiness.imagePullPolicy=IfNotPresent \
        --set global.terraform.enabled=false \
        --set bicep.image=ghcr.io/radius-project/bicep --set bicep.tag=latest \
        --set de.image=ghcr.io/radius-project/deployment-engine \
        --set "de.tag=${RBAC_POC_ENGINE_TAG:-latest}" \
        --set dashboard.enabled=false \
        --set rbac.enabled=true --set "rbac.kubernetesIssuer=${issuer}" \
        --set rbac.acceptIncompleteHistory=true
    )
    local release
    release="$(helm list --kube-context "${CONTEXT}" -n radius-system \
        --all --filter '^radius$' -q)"
    if [[ "${release}" == radius ]]; then
        helm upgrade radius "${ROOT}/deploy/Chart" --kube-context "${CONTEXT}" \
            -n radius-system --reuse-values --wait --timeout 10m "${overrides[@]}"
        # The kit deliberately reuses local tags; restart only its own services.
        kubectl --context "${CONTEXT}" -n radius-system rollout restart \
            deployment/ucp deployment/applications-rp deployment/dynamic-rp \
            deployment/controller
        for image in ucp applications-rp dynamic-rp controller; do
            kubectl --context "${CONTEXT}" -n radius-system rollout status \
                "deployment/${image}" --timeout=180s
        done
    else
        HOME="${STATE}/home" "${rad}" install kubernetes --config "${STATE}/rad.yaml" \
            --kubecontext "${CONTEXT}" --chart "${ROOT}/deploy/Chart" \
            --skip-contour-install "${overrides[@]}"
    fi
    # This is only the Kubernetes gate. Radius authorization still defaults deny.
    kubectl --context "${CONTEXT}" create clusterrolebinding \
        radius-poc-api-users --clusterrole radius-api-user \
        --group system:authenticated --dry-run=client -o yaml |
        kubectl --context "${CONTEXT}" apply -f -
    kubectl --context "${CONTEXT}" create clusterrolebinding \
        radius-poc-bootstrap --clusterrole radius-rbac-bootstrap \
        --user poc-admin --dry-run=client -o yaml |
        kubectl --context "${CONTEXT}" apply -f -
    printf '[INFO] Installed; run: bash hack/user-rbac/poc.sh test\n'
}

logs() {
    owned
    kubectl --context "${CONTEXT}" logs deployment/ucp -n radius-system \
        >"${STATE}/ucp-audit.jsonl"
    kubectl --context "${CONTEXT}" get pods -n radius-system -o json \
        >"${STATE}/pods.json"
    echo "[INFO] Evidence: ${STATE}"
}

direct_spoof() (
    local pid port status attempt
    kubectl --context "${CONTEXT}" -n radius-system port-forward \
        --address 127.0.0.1 service/ucp :443 \
        >"${STATE}/port-forward.log" 2>&1 &
    pid=$!
    trap 'kill "${pid}" 2>/dev/null || true; wait "${pid}" 2>/dev/null || true' EXIT
    for ((attempt = 0; attempt < 30; attempt++)); do
        port="$(sed -n 's/^Forwarding from 127.0.0.1:\([0-9]*\).*$/\1/p' \
            "${STATE}/port-forward.log" | head -1)"
        [[ -z "${port}" ]] || break
        kill -0 "${pid}" 2>/dev/null || die "port-forward failed; inspect port-forward.log"
        sleep 1
    done
    [[ -n "${port}" ]] || die "port-forward readiness timed out"
    # Only this loopback probe skips certificate validation; it sends no credentials.
    status="$(curl --insecure --silent --show-error --max-time 20 \
        -H 'X-Remote-User: poc-admin' -H 'X-Remote-Group: poc-admins' \
        -o "${STATE}/direct-spoof.json" -w '%{http_code}' \
        "https://127.0.0.1:${port}/apis/api.ucp.dev/v1alpha3/providers/Radius.Core/whoami?api-version=2025-08-01-preview")"
    printf '[INFO] Direct forged identity: HTTP %s\n' "${status}" |
        tee "${STATE}/direct-spoof.log"
    [[ "${status}" == 401 || "${status}" == 403 ]] ||
        die "direct forged identity was not rejected"
)

[[ "${CLUSTER}" =~ ^radius-user-rbac-poc(-[a-z0-9-]+)?$ ]] ||
    die "use a clearly dedicated radius-user-rbac-poc[-suffix] name"
case "${1:---help}" in
    up) up ;;
    test)
        owned
        direct_spoof
        export RADIUS_USER_RBAC_KUBECONFIG="${KUBECONFIG}"
        export RADIUS_USER_RBAC_CONTEXT="${CONTEXT}"
        result=0
        go test -count=1 -v "${ROOT}/test/user-rbac" 2>&1 |
            tee "${STATE}/scenarios.log" || result=$?
        logs
        exit "${result}"
        ;;
    logs) logs ;;
    down)
        owned
        runtime
        kind delete cluster --name "${CLUSTER}" --kubeconfig "${KUBECONFIG}"
        ;;
    -h | --help) usage ;;
    *) usage; exit 1 ;;
esac
