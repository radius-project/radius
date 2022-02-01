---
type: docs
title: "Radius components"
linkTitle: "Components"
description: "Learn how to model your Application's pieces with Radius Components"
weight: 200
---

Components describe the code, data, and infrastructure pieces of an application.

Each node of an architecture diagram would map to one Component. Together, an Application's Components capture all of the important behaviors and requirements needed for a runtime to host that app. 

## Component definition

In your app's Bicep file, the Component resource captures: 

| Property | Description | Example |
|----------|-------------|---------|
| **Resource type** | What type of thing is this? | `Container`
| **Name** | The logical name of the Component, must be unique per-Application and resource type | `my-container`
| **Essentials** | How do I run this? | Container image and tag (`my-container:latest`)
| **Connections** | What other Components will I interact with? | Need to read from `my-db` 
| **Routes** | What capabilities do I provide for others? | Offer an HTTP endpoint on `/home`
| **Traits** | What operational behaviors do I offer and interact with? | Need a Dapr sidecar (`dapr.io.App`)

### Example

The following example shows two Components, one representing a [Container]({{< ref container >}}) and the other describing a [Dapr State Store](https://docs.dapr.io/developing-applications/building-blocks/state-management/state-management-overview/).

#### Container

 [Containers]({{< ref container >}}) describe a container workload and how to run it.

In an example eshop app, the storefront code's would include info necessary for the storefront container to run:

{{< rad file="snippets/app.bicep" embed=true marker="//CONTAINER" >}}

#### Dapr State Store
A Dapr State Store can be modeled as a Component as well, with information about what underlying resource provides the Component. Other Components, like the `storefront` container above, can now connect to this State Store and save/get state items.

{{< rad file="snippets/app.bicep" embed=true marker="//STATESTORE" >}}

## Platform resources

Radius Applications can be deployed to a variety of [Radius-supported platforms]({{< ref platforms >}}), and some Components can be backed by different underlying resource types based on platform. For example, a `redislabs.com.Redis` is backed by Azure Redis Cache when deployed to Azure and backed by a Redis pod when deployed to Kubernetes.

### Resource lifecycle
<!-- TODO: overhaul this section as we replace Radius-managed with Bicep modules -->

Radius offers two methods for managing the lifecycle of a Component: Radius-managed and user-managed. Separately, you can use platform specific resources for any Bicep resources that do not have Radius Components that model them.

Radius-managed components are good for development purposes or to write an *all-in-one* deployable application. User-manged resources allow you to bind to infrastructure, such as cloud resources that are deployed separately from your code.

{{% alert title="💡 Key concept" color="info" %}}
User-managed resources allow you to represent cloud resources as part of your application and have an easier experience connecting to them. You can create user-managed resources using the same `.bicep` files as your application or create them separately.
{{% /alert %}}

| | Health tracking | Access creds/properties | Configure RBAC | Customize resource configuration |
|-|:---------------:|:-----------------------:|:--------------:|:--------------------:|
| User-managed | ✅ | ✅ | ✅ | ✅ |
| Radius-managed | ✅ | ✅ | ✅ | ❌ |
| Platform-specific | ✅ | ❔*‡ | ✅ | ✅ |

\* resource specific - See [this doc](https://docs.microsoft.com/azure/azure-resource-manager/bicep/bicep-functions-resource)
<br />
‡ still in development/review

#### User-managed

When `managed` is set to `false` or omitted, you can explicitly specify an existing `resource` which you manage. This allows you to connect your Radius Components to existing databases, queues, and other non-compute resources. When you delete your Application Radius will not change or delete your existing resource.

{{< rad file="snippets/database-usermanaged.bicep" embed=true replace-key-properties="//PROPERTIES" replace-value-properties="properties: {...}" >}}

#### Radius-managed

When `managed` is set to `true`, like in the example above, Radius will manage the lifecycle of the underlying resource, meaning it will deploy and manage the resource itself on the [hosting platform]({{< ref platforms >}}).

{{% alert title="📋 Feedback" color="primary" %}}
We'd love to hear your feedback on User-managed resources. Please visit [GitHub Discussions](https://github.com/project-radius/radius/discussions/1269) and let us know how you currently use and want to use them!
{{% /alert %}}

{{< rad file="snippets/database-managed.bicep" embed=true marker="//SAMPLE" >}}

## Portability

Non-runnable components can work across hosting models without any configuration changes, and will be satisfied using the best means available by the host. They are generally OSS services that are not tied to any particular SaaS or hosting platform and usually have multiple implementations.

For example the resource type [`mongo.com.MongoDatabase`]({{< ref mongodb >}}) specifies a generic MongoDB-compatible database. From the point-of-view of application code, it does not matter if the database is hosted using Kubernetes primitives like a `StatefulSet`, or a MongoDB operator, or a cloud-provider hosted offering like Azure CosmosDB. Radius will provision (or connect to) the appropriate implementation depending on the environment where the application is deployed.

## Platform-specific resources

Some resources you use in your application don't have a Radius type to bind to. For example, Azure Cognitive Services offers a service that isn't portable across environments, having only a service in Azure.

These resources can still be used in Radius Applications. Instead of being modeled as a resources within an Application, they can be placed *beside* an Application, and other Components can still reference resource connection strings and other data.

Additionally, through the use of the [Bicep existing keyword](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/resource-declaration?#reference-existing-resources), you can connect to resources that are deployed and managed separately from the Application.

You can configure role based access control (RBAC) on Azure resources by specifying roles within the connection information. See [the connections page] {{< ref connections-model >}} for more details: 

{{< rad file="snippets/platform-specific-azure.bicep" embed=true marker="//SAMPLE" >}}

## Next step

Now that you are familiar with Radius Components, the next step is to learn about Radius Connections.

{{< button text="Learn about Connections" page="connections-model.md" >}}
