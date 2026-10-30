#!/bin/bash
# Build and install only into the isolated, explicitly named demo cluster.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
readonly ROOT
readonly RUN=/home/brhamilt/delegated-provisioning-deebec8c-run
readonly CLUSTER=delegated-deebec8c
readonly TAG=deebec8c-v1
readonly REGISTRY=delegated-registry-deebec8c
readonly NETWORK=delegated-net-deebec8c
export PATH=/home/brhamilt/.local/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
export KUBECONFIG="${RUN}/kubeconfig"
mkdir -p "${RUN}/home"
cd "${ROOT}"
go test ./pkg/recipes/delegation ./pkg/dynamicrp/backend/controller \
    ./pkg/dynamicrp/frontend ./pkg/corerp/api/v20250801preview \
    ./pkg/corerp/frontend/controller/recipepacks \
    ./pkg/recipes/configloader ./pkg/recipes/recipecontext
make copy-manifests build-rad docker-build-applications-rp docker-build-dynamic-rp docker-build-ucpd \
    DOCKER_REGISTRY=delegated.local DOCKER_TAG_VERSION="${TAG}"
if ! docker network inspect "${NETWORK}" >/dev/null 2>&1; then
    docker network create "${NETWORK}"
fi
if ! kind get clusters | grep -Fxq "${CLUSTER}"; then
    export KIND_EXPERIMENTAL_DOCKER_NETWORK="${NETWORK}"
    kind create cluster --name "${CLUSTER}" --kubeconfig "${KUBECONFIG}" \
        --wait 120s
fi
mkdir -p "${RUN}/modules" "${RUN}/tools/provider-mirror"
if ! docker container inspect "${REGISTRY}" >/dev/null 2>&1; then
    docker run -d --name "${REGISTRY}" --network "${NETWORK}" \
        -p 127.0.0.1:5568:5000 registry:2
fi
curl -fsS --retry 5 --retry-all-errors http://127.0.0.1:5568/v2/
cat >"${RUN}/registry-hosts.toml" <<EOF
server = "http://${REGISTRY}:5000"
[host."http://${REGISTRY}:5000"]
  capabilities = ["pull", "resolve"]
EOF
docker exec "${CLUSTER}-control-plane" mkdir -p \
    "/etc/containerd/certs.d/${REGISTRY}:5000"
docker cp "${RUN}/registry-hosts.toml" \
    "${CLUSTER}-control-plane:/etc/containerd/certs.d/${REGISTRY}:5000/hosts.toml"
if ! docker exec "${CLUSTER}-control-plane" grep -q 'config_path =' \
    /etc/containerd/config.toml; then
    printf '\n[plugins."io.containerd.grpc.v1.cri".registry]\n  config_path = "/etc/containerd/certs.d"\n' |
        docker exec -i "${CLUSTER}-control-plane" \
            tee -a /etc/containerd/config.toml >/dev/null
    docker exec "${CLUSTER}-control-plane" systemctl restart containerd
fi
publish_image() {
    local source="$1" name="$2"
    docker image inspect "${source}" >/dev/null 2>&1 || docker pull "${source}"
    docker tag "${source}" "localhost:5568/${name}:${TAG}"
    docker push "localhost:5568/${name}:${TAG}"
    docker exec "${CLUSTER}-control-plane" crictl pull \
        "${REGISTRY}:5000/${name}:${TAG}"
}
publish_image "delegated.local/applications-rp:${TAG}" applications-rp
publish_image "delegated.local/dynamic-rp:${TAG}" dynamic-rp
publish_image "delegated.local/ucpd:${TAG}" ucpd
publish_image ghcr.io/radius-project/controller:edge controller
publish_image ghcr.io/radius-project/bicep:edge bicep
publish_image ghcr.io/radius-project/deployment-engine:latest deployment-engine
publish_image ghcr.io/radius-project/dashboard:latest dashboard
publish_image ghcr.io/radius-project/pre-upgrade:edge pre-upgrade
publish_image alpine:3.23.4 alpine
for image in nginx:1.27-alpine nginx:1.28-alpine; do
    docker image inspect "${image}" >/dev/null 2>&1 || docker pull "${image}"
    docker save --platform linux/amd64 "${image}" | docker exec -i \
        "${CLUSTER}-control-plane" ctr -n k8s.io images import \
        --platform linux/amd64 -
done
download_release() {
    local product="$1" version="$2" destination="$3"
    local archive="${product}_${version}_linux_amd64.zip"
    curl -fsSL --retry 3 "https://releases.hashicorp.com/${product}/${version}/${archive}" \
        -o "${RUN}/modules/${archive}"
    curl -fsSL --retry 3 "https://releases.hashicorp.com/${product}/${version}/${product}_${version}_SHA256SUMS" \
        -o "${RUN}/modules/${product}_SHA256SUMS"
    (
        cd "${RUN}/modules"
        grep " ${archive}\$" \
            "${product}_SHA256SUMS" | sha256sum -c -
    )
    mkdir -p "${destination}"
    python3 -m zipfile -e "${RUN}/modules/${archive}" "${destination}"
}
download_release terraform 1.16.5 "${RUN}/tools/terraform"
download_release terraform-provider-kubernetes 2.38.0 \
    "${RUN}/tools/provider-mirror/registry.terraform.io/hashicorp/kubernetes/2.38.0/linux_amd64"
find "${RUN}/tools" -name 'terraform*' -type f -exec chmod a+rx {} \;
if ! docker container inspect delegated-recipes-deebec8c >/dev/null 2>&1; then
    docker run -d --name delegated-recipes-deebec8c --network "${NETWORK}" \
        --mount "type=bind,src=${RUN}/modules,dst=/usr/share/nginx/html,readonly" \
        nginx:1.27-alpine
fi
MODULE_IP="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' delegated-recipes-deebec8c)"
readonly MODULE_IP
docker exec "${CLUSTER}-control-plane" curl -fsSI \
    "http://${MODULE_IP}/terraform_1.16.5_linux_amd64.zip"
export HOME="${RUN}/home"
helm upgrade --install radius ./deploy/Chart -n radius-system \
    --create-namespace --wait --timeout 600s \
    --set global.imageRegistry="${REGISTRY}:5000" \
    --set global.imageTag="${TAG}" \
    --set rp.image=applications-rp --set rp.tag="${TAG}" \
    --set dynamicrp.image=dynamic-rp \
    --set dynamicrp.tag="${TAG}" \
    --set ucp.image=ucpd --set ucp.tag="${TAG}" \
    --set de.image=deployment-engine --set de.tag="${TAG}" \
    --set dashboard.tag="${TAG}" \
    --set global.terraform.initImage="${REGISTRY}:5000/alpine:${TAG}" \
    --set global.terraform.downloadUrl="http://${MODULE_IP}/terraform_1.16.5_linux_amd64.zip" \
    --set dynamicrp.resources.limits.memory=1500Mi
kubectl -n radius-system set env deployment/applications-rp \
    deployment/dynamic-rp RADIUS_EXPERIMENTAL_DELEGATION=true
kubectl -n radius-system rollout restart deployment/applications-rp \
    deployment/dynamic-rp deployment/ucp
kubectl -n radius-system rollout status deployment/applications-rp \
    --timeout=300s
kubectl -n radius-system rollout status deployment/dynamic-rp \
    --timeout=300s
kubectl -n radius-system rollout status deployment/ucp --timeout=300s
kubectl wait --for=condition=Available apiservice/v1alpha3.api.ucp.dev \
    --timeout=180s
kubectl get pods -n radius-system -o wide
