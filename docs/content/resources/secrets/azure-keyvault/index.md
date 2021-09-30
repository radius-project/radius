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

## Component spec

{{< tabs Managed User-managed >}}

{{% codetab %}}
In the following example a managed Key Vault Component is defined, where the underlying resource is deployed and managed by the platform:
{{< rad file="snippets/managed.bicep" embed=true marker="//KEYVAULT" >}}
{{% /codetab %}}

{{% codetab %}}
{{% alert title="🚧 Under construction" color="warning" %}}
User-managed resources are not yet supported for Azure Key Vaults. Check back soon for updates.
{{% /alert %}}
{{% /codetab %}}

{{< /tabs >}}


### Resource lifecycle

| Property | Description | Example |
|----------|-------------|---------|
| managed | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. (KeyVault currently only supports `true`) | `true`

## Provided data

### Properties

| Property | Description |
|----------|-------------|
| `uri` | The URI address of the Azure Key Vault resource.
