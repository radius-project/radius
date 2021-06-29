---
type: docs
title: "Radius components"
linkTitle: "Components"
description: "Learn how to model your application pieces with Radius components."
weight: 200
---

Components describe the code, data, and infrastructure pieces of a Radius application. Components only have meaning within the context of an application.

The component is documentation for a piece of code, data, or infrastructure. It can capture all of the important behaviors and requirements needed for a runtime to host that software. An application can have both runnable components (*e.g. containers, web applications*) and non-runnable components (*e.g. databases, message queues*).

## Runnable components

## Configuration

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your component. Used for defining relationships and getting status for your components. | `frontend`
| properties.uses | | Other components which your component depends on for bindings and/or data. Learn more [below](#uses). | [See below](#uses)
| properties.bindings | | [Bindings]({{< ref bindings-model.md >}}) which the component offers to other components or users. | [See below](#bindings).

Different [component types]({{< ref components >}}) may also have additional properties and configuration which can be set as part of the component definition.

## Bindings

The `bindings` configuration defines [bindings]({{< ref bindings-model.md >}}) which the component offers. These bindings can range from HTTP ports being opened on a container to an API that a database resource offers.

### Global bindings configuration

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The type of binding your component provides. | `http`
| name | y | The name of the binding which you provide. | `web`

Different [binding types]({{< ref bindings-model.md >}}) may also have additional properties and configuration which can be set as part of the component binding definition.

### Example

In the following example a container offers an HTTP binding on port 3000:

{{< rad file="snippets/components-model-storefront.bicep" embed=true marker="//SAMPLE" replace-key-hide="//HIDE" replace-value-hide="run: {...}" >}}

Runnable components run your logic & code. They can both provide and consume [bindings]({{< ref bindings-model.md >}}) to/from other components in your Application. For example, a [`radius.dev/Container` component]({{< ref container >}}) can describe and run your container workloads.

## Non-runnable components

Resources like databases and message queues can be described via non-runnable components, which can only provide [bindings]({{< ref bindings-model.md >}}) and not consume them. For example, a [`azure.com/CosmosDBMongo` component]({{< ref cosmos-mongodb >}}) is a non-runnable component that describes an Azure CosmosDb account and database configured with the MongoDb API.

## Configuration

| Key  | Description |
|------|-------------|
| name | The name of your component. Used for defining relationships and getting status for your components.
| properties.bindings | List of [bindings]({{< ref bindings-model.md >}}) which your component offers to other components or users.
| properties.uses | List of [bindings]({{< ref bindings-model.md >}}) which your Component depends on for APIs and/or data.

Different [component types]({{< ref components >}}) may also have additional properties and configuration which can be set as part of the component definition.

## Example

In this example, a [`radius.dev/Container` component]({{< ref container.md >}}) is defined with [bindings]({{< ref bindings-model.md >}}) that it both provides and consumes:

```sh
  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-todoapp'
        }
      }
      uses: [
        {
          binding: db.properties.bindings.mongo
          env: {
            DBCONNECTION: db.properties.bindings.mongo.connectionString
          }
        }
      ]
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
    }
  }
```

## Next step

Now that you are familiar with Radius components, the next step is to learn about Radius bindings.

{{< button text="Learn about bindings" page="bindings-model.md" >}}
