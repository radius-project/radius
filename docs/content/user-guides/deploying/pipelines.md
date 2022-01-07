---
type: docs
title: "Deploy Radius apps with CI/CD pipelines"
linkTitle: "Deployment pipelines"
description: "Learn how to use Radius with your continuous integration and deployment (CI/CD)"
weight: 500
---

{{% alert title="💬 Feedback" color="primary" %}}
Let us know your feedback on deploying Radius apps in our [GitHub Discussions Issue](https://github.com/project-radius/radius/discussions/1516).
{{% /alert %}}

## GitHub Actions

The following steps will walk through how to use GitHub Actions to deploy Radius apps.

### Create an Azure Service Principal

If deploying to Microsoft Azure, begin by creating an Azure Service Principal that will submit your deployment to Azure. Make sure to change the values of `subscriptionId` and `resourceGroupName` to match your target Radius environment.

```bash
az ad sp create-for-rbac -n "GitHub Deploy SP" --scopes /subscriptions/{subscriptionId} --role owner --sdk-auth
```

> While this example uses an Owner assignment at the subscription level, you can also assign your service principal a contributor role on the `resourceGroupName` resource group and a reader role on the `RE-resourceGroupName` resource group if the environment had been previously initialized. During initial environment initialization Owner is required to configure the private resource provider. This is a temporary workaround while a custom resource provider is being used for the Radius control-plane.

Take the output of the above command and paste it into a [GitHub secret](https://docs.github.com/actions/security-guides/encrypted-secrets#creating-encrypted-secrets-for-a-repository) named `AZURE_CREDENTIALS`.

### Create a workflow file

Next, create a new file named `deploy-radius.yml` under `.github/workflows/` and paste the following:

#### Microsoft Azure

```yml
name: Deploy Radius app
on:
  push:
    branches:
      - main
env:
  RADIUS_INSTALL_DIR: ./
  SUBSCRIPTION_ID: paste-subscription-id-here
  RESOURCE_GROUP: paste-resource-group-name-here
  LOCATION: westus2

jobs:
  deploy:
    name: Deploy app
    runs-on: ubuntu-latest
    steps:
    - name: Check out repo
      uses: actions/checkout@v2
    - name: az Login
      uses: azure/login@v1
      with:
        creds: ${{ secrets.AZURE_CREDENTIALS }}
    - name: Download rad CLI and rad-bicep
      run: |
        wget -q "https://get.radapp.dev/tools/rad/install.sh" -O - | /bin/bash
        ./rad bicep download
        ./rad --version
    - name: Initialize Radius environment
      run: ./rad env init azure -s ${SUBSCRIPTION_ID} -g ${RESOURCE_GROUP} -l ${LOCATION}
    - name: Deploy app
      run: ./rad deploy ./iac/app.bicep
```

#### Kubernetes

This example uses an Azure Kubernetes Service cluster, but any Kubernetes cluster context will work.

```yml
name: Deploy Radius app
on:
  push:
    branches:
      - main
env:
  RADIUS_INSTALL_DIR: ./
  NAMESPACE: default
  CLUSTER: paste-cluster-name-here
  SUBSCRIPTION_ID: paste-subscription-id-here
  RESOURCE_GROUP: paste-resource-group-name-here

jobs:
  deploy:
    name: Deploy app
    runs-on: ubuntu-latest
    steps:
    - name: Check out repo
      uses: actions/checkout@v2
    - name: Setup kubectl
      uses: azure/setup-kubectl@v1
    - name: az Login
      uses: azure/login@v1
      with:
        creds: ${{ secrets.AZURE_CREDENTIALS }}
    - name: Configure kubectl context
      run: az aks get-credentials --name ${CLUSTER} --resource-group ${RESOURCE_GROUP} --subscription ${SUBSCRIPTION_ID}
    - name: Download rad CLI and rad-bicep
      run: |
        wget -q "https://get.radapp.dev/tools/rad/install.sh" -O - | /bin/bash
        ./rad bicep download
        ./rad --version
    - name: Initialize Radius environment
      run: ./rad env init kubernetes -n ${NAMESPACE}
    - name: Deploy app
      run: ./rad deploy ./iac/app.bicep
```
