---
type: docs
title: "Add Dapr sidecars and a Dapr statestore to the app"
linkTitle: "Add Dapr"
slug: "add-dapr"
description: "How to enable Dapr sidecars and connect a Dapr state store to the tutorial application"
weight: 3000
---

Currently, the data you send to `backend` will be stored in memory inside the application. If the website restarts then all of your data will be lost!

In this step you will learn how to add a database and connect to it from the application with Dapr.

## Add a Dapr trait

A [`dapr.io/Sidecar` trait]({{< ref dapr-trait >}}) on the `backend` component can be used to describe the Dapr configuration:

{{< rad file="snippets/trait.bicep" embed=true marker="//SAMPLE" replace-key-run="//RUN" replace-value-run="container: {...}" >}}

The `traits` section is used to configure cross-cutting behaviors of components. Since Dapr is not part of the standard definition of a container, it can be added via a trait. Traits have a `kind` so that they can be strongly typed.

## Add a Dapr Invoke Route

Here you are describing how the `backend` Component will provide the `DaprHttpRoute` for other Components to consume.

Add a [`dapr.io.DaprHttpRoute`]({{< ref dapr >}}) resource to the app, and specify that the `backend` Component will provide the Route as part of the `dapr` port.

{{< rad file="snippets/invoke.bicep" embed=true marker="//SAMPLE" replace-key-bindings="//BINDINGS" replace-value-bindings="bindings: {...}" >}}

## Add statestore component

Now that the backend is configured with Dapr, we need to define a state store to save information about orders.

A [`statestore` component]({{< ref dapr-statestore >}}) is used to specify a few properties about the state store:

- **resource type**: `'dapr.io/StateStoreComponent'` represents a resource that Dapr uses to communicate with a database.
- **kind**: `'any'` tells Radius to pick the best available statestore for the platform. For Azure this is Table Storage and for Kubernetes this is a Redis container.
- **managed**: `true` tells Radius to manage the lifetime of the component for you. 

{{< rad file="snippets/app.bicep" embed=true marker="//STATESTORE" >}}

{{% alert title="💡 Resource lifecycle and configuration" color="primary" %}}
With this simple component definition, Radius handles both creation of the Azure Storage resource itself and configuration of Dapr details like connection strings, simplifying the developer workflow.
{{% /alert %}}

## Reference statestore from backend

Radius captures both logical relationships and related operational details. Examples of this include: wiring up connection strings, granting permissions, or restarting components when a dependency changes.

The [`connections` section]({{< ref "connections-model" >}}) is used to configure relationships between a component and bindings provided by other components.

Once the state store is defined as a component, you can connect to it by referencing the `statestore` component from the `backend` component via the [`connections` section]({{< ref "connections-model" >}}). This declares the *intention* from the `backend` component to communicate with the `statestore` component using `dapr.io/StateStore` as the protocol.

{{< rad file="snippets/app.bicep" embed=true marker="//SAMPLE" replace-key-run="//RUN" replace-value-run="container: {...}" replace-key-bindings="//BINDINGS" replace-value-bindings="bindings: {...}" replace-key-statestore="//STATESTORE" replace-value-statestore="resource statestore 'dapr.io.StateStoreComponent' = {...}" replace-key-traits="//TRAITS" replace-value-traits="traits: [...]" >}}

## Deploy application with Dapr

{{% alert title="Make sure Dapr is initialized" color="warning" %}}
For Kubernetes environments, make sure to [initialize Dapr](https://docs.dapr.io/operations/hosting/kubernetes/kubernetes-deploy/) on your cluster so your application can leverage the Dapr control-plane and sidecar.

For Azure environments, Dapr is managed for you and you do not need to manually initialize it.
{{% /alert %}}

1. Make sure your `template.json` file matches the full tutorial file:

   {{< rad file="snippets/app.bicep" download=true >}}

1. Now you are ready to re-deploy the application, including the Dapr state store. Switch to the command-line and run:

   ```sh
   rad deploy template.bicep
   ```

   This may take a few minutes because of the time required to create the Storage Account.

1. You can confirm that the new `statestore` component was deployed by running:

   ```sh
   rad resource list --application dapr-tutorial
   ```

   You should see both `backend` and `statestore` components in your `dapr-tutorial` application. Example output:

   ```
   RESOURCE   KIND
   backend     ContainerComponent
   statestore  dapr.io.StateStoreComponent
   ```

1. To test out the state store, open a local tunnel on port 3000 again:

   ```sh
   rad resource expose backend --application dapr-tutorial --port 3000
   ```

1. Visit the the URL [http://localhost:3000/order](http://localhost:3000/order) in your browser. You should see the following message:

   ```
   {"message":"no orders yet"}
   ```

   If your message matches, then the container is able to communicate with the state store.

1. Press CTRL+C to terminate the port-forward.

<br>{{< button text="Next: Add an frontend component to the app" page="dapr-microservices-add-ui.md" >}}
