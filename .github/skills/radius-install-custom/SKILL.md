---
name: radius-install-custom
description: 'Install Radius on Kubernetes from custom-built container images. Use when deploying a dev build, testing local changes on a cluster, installing from a private registry, or reinstalling Radius with custom images. Complements the radius-build-images skill.'
argument-hint: 'Optional: registry and tag (e.g. ghcr.io/myorg latest) — or leave blank to be prompted'
---

# Install Radius from Custom Images

Install the Radius control plane on Kubernetes using images you built and pushed to a container registry.

## Prerequisites

- Images already pushed to a registry (see the `radius-build-images` skill)
- A running Kubernetes cluster (k3d, kind, AKS, EKS, GKE, etc.)
- `rad` CLI built and available in `$PATH` (see the `radius-build-cli` skill, or use a released version)
- `kubectl` configured to talk to the target cluster

## Procedure

### Step 1: Verify Prerequisites

Run these checks:

1. **rad CLI**: `rad version` — confirm the rad CLI is installed and working.
2. **kubectl**: `kubectl cluster-info` — confirm a Kubernetes cluster is reachable.
3. **Namespace**: `kubectl get namespace radius-system 2>/dev/null` — check if Radius is already installed.

If `rad` is not in `$PATH` but was built locally, the binary is at `./dist/$(go env GOOS)_$(go env GOARCH)/release/rad`.

### Step 2: Resolve Registry and Tag

Check whether `DOCKER_REGISTRY` and `DOCKER_TAG_VERSION` are already set:

```sh
echo "Registry: ${DOCKER_REGISTRY:-<not set>}"
echo "Tag: ${DOCKER_TAG_VERSION:-latest}"
```

- If `DOCKER_REGISTRY` is **not set**, ask the user:
  > What registry did you push the images to?
  > (e.g. `ghcr.io/myorg`, `docker.io/myusername`, `myacr.azurecr.io`)

- If `DOCKER_TAG_VERSION` is not set, default to `latest`.

Set the variables:

```sh
export DOCKER_REGISTRY=<value>
export DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION:-latest}
```

### Step 3: Check for Existing Installation

```sh
rad version
```

Look at the control plane status in the output:

- **Not installed** → proceed to Step 4 (fresh install).
- **Already installed (same version)** → ask the user if they want to reinstall with `--reinstall`.
- **Already installed (different version)** → suggest `rad upgrade kubernetes` or `--reinstall`.

### Step 4: Install Radius

For a **fresh install** using the local Helm chart from the repository:

```sh
rad install kubernetes \
  --chart deploy/Chart/ \
  --set global.imageRegistry=${DOCKER_REGISTRY} \
  --set global.imageTag=${DOCKER_TAG_VERSION} \
  --set de.image=ghcr.io/radius-project/deployment-engine \
  --set dashboard.image=ghcr.io/radius-project/dashboard
```

For a **reinstall** over an existing installation:

```sh
rad install kubernetes \
  --chart deploy/Chart/ \
  --set global.imageRegistry=${DOCKER_REGISTRY} \
  --set global.imageTag=${DOCKER_TAG_VERSION} \
  --set de.image=ghcr.io/radius-project/deployment-engine \
  --set dashboard.image=ghcr.io/radius-project/dashboard \
  --reinstall
```

> **External images:** The `deployment-engine` and `dashboard` images are **not built from this repository** — they come from `ghcr.io/radius-project/`. Setting `global.imageRegistry` overrides the registry for *all* images, including these. The `--set de.image=...` and `--set dashboard.image=...` flags above pin them to their public location so Kubernetes doesn't try to pull them from your custom registry.

#### Private Registry Authentication

If the cluster cannot pull images (401 Unauthorized / `ImagePullBackOff`), the cluster needs credentials to access the registry.

> **Note:** Authenticating locally with `docker login` or `az acr login` only allows your machine to push/pull. The Kubernetes cluster nodes need their own access.

**AKS + Azure Container Registry (recommended):** Attach the ACR to your AKS cluster (one-time):

```sh
az aks update -n <aks-cluster-name> -g <resource-group> --attach-acr <acr-name>
```

This grants the AKS managed identity pull access to the ACR. No image pull secrets needed.

**Any cluster — image pull secret:** Create a Kubernetes secret:

```sh
kubectl create namespace radius-system --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret docker-registry regcred \
  --docker-server=${DOCKER_REGISTRY} \
  --docker-username=<username> \
  --docker-password=<password> \
  -n radius-system
```

Then install with the pull secret:

```sh
rad install kubernetes \
  --chart deploy/Chart/ \
  --set global.imageRegistry=${DOCKER_REGISTRY} \
  --set global.imageTag=${DOCKER_TAG_VERSION} \
  --set-string 'global.imagePullSecrets[0].name=regcred'
```

### Step 5: Verify the Installation

1. Check that all pods are running:

   ```sh
   kubectl get pods -n radius-system
   ```

   Wait for all pods to reach `Running` and `1/1` ready status.

   > **Architecture mismatch:** If pods crash with `exec format error`, the image architecture doesn't match the cluster node architecture. For example, `linux/amd64` images won't run on `arm64` nodes (common with k3d/kind on Apple Silicon). Rebuild images using `make docker-multi-arch-push` (see the `radius-build-images` skill) or target the correct platform.

2. Verify the control plane version matches:

   ```sh
   rad version
   ```

3. Confirm images are from the correct registry:

   ```sh
   kubectl get pods -n radius-system -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.image}{"\n"}{end}{end}'
   ```

### Step 6: Initialize the Environment (Optional)

If this is a fresh cluster, set up a Radius workspace and environment:

```sh
rad init
```

This creates the default workspace, environment, and any required credentials.

## Common Install Options

| Flag | Purpose |
|------|---------|
| `--chart deploy/Chart/` | Use the local Helm chart (for dev builds) |
| `--set global.imageRegistry=...` | Override image registry |
| `--set global.imageTag=...` | Override image tag |
| `--reinstall` | Force reinstall over existing installation |
| `--skip-contour-install` | Skip the Contour ingress controller |
| `--kubecontext <name>` | Target a specific Kubernetes context |
| `--set global.zipkin.url=...` | Enable distributed tracing |
| `--set database.enabled=true` | Enable PostgreSQL database |
| `--set-string 'global.imagePullSecrets[0].name=...'` | Image pull secret for private registries |
| `--set de.image=ghcr.io/radius-project/deployment-engine` | Pin deployment-engine to public registry (required when using custom `global.imageRegistry`) |
| `--set dashboard.image=ghcr.io/radius-project/dashboard` | Pin dashboard to public registry (required when using custom `global.imageRegistry`) |

## Quick Reference

| Goal | Command |
|------|---------|
| Fresh install (custom images) | `rad install kubernetes --chart deploy/Chart/ --set global.imageRegistry=REG --set global.imageTag=TAG --set de.image=ghcr.io/radius-project/deployment-engine --set dashboard.image=ghcr.io/radius-project/dashboard` |
| Reinstall | Add `--reinstall` to the above |
| Check status | `rad version` |
| Check pods | `kubectl get pods -n radius-system` |
| Uninstall | `rad uninstall kubernetes` |
| Rollback stuck Helm release | `helm rollback radius <revision> -n radius-system` |
| Initialize environment | `rad init` |

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `ImagePullBackOff` on deployment-engine or dashboard | `global.imageRegistry` redirected these external images to your custom registry | Add `--set de.image=ghcr.io/radius-project/deployment-engine --set dashboard.image=ghcr.io/radius-project/dashboard` |
| `ImagePullBackOff` with 401 Unauthorized | Cluster nodes can't authenticate to the private registry | AKS: `az aks update --attach-acr <acr>`; other clusters: create an image pull secret |
| `rad install` fails with "another operation in progress" | Helm release stuck in `pending-upgrade` or `pending-install` | `helm rollback radius <last-good-revision> -n radius-system`, then retry |
| Pods crash with `exec format error` | Image architecture doesn't match node architecture | Rebuild with `make docker-multi-arch-push` or target the correct `TARGETARCH` |
