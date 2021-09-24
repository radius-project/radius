---
type: docs
title: "Azure KeyVault Component"
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

## Overview

The Radius KeyVault component `azure.com/KeyVault` offers to the user:

- Managed resource deployment and lifecycle of the KeyVault
- Automatic configuration of Azure Managed Identities and RBAC between consuming components and the KeyVault
- Injection of connection information into connected containers
- Automatic secret management for configured components

## Platform resources

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Key Vault](https://docs.microsoft.com/en-us/azure/key-vault/general/)
| [Kubernetes]({{< ref kubernetes >}}) | Not compatible

## Configuration

| Property | Description | Example(s) |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. (KeyVault currently only supports `true`) | `true`, `false`

## Resource lifecycle

An `azure.com/KeyVault` can be Radius-managed. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

{{< rad file="snippets/managed.bicep" embed=true marker="//KEYVAULT" >}}

## Bindings

### default

The `default` Binding of kind `azure.com/KeyVault` represents the the Key Vault resource itself, and all APIs it offers.

| Property | Description |
|----------|-------------|
| `VaultURI` | The URI address of the Azure Key Vault resource.

## Example

This example will walk through an Application that stores a database connection string in a Key Vault and is accessed from a container.

### Pre-requisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create an environment]({{< ref create-environment.md >}})
- (optional) [Install Radius VSCode extension]({{< ref setup-vscode >}})

### Understand the application

The Radius application you will be deploying is a simple python application that accesses Azure KeyVault for listing secrets. It has two components:

- An Azure KeyVault
- An Azure KeyVault accessor

The accessor uses an [Azure managed identity](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/overview) to access the KeyVault without any connection strings.

#### Azure KeyVault component

The following Radius application component describes a managed Azure KeyVault:

{{< rad file="snippets/managed.bicep" embed=true marker="//KEYVAULT" >}}

#### KeyVault accessor application

The keyvault accessor application is a simple python application that tries to access the keyvault at the KV_URI environment variable and then tries to list the secrets.

{{< rad file="snippets/managed.bicep" embed=true marker="//ACCESSOR">}}

Here, Radius creates the Azure KeyVault and injects the KV_URI environment variable into the container with the uri. The application reads this environment variable to access the KeyVault. By default, the container is granted access as KeyVault Reader with scope as KeyVault

### Deploy application

1. Download the Radius Key Vault application:

   {{< rad file="snippets/managed.bicep" download=true >}}

1. Submit the Radius template to Azure using:

   ```sh
   rad deploy azure-keyvault-managed.bicep
   ```

   This will deploy the application, create the Azure KeyVault, and launch the container.

### Access the application

To see the "radius-keyvault" Application working, you can check logs for the "kvaccessor" component:

```sh
rad component logs kvaccessor --application radius-keyvault 
```

You should see the application accessing the keyvault for secrets as below:

```txt
Getting vault url
Vault url: https://kv-blqmk.vault.azure.net/

.. List Secrets
```

You have completed this tutorial!

{{% alert title="Cleanup" color="warning" %}}
If you're done with testing, you can use the rad CLI to [delete an environment]({{< ref rad_env_delete.md >}}) to **prevent additional charges in your subscription**.
{{% /alert %}}
