---
type: docs
title: "Frequently asked questions"
linkTitle: "FAQ"
description: "Commonly asked questions and answers about Project Radius"
weight: 999
---

## Appllications

### Can I incrementally adopt, or "try out" Radius?

**Yes**. You can use the [Bicep `existing` keyword](https://docs.microsoft.com/azure/azure-resource-manager/bicep/resource-declaration?tabs=azure-powershell#existing-resources) to add a Radius application to previously deployed resources. Learn more in the [Radius authoring guide]({{< ref authoring >}}). In the future we will also support other experiences in Bicep and the Azure portal for adding Radius to existing resources.

## Environments

### Can I connect to an existing environment?

**Yes**. When you initialize an environment via [`rad env init`]({{< ref rad_env_init.md >}})), you can provide an existing Azure subscription or Kubernetes cluster context. Radius will update your `config.yaml` file with the appropriate values.

### When would/should I use more than one environment?

Users can employ multiple environments for isolation and organization, for example based on:
- Permissions (managed at the Resource Group/Subscription level in Azure)
- Purpose (dev vs prod)
- Difference in hosting (standalone Kubernetes vs Microsoft Azure)

### Can an Azure resource group be used for more than one environment?

**Yes**. While not supported in the CLI, a Radius `.config.yaml` file can be manually configured such that multiple environments can point to a single Resource Group.

### Is environment info saved somewhere besides the config.yaml file?

**No**. You can specify a different yaml file as the config (via the `--config` flag), but environments are a local concept. Environment definitions don’t get deployed or saved elsewhere.

## Bicep templates

### Can one bicep file represent more than one application?

**Yes**. You can have multiple Application resources defined in one file.

### Can a bicep file represent something other than applications?

**Yes**. Bicep files can contain both Radius reosurces and Azure resources. Everything in a Bicep file becomes an ARM deployment.

## Components

### Can I modify a component after it’s been deployed?

**Yes**. You will need to modify the component definition in your .bicep file’s application definition and re-deploy the application.

While updating Radius-managed resources in Azure and Kubernetes is possible outside of a Radius deployment, these changes will place your component into an unknown state and may be overridden the next time you deploy your application.

### What does `managed: true` mean?

This flag tells Radius to manage the lifetime of the component for you. The component will be deleted when you delete the application.

### Is Azure App Service supported?

**Not yet**. For now we're focusing on containers, but in the future we plan on expanding to other Azure services such as App Service, Functions, Logic Apps, and others. Stay tuned for more information.

## Does Radius support all Azure resources?

**Yes**. You can use any Azure resource type by modeling it in Bicep outside the `radius.dev/Application` resource and defining a connection to the resource from a `ContainerComponent`. See the [connections page]({{< ref connections-model >}})) for more details.
