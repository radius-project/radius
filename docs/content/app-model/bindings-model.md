---
type: docs
title: "Radius bindings"
linkTitle: "Bindings"
description: "Learn how to model your what your components offer with Radius bindings."
weight: 400
---

## Providing bindings

Radius components offer **bindings**, which are defined sets of capabilities, information, and behavior. For example, an Azure CosmosDB offers a binding for its MongoDB API, and using the mongo binding tells Radius you will be using that API from your application.

A component may have one or more bindings which it provides. These bindings can be *default* or *defined*.

### Pre-built bindings

Some Radius components provide pre-built bindings by default without needing additional definition by users.

For example, the [`azure.com/CosmosDocumentDb`]({{< ref azure-cosmos >}}) component offers both the `azure.com/CosmosDocumentDb` and `mongo.com/MongoDb` bindings without needing to provide any configuration.

You can learn about what default bindings are provided inside the respective [component docs]({{< ref components >}}).

### User-defined bindings

In addition to the pre-built bindings, users can define their own bindings within a component. 

For example, the [`radius.dev/container`]({{< ref container >}}) component can have an "http" binding added to it by definining the "http" binding within the `bindings` section. In the example below, the binding is named `frontend`.

```sh
resource frontend 'Components' = {
  name: 'frontend'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    bindings: 
      frontend: {
        kind: 'http'
        targetPort: 3000
      }
    ]
  }
}
```

You can learn about what defined bindings are provided inside the respective [component docs]({{< ref components >}}).

## Consuming bindings

Components can consume bindings from other components via the [`uses`]({{< ref "components-model.md#uses" >}}) configuration. Depending on the binding being offered, it might require additional configuration through parameters like `env` or `secrets`.

For more information on how to consume bindings from components, visit the [components docs]({{< ref components >}}).

## Next step

Now that you are familiar with Radius bindings, the next step is to learn about Radius traits.

{{< button text="Learn about traits" page="traits-model.md" >}}
