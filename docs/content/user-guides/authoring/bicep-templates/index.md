---
type: docs
title: "Create shareable template modules"
linkTitle: "Create templates"
description: "Learn how to author and publish templates for your resources and application."
weight: 500
---

It can be tedious to duplicate properties and definitions for large applications that have a long list of similar services. Additionally, you may want all services in your application to adhere to specific security and compliance requirements.

Templates can be created with [Bicep modules](https://docs.microsoft.com/azure/azure-resource-manager/bicep/modules), allowing teams to define a base definition for a resource, such as a container, and then consume it in their application as separate resources.

Template modules can be consumed through one of:

- [Local file path](https://docs.microsoft.com/azure/azure-resource-manager/bicep/modules#local-file)
- [Container registry](https://docs.microsoft.com/azure/azure-resource-manager/bicep/modules#file-in-registry)
- [Azure template spec](https://docs.microsoft.com/azure/azure-resource-manager/bicep/modules#file-in-template-spec)

This guide will walk you through creating a template module for a container resource.

## Create a template module

Begin by creating a new Bicep file defining the resources you want to include in your module, as well as any parameters that define other resources or customization values used by these resources.

For this example, we'll use a container resource where a central monitoring team requires a liveness probe to be configured on port 3000:

{{< rad file="snippets/container.bicep" embed=true >}}

Make sure to input any parameters, either required or optional, that you want to use in your module. Also output any resources you want to use in your other resources and modules.

## Configure container registry

While any container registry can be used, an Azure Container Registry is the recommended option. Visit [this guide](https://docs.microsoft.com/azure/azure-resource-manager/bicep/private-module-registry) to learn how to configure a private registry.\

## Publish module to registry

Use the `rad-bicep` CLI to publish your module to the registry:

```bash
$ rad-bicep publish container.bicep --target br:exampleregistry.azurecr.io/templates/container:latest
```

## Consume template module

In your application you can now consume the template module by referencing it in a module definition. Note how the container image, set of ports, and livenessPort are all overridden:

{{< rad file="snippets/app.bicep" embed=true >}}

## Next steps

{{< button page="deploying" text="Deploy your application" >}}
