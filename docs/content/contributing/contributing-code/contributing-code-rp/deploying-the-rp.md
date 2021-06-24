---
type: docs
title: "Deploy a custom Radius resource provider"
linkTitle: "Deploy custom RP"
description: "How to deploy a custom build of the Radius resource provider to an Azure environment"
weight: 40
---

{{% alert title="Note" color="warning" %}}
This guide will cover deploying a Radius environment to Azure manually. This is part of the job that's done by `rad env init azure`. This workflow is useful when you need to test changes to the templates, or when you need to override a setting that's not provided by the CLI.
{{% /alert %}}

### Step 1: (Optional) Build & Push the Image

You'll need a place to push Docker images that is publicly accessible. Alternatively you can use the public images from `radiusteam/radius-rp`.

```sh
docker build . -t <your registry>/radius-rp:latest
docker push <your registry>/radius-rp
```

### Step 2: Deploy the ARM Template

**Deploy**

```sh
az deployment sub create \
  --template-file deploy/rp-full.json \
  --parameter "image=<repository>/radius-rp:latest" \
  --parameter "location=..." \
  --parameter "resourceGroup=..." \
  --parameter "controlPlaneResourcGroup=..."
```

If you open the Azure portal and navigate to your subscription you should see two resource groups you should see an:

- App Resource Group:
  - Custom Resource Provider registration 

- Control Plane Resource Group:
  - App Service Plan
  - App Service Site
  - CosmosDB account

  - Kubernetes Cluster
  - Deployment Script
  - Managed Identity
