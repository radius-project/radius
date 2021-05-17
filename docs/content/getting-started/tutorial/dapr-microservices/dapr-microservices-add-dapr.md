---
type: docs
title: "Add a Dapr statestore to the app"
linkTitle: "Add Dapr"
description: "Connect a Dapr state store to the tutorial application"
weight: 3000
---

At this point, you haven't added Dapr yet or configured the Azure Table Storage state store. Currently, the "todo" items you enter will be stored in memory inside the application. If the website restarts then all of your data will be lost!

In this step you will learn how to add a database and connect to it from the application.

We'll discuss template.bicep changes and then provide the full, updated file before deployment. 

## Add a Dapr trait to the nodeapp component
A *trait* on the `nodeapp` component can be used to describe the Dapr configuration:

```
  resource nodeapplication 'Components' = {
    name: 'nodeapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: { ... }
      provides: [ ... ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'nodeapp'
            appPort: 3000
          }
        }
      ]
    }
  }
```

The `traits` section is used to configure cross-cutting behaviors of components. Since Dapr is not part of the standard definition of a container, it can be added via a trait. Traits have a `kind` so that they can be strongly typed. In this case we're providing some required Dapr configuration: the `app-id` and `app-port`.

{{% alert title="💡 Traits" color="primary" %}}
The `traits` section is one of several top level sections in a *component*. Traits are used to configure the component in a cross-cutting way. Other examples would include handling public traffic (ingress) or scaling.
{{% /alert %}}

## Add statestore component

Now the nodeapp is hooked up to Dapr, but we still need to define a state store to save information about orders.

A `statestore` component is used to specify a few properties about the state store: 

- **kind:** `adapr.io/StateStore@v1alpha1` represents a resource that Dapr uses to communicate with a database.
  - **config > kind:** `state.azure.tablestorage` corresponds to the kind of Dapr state store used for [Azure Table Storage](https://docs.dapr.io/operations/components/setup-state-store/supported-state-stores/setup-azure-tablestorage/)
- **managed:** `true` tells Radius to manage the lifetime of the component for you. 

```
  resource statestore 'Components' = {
    name: 'statestore'
    kind: 'dapr.io/StateStore@v1alpha1'
    properties: {
      config: {
        kind: 'state.azure.tablestorage'
        managed: true
      }
    }
  }
```

Note that with this simple component definition, Radius handles both creation of the Azure Storage resource itself and configuration of Dapr details like connection strings, simplifying the developer workflow.   

## Reference statestore from nodeapp

Radius captures both logical relationships and related operational details. Examples of this include: wiring up connection strings, granting permissions, or restarting components when a dependency changes.

The `dependsOn` section is used to configure relationships between a component and services provided by other components. 

Once the state store is defined as a component, you can connect to it by referencing the `statestore` component from within the `nodeapp` component via a `dependsOn` section. This declares the *intention* from the `nodeapp` component to communicate with the `statestore` component using `dapr.io/StateStore` as the protocol.

```
  resource nodeapplication 'Components' = {
    name: 'nodeapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: { ... }
      dependsOn: [
        {
          kind: 'dapr.io/StateStore'
          name: 'statestore'
        }
      ]
      provides: [ ... ]
      traits: [ ... ]
    }
  }
```

## Update your template.bicep file 

**Update your `template.bicep` file to match the full application definition:** 

{{%expand "❗️ Expand for the full code block" %}}

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

  resource nodeapplication 'Components' = {
    name: 'nodeapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-nodeapp'
        }
      }
      dependsOn: [
        {
          kind: 'dapr.io/StateStore'
          name: 'statestore'
        }
      ]
      provides: [
        {
          kind: 'http'
          name: 'web'
          containerPort: 3000
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'nodeapp'
            appPort: 3000
          }
        }
      ]
    }
  }

  resource statestore 'Components' = {
    name: 'statestore'
    kind: 'dapr.io/StateStore@v1alpha1'
    properties: {
      config: {
        kind: 'state.azure.tablestorage'
        managed: true
      }
    }
  }
}
```
{{% /expand%}}  

## Deploy application with Dapr

1. Now you are ready to re-deploy the application, including the Dapr state store. Switch to the command-line and run: 

   ```sh
   rad deploy template.bicep
   ```

   This may take a few minutes because of the time required to create the Storage Account.


1. You can confirm that the new `statestore` component was deployed by running:

   ```sh
   rad deployment list --application-name dapr-hello
   ```

   You should see both `nodeapp` and `statestore` components in your `dapr-hello` application. Example output: 

   ```
   Using config file: /Users/{USER}/.rad/config.yaml
   {
     "value": [
       {
         "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/dapr-hello/Deployments/default",
         "name": "default",
         "type": "Microsoft.CustomProviders/resourceProviders/Applications/Deployments",
         "properties": {
           "components": [
             {
               "componentName": "nodeapp"
             },
             {
               "componentName": "statestore"
             }
           ]
         }
       }
     ]
   }
   ```

1. To test out the state store, open a local tunnel on port 3000 again:

   ```sh
   rad expose dapr-hello nodeapp --port 3000
   ```

1. Visit the the URL [http://localhost:3000](http://localhost:3000) in your browser. You should see the following message:

  
   `{"message":"no orders yet"}`

   If your message matches, then the container is able to communicate with the state store. 

1. Press CTRL+C to terminate the port-forward. 

<br>{{< button text="Next: Add an order generator component to the app" page="dapr-microservices-add-pythonapp.md" >}}
