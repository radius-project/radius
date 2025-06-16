# Testing Private Terraform Provider Registry with Radius

## Prerequisites
1. Radius installed in your Kubernetes cluster
2. Your private Terraform Provider Registry running (HTTP or HTTPS)
3. Registry implements the Terraform Registry Protocol v1

## Configuration Options

### Option 1: Registry in Same Kubernetes Cluster

If your registry is running as a pod/service in the same cluster:

```bicep
registry: {
  mirror: 'http://terraform-registry-service.default.svc.cluster.local:8080'
  authentication: {
    token: {
      source: localRegistryTokenSecret.id
      key: 'token'
    }
  }
}
```

### Option 2: Registry via NodePort

```bicep
registry: {
  mirror: 'http://<kubernetes-node-ip>:30080'  // Replace with actual node IP
  authentication: {
    token: {
      source: localRegistryTokenSecret.id
      key: 'token'
    }
  }
}
```

### Option 3: Registry via Ingress/LoadBalancer

```bicep
registry: {
  mirror: 'https://terraform-registry.your-domain.com'
  authentication: {
    token: {
      source: localRegistryTokenSecret.id
      key: 'token'
    }
  }
}
```

### Option 4: Local Development (Registry on Host)

```bicep
registry: {
  mirror: 'https://host.docker.internal:8443'  // For Docker Desktop
  // OR
  mirror: 'https://host.minikube.internal:8443'  // For Minikube
  authentication: {
    token: {
      source: localRegistryTokenSecret.id
      key: 'token'
    }
  }
}
```

## Testing Steps

### 1. Deploy Your Registry

Make sure your registry is accessible from the Radius pods:

```bash
# Test from inside the cluster
kubectl run test-curl --image=curlimages/curl --rm -it -- \
  curl -H "Authorization: Bearer test-token-123" \
  http://terraform-registry-service.default.svc.cluster.local:8080/v1/providers/hashicorp/azurerm/versions
```

### 2. Create Test Providers

Your registry should serve provider information. Example structure:
```
/v1/providers/hashicorp/azurerm/versions
/v1/providers/hashicorp/azurerm/1.0.0/download/linux/amd64
```

### 3. Deploy the Bicep File

```bash
rad deploy app-kubernetes-postgres.bicep \
  --parameters username=postgres \
  --parameters password=admin \
  --parameters gitlabPAT=your-gitlab-pat \
  --parameters localRegistryToken=test-token-123
```

### 4. Monitor the Deployment

Check the Radius logs to see if it's using your registry:

```bash
# Get the applications-rp pod
kubectl get pods -n radius-system | grep applications-rp

# Check logs
kubectl logs -n radius-system <applications-rp-pod> -f | grep -i "registry"
```

### 5. Verify Registry Access

Look for log entries like:
- "Configuring Terraform registry with mirror"
- "Successfully configured token authentication"
- HTTP requests to your registry URL

## Troubleshooting

### DNS Resolution Issues

If Radius can't resolve your registry hostname:

1. **For cluster services**: Use the full DNS name
   ```
   service-name.namespace.svc.cluster.local
   ```

2. **For external services**: Add host aliases to Radius deployment:
   ```yaml
   spec:
     hostAliases:
     - ip: "192.168.1.100"
       hostnames:
       - "terraform-registry.local"
   ```

### TLS/Certificate Issues

For HTTPS with self-signed certificates:

1. **HTTP Alternative**: Use HTTP if security allows:
   ```bicep
   mirror: 'http://terraform-registry-service:8080'
   ```

2. **Mount CA Certificate**: Add your CA to Radius pods (requires modifying Radius deployment)

### Network Policies

Ensure network policies allow traffic:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-radius-to-registry
  namespace: radius-system
spec:
  podSelector:
    matchLabels:
      app: applications-rp
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: default  # Where your registry is running
    ports:
    - protocol: TCP
      port: 8080
```

## Testing Provider Download

Create a test recipe that uses a provider from your registry:

```bicep
recipes: {
  'Applications.Core/extenders': {
    test: {
      templateKind: 'terraform'
      templatePath: 'git::https://github.com/example/module.git'
      // This module should use providers that will be fetched from your registry
    }
  }
}
```

## Expected Behavior

When working correctly:
1. Terraform will attempt to download providers from your registry instead of registry.terraform.io
2. Authentication headers will be included in requests
3. The `.terraformrc` file will be created with your registry configuration
4. Environment variables `TF_TOKEN_*` will be set

## Example Registry Implementation

Your registry should respond to:
```
GET /v1/providers/{namespace}/{type}/versions
{
  "versions": [
    {
      "version": "3.0.0",
      "protocols": ["5.0"],
      "platforms": [
        {
          "os": "linux",
          "arch": "amd64"
        }
      ]
    }
  ]
}

GET /v1/providers/{namespace}/{type}/{version}/download/{os}/{arch}
{
  "protocols": ["5.0"],
  "os": "linux",
  "arch": "amd64",
  "filename": "terraform-provider-azurerm_3.0.0_linux_amd64.zip",
  "download_url": "https://your-registry.com/downloads/...",
  "shasum": "...",
  "signing_keys": {...}
}
```