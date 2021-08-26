## Introduction

The Radius helm chart deploys the Radius control plane system services on a Kubernetes cluster using Helm.

### Chart Details

This chart installs the Radius controller manager, which includes:

- CRDs (Custom Resource Definitions) for Radius Applications, Components, Deployments, etc.
- Corresponding controllers for the CRDs, which will act on resources being created, updated, or deleted in the cluster.
- Validating Webhooks, which will validate the CRDs before going to the controller.

### Prerequisities

- Kubernetes cluster with RBAC enabled
- Helm 3

### There are two ways to install 

- Installing from source. In repo, this is done by invoking `make controller-deploy` or by directly installing the helm chart with `helm upgrade --wait --install --set container=<REGISTRY>/radius-controller --set tag=latest radius deploy/Chart -n radius-system`. `make controller-deploy` will also build and push the radius-controller, which is useful for inner loop development.
- Invoking `rad env init kubernetes`, which will install the latest released helm chart into your kubernetes cluster.

### Verify the installation

Verify that the controller is running in the radius-system namespace:

```
kubectl get pods -n radius-system
```

### Uninstalling the Chart

To uninstall the radius release:

```
helm uninstall radius -n radius-system
```

Currently we don't support uninstalling the helm chart via `rad env delete`. This will be addressed in a future release.
