---
type: docs
title: "Radius bindings"
linkTitle: "Bindings"
description: "Learn how to model your what your components offer with Radius bindings."
weight: 400
---

## Providing bindings

Radius components offer **bindings**, which are defined sets of capabilities, information, and behavior. For example, an Azure CosmosDB offers a binding for its MongoDB API, and using the mongo binding tells Radius you will be using that API from your application.

A component may have one or more bindings which it provides. 

There are two types of bindings: 
- default bindings
- defined bindings 

### Default bindings

Some Radius components provide bindings by default. These bindings can be considered "always on" - the user doesn't need to explicitly define the binding as part of their component. 


**Example default binding**  

The [`azure.com/CosmosDocumentDb`]({{< ref azure-cosmos >}}) component offers both the `azure.com/CosmosDocumentDb` and `mongo.com/MongoDb` bindings without needing to provide any configuration. The following component has no user-defined bindings, but users are able to take advantage of its default bindings effortlessly.   

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

So, other components in the application are able consume the default bindings from that CosmosDocumentDb component with minimal configuration work:

```
  ... 
  uses: [
    {
      binding: db.properties.bindings.mongo
      env: {
        DBCONNECTION: db.properties.bindings.mongo.connectionString
      }
    }
  ]
  ...
```


You can learn about what default bindings are provided inside the respective [component docs]({{< ref components >}}).

### Defined bindings

In addition to the default bindings offered by some components, users may explicitly add bindings that they want a component to offer. 

**Example defined binding**  

The [`radius.dev/container`]({{< ref container >}}) component can have an "http" binding added to it by definining the "http" binding within the `bindings` section. Here, the binding is named `frontend`.

```sh
resource store 'Components' = {
    name: 'storefront'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
        run: {
            container: {
                image: 'radiusteam/storefront'
            }
        }
        bindings: {
            web: {
                kind: 'http'
                targetPort: 80
            }
            invoke: {
                kind: 'dapr.io/Invoke'
            }
        }
        uses: [
            {
                binding: inventory.properties.bindings.default
            }
        ]
        traits: [
            {
                kind: 'dapr.io/App@v1alpha1'
                appId: 'storefront'
                appPort: 80
            }
        ]
    }
}
```

These defined bindings can be referenced by other components similarly to the default bindings: 
```
...
uses: [
    {
        binding: store.properties.bindings.invoke
    }
]
...
```


You can learn about what defined bindings are provided inside the respective [component docs]({{< ref components >}}).

## Consuming bindings

Components can consume bindings from other components via the [`uses`]({{< ref "components-model.md#uses" >}}) configuration. Depending on the binding being offered, it might require additional configuration through parameters like `env` or `secrets`.

For more information on how to consume bindings from components, visit the [components docs]({{< ref components >}}).

## Next step

Now that you are familiar with Radius bindings, the next step is to learn about Radius traits.

{{< button text="Learn about traits" page="traits-model.md" >}}
