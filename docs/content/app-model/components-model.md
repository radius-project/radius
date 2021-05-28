---
type: docs
title: "Radius components"
linkTitle: "Components"
description: "Learn how to model your application pieces with Radius components."
weight: 200
---

Components describe the code, data, and infrastructure pieces of an application. Components only have meaning within the context of an application.

The component is documentation for a piece of code, data, or infrastructure. It can capture all of the important behaviors and requirements needed for a runtime to host that software. An application can have both runnable components (*e.g. containers, web applications*) and non-runnable components (*e.g. databases, message queues*).

## Configuration

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your component. Used for defining relationships and getting status for your components. | `frontend`
| properties.dependsOn | | Other components which your component depends on for services and/or data. Learn more [below](#dependson). | [See below](#dependsOn)
| properties.provides | | [Services]({{< ref services-model.md >}}) which the component offers to other components or users. | [See below](#provides).

Different [component types]({{< ref components >}}) may also have additional properties and configuration which can be set as part of the component definition.

## provides

The `provides` configuration defines [services]({{< ref services-model.md >}}) which the component offers. These services can range from HTTP ports being opened on a container to an API that a database resource offers.

### Global provides configuration

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The type of service your component provides. | `http`
| name | y | The name of the service which you provide. | `web`

Different [service types]({{< ref services-model.md >}}) may also have additional properties and configuration which can be set as part of the component service definition.

### Example

In the following example a container offers an HTTP service on port 3000:

```sh
resource store 'Components' = {
  name: 'storefront'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {...}
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

## dependsOn

The `dependsOn` property tells Radius what relationships exist between the different components in your application. Without any supplemental information, a `dependsOn` relationship tells Radius in what order to deploy the resources. With additional configuration, Radis can set environment variables, place secrets within secret stores, and add additional intelligence to your application.

### Global dependsOn configuration

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of the other component to depend on. | `kv.name`
| kind | y | The service on which you depend. Can be the same as the component kind, or an abstract service kind. | `mongodb.com/Mongo`

### Specific dependsOn configuration

Different [component types]({{< ref components >}}) may also have additional `dependsOn` configuration which can be set as part of the component definition. For example, in [container components]({{< ref container >}}) the `setEnv` configuration can be used to configure the environment variables within a container from the values of a component on which it depends (*eg. injecting a database connection string into a container's environment*). Additionally, the `setSecret` configuration can be used to inject credentials into a secret store (*eg. injecting a database connection string into an Azure KeyVault*).

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| setEnv | | List of key/value pairs which Radius will inject into the compute component runtime.  | `KV_URI: 'keyvaulturi'`
| setSecret | | List of key/value pairs which Radius will inject into the secret store component. | `DBCONNECTION: 'connectionString'`

### Example

```sh
  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {...}
      dependsOn: [
        {
          name: 'kv'
          kind: 'azure.com/KeyVault'
          setEnv: {
            KV_URI: 'keyvaulturi'
          }
        }
        {
          kind: 'mongodb.com/Mongo'
          name: 'db'
          setSecret: {
            store: kv.name
            keys: {
              DBCONNECTION: 'connectionString'
            }
          }
        }
      ]
    }
  }

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDocumentDb@v1alpha1'
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

Now that you are familiar with Radius components, the next step is to learn about Radius services.

{{< button text="Learn about services" page="services-model.md" >}}