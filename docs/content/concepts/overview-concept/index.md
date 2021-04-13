---
type: docs
title: "Overview of Project Radius application model"
linkTitle: "Application model"
description: An overview of Project Radius application model and what it provides to the user
weight: 10
---

Project Radius provides an *application mdodel* - a descriptive framework for cloud native applications and their requirements. This section is a conceptual guide for how the Radius model is structured, and explains at high level the concepts at work when you describe and deploy an application with Radius.

Radius uses the [Bicep langauge](https://github.com/azure/bicep) as its file-format and structure. Examples will be presented with both diagrams and using Bicep for clarity. Minimal knowledge of Bicep will be needed to understand the examples here.

## Deployable architecture diagram: the starting point

To start understanding Radius - think about how cloud-native applications are first designed. It's typical to create a *lines-and-boxes* architecture diagram as the starting point.

{{< imgproc app-diagram Fit "700x500">}}
An simple example of an online shopping app has a collection of services and resources.
{{< /imgproc >}}

An architecture diagram would include all of the pieces of the application both the components that run your code as well as other components like databases, messages queues, api gateways, secret stores, and more. These components make up the nodes of the diagram.

An architecture diagram would also include lines connecting the components of the application that indicate the flows of communication between components. These lines can be annotated with the essential information about these points of communication: 

- Protocols in use
- Settings like port numbers or hostnames
- The level of permissions needed
- and more.....

These lines make up the edges of the diagram, they describe the relationships between components.

## Introducing Radius concepts

Radius contains two types of concepts: **primary** and **secondary**.

### Primary concepts

Primary concepts in Radius are surfaced in the Bicep programming model as *resources*, and can be managed through the `rad` CLI. They include:
- Applications
- Components
- Deployments

### Secondary concepts

Secondary concepts in Radius are part of your other definitions. They can be inspected and managed as part of those definitions. They include:
- Services
- Traits

## Developing with Radius

### Applications

The Radius Application contains everything on the diagram, including all the compute, data, and infrastructure. In one line, the Application is defined as: 

{{% alert title="📄 Radius Application" color="primary" %}}
The biggest possible circle you could draw around your software.
{{% /alert %}}

In Radius, the Application concept is the boundary within which names have meaning. The other concepts in Radius define symbolic names - the Application is the scope in which these names must be unique, and in which they are referenced.

{{% alert title="💡 Key concept" color="info" %}}
Since defining an Application only defines a boundary, deploying an empty Application has no real effect. Applications are only containers for your Components.
{{% /alert %}} 

It's up to you, the user, to define what you consider part of the Application. It is recommended that you draw this circle very large to includes as much as possible of the software you work on. You should include the things that run your code (*runnable components*), and your data and infrastructure resources (*non-runnable components*).

{{% alert title="💡 Key concept" color="info" %}}
Applications are **not** units of deployment. An Application can contain multiple units of deployment that version separately. This topic will be explored later.
{{% /alert %}} 

#### Example

{{< imgproc radius-application Fit "700x500">}}
A Radius Application encompases all the containers, databases, and APIs within a simple online store app.
{{< /imgproc >}}

The framework of a Radius application in Bicep would look like:

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'shopping-app'
}
```

### Components

Each node on the diagram maps to one **Component**. Components describe the code, data, and infrastructure pieces of an application. Components only have meaning within the context of an **Application**.

{{% alert title="📄 Radius Components" color="primary" %}}
The description of behavior and requirements for a single unit of software.
{{% /alert %}}

The Component is documentation for a piece of code, data, or infrastructure. It can capture all of the important behaviors and requirements needed for a runtime to host that software. An application can have both runnable components *(e.g. containers, web applications)* and non-runnable components *(e.g. databases, message queues)*. 

#### Runnable Components

Often, components will describe a runnable unit of code like a container, or a web application. When a component describes your code it needs to conceptually document the following details:

- **Kind**: what kind of thing is this? *eg. a container*
- **Name**: the logical name of the Component, must be unique per-Application
- **Essentials**: how do I run this? *eg. container image and tag*
- **Dependencies**: what services do I need to access? (services will be described later)
- **Services**: what services do I provide for others?
- **Traits**: what operational behaviors do I interact with? (traits will be described later)

These details can generally be separated into two categories:

- Details that are **always true** about the code *(eg. use `DB_CONNECTION` to pass SQL Server connection string)*
- Details that are **per-deployment** *(eg. accept traffic with the hostname `myapp.example.com`)*

{{% alert title="💡 Key concept" color="info" %}}
Behaviors and requirements that are per-deployment or otherwise separate from the code can live separately.
{{% /alert %}} 

The Component concept in Radius is designed *to version with the application code*. For maximum flexibility, you should author Components that only capture the details that are always true about the code. Per-deployment settings can be configured separately as part of Deployment definitions.

Its up to your discretion as the user to decide which details of your software are per-deployment and which are always true. Radius will also not stop you from writing *all-in-one* definitions that capture *everything*. 

#### Example

Within the shopping app example, each node is a Radius Component. Taking a look specifically at the storefront container, it would be modeled as:

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
    name: 'shopping-app'

    resource store 'Components' = {
        name: 'storefront'
        kind: 'radius.dev/Container@v1alpha1'
        properties: {
            run: {
                container: {
                    image: 'radiusteam/storefront'
                }
            }
        }
    }

    ...
}
```

#### Non-Runnable Components

When a component defines a non-runnable unit of code: like a database or message queue, the same definitions apply, but generally more of the work is done for you.

- **Kind**: what kind of thing is this? *eg. a PostgreSQL database*
- **Name**: the logical name of the Component, must be unique per-Application
- **Essentials**: how do I configure this? *eg. name of the database*
- **Dependencies**:  what services do I need to access? (services will be described later)
- **Services**: what services do I provide for others?
- **Traits**: what operational behaviors do I interact with? (traits will be described later)

The difference between a runnable and non-runnable Component is that typically more will be done *for you* in a non-runnable component. It's easier to describe a PostgreSQL database than it is to describe a container, because the database has many standard behaviors.

#### Example

An example of a non-runnable Radius Component is the inventory database, modeled as:

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
    name: 'shopping-app'
    
    ...

    resource inventory 'Components' = {
        name: 'inventory-store'
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

### Services

The Service concept is used to describe the connections and relationships between components. In one line, Services are defined as:

{{% alert title="📄 Radius Service" color="primary" %}}
A **logical** unit of communication between Components.
{{% /alert %}}

{{% alert title="😱 Work in progress" color="warning" %}}
We might end up finding a different name for this concept. I'm using Service as a placeholder for now, and it appears in the code, but the term might be too overloaded.
{{% /alert %}}

The Services of an Application define the *logical* relationships and patterns of communication between its Components. The use cases for Services are flexible, and include features like: 

- Configuring routes
- Accessing secret stores
- Generating connection strings
- Granting access to IAM permissions.

Each Service is provided by a single Component, which may specify:

- **Kind**: what kind of Service or protocol is this? *eg. HTTP*
- **Name**: the logical name of the Service, must be unique per-Application
- **Essentials**: the configuration of the Service *eg. the TCP port used for HTTP*

{{% alert title="💡 Key concept" color="info" %}}
A Service might be provided *implicitily* as part of the Component's kind or as part of Component Trait (discussed later). *eg. A PostgreSQL Component will also provide a PostgreSQL Service without manual annotations.* It is typical for compute Components (your code) to require manual annotation of Services, and non-compute Components (databases, infrastructure) to provide them implicitly.
{{% /alert %}} 

#### Dependencies

A Service may be **depended-upon** by multiple Components, which may specify:

- **Kind** - what kind of Service or protocol is this? eg. HTTP
- **Name** - the logical name of the Service, must be unique per-Application
- **Actions** - the ability to inject data into the dependent Component eg. Set an environment variable to contain the connection string to a database

#### Computed values

In addition to expressing a *logical* relationship, a Service may provide access to *computed values* such as a URI or connection string. These values are used to support loose-coupling in application code. The consumer of a service can specify how wire up a *computed value* to an application concern. *eg. set the `DB_CONNECTION` environment variable to the database's connection string*

{{% alert title="💡 Key concept" color="info" %}}
A Service dependency between Components *may* affect the deployment order of Components or it *may not* depending on the kind of service. eg. HTTP communication between components *may* be bi-directional, so it does not affect deployment order.
{{% /alert %}} 

#### Kind

Services have a **Kind** that is protocol-oriented so that Components are minimally-coupled. Documenting protocols provides flexiblity when deploying to multiple environments.

For example, in development you can use MongoDB in a container as a database. In production you might use Azure CosmosDB's MongoDB support. You can swap out a single Component definition (the database component) to make this change since all of your application code Components express a dependency on the MongoDB protocol, rather than a specific implementation.

### Example

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
    name: 'shopping-app'

    resource store 'Components' = {
        name: 'storefront'
        kind: 'radius.dev/Container@v1alpha1'
        properties: {
            run: {
                container: {
                    image: 'radiusteam/storefront'
                }
            }
            dependsOn: [
                {
                    kind: 'dapr.io/StateStore'
                    name: 'inventory-store'
                }
            ]
            provides: [
                {
                    kind: 'http'
                    name: 'web'
                    containerPort: 80
                }
            ]
        }
    }

    resource inventory 'Components' = {
        name: 'inventory-store'
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

## Moving to production

For a production application you will face additional challenges that go above and beyond just describing the application functionally:

- You might need spread manifests across different code repositories
- You might need to configure per-deployment or per-environment behaviors
- You might need to control which things that are deployed together

Radius provides a primary concept called a **Deployment** to configure the groups of things that are deployed together.

Radius defines a secondary concept called a **Trait** to add additional flexibility to your Components. For example, a concern like the *number of replicas to create* is usually orthogonal to the requirements and intentions of the application code.

### Deployments

The **Deployment** specifies a list of Components that change together. You might need to specify Deployments because you are deploying an Application in multiple, separate units. They are defined as:

{{% alert title="📄 Radius Deployment" color="primary" %}}
A unit of deployment for a group of Components.
{{% /alert %}} 

Most of the time you will not see a Deployment in code samples - the compiler has created one implicitly. If you need to, you can specify your own Deployments and have fine-grained control over:

- Which Components are deployed together
- Dependency ordering between Deployments

By default, the compiler creates a *default* Deployment and includes all Components that are not part of an explicitly specified Deployment.

### Traits

{{% alert title="😱 Work in progress" color="warning" %}}
This has been one of the aspects of confusion for a few folks now. Traits are one of the areas where judgement is necessary. A lot of this is matters of opinion. At this point we haven't built *much*, and we've said a lot. I don't want to say too much until we build more.
{{% /alert %}}

A **Trait** is a piece of configuration that specifies an operational behavior. Once defined, a trait can be added to Component or Deployment definitions. Traits serve a few purposes:

- Separation of concerns: removing operational concerns from the Component defintion *(eg. number of replicas)*
- Extensibility: expressing configuration that's not defined by the Component's type specification *(eg. specifying Kubernetes labels)*

Traits are defined as:

{{% alert title="📄 Radius Trait" color="primary" %}}
A structured piece of orthogonal configuration that can applied to a Component as part of its definition or a Deployment definition.
{{% /alert %}}

The keys to this definition are that traits:

- Are strongly-typed and can be validated
- Sometimes part of the Component's definition
- Sometimes part of the Deployment's definition

To understand why and when Traits should be part of a Component definition, consider the following use cases.

#### Case study: Manual scaling

For an example, consider manual scaling for compute resources. The number of replicas desired for a component is usually a per-deployment decision - it is not a requirement or a characteristic of how the code was written.

Therefore it is desirable to move the declaration of *how many replicas* out of the Component definition, and into the Deployment definition associated with the Component. This approach is much more flexible and organized, since the Component only contains deployment-agnostic details. The decision of *how many replicas* can be made by another person, or could live in another source code repository.

{{% alert title="💡 Key concept" color="info" %}}
This use of a manual scalar trait is an example of separation of concerns. The concern of *how many replicas* is separated from describing the intentions and requirements of the code.
{{% /alert %}} 

Another benefit of traits is that for operational behaviors like the *number of replicas*, Radius provides a consistent vocabulary. The trait definition for manual scaling is the same across a variety of different resource types.

#### Case study: Kubernetes Labels

For an example, consider a trait that applies [Kubernetes labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/) when a Component is deployed to a Kubernetes environment. For many organizations using Kubernetes in production, they require workloads to be tagged with labels according to an internal convention. This is useful for consistency and governance across the organization.

This could create a problem when using Radius on Kubernetes, because Kubernetes labels are not part of the definition of any type of Radius Component. For instance the *generic container primitive* (`radius.dev/Container@v1alpha1`) does not include Kubernetes concepts like labels.

To solve this, you could define a *Kubernetes label trait* that *extends* the definition of a container with additional data. This is desirable because the labels are additional data - the addition of labels does not *change the nature* of the Component - it is still a container.

{{% alert title="💡 Key concept" color="info" %}}
This use of a *Kubernetes Label trait* is an example of extensibility. The definition of a *generic container* can be extended to support additional features as long as they are additive and supported by the runtime environment.
{{% /alert %}} 

Another benefit of using a trait like this is that you *also* benefit from separation of concerns. It seems likely that a *Kubernetes label trait* would be applied per-deployment rather than as part of the Component definition.

### Example

The above application updated to include traits would look like:

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
    name: 'shopping-app'

    resource store 'Components' = {
        name: 'storefront'
        kind: 'radius.dev/Container@v1alpha1'
        properties: {
            run: {
                container: {
                    image: 'radiusteam/storefront'
                }
            }
            dependsOn: [
                {
                    kind: 'dapr.io/StateStore'
                    name: 'inventory-store'
                }
            ]
            provides: [
                {
                    kind: 'http'
                    name: 'web'
                    containerPort: 80
                }
            ]
            traits: [
                {
                    kind: 'dapr.io/App@v1alpha1'
                    properties: {
                        appId: 'storefront'
                        appPort: 80
                    }
                }
            ]
        }
    }

    resource cart 'Components' = {
        name: 'cart-api'
        kind: 'radius.dev/Container@v1alpha1'
            properties: {
                run: {
                    container: {
                        image: 'radiusteam/cart-api'
                    }
                }
                dependsOn: [
                    {
                        kind: 'dapr.io/Invoke'
                        name: 'storefront'
                    }
                ]
                traits: [
                    {
                        kind: 'dapr.io/App@v1alpha1'
                        properties: {
                            appId: 'cart-api'
                        }
                    }
                ]
            }
        }
    }

    resource inventory 'Components' = {
        name: 'inventory-store'
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
