---
type: docs
title: "Radius services"
linkTitle: "Services"
description: "Learn how to model your what your components offer with Radius services."
weight: 400
---

## Providing services

Radius components offer **services**, which are defined sets of functionality.

A component may have one or more services which it provides. These services can be *implicit* or *defined*.

### Implicit services

Implicit services are offered without defining any services which your service provides. For example, the [`azure.com/CosmosDocumentDb` component]({{< ref azure-cosmos >}}) offers both the `azure.com/CosmosDocumentDb` and `mongo.com/MongoDb` services without needing to provide any configuration.

You can learn about what implicit services are provided inside the respective [component docs]({{< ref components >}}).

### Explicit services

Explicit services are offered after a user defines them within a component. For example, the [`radius.dev/container`]({{< ref container >}}) component can have an 'http' service added to it by definining the 'http' service within the 'provides' section.

```sh
resource frontend 'Components' = {
  name: 'frontend'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    provides: [
      {
        name: 'frontend'
        kind: 'http'
        containerPort: 80
      }
    ]
  }
}
```

You can learn about what defined services are provided inside the respective [component docs]({{< ref components >}}).

## Consuming services

Components can consume services from other components via the [`dependsOn`]({{< ref "components-model.md#dependson" >}}) configuration. Depending on the service being offered, it might require additional configuration through parameters like `setEnv` or `setSecret`.

For more information on how to consume services from components, visit the [components docs]({{< ref components >}}).

## Next step

Now that you are familiar with Radius services, the next step is to learn about Radius traits.

{{< button text="Learn about traits" page="traits-model.md" >}}
