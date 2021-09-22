---
type: docs
title: "Add Dapr sidecars and a Dapr statestore to the app"
linkTitle: "Add Dapr trait"
slug: "add-dapr"
description: "How to enable Dapr sidecars and connect a Dapr state store to the tutorial application"
weight: 3000
---

Currently, the data you send to `backend` will be stored in memory inside the application. If the website restarts then all of your data will be lost!

In this step you will learn how to add a database and connect to it from the application.

## Add a Dapr trait

A [`dapr.io/App` trait]({{< ref "container.md#dapr-sidecar" >}}) on the `backend` component can be used to describe the Dapr configuration:

{{< rad file="snippets/trait.bicep" embed=true marker="//SAMPLE" replace-key-run="//RUN" replace-value-run="run: {...}" >}}

The `traits` section is used to configure cross-cutting behaviors of components. Since Dapr is not part of the standard definition of a container, it can be added via a trait. Traits have a `kind` so that they can be strongly typed. In this case we're providing some required Dapr configuration: the `app-id` and `app-port`.

## Add a Dapr Invoke binding

Add a [`radius.dev/invoke` binding]({{< ref "container.md#dapr-invoke" >}}) on the `backend` component to declare that you intend to accept service invocation requests on this component.

{{< rad file="snippets/invoke.bicep" embed=true marker="//SAMPLE" replace-key-run="//RUN" replace-value-run="run: {...}" replace-key-bindings="//BINDINGS" replace-value-bindings="bindings: {...}" replace-key-traits="//TRAITS" replace-value-traits="traits: [...]" >}}

## Add statestore component

Now that the backend is configured with Dapr, we need to define a state store to save information about orders.

A [`statestore` component]({{< ref dapr-statestore >}}) is used to specify a few properties about the state store:

- `kind: 'dapr.io/StateStore@v1alpha1'` represents a resource that Dapr uses to communicate with a database.
- `properties.config.kind: 'state.azure.tablestorage'` corresponds to the kind of Dapr state store used for [Azure Table Storage](https://docs.dapr.io/operations/components/setup-state-store/supported-state-stores/setup-azure-tablestorage/)
- `properties.config.managed: true` tells Radius to manage the lifetime of the component for you. 

{{< rad file="snippets/app.bicep" embed=true marker="//STATESTORE" >}}

{{% alert title="💡 Resource lifecycle and configuration" color="primary" %}}
With this simple component definition, Radius handles both creation of the Azure Storage resource itself and configuration of Dapr details like connection strings, simplifying the developer workflow.
{{% /alert %}}

## Reference statestore from backend

Radius captures both logical relationships and related operational details. Examples of this include: wiring up connection strings, granting permissions, or restarting components when a dependency changes.

The [`uses` section]({{< ref "bindings-model.md#consumiung-bindings" >}}) is used to configure relationships between a component and bindings provided by other components.

Once the state store is defined as a component, you can connect to it by referencing the `statestore` component from within the `backend` component via a `uses` section. This declares the *intention* from the `backend` component to communicate with the `statestore` component using `dapr.io/StateStore` as the protocol.

{{% alert title="💡 Implicit Bindings" color="primary" %}}
The [`statestore` component]({{< ref dapr-statestore.md >}}) implicitly declares a built-in binding named `default` of type `dapr.io/StateStore`. In general, components that define infrastructure and data-stores will come with [built-in bindings]({{< ref "bindings-model.md#implicit-bindings" >}}) as part of their type declaration. In this example, a Dapr state store component can be used as a state store without extra configuration.
{{% /alert %}}

{{< rad file="snippets/app.bicep" embed=true marker="//SAMPLE" replace-key-run="//RUN" replace-value-run="run: {...}" replace-key-bindings="//BINDINGS" replace-value-bindings="bindings: {...}" replace-key-statestore="//STATESTORE" replace-value-statestore="resource statestore 'Components' = {...}" replace-key-traits="//TRAITS" replace-value-traits="traits: [...]" >}}

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
   rad deployment list --application dapr-tutorial
   ```

   You should see both `backend` and `statestore` components in your `dapr-tutorial` application. Example output:

   ```
   DEPLOYMENT  COMPONENTS
   default     backend statestore
   ```

1. To test out the state store, open a local tunnel on port 3000 again:

   ```sh
   rad component expose backend --application dapr-tutorial --port 3000
   ```

1. Visit the the URL [http://localhost:3000/order](http://localhost:3000/order) in your browser. You should see the following message:

   ```
   {"message":"no orders yet"}
   ```

   If your message matches, then the container is able to communicate with the state store.

1. Press CTRL+C to terminate the port-forward.

<br>{{< button text="Next: Add an frontend component to the app" page="dapr-microservices-add-ui.md" >}}
