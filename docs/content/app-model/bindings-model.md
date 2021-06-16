---
type: docs
title: "Radius bindings"
linkTitle: "Bindings"
description: "Learn how to model your what your components offer with Radius bindings."
weight: 400
---

Radius components offer **bindings**, which are logical units of communication between [Components]({{< ref components-model.md >}}), such as:

- API interfaces
- Secret store access
- Connection strings

## Consumiung bindings

The `properties.uses` configuration contains references to [bindings]({{< ref bindings-model.md >}}) which your Component consumes.

Without any supplemental information, a `uses` relationship tells Radius in what order to deploy the resources. With additional configuration, Radius can use [actions](#actions) to do things like set environment variables, place secrets within secret stores, and add additional intelligence to your application.

Only runnable [components]({{< ref components >}}) (e.g. containers) can consume bindings with `uses`.

## Providing bindings

A component may have one or more bindings which it provides to other runnable (*compute*) components. They can be defined:

- Within the component definition implementation, where the Component offers the binding without any configuration that is "always on"
- Within the app model declaration, where the Component offers the binding once a user adds it to the configuration and "defines" it

### "Always on" bindings

Some Radius components provide bindings without any configuration. These bindings can be considered "always on" - the user doesn't need to explicitly define the binding as part of their Component.

#### Example

The [`azure.com/CosmosDBMongo`]({{< ref cosmos-mongodb.md >}}) component offers  the `azure.com/CosmosDBMongo` binding without needing to provide any configuration. The following component has no user-defined bindings, but users are able to take advantage of its "always on" binding effortlessly:

```sh
resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
```

Other components in the application are now able consume the `mongo` binding from that CosmosDBMongo component:

```sh
resource frontend 'Components' = {
  name: 'frontend'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    uses: [
      {
        binding: db.properties.bindings.mongo
        env: {
          DBCONNECTION: db.properties.bindings.mongo.connectionString
        }
      }
    ]
  }
}
```

You can learn about what default bindings are provided inside the respective [component docs]({{< ref components >}}).

### Defined bindings

The `properties.bindings` configuration defines additional bindings which your [Component]({{< ref components-model.md >}}) offers. These bindings can range from HTTP ports being opened on a container to an API that a database resource offers.

Different [binding types]({{< ref bindings-model.md >}}) may also have additional properties and configuration which can be set as part of the component binding definition.

#### Example

The [`radius.dev/container`]({{< ref container >}}) component can have an "http" binding added to it by definining the "http" binding within the `bindings` section:

```sh
resource store 'Components' = {
  name: 'storefront'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    bindings: {
      web: {
        kind: 'http'
        targetPort: 80
      }
    }
  }
}
```

This defined bindings can be referenced by other runnable components similarly to the default bindings:

```sh
resource cart 'Components' = {
  name: 'cartservice'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    uses: [
      {
        binding: store.properties.bindings.web
        env: {
          STORE_HOST: store.properties.bindings.web.host
          STORE_PORT: store.properties.bindings.web.port
        }
      }
    ]
  }
}    
```

You can learn about what defined bindings are provided inside the respective [component docs]({{< ref components >}}).

## Actions

Bindings can have actions associated with them, which configure the underlying components with data or metadata from the binding. Actions are defined with the runnable component (*e.g. container*) which will be using the other non-runnable components (*e.g. databases, secret stores*).

For example, you can take a uri provided by a Key Vault binding and pass it in to a container's environment via the `env` action. Then you can place a database connection string and store it in the Key Vault via the `secrets` action:

```sh
  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {...}
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

You can learn about what actions are provided inside the respective [component docs]({{< ref components >}}).

## Next step

Now that you are familiar with Radius bindings, the next step is to learn about Radius traits.

{{< button text="Learn about traits" page="traits-model.md" >}}
