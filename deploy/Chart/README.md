## Introduction

The Radius helm chart deploys the Radius services on a Kubernetes cluster using Helm.

### Prerequisites

- Kubernetes cluster with RBAC enabled
- Helm 3

### Installing the Chart

To install the chart with the release name `radius`:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system
```

### Configuration Options

#### Terraform Binary Pre-downloading

By default, Radius downloads Terraform binaries at runtime when Terraform recipes are executed. You can optionally configure Radius to pre-download Terraform binaries during pod startup to improve performance.

To enable Terraform pre-downloading:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.enabled=true
```

This automatically downloads the latest Terraform version. For custom sources (private repositories, proxies, etc.), specify a complete download URL:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.enabled=true \
  --set global.terraform.downloadUrl="https://my-artifactory.com/terraform_1.5.7_linux_amd64.zip"
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

Uninstalling the chart will not delete any data stored by Radius. To clean up any remaining data, delete the radius-system namespace.
