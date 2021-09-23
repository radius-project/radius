---
type: docs
title: "Radius bindings"
linkTitle: "Bindings"
description: "Learn how to model your what your components offer with Radius bindings."
weight: 400
---

### Bindings

The Binding concept is used to describe the connections and relationships between components. In one line, Bindings are defined as:

{{% alert title="📄 Radius Binding" color="primary" %}}
A **logical** unit of communication between Components.
{{% /alert %}}

The Bindings of an Application define the *logical* relationships and patterns of communication between its Components. The use cases for Bindings are flexible, and include features like: 

- Configuring routes
- Accessing secret stores
- Generating connection strings
- Granting access to IAM permissions.

Each Binding is provided by a single Component, which may specify:

- **Kind**: what kind of Binding or protocol is this? *eg. HTTP*
- **Name**: the logical name of the Binding, must be unique per-Component
- **Essentials**: the configuration of the Binding *eg. the TCP port used for HTTP*

{{% alert title="💡 Key concept" color="info" %}}
A Binding might be provided *implicitily* as part of the Component's kind. *eg. A PostgreSQL Component will also provide a PostgreSQL Binding without manual annotations.* It is typical for compute Components (your code) to require manual annotation of Binding, and non-compute Components (databases, infrastructure) to provide them implicitly.
{{% /alert %}} 

#### Using a binding 

A Binding may be **depended-upon** by multiple Components (via `uses`), which may specify:

- **Name** - the logical name of the Binding and Component that is being used
- **Actions** - the ability to inject data into the dependent Component. eg. Set an environment variable to contain the connection string to a database (code example below)

#### Using computed values in a binding 

In addition to expressing a *logical* relationship, a Binding may provide access to *computed values* such as a URI or connection string. These values are used to support loose-coupling in application code. The consumer of a binding can specify how wire up a *computed value* to an application concern. 

For example, in the Radius application below, the `webapp` Component `uses` a Binding from the Component named `db`. For that Binding, the environment variable `DBCONNECTION` is being set to the `db` Component's MongoDB connection string. 

{{< rad file="snippets/full-app.bicep" embed=true replace-key-run="//RUN" replace-value-run="run: {...}" replace-key-bindings="//BINDINGS" replace-value-bindings="bindings: {...}"  >}}

{{% alert title="💡 Key concept" color="info" %}}
A Binding dependency between Components *may* affect the deployment order of Components depending on the kind of binding. eg. HTTP communication between components *may* be bi-directional, so it does not affect deployment order.
{{% /alert %}} 

#### Binding kind

Bindings have a **Kind** that is protocol-oriented so that Components are minimally-coupled. Documenting protocols provides flexiblity when deploying to multiple environments.

For example, in development you might use MongoDB in a container for your database, but in production you might use Azure CosmosDB's MongoDB support instead. You can swap out a single Component definition (the database component) to make this change since all of your application code Components express a dependency on the MongoDB protocol, rather than a specific implementation.

This flexibility is demonstrated in the above code example where the `webapp` Component `uses` the `db` Component's *MongoDB* binding: `db.properties.bindings.mongo`. 

## Old

Radius components offer **Bindings**, which are logical units of communication between [Components]({{< ref components-model.md >}}), such as:

- API interfaces
- Secret store access
- Connection strings

## Providing bindings

A component may provide one or more bindings to other runnable (*compute*) components. They can be defined:

- Within the component definition implementation, where the Component offers the binding without any configuration that is "always on"
- Within the app model declaration, where the Component offers the binding once a user adds it to the configuration and "defines" it

### Implicit bindings

Some Radius components provide bindings without any configuration. These bindings can be considered "always on" - the user doesn't need to explicitly define the binding as part of their Component.

For example: the [`azure.com/CosmosDBMongo`]({{< ref cosmos-mongodb.md >}}) Component offers  the `mongo` Binding of type `azure.com/CosmosDBMongo`:

{{< rad file="snippets/providing.bicep" embed=true marker="//COSMOS" >}}

You can learn about what default bindings are provided inside the respective [component docs]({{< ref components-model >}}).

### User-defined bindings

The `properties.bindings` section can define additional bindings which your [Component]({{< ref components-model.md >}}) offers. These bindings can range from HTTP ports being opened on a container to an API that a database resource offers.

For example, the [`radius.dev/container`]({{< ref container >}}) component can have an "http" binding added to it by definining the "http" binding within the `bindings` section:

{{< rad file="snippets/providing.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}

You can learn about what defined bindings are provided inside the respective [component docs]({{< ref components-model >}}).

## Consumiung bindings

The `properties.uses` section contains references to [bindings]({{< ref bindings-model.md >}}) which your Component consumes.

Without any supplemental information, a `uses` relationship tells Radius about a logical dependency between components. With additional configuration, Radius can use [actions](#actions) to do things like set environment variables, place secrets within secret stores, and add additional intelligence to your application.

Only runnable [components]({{< ref components-model >}}) (e.g. containers) can consume bindings with `uses`:

{{< rad file="snippets/consuming.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}

## Actions

Bindings can have actions associated with them, which configure the underlying components with data or metadata from the binding. Actions are defined with the runnable component (*e.g. container*) which will be using the other non-runnable components (*e.g. databases, secret stores*).

For example, you can take a uri provided by a Key Vault binding and pass it in to a container's environment via the `env` action. Then you can place a database connection string and store it in the Key Vault via the `secrets` action:

{{< rad file="snippets/full.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}

You can learn about what actions are provided inside the respective [component docs]({{< ref components-model >}}).

## Next step

Now that you are familiar with Radius bindings, the next step is to learn about Radius traits.

{{< button text="Learn about Traits" page="traits-model.md" >}}
