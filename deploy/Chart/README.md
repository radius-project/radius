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

#### Terraform Binary Pre-mounting

By default, Radius downloads Terraform binaries at runtime for executing Terraform recipes. You can optionally configure Radius to use pre-mounted Terraform binaries from a container image instead. This can improve performance and reduce internet dependencies.

To enable Terraform pre-mounting:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.enabled=true \
  --set global.terraform.image=ghcr.io/hashicorp/terraform \
  --set global.terraform.tag=latest
```

Available configuration options under `global.terraform`:

- `enabled`: Whether to enable pre-mounting (default: `false`)
- `image`: Container image containing Terraform binaries (default: `ghcr.io/hashicorp/terraform`)
- `tag`: Image tag to use (default: `latest`)
- `binaryPath`: Path to Terraform binary inside the container (default: `/bin/terraform`)

Example with a private registry:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.enabled=true \
  --set global.terraform.image=myregistry.azurecr.io/terraform \
  --set global.terraform.tag=1.6.0
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
