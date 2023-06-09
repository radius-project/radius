## Introduction

The Radius helm chart deploys the Radius services on a Kubernetes cluster using Helm.

### Prerequisities

- Kubernetes cluster with RBAC enabled
- Helm 3

### Installing the Chart

To install the chart with the release name `radius`:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system
```

### Verify the installation

Verify that the controller is running in the radius-system namespace:

```
kubectl get pods -n radius-system
```

### Uninstalling the Chart

To uninstall/delete the `radius` deployment:

```console
helm delete radius
```

The command removes all the Kubernetes components associated with the chart and deletes the release.
