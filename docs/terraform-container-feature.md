# Terraform Container Feature

## Overview

The Terraform container feature allows Radius to use pre-mounted Terraform binaries from a container image instead of downloading them at runtime during recipe execution. This improves performance and reduces internet dependencies for Terraform-based recipes.

## How It Works

When enabled, Radius adds an init container to the `applications-rp` and `dynamic-rp` pods that copies the Terraform binary from a specified container image to a shared volume. The main container then uses this pre-mounted binary instead of downloading Terraform via the hashicorp/hc-install library.

### Architecture

1. **Init Container**: Copies Terraform binary from the source image to `/terraform/terraform`
2. **Shared Volume**: An emptyDir volume mounted at `/terraform` in both init and main containers
3. **Fallback Logic**: If the pre-mounted binary is not found or invalid, the system falls back to downloading

## Usage

### CLI Installation

Use the `--terraform-container` flag with `rad install kubernetes`:

```bash
# Use default hashicorp terraform image
rad install kubernetes --terraform-container ghcr.io/hashicorp/terraform:latest

# Use private registry
rad install kubernetes --terraform-container myregistry.azurecr.io/terraform:1.6.0

# Use specific version
rad install kubernetes --terraform-container hashicorp/terraform:1.6.0
```

### Helm Chart Configuration

Configure directly via Helm values:

```yaml
global:
  terraform:
    enabled: true
    image: "ghcr.io/hashicorp/terraform"
    tag: "latest"
    binaryPath: "/bin/terraform"
```

Or via command line:

```bash
helm install radius deploy/Chart \
  --set global.terraform.enabled=true \
  --set global.terraform.image=myregistry.azurecr.io/terraform \
  --set global.terraform.tag=1.6.0
```

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `global.terraform.enabled` | `false` | Enable/disable the terraform container feature |
| `global.terraform.image` | `ghcr.io/hashicorp/terraform` | Container image containing terraform binaries |
| `global.terraform.tag` | `latest` | Image tag to use |
| `global.terraform.binaryPath` | `/bin/terraform` | Path to the terraform binary inside the source container |

## Benefits

1. **Performance**: Eliminates download time during recipe execution
2. **Reliability**: Reduces dependency on internet connectivity during runtime
3. **Consistency**: Ensures all pods use the same Terraform version
4. **Private Registries**: Supports private container registries for air-gapped environments
5. **Security**: Allows using verified/scanned terraform images from trusted registries

## Fallback Behavior

If the pre-mounted terraform binary is not available or fails validation:
- The system logs a warning
- Falls back to the original download behavior using hashicorp/hc-install
- Recipe execution continues normally

## Implementation Details

### Modified Components

- **CLI**: `pkg/cli/cmd/install/kubernetes/kubernetes.go` - Added `--terraform-container` flag
- **Helm**: `pkg/cli/helm/` - Added TerraformContainer field to ChartOptions
- **Templates**: 
  - `deploy/Chart/templates/rp/deployment.yaml` - Added init container
  - `deploy/Chart/templates/dynamic-rp/deployment.yaml` - Added init container
- **Values**: `deploy/Chart/values.yaml` - Added global.terraform configuration
- **Install Logic**: `pkg/recipes/terraform/install.go` - Added pre-mounted binary detection

### Container Image Requirements

The terraform container image must:
- Contain a terraform binary at the specified path (default: `/bin/terraform`)
- Be accessible from the Kubernetes cluster
- Have appropriate permissions for the init container to copy the binary

### Security Considerations

- Init containers run with non-root user (UID 65532)
- No privilege escalation allowed
- Only read access to the source image needed
- Shared volume is scoped to the pod only

## Testing

The feature includes:
- Unit tests for CLI flag parsing and validation
- Integration tests for Helm value propagation
- Fallback behavior verification
- Container image parsing tests

## Troubleshooting

### Common Issues

1. **Binary not found**: Check the `binaryPath` configuration matches the terraform location in your image
2. **Permission denied**: Ensure the init container has read access to the terraform binary in the source image
3. **Image pull failures**: Verify image name, tag, and registry access from the cluster
4. **Version mismatch**: Check terraform version compatibility with your recipes

### Debugging

Check init container logs:
```bash
kubectl logs -n radius-system <pod-name> -c terraform-init
```

Check main container logs for fallback behavior:
```bash
kubectl logs -n radius-system <applications-rp-pod> | grep terraform
```
