---
name: radius-build-images
description: 'Build Radius container images from source for local development and testing. Supports local registries for k3d and kind clusters, and Azure Container Registry for AKS clusters.'
argument-hint: 'Optional: target registry (e.g. k3d-myregistry:5050, localhost:5001, or myacr.azurecr.io/radius) — or leave blank to be prompted'
---

# Radius: Build and Push Container Images

Build Radius service images from source and push them to a registry reachable by your cluster. Use a local registry for k3d or kind, or Azure Container Registry for AKS.

## Images

The following images are built from this repository:

| Image | Source |
|-------|--------|
| `ucpd` | `deploy/images/ucpd` |
| `applications-rp` | `deploy/images/applications-rp` |
| `dynamic-rp` | `deploy/images/dynamic-rp` |
| `controller` | `deploy/images/controller` |
| `bicep` | `deploy/images/bicep` |
| `pre-upgrade` | `deploy/images/pre-upgrade` |
| `testrp` | `test/testrp` |
| `magpiego` | `test/magpiego` |

> **Not built here:** The `deployment-engine` and `dashboard` images are **not** built from this repository. They are published separately to `ghcr.io/radius-project/`. When installing with a custom registry you need to pin these to their public location — see the `radius-install-custom` skill.

## Procedure

### Step 1: Verify Prerequisites

Check that the required tools are available:

1. **Docker**: Run `docker info` — confirm the Docker daemon is running.
2. **Make**: Run `make --version` — confirm GNU Make is installed.
3. **Go**: Run `go version` — the required version is in `go.mod`.
4. **kubectl context**: Run `kubectl config current-context` — confirm the current kubeconfig context points to the cluster where you want to run Radius.
5. **kubectl connectivity**: Run `kubectl cluster-info` — confirm the current kubeconfig context is reachable.
6. **k3d** *(k3d path only)*: Run `k3d version` — confirm k3d is installed.
7. **kind** *(kind path only)*: Run `kind version` — confirm kind is installed.
8. **Azure CLI** *(AKS path only)*: Run `az version` — confirm the Azure CLI is installed and you are logged in with `az login`.

If any prerequisite is missing, stop and report which tool needs to be installed.

### Step 2: Set Up the Registry

Choose the registry setup that matches the cluster in your current kubeconfig context.

#### Option A: k3d local registry

Create a local registry and a k3d cluster wired to use it:

```sh
k3d registry create myregistry --port 5050
k3d cluster create mycluster --registry-use k3d-myregistry:5050
```

Then set the registry and tag:

```sh
export DOCKER_REGISTRY=k3d-myregistry:5050
export DOCKER_TAG_VERSION=latest
```

#### Option B: kind local registry

Create a local registry container and a kind cluster configured to use it:

```sh
docker run -d --restart=always -p "127.0.0.1:5001:5000" --name kind-registry registry:2
cat <<'EOF' | kind create cluster --name kind --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
	[plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5001"]
		endpoint = ["http://kind-registry:5000"]
EOF
docker network connect kind kind-registry || true
```

Then set the registry and tag:

```sh
export DOCKER_REGISTRY=localhost:5001
export DOCKER_TAG_VERSION=latest
```

If your kind cluster already exists and was not created with the registry mirror patch, recreate it with the configuration above before continuing.

#### Option C: AKS + Azure Container Registry

Log in to ACR so Docker can push to it:

```sh
az acr login --name <acr-name>
```

Then set the registry and tag:

```sh
export DOCKER_REGISTRY=<acr-name>.azurecr.io/radius
export DOCKER_TAG_VERSION=latest
```

Grant the AKS cluster pull access to ACR once per cluster:

```sh
az aks update -n <aks-cluster-name> -g <resource-group> --attach-acr <acr-name>
```

> **Note:** The Makefile default is `ghcr.io/radius-project/dev` (from `build/test.mk`). Always set `DOCKER_REGISTRY` explicitly so images aren't confused with public ones.

### Step 3: Build the Images

```sh
DOCKER_REGISTRY=${DOCKER_REGISTRY} DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION} make docker-build
```

This compiles all Go binaries and builds each Docker image. The `copy-manifests` step runs automatically.

> **Apple Silicon (arm64):** The default build produces `linux/amd64` images via emulation. For native-speed images on an arm64 Kubernetes cluster, use `make docker-multi-arch-push` instead — see [Multi-Architecture Builds](#multi-architecture-builds).

To build a **single image**:

```sh
make docker-build-<image-name>   # e.g. make docker-build-controller
```

### Step 4: Push to the Local Registry

Push the images to the registry selected in Step 2:

```sh
DOCKER_REGISTRY=${DOCKER_REGISTRY} DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION} make docker-push
```

For **k3d** and **kind**, the images are immediately available inside the cluster when the cluster has been configured to use the local registry.

For **AKS + ACR**, the images are available in ACR and AKS will pull them using the attached identity.

To push a single image:

```sh
make docker-push-<image-name>   # e.g. make docker-push-ucpd
```

### Step 5: Verify

Confirm the images were pushed to the selected registry:

```sh
docker images --filter "reference=${DOCKER_REGISTRY}/*:${DOCKER_TAG_VERSION}"
```

For **AKS + ACR**, you can also verify the repositories in ACR:

```sh
az acr repository list --name <acr-name>
```

Then proceed to the `radius-install-custom` skill to install Radius on your cluster.

## Multi-Architecture Builds

For native `linux/arm64` images on Apple Silicon, first set up buildx (one-time):

```sh
make configure-buildx
```

Then build and push all architectures:

```sh
DOCKER_REGISTRY=${DOCKER_REGISTRY} DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION} make docker-multi-arch-push
```

## Quick Reference

| Goal | Command |
|------|---------|
| Check target cluster | `kubectl config current-context && kubectl cluster-info` |
| Create k3d registry + cluster | `k3d registry create myregistry --port 5050 && k3d cluster create mycluster --registry-use k3d-myregistry:5050` |
| Set k3d registry env vars | `export DOCKER_REGISTRY=k3d-myregistry:5050 && export DOCKER_TAG_VERSION=latest` |
| Create kind registry + cluster | `docker run -d --restart=always -p "127.0.0.1:5001:5000" --name kind-registry registry:2` plus `kind create cluster` with a `containerdConfigPatches` mirror for `localhost:5001` |
| Set kind registry env vars | `export DOCKER_REGISTRY=localhost:5001 && export DOCKER_TAG_VERSION=latest` |
| Log in to ACR | `az acr login --name <acr-name>` |
| Set ACR registry env vars | `export DOCKER_REGISTRY=<acr-name>.azurecr.io/radius && export DOCKER_TAG_VERSION=latest` |
| Grant AKS pull access | `az aks update -n <cluster> -g <rg> --attach-acr <acr-name>` |
| Build all images | `make docker-build` |
| Push all images | `make docker-push` |
| Build + push (one step) | `make docker-build docker-push` |
| Single image build | `make docker-build-<name>` |
| Single image push | `make docker-push-<name>` |
| Multi-arch build + push | `make docker-multi-arch-push` |
| Setup buildx (one-time) | `make configure-buildx` |

## Key Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOCKER_REGISTRY` | `ghcr.io/radius-project/dev` (set by `build/test.mk`) | Target registry for built images. Set to `k3d-myregistry:5050` for k3d, `localhost:5001` for kind, or `<acr-name>.azurecr.io/radius` for AKS. |
| `DOCKER_TAG_VERSION` | `latest` | Image tag |
| `DOCKER_CACHE_GHA` | `0` | Set to `1` to enable GitHub Actions layer caching |
