---
type: docs
title: "Radius components"
linkTitle: "Components"
description: "Learn how to model your application pieces with Radius components."
weight: 200
---

Components describe the code, data, and infrastructure pieces of a Radius application. Components only have meaning within the context of an application.

The component is documentation for a piece of code, data, or infrastructure. It can capture all of the important behaviors and requirements needed for a runtime to host that software. An application can have both runnable components (*e.g. containers, web applications*) and non-runnable components (*e.g. databases, message queues*).

## Configuration

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your component. Used for defining relationships and getting status for your components. | `frontend`
| properties.uses | | Other components which your component depends on for bindings and/or data. Learn more [below](#uses). | [See below](#uses)
| properties.provides | | [Bindings]({{< ref bindings-model.md >}}) which the component offers to other components or users. | [See below](#provides).

Different [component types]({{< ref components >}}) may also have additional properties and configuration which can be set as part of the component definition.

## provides

The `provides` configuration defines [bindings]({{< ref bindings-model.md >}}) which the component offers. These bindings can range from HTTP ports being opened on a container to an API that a database resource offers.

### Global provides configuration

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The type of binding your component provides. | `http`
| name | y | The name of the binding which you provide. | `web`

Different [binding types]({{< ref bindings-model.md >}}) may also have additional properties and configuration which can be set as part of the component binding definition.

### Example

In the following example a container offers an HTTP binding on port 3000:

```sh
resource store 'Components' = {
  name: 'storefront'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    provides: [
      {
        kind: 'http'
        name: 'web'
        containerPort: 3000
      }
    ]
  }
}
```

## Uses

The `uses` property tells Radius what relationships exist between the different components in your application. Without any supplemental information, a `uses` relationship tells Radius in what order to deploy the resources. With additional configuration, Radis can set environment variables, place secrets within secret stores, and add additional intelligence to your application.

Only runnable [components]({{< ref components >}}) can define relationships with `uses`.

### Binding configuration

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| binding | y | The binding of the other component to depend on. | `kv.properties.bindings.default`

### Action configuration

[Components]({{< ref components >}}) may also have additional *actions* which can be set as part of the component definition. For example, in [container components]({{< ref container >}}) the `env` action can be used to configure the environment variables within a container from the values of a component on which it depends (*eg. injecting a database connection string into a container's environment*). Additionally, the `secrets` action can be used to inject credentials into a secret store (*eg. injecting a database connection string into an Azure KeyVault*).

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| env | | List of key/value pairs which Radius will inject into the compute component runtime.  | `KV_URI: kv.properties.bindings.default.uri`
| secrets | | List of key/value pairs which Radius will inject into the secret store component. | `DBCONNECTION: db.properties.bindings.default.mongo.connectionString`

### Example

```sh
  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {...}
      uses: [
        {
          binding: kv.properties.bindings.default
          env: {
            KV_URI: kv.properties.bindings.default.uri
          }
        }
        {
          binding: db.properties.bindings.mongo
          secrets: {
            store: kv.properties.bindings.default
            keys: {
              DBCONNECTION: db.properties.bindings.mongo.connectionString
            }
          }
        }
      ]
    }
  }

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {...}
  }

  resource kv 'Components' = {
    name: 'kv'
    kind: 'azure.com/KeyVault@v1alpha1'
    properties: {...}
  }
}
```

## Next step

Now that you are familiar with Radius components, the next step is to learn about Radius bindings.

{{< button text="Learn about bindings" page="bindings-model.md" >}}
