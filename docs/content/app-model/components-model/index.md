---
type: docs
title: "Radius components"
linkTitle: "Components"
description: "Learn how to model your Application pieces with Radius Components"
weight: 200
---

 Component describe the "documentation" for a piece of code, data, or infrastructure. It can capture all of the important behaviors and requirements needed for a runtime to host that software. An Application can have both runnable Components (*e.g. containers, web applications*) and non-runnable Components (*e.g. databases, message queues*).

## Runnable Components

Runnable components run your logic & code. They can both provide and consume [bindings]({{< ref bindings-model.md >}}) to/from other components in your Application. For example, a [`radius.dev/Container` component]({{< ref container >}}) can describe and run your container workloads.

{{< rad file="snippets/container.bicep" embed=true marker="//SAMPLE" replace-key-run="//RUN" replace-value-run="run: {...}" replace-key-bindings="//BINDINGS" replace-value-bindings="bindings: {...}" >}}

## Non-runnable components

Resources like databases and message queues can be described via non-runnable Components, which can only provide [Bindings]({{< ref bindings-model.md >}}) and not consume them. For example, a [`azure.com/CosmosDBMongo` Component]({{< ref cosmos-mongodb >}}) is a non-runnable Component that describes an Azure CosmosDb account and database configured with the MongoDb API.

{{< rad file="snippets/database-managed.bicep" embed=true marker="//SAMPLE" >}}

## Portable components

Radius offers components that can work across any hosting model and will be satisfied using the best means available by the host. These are called *portable components* because application descriptions that use them can be *portable* across hosts without any configuration changes. Portable components are generally OSS services that are not tied to any particular SaaS or hosting platform and usually have multiple implementations.

For example the kind [`mongodb.com/Mongo@v1alpha1`]({{< ref mongodb >}}) specifies a generic MongoDB-compatible database. From the point-of-view of application code, it does not matter if the database is hosted using Kubernetes primitives like a `StatefulSet`, or a MongoDB operator, or a cloud-provider hosted offering like Azure CosmosDB. Radius will provision (or connect to) the appropriate implementation depending on the environment where the application is deployed.

## Bindings

The `bindings` configuration defines the [Bindings]({{< ref bindings-model.md >}}) which the Component offers. These Bindings can range from HTTP ports being opened on a container to an API that a database resource offers. For more information on Bindings visit the [Bindings documentation]({{< ref bindings-model.md >}}).

## Resource lifecycle

Radius components can be deployed and managed by either Radius (managed) or by a user (unmanaged).

### Radius managed

When `properties.config.managed` is set to `true`, Radius will manage the lifecycle of the underlying resource, meaning it will deploy and manage the resource itself on the [hosting platform]({{< ref environments >}}).

{{< rad file="snippets/database-managed.bicep" embed=true marker="//SAMPLE" >}}

### User managed

When `properties.config.managed` is set to `false` or omitted, you can explicitly specify an existing resource which you manage. This allows you to connect your Radius Components to existing databases, queues, and other non-compute resources. When you delete your application Radius will not change or delete your existing resource.

#### Radius component

{{< rad file="snippets/database-usermanaged.bicep" embed=true marker="//SAMPLE" >}}

#### Bicep resources

{{< rad file="snippets/database-usermanaged.bicep" embed=true marker="//COSMOS" >}}

## Configuration

| Key  | Description |
|------|-------------|
| name | The name of your component. Used for defining relationships and getting status for your components.
| properties.bindings | List of [bindings]({{< ref bindings-model.md >}}) which your Component offers to other Components or users.
| properties.uses | List of [bindings]({{< ref bindings-model.md >}}) which your runnable Component depends on for APIs and/or data.
| properties.config | Configuration of the Radius component, such as `managed` and `resource`.

Different [component types]({{< ref components >}}) may also have additional properties and configuration which can be set as part of the component definition.

## Next step

Now that you are familiar with Radius components, the next step is to learn about Radius bindings.

{{< button text="Learn about Bindings" page="bindings-model.md" >}}
