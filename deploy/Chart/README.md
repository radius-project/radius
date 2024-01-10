## Introduction

The Radius helm chart deploys the Radius services on a Kubernetes cluster using Helm.

### Prerequisites

- Kubernetes cluster with RBAC enabled
- Helm 3

### Installing the Radius Chart

To install the chart with the release name `radius`:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system

### Installing the Contour Chart

To install the chart with the release name `contour`:

helm repo add bitnami https://charts.bitnami.com/bitnami
helm install contour bitnami/contour --namespace demo -f deploy/contour-values.yaml
```

### Verify the installation

Verify that the controller is running in the radius-system namespace:

```
kubectl get pods -n radius-system
```

### Uninstalling the Chart

To uninstall/delete the `contour` deployment:

helm uninstall contour -n radius-system


To uninstall/delete the `radius` deployment:

```console
helm uninstall radius -n radius-system
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

Uninstalling the chart will not delete any data stored by Radius. To clean up any remaining data, delete the radius-system namespace. 
