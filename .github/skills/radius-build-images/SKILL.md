---
name: radius-build-images
description: 'Build Radius container images from source for local development and testing. Push to any container registry that your Kubernetes cluster can pull from.'
argument-hint: 'Optional: target registry (e.g. ghcr.io/my-username, myacr.azurecr.io/radius) — or leave blank to be prompted'
---

# Radius: Build and Push Container Images

Build Radius service images from source and push them to a container registry that your Kubernetes cluster can reach. This follows the workflow documented in `docs/contributing/contributing-code/contributing-code-building/README.md` and `docs/contributing/contributing-code/contributing-code-control-plane/generating-and-installing-custom-build.md`.

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

If any prerequisite is missing, stop and report which tool needs to be installed.

### Step 2: Set the Registry

Ask the user for their container registry if `DOCKER_REGISTRY` is not already set. The registry must be one the user can push to and their Kubernetes cluster can pull from.

```sh
export DOCKER_REGISTRY=ghcr.io/<your-registry>
export DOCKER_TAG_VERSION=latest
```

> **Note:** The user must already be logged in to the registry (`docker login`, `az acr login`, etc.). If you get authentication errors, ask the user to log in first.

> **Default:** If `DOCKER_REGISTRY` is not set, the Makefile defaults to your OS username (from `build/docker.mk`). Always set it explicitly.

### Step 3: Build and Push the Images

Build all images and push them to the registry in one command:

```sh
DOCKER_REGISTRY=${DOCKER_REGISTRY} DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION} make docker-build docker-push
```

This compiles all Go binaries, builds each Docker image, and pushes them to the registry. The `copy-manifests` step runs automatically.

To build and push a **single image** (useful for fast iteration):

```sh
make docker-build-<image-name> && make docker-push-<image-name>
# e.g. make docker-build-applications-rp && make docker-push-applications-rp
```

### Step 4: Verify

Confirm the images were pushed:

```sh
docker images --filter "reference=${DOCKER_REGISTRY}/*:${DOCKER_TAG_VERSION}"
```

Then proceed to the `radius-install-custom` skill to install Radius on your cluster.

## Private Registries

For private registries that require authentication to pull, you will need to create a Kubernetes image pull secret when installing. See the `radius-install-custom` skill for details.

## Multi-Architecture Builds

For native `linux/arm64` images (e.g. on Apple Silicon with an arm64 Kubernetes cluster), first set up buildx (one-time):

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
| Set registry | `export DOCKER_REGISTRY=ghcr.io/<your-registry> && export DOCKER_TAG_VERSION=latest` |
| Build + push all images | `make docker-build docker-push` |
| Build all images | `make docker-build` |
| Push all images | `make docker-push` |
| Single image build + push | `make docker-build-<name> && make docker-push-<name>` |
| Multi-arch build + push | `make docker-multi-arch-push` |
| Setup buildx (one-time) | `make configure-buildx` |

## Key Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOCKER_REGISTRY` | OS username (from `build/docker.mk`) | Target registry for built images. Set explicitly to your registry. |
| `DOCKER_TAG_VERSION` | `latest` | Image tag |
| `DOCKER_CACHE_GHA` | `0` | Set to `1` to enable GitHub Actions layer caching |
