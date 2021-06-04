---
type: docs
title: "Azure KeyVault component"
linkTitle: "KeyVault"
description: "Deploy and orchestrate Azure KeyVault using Radius"
---

## Background

Without Radius, there are multiple steps to connect a KeyVault to a containerized application:

1. Create AKS cluster with Managed Identity and Pod Identity Profile enabled
1. Grant at least "Managed Identity Operator" permissions to the AKS Cluster Identity to be able to associate pod identity with pod
1. Create a User Assigned Managed Identity for each container that needs to access the KeyVault
1. Grant Keyvault access to the User Assigned Managed Identity created
1. Create an AAD Pod Identity which creates a binding between a pod label and the User Assigned Managed Identity created above.
1. Modify the k8s spec for the container to use Pod Identity label.
1. Create application container in the same namespace as the Pod Identity namespace.

Radius automates all these steps and the user application can simply use the Azure KeyVault deployed by the spec.

## KeyVault component

The Radius KeyVault component offers to the user:

- Managed resource deployment and lifecycle of the KeyVault
- Automatic configuration of Azure Managed Identities and RBAC between consuming components and the KeyVault
- Injection of connection information into connected containers
- Automatic secret management for configured components

## Configuraiton

| Key  | Required | Description | Example | Default |
|------|:--------:|-------------|---------|---------|
| name | y | The name of your component. Used to provide status and visualize the component. | `myvault` | -
| kind | y |The component kind and version. | `azure.com/KeyVault@v1alpha1` | -
| properties.config.managed | n | Tells Radius to manage the deployment and lifecycle of the underlying resource. | 'true' | `false`

### Example

The following examples shows a KeyVault that is deployed and managed by Radius.

```sh
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

## Bindings

KeyVault bindings are offered when other compute resources use them and are specified in the compute component's [`uses` configuration]({{< ref "components-model.md#uses" >}}).

### Access KeyVault from a container

Radius can place the KeyVault URI into a compute component's environment using the [`env` component functionality]({{< ref "components-model.md#uses" >}}).

In this example the URI used to access KeyVault is injected into the environment variable `KV_URI` within the container:

```sh
resource kvaccessor 'Components' = {
  name: 'kvaccessor'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {
      container: {
        image: 'radiusteam/azure-keyvault-app:latest'
      }
    }
    uses: [
      {
        binding: kv.properties.bindings.default
        env: {
          KV_URI: kv.properties.bindings.default.uri
        }
      }
    ]
  }
}
```

### Place secrets in a KeyVault

Radius can place secrets within a KeyVault automatically using the [`secrets` component functionality]({{< ref "components-model.md#uses" >}}).

For example, you can place connection strings for a database in a KeyVault that will be used by your container:

```sh
resource kvaccessor 'Components' = {
  name: 'kvaccessor'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {
      container: {
        image: 'radiusteam/azure-keyvault-app:latest'
      }
    }
    uses: [
      {
        binding: kv.properties.bindings.default
        env: {
          KV_URI: kv.properties.bindings.default.uri
        }
      }
      {
        binding: db.properties.bindings.mongo
          secrets: {
          store: kv.properties.bindings.default
          keys: {
            DBCONNECTION: db.properties.bindings.mongo.connectionString
          }
        }
      }
    ]
  }
}

resource db 'Components' = {
  name: 'db'
  kind: 'azure.com/CosmosDBMongo@v1alpha1'
  properties: {...}
}
```

## Tutorial

### Prerequisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create an environment]({{< ref create-environment.md >}})
- [Install Kubectl](https://kubernetes.io/docs/tasks/tools/)

If you are using Visual Studio Code with the Project Radius extension you should see syntax highlighting. If you have the offical Bicep extension installed, you should disable it for this tutorial. The instructions will refer to VS Code features like syntax highlighting and the problems windows - however, you can complete this tutorial with just a basic text editor.

### Understand the application

The Radius application you will be deploying is a simple python application that accesses Azure KeyVault for listing secrets. It has two components:

- An Azure KeyVault
- An Azure KeyVault accessor

The accessor uses an [Azure managed identity](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/overview) to access the KeyVault without any connection strings.

#### Azure KeyVault component

The following Radius application component describes a managed Azure KeyVault:

```sh
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

#### KeyVault accessor application

The keyvault accessor application is a simple python application that tries to access the keyvault at the KV_URI environment variable and then tries to list the secrets.

```sh
resource kvaccessor 'Components' = {
  name: 'kvaccessor'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {
      container: {
        image: 'radiusteam/azure-keyvault-app:latest'
      }
    }
    uses: [
      {
        binding: kv.properties.bindings.default
        env: {
          KV_URI: kv.properties.bindings.default.uri
        }
      }
    ]
  }
}
```

Here, Radius creates the Azure KeyVault and injects the KV_URI environment variable into the container with the uri. The application reads this environment variable to access the KeyVault. By default, the container is granted access as KeyVault Reader with scope as KeyVault

### Deploy application

#### Pre-requisites

- Make sure you have an active [Radius environment]({{< ref create-environment.md >}})
- Ensure you are logged into Azure using `az login`

#### Download Bicep file

Begin by creating a file named `template.bicep` and pasting the above components into an `app` resource. Alternately you can download it [below](#bicep-file).

#### Deploy template file

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

### Cleanup (optional)

When you are ready to clean up and delete the resources you can delete your environment. This will delete:

- The resource group
- Your Radius environment
- The application you just deployed

```sh
rad env delete --name azure --yes
```

### Bicep file

{{< rad file="template.bicep">}}
