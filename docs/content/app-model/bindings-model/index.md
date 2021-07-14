---
type: docs
title: "Radius bindings"
linkTitle: "Bindings"
description: "Learn how to model your what your components offer with Radius bindings."
weight: 400
---

Radius components offer **Bindings**, which are logical units of communication between [Components]({{< ref components-model.md >}}), such as:

- API interfaces
- Secret store access
- Connection strings

## Providing bindings

A component may provide one or more bindings to other runnable (*compute*) components. They can be defined:

- Within the component definition implementation, where the Component offers the binding without any configuration that is "always on"
- Within the app model declaration, where the Component offers the binding once a user adds it to the configuration and "defines" it

### "Always on" bindings

Some Radius components provide bindings without any configuration. These bindings can be considered "always on" - the user doesn't need to explicitly define the binding as part of their Component.

For example: the [`azure.com/CosmosDBMongo`]({{< ref cosmos-mongodb.md >}}) Component offers  the `mongo` Binding of type `azure.com/CosmosDBMongo`:

{{< rad file="snippets/providing.bicep" embed=true marker="//COSMOS" >}}

You can learn about what default bindings are provided inside the respective [component docs]({{< ref components >}}).

### User-defined bindings

The `properties.bindings` section can define additional bindings which your [Component]({{< ref components-model.md >}}) offers. These bindings can range from HTTP ports being opened on a container to an API that a database resource offers.

For example, the [`radius.dev/container`]({{< ref container >}}) component can have an "http" binding added to it by definining the "http" binding within the `bindings` section:

{{< rad file="snippets/providing.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}

You can learn about what defined bindings are provided inside the respective [component docs]({{< ref components >}}).

## Consumiung bindings

The `properties.uses` section contains references to [bindings]({{< ref bindings-model.md >}}) which your Component consumes.

Without any supplemental information, a `uses` relationship tells Radius about a logical dependency between components. With additional configuration, Radius can use [actions](#actions) to do things like set environment variables, place secrets within secret stores, and add additional intelligence to your application.

Only runnable [components]({{< ref components >}}) (e.g. containers) can consume bindings with `uses`:

{{< rad file="snippets/consuming.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}

## Actions

Bindings can have actions associated with them, which configure the underlying components with data or metadata from the binding. Actions are defined with the runnable component (*e.g. container*) which will be using the other non-runnable components (*e.g. databases, secret stores*).

For example, you can take a uri provided by a Key Vault binding and pass it in to a container's environment via the `env` action. Then you can place a database connection string and store it in the Key Vault via the `secrets` action:

{{< rad file="snippets/full.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}

You can learn about what actions are provided inside the respective [component docs]({{< ref components >}}).

## Next step

Now that you are familiar with Radius bindings, the next step is to learn about Radius traits.

{{< button text="Learn about Traits" page="traits-model.md" >}}
