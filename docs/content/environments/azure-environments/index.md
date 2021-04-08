---
type: docs
title: "Microsoft Azure Radius environments"
linkTitle: "Microsoft Azure"
description: "Information on Radius environments hosted on Microsoft Azure"
weight: 20
---

## Azure environments

An Azure Radius environment consists of various resources that together act as the private resource provider (control plane) and the application hosting environment to which you deploy Radius applications (data plane):

<img src="./azure-overview.png" width=900 alt="Overview of an Azure Radius environment">

{{% alert title="⚠ Caution" color="warning" %}}
While this page describes the current implementation of Azure Radius environments, this is subject to change as the project matures and as Radius moves toward the goal of a fully hosted, multi-tenant, service.
{{% /alert %}}

Specifically, the following resources are created:

| Resource | Description |
|----------|-------------|
|**Data plane**
| Azure Kubernetes Service | The runtime into which containers and workloads are deployed.
| Azure CosmosDB account | The default database to user for Radius applications when `managed` is specified.
|**Control plane**
| Managed Identity | Identity used by the deployment script when the rad CLI deploys the environment for the first time
| App Service | Radius private resource provider (control plane)
| App Service plan | Underlying plan for the private RP app service

## Managing environments

These steps will walk through how to deploy, manage, and delete environments in Microsoft Azure.

### Pre-requisites

- [Azure subscription](https://signup.azure.com)
- [az CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli)
- [rad CLI]({{< ref install-cli.md >}})

### Deploy an environment

{{% alert title="⚠ Caution" color="warning" %}}
While Radius environments are optimized for cost, any costs incurred by the deployment and use of a Radius environment in an Azure subscription are the responsibility of the user.
{{% /alert %}}

1. Sign into Azure using the az CLI:
   
   ```bash
   az login
   ```
1. Set your preferred Azure subscription into which you want to deploy your environment:
   
   ```bash
   az account set --subscription SUB-ID
   ```
1. Deploy a Radius environment interactively:
   
   ```bash
   rad env init azure -i
   ```

   Follow the prompts, specifying the resource group you wish to create and selecting which region to deploy into.

   This step may take up to 10 minutes to deploy.

1. Verify deployment

   To verify the environment deployment succeeded, navigate to your subscription at https://portal.azure.com. You should see a new Resource Group:

   <img src="./azure-rg.png" width=200 alt="New resource group that was created">

   Inside you will see the [environment resources](#azure-environments) created for the environment:

   <img src="./azure-resources.png" width=500 alt="New resource group that was created">

### Delete an environment

1. Ensure you are still signed into Azure using the az CLI:
   
   ```bash
   az login
   ```
1. Use the rad CLI to delete the environment:

   ```bash
   rad env delete azure --yes
   ```

## Related links

- [Radius tutorials]({{< ref tutorial >}})
- [rad CLI reference]({{< ref cli >}})
