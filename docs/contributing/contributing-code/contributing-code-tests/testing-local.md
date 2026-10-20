# Accelerating local verification on a Kubernetes cluster

## Purpose

This page describes the fast local-iteration loop for working on Radius control-plane images — build, push, and redeploy a single component against a running Kubernetes cluster instead of re-running the full functional suite. It is for contributors iterating on a resource provider (for example the applications RP) who want to validate a change on a real cluster quickly. For the full set of test tiers and when to run each, start at the [test matrix overview](./README.md); for the end-to-end suite, see [running functional tests](./running-functional-tests.md).

This loop applies when you only need to update a control-plane image. If you change the deployment or environment, re-run `rad init` (or the install steps below) instead.

## Prerequisites

- A Kubernetes cluster and a working [local dev environment](../contributing-code-debugging/radius-os-processes-debugging.md).
- A container registry your cluster can pull from (for example Azure Container Registry or `ghcr.io`).
- The `DOCKER_REGISTRY` environment variable set to that registry:

  ```sh
  export DOCKER_REGISTRY=ghcr.io/your-registry
  ```

- A login to the registry with anonymous pull enabled. The login must be refreshed periodically (about every 3 hours) because it logs out frequently:

  ```sh
  az acr login -n <registry>
  az acr update --name <registry> --anonymous-pull-enabled
  ```

## Steps

### 1. Build and push the initial images

Build and push every Radius image (applications-rp, ucpd, and the others required to run Radius):

```sh
make docker-build && make docker-push
```

### 2. Install Radius on the cluster

```sh
go run ./cmd/rad/main.go install kubernetes --chart deploy/Chart --set global.imageRegistry=ghcr.io/your-registry --set global.imageTag=latest
go run ./cmd/rad/main.go workspace create kubernetes
go run ./cmd/rad/main.go group create radius-rg
go run ./cmd/rad/main.go switch radius-rg
go run ./cmd/rad/main.go env create radius-rg --kubernetes-namespace default
go run ./cmd/rad/main.go env switch radius-rg
```

If your registry requires authentication, create a Kubernetes secret and pass it with `imagePullSecrets`:

```sh
kubectl create secret docker-registry regcred \
  --docker-server=ghcr.io/your-registry \
  --docker-username=<username> \
  --docker-password=<password> \
  -n radius-system

go run ./cmd/rad/main.go install kubernetes --chart deploy/Chart \
  --set global.imageRegistry=ghcr.io/your-registry \
  --set global.imageTag=latest \
  --set 'global.imagePullSecrets[0].name=regcred'
```

### 3. (Optional) Configure Azure resources

The steps above do not let Radius talk to Azure resources. To enable that, run steps 1–2 and then create a service principal:

```sh
az ad sp create-for-rbac --role Owner --scope /subscriptions/<subscriptionId>/resourceGroups/<resourcegroupname>
```

The command prints `appId`, `displayName`, `password`, and `tenant`. Use those values to register the credential:

```sh
go run ./cmd/rad/main.go env update radius-rg --azure-subscription-id <subscriptionId> --azure-resource-group <resourcegroupName>
go run ./cmd/rad/main.go credential register azure sp --client-id <appId> --client-secret <pwd> --tenant-id <tenantId>
```

### 4. Deploy and iterate

Deploy a Bicep file to the cluster:

```sh
go run ./cmd/rad/main.go deploy <bicep>
```

### Redeploy a single resource provider

Once Radius is installed, iterate on a single component — for example the applications resource provider — without reinstalling:

```sh
make docker-build-applications-rp && make docker-push-applications-rp && kubectl delete pod -l control-plane=applications-rp
```

This builds and pushes the applications-rp image and deletes its running pod. Because the deployment uses the [`latest` tag](https://kubernetes.io/docs/concepts/containers/images/#updating-images), Kubernetes re-pulls the image on restart, so the pod comes back with your newly pushed build.

### Redeploy the environment

Re-deploying an environment is a bit clunky today. Tear it down and deploy Radius again:

```sh
go run ./cmd/rad/main.go env delete <envname>
go run ./cmd/rad/main.go uninstall kubernetes
go run ./cmd/rad/main.go workspace delete
```

Alternatively, delete and recreate the Kubernetes cluster.

## Verification

- After `kubectl delete pod -l control-plane=applications-rp`, the pod restarts and reports `Running` with the freshly pushed image (`kubectl get pods -n radius-system`).
- `go run ./cmd/rad/main.go deploy <bicep>` completes and deploys your file to the cluster.

## Troubleshooting

- **Push or pull is rejected after a while.** The registry login expires roughly every 3 hours; re-run `az acr login -n <registry>`.
- **The cluster cannot pull your images.** Confirm anonymous pull is enabled (`az acr update --name <registry> --anonymous-pull-enabled`) or that the `regcred` `imagePullSecrets` secret is set.
- **The pod restarts with the old image.** Make sure the image tag is `latest` so Kubernetes re-pulls on restart, and that the push succeeded before you deleted the pod.

> 💡 Tip: switch your default namespace to `radius-system` with `kubectl config set-context --current --namespace=radius-system`.
