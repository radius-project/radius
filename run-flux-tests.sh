#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME=${CLUSTER_NAME:-radius}
REGISTRY_NAME=${REGISTRY_NAME:-radius-registry}
REGISTRY_HOST=${REGISTRY_HOST:-localhost}
REGISTRY_PORT=${REGISTRY_PORT:-5000}
REL_VERSION=${REL_VERSION:-dev}
FLUX_VERSION=${FLUX_VERSION:-2.6.4}
GIT_HTTP_USERNAME=${GIT_HTTP_USERNAME:-testuser}
GIT_HTTP_PASSWORD=${GIT_HTTP_PASSWORD:-testpass}
GIT_HTTP_EMAIL=${GIT_HTTP_EMAIL:-testuser@radapp.io}
GIT_HTTP_IMAGE=${GIT_HTTP_IMAGE:-${REGISTRY_NAME}:${REGISTRY_PORT}/githttpbackend:${REL_VERSION}}
KEEP_CLUSTER=${KEEP_CLUSTER:-}

WORKDIR=$(pwd)
PORT_FORWARD_PID=""

declare -a CLEANUP_CMDS

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S')" "$*"
}

die() {
  echo "Error: $*" >&2
  exit 1
}

is_truthy() {
  case "$1" in
    1|[Tt][Rr][Uu][Ee]) return 0 ;;
    *) return 1 ;;
  esac
}

cleanup() {
  if [[ -n "${PORT_FORWARD_PID}" ]] && ps -p "${PORT_FORWARD_PID}" >/dev/null 2>&1; then
    kill "${PORT_FORWARD_PID}" 2>/dev/null || true
  fi
  if ! is_truthy "${KEEP_CLUSTER}"; then
    for cmd in "${CLEANUP_CMDS[@]:-}"; do
      eval "$cmd" || true
    done
  fi
}
trap cleanup EXIT

check_command() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

log "Checking prerequisites"
for tool in docker kind kubectl rad go make curl; do
  check_command "$tool"
done

log "Deleting existing KinD clusters (if any)"
kind delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true
kind delete cluster --name kind >/dev/null 2>&1 || true

docker rm -f "${CLUSTER_NAME}-control-plane" "${CLUSTER_NAME}-worker" "${CLUSTER_NAME}-worker2" >/dev/null 2>&1 || true

log "Removing existing registry container (if any)"
docker rm -f "${REGISTRY_NAME}" >/dev/null 2>&1 || true

log "Starting local registry container ${REGISTRY_NAME}"
docker run -d \
  -p "${REGISTRY_PORT}:5000" \
  --restart=always \
  --name "${REGISTRY_NAME}" \
  registry:2 >/dev/null
CLEANUP_CMDS+=("docker rm -f ${REGISTRY_NAME}")

log "Creating KinD cluster ${CLUSTER_NAME}"
KIND_CONFIG=$(mktemp)
cat <<EOF > "${KIND_CONFIG}"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
    - containerPort: 30080
      hostPort: 30080
      protocol: TCP
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."${REGISTRY_NAME}:${REGISTRY_PORT}"]
    endpoint = ["http://${REGISTRY_NAME}:${REGISTRY_PORT}"]
EOF
kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}" || die "Failed to create KinD cluster"
CLEANUP_CMDS+=("kind delete cluster --name ${CLUSTER_NAME}")
rm -f "${KIND_CONFIG}"

docker network connect kind "${REGISTRY_NAME}" >/dev/null 2>&1 || true
REGISTRY_IP=$(docker inspect -f '{{if .NetworkSettings.Networks.kind}}{{.NetworkSettings.Networks.kind.IPAddress}}{{end}}' "${REGISTRY_NAME}")
if [[ -z "${REGISTRY_IP}" ]]; then
  die "Failed to determine registry IP on kind network"
fi
KIND_NODES=$(kind get nodes --name "${CLUSTER_NAME}" 2>/dev/null || true)
if [[ -z "${KIND_NODES}" ]]; then
  die "Failed to enumerate KinD nodes for cluster ${CLUSTER_NAME}"
fi

for node in ${KIND_NODES}; do
  docker exec "${node}" /bin/sh -c "echo '${REGISTRY_IP} ${REGISTRY_NAME}' >> /etc/hosts"
  docker exec "${node}" mkdir -p "/etc/containerd/certs.d/${REGISTRY_NAME}:${REGISTRY_PORT}"
  cat <<NODECONF | docker exec -i "${node}" tee "/etc/containerd/certs.d/${REGISTRY_NAME}:${REGISTRY_PORT}/hosts.toml" >/dev/null
[host."http://${REGISTRY_NAME}:${REGISTRY_PORT}"]
  capabilities = ["pull", "resolve", "push"]
  skip_verify = true
NODECONF
  docker exec "${node}" systemctl restart containerd
done

log "Building project binaries"
make build

log "Building Docker images"
DOCKER_REGISTRY="${REGISTRY_HOST}:${REGISTRY_PORT}" DOCKER_TAG_VERSION="${REL_VERSION}" make docker-build

log "Pushing Docker images to ${REGISTRY_HOST}:${REGISTRY_PORT}"
DOCKER_REGISTRY="${REGISTRY_HOST}:${REGISTRY_PORT}" DOCKER_TAG_VERSION="${REL_VERSION}" make docker-push

log "Publishing Bicep test recipes"
BICEP_PLAIN_HTTP=false BICEP_RECIPE_REGISTRY="ghcr.io/willdavsmith/radius" BICEP_RECIPE_TAG_VERSION="${REL_VERSION}" make publish-test-bicep-recipes

log "Publishing Terraform test recipes"
make publish-test-terraform-recipes

log "Installing Radius into cluster ${CLUSTER_NAME}"
export PATH="${WORKDIR}/bin:${PATH}"
RAD_CMD=(
  rad install kubernetes
  --chart deploy/Chart
  --set rp.image="${REGISTRY_NAME}:${REGISTRY_PORT}/applications-rp",rp.tag="${REL_VERSION}"
  --set dynamicrp.image="${REGISTRY_NAME}:${REGISTRY_PORT}/dynamic-rp",dynamicrp.tag="${REL_VERSION}"
  --set controller.image="${REGISTRY_NAME}:${REGISTRY_PORT}/controller",controller.tag="${REL_VERSION}"
  --set ucp.image="${REGISTRY_NAME}:${REGISTRY_PORT}/ucpd",ucp.tag="${REL_VERSION}"
  --set bicep.image="${REGISTRY_NAME}:${REGISTRY_PORT}/bicep",bicep.tag="${REL_VERSION}"
  --set preupgrade.image="${REGISTRY_NAME}:${REGISTRY_PORT}/pre-upgrade",preupgrade.tag="${REL_VERSION}"
  --reinstall
)
"${RAD_CMD[@]}"

log "Waiting for Radius system pods to become ready"
kubectl wait --for=condition=Ready pods --all -n radius-system --timeout=300s

log "Installing Flux ${FLUX_VERSION}"
./.github/actions/install-flux/install-flux.sh "${FLUX_VERSION}"

log "Deploying Git HTTP backend"
./.github/actions/install-git-http-backend/install-git-http-backend.sh \
  "${GIT_HTTP_USERNAME}" "${GIT_HTTP_PASSWORD}" githttpbackend "${GIT_HTTP_IMAGE}"

log "Port-forwarding git service on localhost:30080"
kubectl port-forward -n githttpbackend svc/git-http 30080:3000 >/tmp/git-http-forward.log 2>&1 &
PORT_FORWARD_PID=$!
sleep 5

export GIT_HTTP_SERVER_URL="http://localhost:30080"
export GIT_HTTP_USERNAME="${GIT_HTTP_USERNAME}"
export GIT_HTTP_EMAIL="${GIT_HTTP_EMAIL}"
export GIT_HTTP_PASSWORD="${GIT_HTTP_PASSWORD}"
export DOCKER_REGISTRY="${REGISTRY_NAME}:${REGISTRY_PORT}"
export REL_VERSION="${REL_VERSION}"
export BICEP_RECIPE_REGISTRY="ghcr.io/willdavsmith/radius"
export BICEP_RECIPE_TAG_VERSION="${REL_VERSION}"
export TF_RECIPE_MODULE_SERVER_URL="http://tf-module-server.radius-test-tf-module-server.svc.cluster.local"
export RADIUS_TEST_FAST_CLEANUP=true
export TEST_TIMEOUT=1m
export PATH="${WORKDIR}/bin:${PATH}"

log "Running Flux non-cloud functional tests"
go test ./test/functional-portable/kubernetes/noncloud/... -count=1

log "Flux functional tests completed successfully"
