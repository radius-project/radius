#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

# ============================================================================
# Creates (or reuses) a kind cluster with a local registry, builds and pushes
# the Radius images from this checkout, and installs Radius from deploy/Chart
# in the requested authorization mode.
# ============================================================================

set -euo pipefail

# shellcheck source=hack/authz/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Images built from this repository that the chart installs.
readonly IMAGES=(ucpd applications-rp dynamic-rp controller bicep pre-upgrade)
readonly DEPLOYMENTS=(ucp applications-rp dynamic-rp controller bicep-de)
readonly LOG_MODE_DEPLOYMENTS=(ucp applications-rp dynamic-rp controller)
readonly TAG_FILE="${AUTHZ_REPO_ROOT}/dist/authz-kit/image-tag"

MODE="dryRun"
SKIP_BUILD=false
IMAGE_TAG="${AUTHZ_IMAGE_TAG:-}"

usage() {
    cat <<EOF
Usage: $(basename "$0") [off|dryRun|enforce] [--skip-build] [--tag TAG]

Creates or reuses the kind cluster "${AUTHZ_CLUSTER_NAME}" with a local registry at
localhost:${AUTHZ_REGISTRY_PORT}, builds and pushes the Radius images from this checkout,
and installs Radius from deploy/Chart in the given authorization mode
(default: ${MODE}). Rerun with another mode to switch modes.

  off      global.rbac.enabled=false, global.rbac.dryRun=false
  dryRun   global.rbac.enabled=false, global.rbac.dryRun=true
  enforce  global.rbac.enabled=true,  global.rbac.dryRun=false

Options:
  --skip-build  Reuse images already in the local registry.
  --tag TAG     Image tag (env: AUTHZ_IMAGE_TAG). Defaults to a new tag per
                build, or the last built tag with --skip-build.
  -h, --help    Show this help.

Environment:
  AUTHZ_CLUSTER_NAME   kind cluster name (default: ${AUTHZ_CLUSTER_NAME}).
  AUTHZ_REGISTRY_NAME  Registry container name (default: ${AUTHZ_REGISTRY_NAME}).
  AUTHZ_REGISTRY_PORT  Registry port on localhost (default: ${AUTHZ_REGISTRY_PORT}).
  RAD                  rad CLI to use (default: rad).

Requires docker, kind, kubectl, helm, rad, make, and go.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        off | dryRun | enforce)
            MODE="$1"
            shift
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --tag)
            [[ $# -ge 2 ]] || authz_die "--tag requires a value"
            IMAGE_TAG="$2"
            shift 2
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            usage >&2
            authz_die "unknown argument '$1': expected off, dryRun, or enforce"
            ;;
    esac
done

readonly RAD="${RAD:-rad}"
readonly REGISTRY="localhost:${AUTHZ_REGISTRY_PORT}"

ensure_registry() {
    local running
    running="$(docker inspect --format '{{.State.Running}}' \
        "${AUTHZ_REGISTRY_NAME}" 2>/dev/null || true)"
    if [[ "${running}" == "true" ]]; then
        return
    fi
    if [[ -n "${running}" ]]; then
        docker start "${AUTHZ_REGISTRY_NAME}" >/dev/null
        return
    fi
    echo "Creating local registry ${AUTHZ_REGISTRY_NAME} at ${REGISTRY}"
    docker run --detach --restart=always \
        --publish "127.0.0.1:${AUTHZ_REGISTRY_PORT}:5000" \
        --name "${AUTHZ_REGISTRY_NAME}" registry:2 >/dev/null
}

# Follows https://kind.sigs.k8s.io/docs/user/local-registry/ so that nodes pull
# localhost:<port>/<image> from the registry container.
ensure_cluster() {
    if kind get clusters | grep -Fxq "${AUTHZ_CLUSTER_NAME}"; then
        echo "Reusing kind cluster ${AUTHZ_CLUSTER_NAME}"
    else
        echo "Creating kind cluster ${AUTHZ_CLUSTER_NAME}"
        kind create cluster --name "${AUTHZ_CLUSTER_NAME}" --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"
EOF
    fi

    local registry_dir="/etc/containerd/certs.d/${REGISTRY}"
    local node
    for node in $(kind get nodes --name "${AUTHZ_CLUSTER_NAME}"); do
        docker exec "${node}" mkdir -p "${registry_dir}"
        printf '[host."http://%s:5000"]\n' "${AUTHZ_REGISTRY_NAME}" |
            docker exec --interactive "${node}" \
                cp /dev/stdin "${registry_dir}/hosts.toml"
    done

    if [[ "$(docker inspect --format '{{json .NetworkSettings.Networks.kind}}' \
        "${AUTHZ_REGISTRY_NAME}")" == "null" ]]; then
        docker network connect kind "${AUTHZ_REGISTRY_NAME}"
    fi

    authz_kubectl apply --filename - >/dev/null <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "${REGISTRY}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

build_images() {
    local targets=(copy-manifests)
    local image
    for image in "${IMAGES[@]}"; do
        targets+=("docker-build-${image}" "docker-push-${image}")
    done

    echo "Building and pushing ${IMAGES[*]} as ${REGISTRY}/<image>:${IMAGE_TAG}"
    if [[ "$(docker info --format '{{.Architecture}}')" != "x86_64" ]]; then
        echo "note: make docker-build builds linux/amd64 images; on this host they run under Docker's amd64 emulation."
    fi
    make -C "${AUTHZ_REPO_ROOT}" "${targets[@]}" \
        DOCKER_REGISTRY="${REGISTRY}" DOCKER_TAG_VERSION="${IMAGE_TAG}"

    mkdir -p "$(dirname "${TAG_FILE}")"
    echo "${IMAGE_TAG}" >"${TAG_FILE}"
}

install_radius() {
    local set_args=()
    local value
    while IFS= read -r value; do
        set_args+=(--set "${value}")
    done < <(authz_install_set_values "${MODE}" "${REGISTRY}" "${IMAGE_TAG}")

    echo "Installing Radius in authz mode ${MODE}"
    "${RAD}" install kubernetes --kubecontext "${AUTHZ_KUBE_CONTEXT}" \
        --chart "${AUTHZ_REPO_ROOT}/deploy/Chart" --reinstall "${set_args[@]}"

    local deployment
    for deployment in "${DEPLOYMENTS[@]}"; do
        authz_kubectl rollout status "deployment/${deployment}" \
            --namespace "${AUTHZ_RADIUS_NAMESPACE}" --timeout 300s
    done
}

print_modes() {
    local deployment line
    echo
    echo "Logged authz modes:"
    for deployment in "${LOG_MODE_DEPLOYMENTS[@]}"; do
        line="$(authz_kubectl logs "deployment/${deployment}" \
            --namespace "${AUTHZ_RADIUS_NAMESPACE}" |
            grep -o -m 1 'authz mode=[A-Za-z]*' || true)"
        printf '  %-16s %s\n' "${deployment}" "${line:-<not logged>}"
    done
}

print_next_steps() {
    cat <<EOF

Radius is installed on ${AUTHZ_KUBE_CONTEXT} in authz mode ${MODE}. Next steps:

  # Call applications-rp from a rogue workload
  hack/authz/rogue.sh exec -- curl -s -o /dev/null -w '%{http_code}\n' \\
    'http://applications-rp.${AUTHZ_RADIUS_NAMESPACE}:5443/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview'

  # Fail if any component logged a dry-run would-deny line
  hack/authz/would-deny.sh

See hack/authz/README.md for the manual verification checklist. To remove
the cluster and registry:
  kind delete cluster --name ${AUTHZ_CLUSTER_NAME} && docker rm --force ${AUTHZ_REGISTRY_NAME}
EOF
}

main() {
    authz_mode_helm_values "${MODE}" >/dev/null || exit 2
    authz_require_tools docker kind kubectl helm "${RAD}" make go
    docker info >/dev/null || authz_die "the Docker daemon is not reachable"

    if [[ -z "${IMAGE_TAG}" ]]; then
        if [[ "${SKIP_BUILD}" == true ]]; then
            [[ -f "${TAG_FILE}" ]] || authz_die "--skip-build needs --tag: no previous build found in ${TAG_FILE}"
            IMAGE_TAG="$(<"${TAG_FILE}")"
        else
            IMAGE_TAG="authz-$(git -C "${AUTHZ_REPO_ROOT}" rev-parse --short HEAD)-$(date +%s)"
        fi
    fi

    ensure_registry
    ensure_cluster
    if [[ "${SKIP_BUILD}" == false ]]; then
        build_images
    fi
    install_radius
    print_modes
    print_next_steps
}

main
