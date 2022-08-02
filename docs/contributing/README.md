# Contributing to Project Radius

## Welcome!

Welcome to Project Radius. Whether you are a new person working on the Radius team or a third party contributor, this set of documentation should be useful for you!

## Architecture

Project Radius can be best understood as the combination of the following components:

- The **`rad` CLI** for deploying and managing environments and applications
- The **application model** for describing applications as a set of components, groupings, and relationships
- The **Bicep** language and tools which includes the application model as a set of types
- The **ARM Deployment Engine** which process the output of the Bicep compiler
- The **Universal Control Plane** which provides central routing for all services running in Radius
- The **Radius API** which interfaces with the ARM deployment engine and other actors to handle individual operations on the application model
- The **Radius Control Plane** that implements operations on the application model as a sequence of resource management operations on a hosting platform
- The **Hosting Platform** that provides the underlying capabilies used by the application model such as running containers and databases

The remainder of this document will explain these components and processes in detail. The intention is to provide enough detail to be useful to new contributors to the project as well as understanding for curious users that want to know about the inner workings of the system. As such, it is recommended that you read and understand the application model concepts before diving into architecture.

## Callouts

As Project Radius is still being developed, some of the information here is speculative, incomplete, or otherwise reflective of the *future state* of the project. 

Here are some examples of how this kind of information will be emphasized.

> 🚀 Future State 🚀 <br>
A description of how things will work in the future.

> 🚧 Under Construction 🚧 <br>
A description of a gap or limitation that we currently have.

>❓ Open Question ❓<br>
A description of something we don't quite know yet. 

## Current state

As an overview, here are some important details about the current state of Radius. At the time of writing we've finished the 0.12 release.

The 0.12 release is suitable for deploying our limited set of tutorial content on a Kubernetes cluster.

We have the goal to target and support multiple *hosting platforms* in the future:

- Kubernetes: currently the mainline experience
- Azure + AKS: once we start building the Azure Service, this will be the first scenario we support
- Azure + Serverless: this is the *future state* for Azure, we target Azure managed services and a TBD system for compute
- Local Dev: will build this after Kubernetes, using your dev box and likely Docker as a hosting platform. Currently implemented but not enabled for users.

## Current status

- The only supported **hosting platform** is Kubernetes, which supports deploying Azure resources, Kubernetes resources, and Radius resources.
  - Any type of Kubernetes cluster should be supported (AKS, KinD, EKS, etc.)
- The **Radius API** is implemented as an [Azure RP](https://armwiki.azurewebsites.net/api_contracts/Intro.html), allowing us to have a service that can run on Azure and Kubernetes.
- We require a custom build of **Bicep** which includes our types and some custom compiler support
- The **application model** has support for a very limited set of components and traits
- The **rad** CLI can be used to manage applications using ARM as the protocol by communicating with the Radius API
- The **rad** CLI can be used to perform diagnostic operations on applications by communicating with the Kubernetes Cluster

## rad CLI

The `rad` CLI provides features in three major areas:

- Environment and Control Plane setup and management
- Application deployment and management (includes CRUD operations on Radius concepts)
- Application diagnostics and troubleshooting

It can be hard to draw a clear line between *management* and *troubleshooting*, but I think the distinction is useful even through it is sometimes ambiguous. In general the CLI needs to interact both the management plane of the hosting platform (ARM in Azure) as well as the data plane of the hosting platform (logs, port-forwards).

### Control Plane setup

Control Plane setup is the process of binding an entry in a local config file to a context where applications can be deployed.

An Control Plane currently consists of:

- A Kubernetes Cluster
- (optionally) A resource group in Azure
- (optionally) A user's local Docker daemon

Conceptually, any hosting platform that Radius could target in the future is a Radius control plane.

### Workspaces

To track the context needed to connect to the hosting platform, Radius stores information in a local file called `config.yaml`.

MacOS/Linux: `~/.rad/config.yaml`
Windows: `C:\Users\<username>\.rad\config.yaml`

The config.yaml looks like:

```yaml
workspaces:
  default: my-cluster # Default workspace to use when invoking commands like rad application, rad env, etc. 
  items:
    my-cluster:
      connection: # Types of connections this workspace can make, today only supports Kubernetes
        context: my-cluster
        kind: kubernetes
      environment: /planes/radius/local/resourcegroups/my-cluster-rg/providers/applications.core/environments/my-cluster # The environment ID this workspace will use by default, more details later!
      scope: /planes/radius/local/resourceGroups/my-cluster-rg # The scope where Radius resources will be deployed to.
      providerConfig:
        azure: # Additional providers that enable differernt resources types to be deployed. If a subscription ID and resourcegroup are set and the cluster is configured to be able to authenticate with Azure, Azure resources can be deployed as well as Radius resources. 
          subscriptionid: "<some id>"
          resourcegroup: "<some resource group>"
```

The workspace file is again local, meaning that it only tracks client side state. Someone can delete resources on the server side to make this configuration invalid.

### Environments

See the [conceptual documentation on environments](https://edge.radapp.dev/concepts/environments-concept/) for a great explanation of environments.

Environments are created as part of calls to `rad env init`. `rad env init` will:

- Install Radius onto the control plane if not already installed
- Initialize a local workspace
- Create a server side environment resources where resources will be deployed to

### Application Model & Bicep

The application model represents a set of core concepts for deploying and managing cloud-native applications. The application model has extensive coverage via its own [conceptual documentation](https://edge.radapp.dev/concepts/appmodel-concept/). This section will focus on the technical details of the representation.

We use Bicep because it's an expressive and productive language for describing infrastructure and deployments. Using Bicep for Azure allows a transparent abstraction over the rest of Azure - Radius can reference ARM types and vice-versa. This also includes Kubernetes resources as well, where I can define a Kuberentes resource definition in Bicep and deploy it to a Kubernetes cluster.

ARM stands for [Azure Resource Manager](https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/overview) - it is the service that handles all deployments coming into Azure. [ARM templates](https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/overview) are a JSON format for defining resources that will be deployed to Azure, commonly called ARM.JSON . **Bicep compiles into ARM.JSON**, you can think of ARM.JSON as the intermediate language for Bicep (Like C to Assembly).

To enable Radius types to be defined outside of the context of Azure, we are using Bicep Extensibility. Bicep extensibility allows for defining custom types in the Bicep Compiler and also enables a server side component to handle how these custom resources should be deployed. 

For now, we have our own fork of Bicep here: https://github.com/project-radius/bicep. This is due to needing to keep Radius private for now as well as a few feature gaps that we are still designing with the ARM/Bicep Team.

Radius types are defined via [swagger files](https://github.com/project-radius/radius/tree/main/swagger/specification/applications/resource-manager). These swagger files are used by not only the Bicep compiler, but the Radius RP server side component as well. The swagger files are also required if/when we build an Azure service.

When someone updates these swagger files on a Pull Request (PR) to the Radius repo, we automatically create a PR to the Bicep repo. See [an example PR](https://github.com/project-radius/bicep/pull/473). 

Though we are using Bicep extensibility to define Radius types, we still want Radius types to be able to be deployed to Azure in the future. Therefore, even though we are using Bicep Extensibility to define types, they will look very similar to ARM like resources.

For example:

```bicep
import radius as radius // Required to allow defining Radius types in bicep 

import kubernetes as kubernetes {
  namespace: 'default'
  kubeConfig: ''
}

@description('Specifies the environment for resources.')
param environment string // 0.12 point in time implementation - Allows for auto injecting the environment ID into bicep to be used in resource definitions 

// Defines an Application resources
resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container'
  location: 'global' // Location is a required property for Radius resources. We are going to make this experience better in the future where this isn't hardcoded.
  properties: {
    environment: environment // Environment that the application will live in.
  }
}

// Defining a container that will be part of the Application
resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'ctnr-ctnr'
  location: 'global'
  properties: {
    application: app.id // Radius resources have "ids" on them as well.
    container: {
      // Container that will run on port 3000
      image: 'myimage:latest'
      ports: {
        web: {
          containerPort: 3000
        }
      }
    }
  }
}

// Defining a Kubernetes secret to be deployed to the Kubernetes Cluster.
resource secret 'core/Secret@v1' = {
  metadata: {
    name: 'my-secret'
    namespace: 'default'
    labels: {
      format: 'k8s-extension'
    }
  }

  stringData: {
    someSecret: 'SOME_VALUE'
  }
}


// Defining a Azure SQL server to deploy to Azure.
resource server 'Microsoft.Sql/servers@2021-02-01-preview' = {
  name: 'mssql'
  location: resourceGroup().location
  properties: {
    administratorLogin: 'Login'
    administratorLoginPassword: 'Password' // Note this isn't secure!
  }

  resource dbinner 'databases' = {
    name: 'cool-database'
    location: resourceGroup().location
  }

  resource firewall 'firewallRules' = {
    name: 'allow'
    properties: {
      startIpAddress: '0.0.0.0'
      endIpAddress: '0.0.0.0'
    }
  }
}

```

This would compile into the following ARM.JSON representation. Not great on the eyes but all the information expressed in Bicep is represented in this file as well. As you'll see in further steps, this ARM.JSON format is what eventually is operated on once a deployment kicks off.

```json
{
  "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
  "languageVersion": "1.9-experimental",
  "contentVersion": "1.0.0.0",
  "metadata": {
    "_generator": {
      "name": "bicep",
      "version": "0.7.16.48756",
      "templateHash": "14305173729977962484"
    }
  },
  "parameters": {
    "environment": {
      "type": "string",
      "metadata": {
        "description": "Specifies the environment for resources."
      }
    }
  },
  "imports": {
    "radius": {
      "provider": "Radius",
      "version": "1.0"
    },
    "kubernetes": {
      "provider": "Kubernetes",
      "version": "1.0",
      "config": {
        "namespace": "default",
        "kubeConfig": ""
      }
    }
  },
  "resources": {
    "server::dbinner": {
      "type": "Microsoft.Sql/servers/databases",
      "apiVersion": "2021-02-01-preview",
      "name": "[format('{0}/{1}', 'mssql', 'cool-database')]",
      "location": "[resourceGroup().location]",
      "dependsOn": [
        "server"
      ]
    },
    "server::firewall": {
      "type": "Microsoft.Sql/servers/firewallRules",
      "apiVersion": "2021-02-01-preview",
      "name": "[format('{0}/{1}', 'mssql', 'allow')]",
      "properties": {
        "startIpAddress": "0.0.0.0",
        "endIpAddress": "0.0.0.0"
      },
      "dependsOn": [
        "server"
      ]
    },
    "app": {
      "import": "radius",
      "type": "Applications.Core/applications@2022-03-15-privatepreview",
      "properties": {
        "name": "corerp-resources-container",
        "location": "global",
        "properties": {
          "environment": "[parameters('environment')]"
        }
      }
    },
    "container": {
      "import": "radius",
      "type": "Applications.Core/containers@2022-03-15-privatepreview",
      "properties": {
        "name": "ctnr-ctnr",
        "location": "global",
        "properties": {
          "application": "[reference('app').id]",
          "container": {
            "image": "myimage:latest",
            "ports": {
              "web": {
                "containerPort": 3000
              }
            }
          }
        }
      },
      "dependsOn": [
        "app"
      ]
    },
    "secret": {
      "import": "kubernetes",
      "type": "core/Secret@v1",
      "properties": {
        "metadata": {
          "name": "my-secret",
          "namespace": "default",
          "labels": {
            "format": "k8s-extension"
          }
        },
        "stringData": {
          "someSecret": "SOME_VALUE"
        }
      }
    },
    "server": {
      "type": "Microsoft.Sql/servers",
      "apiVersion": "2021-02-01-preview",
      "name": "mssql",
      "location": "[resourceGroup().location]",
      "properties": {
        "administratorLogin": "Login",
        "administratorLoginPassword": "Password"
      }
    }
  }
}
```

### Universal Control Plane

TODO

### Deployment Engine

The ARM Deployment Engine is responsible for translating an ARM JSON template into a set of operations on individual resources. The [Radius Self-Hosted Deployment Engine](https://github.com/project-radius/deployment-engine) is our own implementation of the Deployment Engine.


In Azure the ARM deployment engine is a centrally-hosted component that calls into other resource providers.


### Radius API

TODO 

### Radius control plane

TODO

#### Computing desired state (rendering)

TODO can use a slight update here

Since the desired state of the application is computed in terms of the underlying hosting platform it means that Radius needs to have multiple pluggable implementations of the *backend*.

Example:

- Azure + AKS: `radius.dev/Container` maps to a Kubernetes deployment
- Azure + Serverless: `radius.dev/Container` maps to an Azure managed service
- Kubernetes: `radius.dev/Container` maps to a Kubernetes deployment

> 🚧 Under Construction 🚧 <br>
The current implementation of the control plane only understands Azure + AKS as a hosting platform, the *backend* is not pluggable.

> 🚧 Under Construction 🚧 <br>
The current implementation of the control plane is quite naive and doesn't totally satisfy our requirements.

> 🚀 Future State 🚀 <br>

The results of desired state computation (rendering) are a set of objects per-component that reflect the desired state of the hosting platform as well as how to fetch the values of *bindings* (the logical connection points between the components). We call these objects *output resources*. 

Output resources each have an assigned logical name. These names are stable identifiers that represent the *role* of the output resource with respect to the component.

Output resources are parameterized objects that contain placeholders for values that can only be known when applying the state. These placeholders represent all of the concerns that cannot be reasoned about idempotently:

- Physical names: runtime-generated names for hosting platform resources
- Bindings: dependencies and connection-points between components

It is critical the the rendering process of a component has the following characteristics with result in valuable guarantees:

- Will result in the same output based on the same input
  - Guarantee: there are no infinite loops in the operations performed by Radius
- Output resource logical names are stable
  - Guarantee: orphaned output resources can be detected and deleted when comparing output resources against the actual state of the system
- Can complete without applying any state changes to the underlying hosting platform
  - Guarantee: rendering output can be diffed against current state because current state is not a moving target
- Can complete without knowing the outputs of other components
  - Guarantee: the per-operation work done by the control plane scales with complexity rather than application size

#### Gathering actual state

In order to apply the desired state we need to reconcile it with the actual state of the hosting platform. This is a pluggable concern that is part of the Radius API and specialized to the hosting environment.

> 🚧 Under Construction 🚧 <br>
We currently track the *expected* state of resources for Azure in our database. This is not the same as the *actual state* because it is based on our book-keeping and not the real source of truth (ARM).

Another way of saying this is that we have not implemented drift-detection. We do not provide any guarantees when a user changes or deletes a Radius-managed resource in Azure.

>❓ Open Question ❓<br>
We will need to learn from some others how to implement this for Azure. Some other teams that run app-model projects within their team have already implemented good approaches that we can learn from.

> 🚀 Future State 🚀 <br>
In Kubernetes this is very easy to accomplish, it is built-in to the controller framework.

#### Applying desired state

This stage deals with apply the output resources through a series of PUT and PATCH operations.

We need to communicate with potentially multiple systems in this layer. The set of systems that we need to communicate with is a function of the hosting platform:

- ARM: needed in Azure-based hosting platforms
- Kubernetes: needed in Kubernetes and Azure + AKS hosting platforms

Conceptually this is an open set. We could target any hosting platform's native resource format as long as it provides the right guarantees for idempotency.

> 🚧 Under Construction 🚧 <br>
We currently have a set of hand-coded *handlers* that deal with specified Azure resource types. Over time this will become the exception not the norm. We need to build the processing logic to handle ARM resources in a generic form - and only hand-code special cases when absolutely required.

> 🚧 Under Construction 🚧 <br>
We currrently build an ordering of all components and apply their desired state in dependency order of the components. This comes with the limitation that cycles in dependecies between output resources of components are not supported.

> 🚀 Future State 🚀 <br>
In the future we will use the output of the rendering phase to apply state in the dependency order of the output resources, not the dependency order of the components.

[This issue](https://github.com/project-radius/radius/issues/300) describes the future state.
