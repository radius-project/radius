---
type: docs
title: "Radius Connections concept"
linkTitle: "Connections"
description: "Learn how to model your what your components offer with Radius Connections."
weight: 400
---

Once you define your Application and the Components inside of it, you can define the Connections between them. Connections are defined as:

{{% alert title="📄 Connections" color="primary" %}}
A **logical** unit of communication between Components.
{{% /alert %}}

The use cases for Connections are flexible, and include features like: 

- Configuring role based access control (RBAC) between Components.
- Passing resource information like connection strings, credentials, etc. between Components.
- Configuring security policies between Components and runtimes like Dapr.

{{% alert title="💡 Key concept" color="info" %}}
A Connection dependency between Components *may* affect the deployment order of Components depending on the kind. (*eg. a database will be deployed and connection string generated before a container is deployed and the value passed in*)
{{% /alert %}} 

## Types of Connections

Connections come in three forms:

- **Direct** to non-runnable Components
- Through **routes** to runnable Components
- To **platform-specific resources** without a Component

### Direct

When a runnable Component needs to interact with a non-runnable Component, it can be done by directly connecting to it. For example, a shopping basket service might need to connect to an inventory store database to read and write to it.

Each [Component kind]({{ ref resources }}) implementation defines the RBAC roles, environment variables, secrets, and other configuration that are mapped into the runnable component automatically.

#### Bicep example

{{< rad file="snippets/direct.bicep" embed=true marker="//SAMPLE" replace-key-container="//CONTAINER" replace-value-container="container: {...}" replace-key-properties="//PROPERTIES" replace-value-properties="properties: {...}" >}}

### Routes

Runnable components are also able talk to interact with eachother, even when cycles exist, via Routes. For example, the `HttpRoute` allows runnable Components to provide and consume HTTP endpoints.

#### Bicep example

In the following example, a frontend service connects to a backend service via an `HttpRoute`. Backend defines a port and provides the Route, and frontend defines a connection to the Route.

{{< rad file="snippets/route.bicep" embed=true marker="//SAMPLE" replace-key-container="//CONTAINER" replace-value-container="container: {...}" >}}

### Platform-specific resources

Often, platforms will have resources that are not portable across Radius platforms. For example, your application may use an Azure storage account, or specific features of Azure CosmosDB that cannot be abstracted with MongoDB. In this case, runnable Components can define a connection directly to the platform resource in Bicep.

{{% alert title="🚧 Under construction" color="info" %}}
Connecting directly to platform resources in Bicep is still in design/development. Stay tuned for updates.
{{% /alert %}}

## Next step

Now that you are familiar with Radius Connections, the next step is to learn about Radius Traits.

{{< button text="Learn about Traits" page="traits-model.md" >}}
