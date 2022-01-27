---
type: docs
title: "Radius traits"
linkTitle: "Traits"
description: "Learn how to model your application component behavior with Radius traits."
weight: 400
---

{{% alert title="🚧 Under Construction" color="info" %}}
Traits are still being designed and implemented by the Radius team. Stay tuned for additional traits and updates.
{{% /alert %}}

## Moving to production

For a production application you will face additional challenges that go above and beyond just describing the application functionally:

- You might need spread manifests across different code repositories
- You might need to configure per-deployment or per-environment behaviors

Radius defines a secondary concept called a **Trait** to add additional flexibility to your Components. For example, a concern like the *number of replicas to create* is usually orthogonal to the requirements and intentions of the application code.

## Definition

A **Trait** is a piece of configuration that specifies an operational behavior. Once defined, a trait can be added to Component definitions. Traits serve a few purposes:

- Separation of concerns: removing operational concerns from the Component defintion *(eg. number of replicas)*
- Extensibility: expressing configuration that's not defined by the Component's type specification *(eg. specifying Kubernetes labels)*

Traits are defined as:

{{% alert title="📄 Radius Trait" color="primary" %}}
A structured piece of orthogonal configuration that can applied to a Component as part of its definition or a Deployment definition.
{{% /alert %}}

The keys to this definition are that traits:

- Are strongly-typed and can be validated
- Are part of the Component's definition

## Case studies

### Manual scaling

For an example, consider manual scaling for compute resources. The number of replicas desired for a component is usually a per-deployment decision - it is not a requirement or a characteristic of how the code was written.

Therefore it is desirable to move the declaration of *how many replicas* out of the Component definition, and into the Deployment definition associated with the Component. This approach is much more flexible and organized, since the Component only contains deployment-agnostic details. The decision of *how many replicas* can be made by another person, or could live in another source code repository.

{{% alert title="💡 Key concept" color="info" %}}
This use of a manual scalar trait is an example of separation of concerns. The concern of *how many replicas* is separated from describing the intentions and requirements of the code.
{{% /alert %}} 

Another benefit of traits is that for operational behaviors like the *number of replicas*, Radius provides a consistent vocabulary. The trait definition for manual scaling is the same across a variety of different resource types.

### Kubernetes labels

For an example, consider a trait that applies [Kubernetes labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/) when a Component is deployed to a Kubernetes environment. For many organizations using Kubernetes in production, they require workloads to be tagged with labels according to an internal convention. This is useful for consistency and governance across the organization.

This could create a problem when using Radius on Kubernetes, because Kubernetes labels are not part of the definition of any type of Radius Component. For instance the *generic container primitive* (`Container`) does not include Kubernetes concepts like labels.

To solve this, you could define a *Kubernetes label trait* that *extends* the definition of a container with additional data. This is desirable because the labels are additional data - the addition of labels does not *change the nature* of the Component - it is still a container.

{{% alert title="💡 Key concept" color="info" %}}
This use of a *Kubernetes Label trait* is an example of extensibility. The definition of a *generic container* can be extended to support additional features as long as they are additive and supported by the runtime environment.
{{% /alert %}} 

Another benefit of using a trait like this is that you *also* benefit from separation of concerns. It seems likely that a *Kubernetes label trait* would be applied per-deployment rather than as part of the Component definition.

#### Bicep example

Here is a full example of a Radius application that uses multiple components with provided connections and traits.

{{< rad file="snippets/storeapp.bicep" embed=true >}}

## Next steps

Now that you understand the Radius app model, head over to the [environments concept page]({{< ref environments-concept >}}) to learn how Radius turns a Bicep file into a running application.

{{< button text="Learn about environments" page="environments-concept" >}}
