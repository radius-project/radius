---
name: radius-build-images
description: 'Build and push Radius container images to a registry. Use when building Radius Docker images from source, pushing images to a custom registry, building multi-arch images, testing custom builds, or deploying a custom Radius installation. Prompts for DOCKER_REGISTRY if not set.'
argument-hint: 'Optional: target registry (e.g. ghcr.io/myorg) — or leave blank to be prompted'
---

# Radius: Build and Push Container Images

Build Radius service images from source and push them to a container registry.

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

### Step 2: Resolve the Target Registry

Check the **effective** `DOCKER_REGISTRY` value — both the shell environment and Makefile:

```sh
echo "Shell: ${DOCKER_REGISTRY:-<not set>}"
make -p 2>/dev/null | grep '^DOCKER_REGISTRY'
```

> **Important:** Even if `DOCKER_REGISTRY` is not set in your shell, the Makefile may pick up a default from `build/test.mk` (`ghcr.io/radius-project/dev`). This overlaps with real public images on GHCR and should **not** be used for local builds — it makes it impossible to tell local images apart from pulled ones.

**Always explicitly set `DOCKER_REGISTRY`** for local builds:

```sh
export DOCKER_REGISTRY=<value>
```

- For **pushing to a registry**: use your own registry (e.g. `ghcr.io/myorg`, `docker.io/myusername`, `myacr.azurecr.io`)
- For **local-only builds**: use a clearly local prefix (e.g. `local`, `dev`, or `$(whoami)`):
  ```sh
  export DOCKER_REGISTRY=local
  ```
  Images will be tagged as `local/<image>:latest`.

Ask the user which registry to use if not already set. Do not proceed without an explicit value.

### Step 3: Set the Image Tag (Optional)

By default the tag is `latest`. To use a different tag:

```sh
export DOCKER_TAG_VERSION=<tag>   # e.g. 0.48.0, pr-1234, dev
```

If the user does not specify a tag, proceed with `latest`.

### Step 4: Authenticate to the Registry

Ensure the user is logged in before pushing:

```sh
docker login <registry-host>
```

For common registries:
- **GitHub (ghcr.io)**: `docker login ghcr.io -u <username> --password-stdin` (use a PAT with `write:packages` scope)
- **Docker Hub**: `docker login`
- **Azure Container Registry**: `az acr login --name <acr-name>`

If the user is already authenticated, skip this step.

### Step 5: Build the Images

Run the build:

```sh
DOCKER_REGISTRY=${DOCKER_REGISTRY} DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION:-latest} make docker-build
```

This compiles all Go binaries for `linux/amd64` and builds each Docker image. The `copy-manifests` step runs automatically.

> **Apple Silicon / arm64 hosts:** The default `docker-build` produces `linux/amd64` images (via emulation), which is slower and won't run natively on `arm64` Kubernetes clusters (k3d/kind on macOS). Use `make docker-multi-arch-push` for native multi-architecture images — see the Multi-Architecture Builds section below.

To build a **single image** instead of all:

```sh
make docker-build-<image-name>   # e.g. make docker-build-controller
```

### Step 6: Push the Images

```sh
DOCKER_REGISTRY=${DOCKER_REGISTRY} DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION:-latest} make docker-push
```

To push a single image:

```sh
make docker-push-<image-name>   # e.g. make docker-push-ucpd
```

### Step 7: Verify

Confirm the images were pushed successfully:

```sh
docker images --filter "reference=${DOCKER_REGISTRY}/*:${DOCKER_TAG_VERSION:-latest}"
```

## Multi-Architecture Builds

For `linux/amd64`, `linux/arm64`, and `linux/arm` images, first set up the buildx environment (one-time):

```sh
make configure-buildx
```

Then build and push all architectures in one step:

```sh
DOCKER_REGISTRY=${DOCKER_REGISTRY} DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION:-latest} make docker-multi-arch-push
```

Or build without pushing:

```sh
make docker-multi-arch-build
```

## Installing a Custom Build on Kubernetes

After pushing, install the custom build:

```sh
rad install kubernetes \
  --chart deploy/Chart/ \
  --set global.imageRegistry=${DOCKER_REGISTRY} \
  --set global.imageTag=${DOCKER_TAG_VERSION:-latest}
```

For private registries requiring a pull secret:

```sh
kubectl create secret docker-registry regcred \
  --docker-server=${DOCKER_REGISTRY} \
  --docker-username=<username> \
  --docker-password=<password> \
  -n radius-system

rad install kubernetes \
  --chart deploy/Chart/ \
  --set global.imageRegistry=${DOCKER_REGISTRY} \
  --set global.imageTag=${DOCKER_TAG_VERSION:-latest} \
  --set-string 'global.imagePullSecrets[0].name=regcred'
```

## Quick Reference

| Goal | Command |
|------|---------|
| Build all images | `make docker-build` |
| Push all images | `make docker-push` |
| Build + push (one step) | `make docker-build docker-push` |
| Multi-arch build | `make docker-multi-arch-build` |
| Multi-arch build + push | `make docker-multi-arch-push` |
| Single image build | `make docker-build-<name>` |
| Single image push | `make docker-push-<name>` |
| Save images to `.tar` | `make docker-save-images` |
| Load images from `.tar` | `make docker-load-images` |
| Setup buildx (one-time) | `make configure-buildx` |

## Key Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOCKER_REGISTRY` | `$(whoami)` | Target registry (e.g. `ghcr.io/myorg`) |
| `DOCKER_TAG_VERSION` | `latest` | Image tag |
| `DOCKER_CACHE_GHA` | `0` | Set to `1` to enable GitHub Actions layer caching |
