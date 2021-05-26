---
type: docs
title: "Project Radius components"
linkTitle: "Components"
description: "Available components in Project Radius with examples"
weight: 30
---

Radius offers components, which are pluggable resources for Radius to deploy and orchestrate.

For a deeper dive into what Radius is doing and how Radius applications work, check out our [concepts]({{< ref concepts >}}) and in-depth [tutorials]({{< ref tutorial >}})

## Services

Radius components offer **services**, which are defined sets of functionality. A component may have one or more services which it provides.

For example, `azure.com/CosmosDocumentDb` offers the services:
- `azure.com/CosmosDocumentDb`: Users can interact with this component through the resource's dedicated APIs and SDK.
- `mongo.com/MongoDb`: Users can interact with this component through the Mongo API.

## Dependencies

Components can `dependOn` other components and services, indicating that there is a relationship between the two. Dependencies can indicate API calls from one component to another, connections to databases, and other configurations where one resource reads to or writes from another.

Within a `dependsOn` dependency, components can configure settings like environment variables or secret store values. These configuration parameters live with the component which will be using the variables and values.

For example, a container may need to access the connection string for a MongoDB databse from an Azure KeyVault. For this to work, Radius needs to deploy the database first, then place the connection string in the Key Vault, then configure the container with the correct access permissions, and finally add an environment variable with the Key Vault URL. In Radius, this looks like:

```sh
resource fe 'Components' = {
  name: 'frontend'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    dependsOn: [
      {
        kind: 'azure.com/KeyVault'
        name: kv.name
        setEnv: {
          KB_URI: kv.uri
        }
      }
      {
        kind: 'mongodb.com/Mongo'
        name: db.name
        setSecret: {
          store: kv.name
          keys: {
            DBCONNECTION: db.connectionString
          }
        }
      }
    ]
  }
}

resource db 'Components' = {
  name: 'database'
  kind: 'azure.com/CosmosDocumentDb@v1alpha1'
  properties: {...}
}

resource kv 'Components' = {
  name: 'keyvault'
  kind: 'azure.com/KeyVault@v1alpha1'
  properties: {...}
}

```

## Radius components