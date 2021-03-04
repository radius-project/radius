---
type: docs
title: "Project Radius concepts"
linkTitle: "Concepts"
weight: 30
---

# Project Radius concepts

Project Radius provides an *application mdodel* - a descriptive framework for cloud native applications and their requirements. This section is a conceptual guide for how the Radius model is structured, and explains at high level the concepts at work when you describe and deploy an application with Radius.

Radius uses the [Bicep langauge](https://github.com/azure/bicep) as its file-format and structure. Examples will be presented with both diagrams and using Bicep for clarity. Minimal knowledge of Bicep will be needed to understand the examples here.

## Deployable architecture diagram: the starting point

To start understanding Radius - think about how cloud-native applications are first designed. It's typical to create a *lines-and-boxes* architecture diagram as the starting point.

<TODO:> have a diagram here :)

An architecture diagram would include all of the pieces of the application both the components that run your code as well as other components like databases, messages queues, api gateways, secret stores, and more. These components make up the nodes of the diagram.

An architecture diagram would also include lines connecting the components of the application that indicate the flows of communication between components. These lines can be annotated with the essential information about these points of communication: 

- Protocols in use
- Settings like port numbers or hostnames
- The level of permissions needed
- ... and more 

These lines make up the edges of the diagram, they describe the relationships between components.

## Introducing Radius concepts

At this point you can understand two of the primary concepts in Radius: **Applications** and **Components**, as well as a secondary concept: **Services**.

After introducing these components, there is one more primary concept (**Deployments**) one more secondary concept (**Traits**) for you to learn.

Primary concepts in Radius are surfaced in the Bicep programming model as *resources*. Primary concepts can also be managed through the `rad` CLI.

Secondary concepts in Radius are part of your other definitions. They can be inspected and managed as part of those definitions.

### Applications

The **Application** contains everything on the diagram. Applications contain all of the compute, data, and infrastructure pieces of an application.

The Application is defined as: 

> The **biggest** possible circle you could draw around your software.

The Application concept in Radius is the boundary within which names have meaning. The other concepts in Radius define symbolic names - the Application is the scope in which these names must be unique, and in which they are referened.

> 💡 Since defining an Application only defines a boundary, deploying an empty Application has no real effect. Applications are only containers for your **Components**.

It's up to you, the user, to define what you consider part of the Application. It is recommended that you draw this circle very large to includes as much as possible of the software you work on. You should include the things that run your code (runnable components) and your data and infrastructure resources (non-runnable components).

> 💡 Applications are **not** units of deployment. An Application can contain multiple units of deployment that version separately. This topic will be explored later.

#### Example

<TODO:> put in an example based on the sample diagram

### Components

The **Components** describe each node on the diagram. Components can describe compute (your code), data, and infrastructure pieces of an application. Components are always part of an **Application** - the logical name defined by a component only has meaning within the context of an **Application**.

The Component is defined as:

> The description of behavior and requirements for a single unit of software.

The *Component* is documentation for a piece of code, data, or infrastructure. It can capture all of the important behaviors and requirements needed for a runtime to host that software.

#### Runnable Components

Often, components will describe a runnable unit of code like a container, or a web application. When a component describes your code it needs to conceptually document the following details:

- *Kind* - what kind of thing is this? eg. a container
- *Name* - the logical name of the Component, must be unique per-Application
- *Essentials* - how do I run this? eg. container image and tag
- *Dependencies* - what services do I need to access? (services will be described next)
- *Services* - what services do I provide for others? (services will be described next)
- *Traits* - what operational behaviors do I interact with? (traits will be described later)

These details can generally be separated into two categories:

- Details that are always true about the code (eg. *use DB_CONNECTION to pass SQL Server connection string*)
- Details that are per-deployment (eg. *accept traffic with the hostname myapp.example.com*)

> 💡 Behaviors and requirements that are per-deployment or otherwise separate from the code can live separately.

The Component concept in Radius is designed *to version with the application code*. For maximum flexiblity you should author Components that only capture the details that are always true about the code -- you have the option to configure per-deployment settings separately. 

Its up to your discretion as the user to decide which details of your software are per-deployment and which are always true. Radius will also not stop you from writing *all-in-one* definitions that capture *everything*. 

<TODO:> put in an example based on the sample diagram

#### non-Runnable Components

When a component defines a non-runnable unit of code: like a database or message queue, the same definitions apply, but generally more of the work is done for you.

- *Kind* - what kind of thing is this? eg. a PostgreSQL database
- *Name* - the logical name of the Component, must be unique per-Application
- *Essentials* - how do I configure this? eg. name of the database
- *Dependencies* - what services do I need to access? (services will be described next)
- *Services* - what services do I provide for others? (services will be described next)
- *Traits* - what operational behaviors do I interact with? (traits will be described later)

The difference between a runnable and non-runnable Component is that typically more will be done *for you*. It's easier to describe a PostgreSQL database than it is to describe a container, because the database has many standard behaviors.

#### Example

<TODO:> put in an example based on the sample diagram

#### Services

> 😱 we might end up finding a different name for this concept. I'm using Service as a placeholder for now, and it appears in the code, but the term might be too overloaded.

The **Service** concept is used to describe the connections and relationships between components.

Services are defined as:

> A **logical** unit of communication between Components.

The **Services** of an Application define the *logical* relationships and patterns of communication between its Components. The use cases for Services are flexible, and include features like: 

- Configuring routes
- Accessing secret stores
- Generating connection strings
- Granting access to IAM permissions.

Each Service is *provided* by a single Component, which may specify:

- *Kind* - what kind of Service or protocol is this? eg. HTTP
- *Name* - the logical name of the Service, must be unique per-Application
- *Essentials* - the configuration of the Service eg. the TCP port used for HTTP

> 💡 A Service might be provided *implicitily* as part of the Component's kind or as part of Component Trait (discussed later). eg. A PostgreSQL Component will also provide a PostgreSQL Service without manual annotations. It is typical for compute Components (your code) to require manual annotation of Services, and non-compute Components (databases, infrastructure) to provide them implicitly.

A Service my be *depended-upon* by multiple Components, which may specify:

- *Kind* - what kind of Service or protocol is this? eg. HTTP
- *Name* - the logical name of the Service, must be unique per-Application
- *Actions* - the ability to inject data into the dependent Component eg. Set an environment variable to contain the connection string to a database

In addition to expressing a *logical* relationship, a Service may provide access to *computed values* such as a URI or connection string. These values are used to support loose-coupling in application code. The consumer of a service can specify how wire up a *computed value* to an application concern (eg. set the `DB_CONNECTION` environment variable to the database's connection string).

> 💡 A Service dependency between Components *may* affect the deployment order of Components or it *may not* depending on the kind of service. eg. HTTP communication between components *may* be bi-directional, so it does not affect deployment order.

Services have a *Kind* that is protocol-oriented so that Components are minimally-coupled. Documenting protocols provides flexiblity when deploying to multiple environments:

> In development you can use MongoDB in a container as a database. In production you might use Azure CosmosDB's MongoDB support. You can swap out a single Component definition (the database component) to make this change since all of your application code Components express a dependency on the MongoDB protocol, rather than a specific implementation.

#### Example

<TODO:> put in an example based on the sample diagram

## Example in code

TODO: given an example application describe the above points.

- at least 3 components
- at least 2 services

## Growing up

For a production application you will face additional challenges that go above and beyond just describing the application functionally:

- You might need spread manifests across different code repositories
- You might need to configure per-deployment or per-environment behaviors
- You might need to control which things that are deployed together

Radius provides a primary concept called a **Deployment** to configure the groups of things that are deployed together. A Deployment is a group of components that are deployed as single unit.

Radius defines a secondary concept called a **Trait** to add additional flexibility to your Components. For example, a concern like the *number of replicas to create* is usually orthogonal to the requirements and intentions of the application code.

### Deployments

The **Deployment** specifies a list of Components that change together. You might need to specify Deployments because you are deploying an Application in multiple, separate units.

Deployments are defined as:

> A unit of deployment for a group of Components.

Most of the time you will not see a Deployment in code samples - the compiler has created one implicitly. If you need to, you can specify your own Deployments and have fine-grained control over:

- Which Components are deployed together
- Dependency ordering between Deployments

By default the compiler creates a *default* Deployment and includes all Components that are not part of an explicitly specified Deployment.

### Traits

> 😱 this has been one of the aspects of confusion for a few folks now. Traits are one of the areas where judgement is necessary. A lot of this is matters of opinion. At this point we haven't built *much*, and we've said a lot. I don't want to say too much until we build more.

A **Trait** is a piece of configuration attached to a Component that specifies an operational behavior. Traits can be confusing to understand at first because they serve a few different purposes for making Radius more flexible:

- Separation of concerns: removing operational concerns from the Component defintion (eg. number of replicas)
- Extensibility: expressing configuration that's not defined by the Component's type specification (eg. specifying Kubernetes labels)

Traits are defined as:

> A structured piece of orthogonal configuration that can applied to a Component as part of its definition or its deployment.

The keys to this definition are that traits:

- Are strongly-typed and can be validated
- Sometimes part of the Component's definition
- Sometimes part of the Component's deployment

To understand why and when Traits should be part of a Component definition, consider the following use cases.

#### Case Study: Manual Scaling

For an example, consider manual scaling for compute resources. The number of replicas desired for a component is usually a per-deployment decision - it is not a requirement or a characteristic of how the code was written.

Therefore it is desirable to move the declaration of *how many replicas* out of the Component definition, and into where the Component is deployed. This approach is much more flexible and organized, since the Component only contains deployment-agnostic details. The decision of *how many replicas* can be made by another person, or could live in another source code repository.

> 💡 This use of a manual scalar trait is an example of separation of concerns. The concern of *how many replicas* is separated from describing the intentions and requirements of the code.

Another benefit of traits is that for operational behaviors like the *number of replicas*, Radius provides a consistent vocabulary. The trait definition for manual scaling is the same across a variety of different resource types.

#### Case Study: Kubernetes Labels

For an example, consider a trait that applies [Kubernetes labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/) when a Component is deployed to a Kubernetes environment. For many organizations using Kubernetes in production, they require workloads to be tagged with labels according to an internal convention. This is useful for consistency and governance across the organization.

This could create a problem when using Radius on Kubernetes, because Kubernetes labels are not part of the definition of any type of Radius Component. For instance the *generic container primitive* (`radius.dev/Container@v1alpha1`) does not include Kubernetes concepts like labels.

To solve this, you could define a *Kubernetes label trait* that *extends* the definition of a container with additional data. This is desirable because the labels are additional data - the addition of labels does not *change the nature* of the Component - it is still a container.

> 💡 This use of a *Kubernetes Label trait* is an example of extensibility. The definition of a *generic container* can be extended to support additional features as long as they are additive and supported by the runtime environment.

Another benefit of using a trait like this is that you *also* benefit from separation of concerns. It seems likely that a *Kubernetes label trait* would be applied per-deployment rather than as part of the Component definition.

## Advanced example

TODO: given an example application describe the above points.

- at least 3 components
- at least 2 deployments
- at least 2 traits
- at least 2 services