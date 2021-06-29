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

## Non-runnable components

Resources like databases and message queues can be described via non-runnable Components, which can only provide [Bindings]({{< ref bindings-model.md >}}) and not consume them. For example, a [`azure.com/CosmosDBMongo` Component]({{< ref cosmos-mongodb >}}) is a non-runnable Component that describes an Azure CosmosDb account and database configured with the MongoDb API.

## Bindings

The `bindings` configuration defines the [Bindings]({{< ref bindings-model.md >}}) which the Component offers. These Bindings can range from HTTP ports being opened on a container to an API that a database resource offers. For more information on Bindings visit the [Bindings documentation]({{< ref bindings-model.md >}}).

## Configuration

| Key  | Description |
|------|-------------|
| name | The name of your component. Used for defining relationships and getting status for your components.
| properties.bindings | List of [bindings]({{< ref bindings-model.md >}}) which your Component offers to other Components or users.
| properties.uses | List of [bindings]({{< ref bindings-model.md >}}) which your runnable Component depends on for APIs and/or data.

Different [component types]({{< ref components >}}) may also have additional properties and configuration which can be set as part of the component definition.

## Example

In the following example a container offers an HTTP binding on port 3000:

{{< rad file="snippets/components-model-storefront.bicep" embed=true marker="//SAMPLE" replace-key-hide="//HIDE" replace-value-hide="run: {...}" >}}

## Next step

Now that you are familiar with Radius components, the next step is to learn about Radius bindings.

{{< button text="Learn about bindings" page="bindings-model.md" >}}
