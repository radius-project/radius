---
name: radius-install-custom
description: 'Install Radius on a Kubernetes cluster from custom-built container images. Works with any cluster and any registry the cluster can pull from.'
argument-hint: 'Optional: registry and tag (e.g. ghcr.io/my-username latest) — or leave blank to be prompted'
---

# Install Radius from Custom Images

Install the Radius control plane on a Kubernetes cluster using images you built from source. This follows the workflow documented in `docs/contributing/contributing-code/contributing-code-control-plane/generating-and-installing-custom-build.md`.

## Prerequisites

- Images built and pushed using the `radius-build-images` skill
- `kubectl` configured for the target cluster (`kubectl cluster-info`)
- `rad` CLI available in `$PATH` (see the `radius-build-cli` skill, or use a released version)

## Procedure

### Step 1: Verify Prerequisites

Run these checks:

1. **rad CLI**: `rad version` — confirm the rad CLI is installed and working.
2. **kubectl context**: `kubectl config current-context` — confirm the current kubeconfig context points to the target cluster.
3. **kubectl connectivity**: `kubectl cluster-info` — confirm the cluster is reachable.

If `rad` is not in `$PATH` but was built locally, the binary is at `./dist/$(go env GOOS)_$(go env GOARCH)/release/rad`.

### Step 2: Confirm Registry and Tag

Confirm the registry and tag match what was used in the build step:

```sh
echo "Registry: ${DOCKER_REGISTRY:-<not set>}"
echo "Tag: ${DOCKER_TAG_VERSION:-latest}"
```

If not set, ask the user for the registry they pushed images to during the `radius-build-images` skill. Set the variables:

```sh
export DOCKER_REGISTRY=ghcr.io/<your-registry>
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

Install from the local Helm chart, pointing to your custom image registry:

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

For **private registries** that require authentication to pull, first create a Kubernetes secret:

```sh
kubectl create namespace radius-system 2>/dev/null || true
kubectl create secret docker-registry regcred \
  --docker-server=${DOCKER_REGISTRY} \
  --docker-username=<username> \
  --docker-password=<password> \
  -n radius-system

rad install kubernetes \
  --chart deploy/Chart/ \
  --set global.imageRegistry=${DOCKER_REGISTRY} \
  --set global.imageTag=${DOCKER_TAG_VERSION} \
  --set de.image=ghcr.io/radius-project/deployment-engine \
  --set de.tag=latest \
  --set dashboard.image=ghcr.io/radius-project/dashboard \
  --set dashboard.tag=latest \
  --set-string 'global.imagePullSecrets[0].name=regcred'
```

To target a different cluster without changing the active kubeconfig context, add `--kubecontext <context-name>`.

> **Why pin `de.image`/`de.tag` and `dashboard.image`/`dashboard.tag`?** The `deployment-engine` and `dashboard` images are not built in this repository — they come from `ghcr.io/radius-project/`. Setting `global.imageRegistry` would otherwise redirect those pulls to your registry where they don't exist.

### Step 5: Verify the Installation

1. Check that all pods are running:

   ```sh
   kubectl get pods -n radius-system
   ```

   Wait for all pods to reach `Running` and `1/1` ready status.

2. Verify the control plane version:

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

## Quick Reference

| Goal | Command |
|------|---------|
| Set registry | `export DOCKER_REGISTRY=ghcr.io/<your-registry> && export DOCKER_TAG_VERSION=latest` |
| Fresh install | `rad install kubernetes --chart deploy/Chart/ --set global.imageRegistry=${DOCKER_REGISTRY} --set global.imageTag=${DOCKER_TAG_VERSION} --set de.image=ghcr.io/radius-project/deployment-engine --set de.tag=latest --set dashboard.image=ghcr.io/radius-project/dashboard --set dashboard.tag=latest` |
| Reinstall | Add `--reinstall` to the above |
| Check status | `rad version` |
| Check pods | `kubectl get pods -n radius-system` |
| Uninstall | `rad uninstall kubernetes --yes` |
| Initialize environment | `rad init` |

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `ImagePullBackOff` on deployment-engine or dashboard | `global.imageRegistry` redirected external images to your registry | Ensure `--set de.image=ghcr.io/radius-project/deployment-engine --set de.tag=latest --set dashboard.image=ghcr.io/radius-project/dashboard --set dashboard.tag=latest` are set |
| `ImagePullBackOff` on Radius images | Cluster cannot pull from the registry | Verify images were pushed (`docker images`), the cluster can reach the registry, and authentication is configured if needed |
| `rad install` fails with "another operation in progress" | Helm release stuck in `pending-upgrade` or `pending-install` | `helm rollback radius <last-good-revision> -n radius-system`, then retry |
| Pods crash with `exec format error` | Image architecture doesn't match node architecture | Rebuild with `make docker-multi-arch-push` (see the `radius-build-images` skill) |
