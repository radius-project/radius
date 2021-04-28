---
type: docs
title: "Azure KeyVault application"
linkTitle: "Azure KeyVault"
description: "Sample application that deploys and accesses Azure KeyVault"
weight: 20
---

This application showcases how Radius can deploy Azure KeyVault.

## Prerequisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create an environment]({{< ref create-environment.md >}})
- [Install Kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Register your subscription for the AAD Pod Identity preview feature](https://docs.microsoft.com/en-us/azure/aks/use-azure-ad-pod-identity#register-the-enablepodidentitypreview)

If you are using Visual Studio Code with the Project Radius extension you should see syntax highlighting. If you have the offical Bicep extension installed, you should disable it for this tutorial. The instructions will refer to VS Code features like syntax highlighting and the problems windows - however, you can complete this tutorial with just a basic text editor.

## Understanding the application

The application you will be deploying is a simple python application that accesses Azure KeyVault for listing secrets. It has two components:

- An Azure KeyVault
- An Azure KeyVault accessor

You can find the source code for the keyvault accessor application [here](https://github.com/Azure/radius/tree/main/examples/azure-examples/azure-keyvault/apps).

### Azure KeyVault component

The Radius application describes an Azure KeyVault as below:-

```
resource kv 'Components' = {
  name: 'kv'
  kind: 'azure.com/KeyVault@v1alpha1'
  properties: {
      config: {
          managed: true
      }
  }
}
```

### KeyVault accessor application

The keyvault accessor application is a simple python application that tries to access the keyvault at the KV_URI environment variable and then tries to list the secrets.

```
resource kvaccessor 'Components' = {
  name: 'kvaccessor'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {
      container: {
        image: 'vinayada/azure-keyvault-app:latest'
      }
    }
    dependsOn: [
      {
        name: 'kv'
        kind: 'azure.com/KeyVault'
        setEnv: {
          KV_URI: 'kvuri'
        }
      }
    ]
  }
}
```

Here, Radius creates the Azure KeyVault and injects the KV_URI environment variable into the container with the uri. The application reads this environment variable to access the KeyVault. By default, the container is granted access as KeyVault Reader with scope as KeyVault

#### (optional) Updating the application

If you wish to modify the application code, you can do so and create a new image as follows:

```bash
cd <Radius Path>/examples/azure-examples/azure-examples/apps/app
docker build -t <your docker hub>/azure-keyvault-app:change1 .
docker push <your docker hub>/azure-keyvault-app:change1
```

Make sure to update the container images in the application resource of your deployment template if you create your own image.

## Deploy application

### Pre-requisites

- Make sure you have an active [Radius environment]({{< ref create-environment.md >}}).
- Ensure you are logged into Azure using `az login`

### Download Bicep file

Begin by creating a file named `template.bicep` and pasting the above components. Alternately you can download it [below](#bicep-file).

### Deploy template file

Submit the Radius template to Azure using:

```sh
rad deploy template.bicep
```

This will deploy the application, create the Azure KeyVault, and launch the container.

### Access the application

{{% alert title="⚠️ Temporary" color="warning" %}}
To gain access to the application now that it is deployed, make sure to merge the underlying AKS cluster into your Kubectl config:
```sh
rad env merge-credentials --name azure 
```
This step will eventually be removed.
{{% /alert %}}

To see the keyvault application working, you can check logs:

```sh
rad logs radius-keyvault kvaccessor
```

You should see the application accessing the keyvault for secrets as below:

```
Getting vault url
Vault url: https://kv-blqmk.vault.azure.net/

.. List Secrets
```

You have completed this tutorial!

## Cleanup (optional)

When you are ready to clean up and delete the resources you can delete your environment. This will delete:

- The resource group
- Your Radius environment
- The application you just deployed

```sh
rad env delete --name azure --yes
```


## Bicep file

{{< rad file="template.bicep">}}