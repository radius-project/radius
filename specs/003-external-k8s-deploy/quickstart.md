# Quickstart: External Kubernetes Cluster Deployment

**Feature**: 003-external-k8s-deploy
**Date**: 2026-04-14

## Prerequisites

- A running Radius installation on a Kubernetes cluster
- `rad` CLI configured and connected to the Radius installation
- For EKS: AWS credentials registered using `rad credential register aws access-key` or `rad credential register aws irsa`, with `eks:DescribeCluster` permission
- For AKS: Azure credentials registered using `rad credential register azure sp` or `rad credential register azure wi`, with `Azure Kubernetes Service Cluster User Role` and a Kubernetes RBAC role on the target cluster
- An external EKS or AKS cluster accessible from the Radius cluster's network

## Deploy to an External EKS Cluster

### 1. Register AWS credentials (if not already done)

```bash
rad credential register aws access-key \
  --access-key-id $AWS_ACCESS_KEY_ID \
  --secret-access-key $AWS_SECRET_ACCESS_KEY
```

### 2. Create an environment targeting the external EKS cluster

```bash
rad env create my-eks-env \
  --aws-account-id 123456789012 \
  --aws-region us-west-2 \
  --kubernetes-target external \
  --kubernetes-cluster-type eks \
  --kubernetes-cluster-name my-eks-cluster \
  --kubernetes-namespace my-app 
```

Or via Bicep:

```bicep
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-eks-env'
  properties: {
    providers: {
      aws: {
        accountId: '123456789012'
        region: 'us-west-2'
      }
      kubernetes: {
        namespace: 'my-app'
        target: 'external'
        clusterType: 'eks'
        clusterName: 'my-eks-cluster'
      }
    }
  }
}
```

### 3. Deploy a recipe

```bash
rad deploy app.bicep --environment my-eks-env
```

Resources created by recipes will land on the external EKS cluster in the `my-app` namespace.

## Deploy to an External AKS Cluster

### 1. Register Azure credentials (if not already done)

```bash
rad credential register azure sp \
  --client-id $AZURE_CLIENT_ID \
  --client-secret $AZURE_CLIENT_SECRET \
  --tenant-id $AZURE_TENANT_ID
```

### 2. Create an environment targeting the external AKS cluster

```bash
rad env create my-aks-env \
  --azure-subscription-id $AZURE_SUBSCRIPTION_ID \
  --azure-resource-group my-rg \
  --kubernetes-target external \
  --kubernetes-cluster-type aks \
  --kubernetes-cluster-name my-aks-cluster \
  --kubernetes-namespace my-app
```

Or via Bicep:

```bicep
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-aks-env'
  properties: {
    providers: {
      azure: {
        subscriptionId: '<subscription-id>'
        resourceGroupName: 'my-rg'
      }
      kubernetes: {
        namespace: 'my-app'
        target: 'external'
        clusterType: 'aks'
        clusterName: 'my-aks-cluster'
      }
    }
  }
}
```

### 3. Deploy a recipe

```bash
rad deploy app.bicep --environment my-aks-env
```

## Verify

After deployment, confirm resources exist on the external cluster:

```bash
# Point kubectl at the external cluster
aws eks update-kubeconfig --name my-eks-cluster --region us-west-2
# or
az aks get-credentials --resource-group my-rg --name my-aks-cluster

kubectl get all -n my-app
```
