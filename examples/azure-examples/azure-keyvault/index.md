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

If you are using Visual Studio Code with the Project Radius extension you should see syntax highlighting. If you have the offical Bicep extension installed, you should disable it for this tutorial. The instructions will refer to VS Code features like syntax highlighting and the problems windows - however, you can complete this tutorial with just a basic text editor.

## Understanding the application

The Radius application you will be deploying is a simple python application that accesses Azure KeyVault for listing secrets. It has two components:

- An Azure KeyVault
- An Azure KeyVault accessor


### Azure KeyVault component

The following Radius application component describes a managed Azure KeyVault:

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
        image: 'radiusteam/azure-keyvault-app:latest'
      }
    }
    dependsOn: [
      {
        name: 'kv'
        kind: 'azure.com/KeyVault'
        setEnv: {
          KV_URI: 'keyvaulturi'
        }
      }
    ]
  }
}
```

Here, Radius creates the Azure KeyVault and injects the KV_URI environment variable into the container with the uri. The application reads this environment variable to access the KeyVault. By default, the container is granted access as KeyVault Reader with scope as KeyVault


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
