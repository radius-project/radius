---
name: radius-install-custom
description: 'Install Radius on a Kubernetes cluster from custom-built container images. Use the local registry path for k3d and kind clusters, and the ACR path only when the target cluster is AKS.'
argument-hint: 'Optional: registry and tag (e.g. k3d-myregistry:5050 latest, localhost:5001 latest, or myacr.azurecr.io/radius latest) — or leave blank to use the defaults from the build skill'
---

# Install Radius from Custom Images

Install the Radius control plane on a Kubernetes cluster using images you built from source. Use the local registry flow for k3d and kind clusters. Use Azure Container Registry only when the target cluster is AKS.

## Prerequisites

- Images built and pushed using the `radius-build-images` skill
- `kubectl` configured for the target cluster (`kubectl cluster-info`)
- `rad` CLI available in `$PATH` (see the `radius-build-cli` skill, or use a released version)

## Procedure

### Step 1: Verify Prerequisites

Run these checks:

1. **rad CLI**: `rad version` — confirm the rad CLI is installed and working.
2. **kubectl context**: `kubectl config current-context` — confirm the current kubeconfig context points to the cluster where you want to install Radius.
3. **kubectl connectivity**: `kubectl cluster-info` — confirm the current kubeconfig context is reachable.
4. **Existing install**: `kubectl get namespace radius-system 2>/dev/null` — check if Radius is already installed.

If `rad` is not in `$PATH` but was built locally, the binary is at `./dist/$(go env GOOS)_$(go env GOARCH)/release/rad`.

`rad install kubernetes` installs to the active kubeconfig context by default. If the current context is not the target cluster, switch it before continuing:

```sh
kubectl config use-context <context-name>
```

If the current context points to an AKS cluster, use the ACR path below. Otherwise use the local registry path for k3d or kind.

### Step 2: Confirm Registry and Tag

Confirm the registry and tag match what was used in the build step:

```sh
echo "Registry: ${DOCKER_REGISTRY:-<not set>}"
echo "Tag: ${DOCKER_TAG_VERSION:-latest}"
```

If not set, use the registry values from the `radius-build-images` skill:

**k3d cluster:**

```sh
export DOCKER_REGISTRY=k3d-myregistry:5050
export DOCKER_TAG_VERSION=latest
```

**kind cluster:**

```sh
export DOCKER_REGISTRY=localhost:5001
export DOCKER_TAG_VERSION=latest
```

**AKS cluster:**

```sh
export DOCKER_REGISTRY=<acr-name>.azurecr.io/radius
export DOCKER_TAG_VERSION=latest
```

### Step 3: Check for Existing Installation

```sh
rad version
```

Look at the control plane status in the output:

- **Not installed** → proceed to Step 4 (fresh install).
- **Already installed** → add `--reinstall` to the install command in Step 4.

### Step 4: Install Radius

The following command installs Radius on the cluster referenced by your current kubeconfig context.

Use the registry that matches the target cluster:

- **k3d cluster**: `DOCKER_REGISTRY=k3d-myregistry:5050`
- **kind cluster**: `DOCKER_REGISTRY=localhost:5001`
- **AKS cluster**: `DOCKER_REGISTRY=<acr-name>.azurecr.io/radius`

```sh
rad install kubernetes \
  --chart deploy/Chart/ \
  --set global.imageRegistry=${DOCKER_REGISTRY} \
  --set global.imageTag=${DOCKER_TAG_VERSION} \
  --set de.image=ghcr.io/radius-project/deployment-engine \
  --set de.tag=latest \
  --set dashboard.image=ghcr.io/radius-project/dashboard \
  --set dashboard.tag=latest
```

If you want to target a different cluster without changing the active kubeconfig context, add `--kubecontext <context-name>`.

For a **reinstall** over an existing installation, add `--reinstall`:

```sh
rad install kubernetes \
  --chart deploy/Chart/ \
  --set global.imageRegistry=${DOCKER_REGISTRY} \
  --set global.imageTag=${DOCKER_TAG_VERSION} \
  --set de.image=ghcr.io/radius-project/deployment-engine \
  --set de.tag=latest \
  --set dashboard.image=ghcr.io/radius-project/dashboard \
  --set dashboard.tag=latest \
  --reinstall
```

> **Why pin `de.image`/`de.tag` and `dashboard.image`/`dashboard.tag`?** The `deployment-engine` and `dashboard` images are not built in this repository — they come from `ghcr.io/radius-project/`. Setting `global.imageRegistry` would otherwise redirect those pulls to your selected registry where they don't exist. Pinning the image and tag for just these two components keeps them pointed at the public source.

> **AKS + ACR:** If the target cluster is AKS, make sure `az aks update --attach-acr` has already been run for the cluster so nodes can pull the custom images.

### Step 5: Verify the Installation

1. Check that all pods are running:

   ```sh
   kubectl get pods -n radius-system
   ```

   Wait for all pods to reach `Running` and `1/1` ready status.

   > **Architecture mismatch:** If pods crash with `exec format error`, the image architecture doesn't match the cluster node architecture. Rebuild images using `make docker-multi-arch-push` (see the `radius-build-images` skill).

2. Verify the control plane version:

   ```sh
   rad version
   ```

3. Confirm images are from the selected registry:

   ```sh
   kubectl get pods -n radius-system -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.image}{"\n"}{end}{end}'
   ```

### Step 6: Initialize the Environment (Optional)

If this is a fresh cluster, set up a Radius workspace and environment:

```sh
rad init
```

## Quick Reference

| Goal | Command |
|------|---------|
| Fresh install | `rad install kubernetes --chart deploy/Chart/ --set global.imageRegistry=${DOCKER_REGISTRY} --set global.imageTag=${DOCKER_TAG_VERSION} --set de.image=ghcr.io/radius-project/deployment-engine --set de.tag=latest --set dashboard.image=ghcr.io/radius-project/dashboard --set dashboard.tag=latest` |
| Reinstall | Add `--reinstall` to the above |
| k3d registry | `export DOCKER_REGISTRY=k3d-myregistry:5050 && export DOCKER_TAG_VERSION=latest` |
| kind registry | `export DOCKER_REGISTRY=localhost:5001 && export DOCKER_TAG_VERSION=latest` |
| AKS ACR registry | `export DOCKER_REGISTRY=<acr-name>.azurecr.io/radius && export DOCKER_TAG_VERSION=latest` |
| Check status | `rad version` |
| Check pods | `kubectl get pods -n radius-system` |
| Uninstall | `rad uninstall kubernetes` |
| Initialize environment | `rad init` |

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `ImagePullBackOff` on deployment-engine or dashboard | `global.imageRegistry` redirected external images to the selected registry | Ensure `--set de.image=ghcr.io/radius-project/deployment-engine --set de.tag=latest --set dashboard.image=ghcr.io/radius-project/dashboard --set dashboard.tag=latest` are set |
| `ImagePullBackOff` on Radius images in k3d | The k3d cluster is not wired to the local registry | Recreate the cluster with `k3d cluster create mycluster --registry-use k3d-myregistry:5050` |
| `ImagePullBackOff` on Radius images in kind | The kind cluster was not created with the local registry mirror configuration | Recreate the kind cluster with the `containerdConfigPatches` mirror for `localhost:5001` from the `radius-build-images` skill |
| `ImagePullBackOff` on Radius images in AKS | AKS does not have pull access to ACR | Run `az aks update -n <cluster> -g <rg> --attach-acr <acr-name>` |
| `rad install` fails with "another operation in progress" | Helm release stuck in `pending-upgrade` or `pending-install` | `helm rollback radius <last-good-revision> -n radius-system`, then retry |
| Pods crash with `exec format error` | Image architecture doesn't match node architecture | Rebuild with `make docker-multi-arch-push` |
