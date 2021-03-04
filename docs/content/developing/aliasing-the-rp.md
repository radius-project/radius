---
type: docs
title: "Use an existing Radius RP deployment"
linkTitle: "Use existing RP"
description: "How to alias the RP to map an existing Radius RP deployment"
weight: 50
---

We provide a separate ARM template `rp-only.json` to map the Radius RP into a resource group using an *existing* deployment of the Radius RP.

## Step 1: Deploy the ARM template

**Create a resource group (if desired):**

```sh
az group create --name <resource group> --location <location>
```

**Deploy**

```sh
az deployment group create \
  --template-file deploy/rp-only.json \
  --resource-group <resource group>
```

This will deploy the the custom resource provider mapping for your resource group. This will allow you to deploy Radius applications using your resource group and a centrally maintained RP and Runtime.

You will **not** see your custom resources listed in the resource group in the portal even if deployment was successful.
