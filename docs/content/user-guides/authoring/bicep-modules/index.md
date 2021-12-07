---
type: docs
title: "Separate your application components into Bicep modules"
linkTitle: "Break into modules"
description: "Learn how to grow a single-file Radius application into a multi-file, large scale application with Bicep modules."
weight: 400
---

So far you've been creating and deploying Radius applications that are a single file. For larger application you may want to break your application into multiple files, each representing a separate microservice. [Bicep modules](https://docs.microsoft.com/azure/azure-resource-manager/bicep/modules) provide this capability. A module is a Bicep file that is deployed from another Bicep file, registry, or template spec.

## Start with your application

For this example, we'll be using a frontend/backend Radius application with a database that we want to break into separate files:

{{< rad file="snippets/all-in-one.bicep" embed=true >}}

## Break into files

The above application can be broken up into modules based on lifecycle and development teams.

### app.bicep

The file *app.bicep* is the main entry point for your application. It defines the main application configuration and adds any required modules:

{{< rad file="snippets/app.bicep" embed=true >}}

### infra.bicep

Teams often have central infrastructure teams manage the infrastructure resources for an application. All required infrastructure can be placed in a dedicated Bicep module, and even swapped out for canary/test/production infra resources.

{{% alert title="Swappable resources" color="info" %}}
Once infrasturcture is broken out into a module, you can easily swap out the module file depending on what environment you're deploying to. For example, you may have a dev-tier Cosmos for dev environments and a high-scale Cosmos for production.
{{% /alert %}}

{{< rad file="snippets/infra.bicep" embed=true >}}

Note that only the portable Mongo component is output from this module. This allows you to define only what you want to pass to other modules.

### frontend.bicep

The frontend container and HTTP route can be placed into a module. This allows the frontend dev team to author and deploy their service separate from backend.

{{< rad file="snippets/frontend.bicep" embed=true >}}

### backend.bicep

The backend microservice and route can also be placed into a module.

{{< rad file="snippets/backend.bicep" embed=true >}}

## Next steps

With your application broken up into modules, you can [deploy your application]({{< ref deploying >}}) or [create module templates]({{< ref bicep-templates >}}).
