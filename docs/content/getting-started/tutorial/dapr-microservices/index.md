---
type: docs
title: "Dapr Microservices Tutorial"
linkTitle: "Dapr microservices"
description: "Learn Project Radius by authoring templates and deploying a working Dapr application"
weight: 200
---

## Before you begin

This tutorial will teach you how to use Radius to deploy a microservices application from first principles. You will learn:  

- The concepts of the Radius application model 
- The basic syntax of the Bicep language 

The Radius application in this tutorial uses [Dapr](https://dapr.io). No prior knowledge of Dapr, Radius, or [Bicep](https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/bicep-overview) is needed.

## Prerequisites

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create a Radius environment]({{< ref create-environment.md >}})
- [(recommended) Install Visual Studio Code](https://code.visualstudio.com/)
   - The [Radius VSCode extension]({{< ref "install-cli.md#2-install-custom-vscode-extension" >}}) provides syntax highlighting, completion, and linting.
   - You can also complete this tutorial with any basic text editor.

## Overview of the tutorial application

You will be deploying a microservices order processing application. It will have three Radius *components*:

- An containerized order processing microservice written in Node.JS ("nodeapp")
- A Dapr state store used to store the orders ("statestore")
- An order generating microservice ("pythonapp")

<img src="https://raw.githubusercontent.com/dapr/quickstarts/v1.0.0/hello-world/img/Architecture_Diagram_B.png" alt="The complete application" width=800>

### Order processing microservice

The order processing microservice (`nodeapp`) accepts HTTP requests to create or display orders. Here is a diagram focused on the order processing service:

<img src="https://raw.githubusercontent.com/dapr/quickstarts/v1.0.0/hello-world/img/Architecture_Diagram.png" alt="The nodeapp order processing service" width=700>

The nodeapp accepts HTTP requests on two endpoints: `GET /order` and `POST /neworder`.

### Dapr state store
The state store (`statestore`) stores information about orders. It could be any compatible [Dapr state store](https://docs.dapr.io/developing-applications/building-blocks/state-management/state-management-overview/). In this tutorial we will use Azure Table Storage.

### Order generating service

The order generting service (`pythonapp`) uses [Dapr service invocation](https://docs.dapr.io/developing-applications/building-blocks/service-invocation/service-invocation-overview/) to send requests to nodeapp. It does not accept any incoming traffic. 

## The Radius mindset

The diagrams shown so far document the communication flows, but a Radius application also describes additional details. 

A Radius template includes 

- the logical relationships of an application 
- the operational details associated with those relationships 

Here is an updated diagram that shows what the Radius template needs to capture:

<img src="https://user-images.githubusercontent.com/1430011/111005089-04b3c280-833f-11eb-9ce1-bdd12beef78b.png" alt="The application logical diagram" width=800>

This diagram reflects important details of the Radius model that are different from other deployment technologies you may have used:

- The data component (`statestore` here) is part of the application
- Relationships between components are fully specified with protocols and other strongly-typed information

In addition to this high level information, the Radius model also uses typical details like:

- Container images
- Listening ports
- Configuration like connection strings

Keep the diagram in mind as you proceed through the following steps. Your Radius deployment template will aim to match it. 

## Step 1: Creating the application definition

Radius uses the Bicep langauge as its file-format and structure. Create a new `template.bicep` file with the following content:

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

}
```

This defines the basic structure of an application. This declaration:

- Defines an application resource with the variable name of `app`. 
- Assigns the name `dapr-hello` to the application resource that will be created.

Declarations in Bicep start with `resource`. They also declare a variable, assign a resource type, and then are followed by an equals-sign `=` and then an object. 

{{% alert title="💡 Declarations" color="primary" %}}
Declarations in Bicep have symbolic (variable) names associated with them. The variable name `app` could be used in this file to reference the application in other declarations. The value of the `name` property (`dapr-hello`) is what will be used to identify the application during management operations.
{{% /alert %}}

{{% alert title="💡 Bicep Syntax" color="primary" %}}
Objects in Bicep don't need quotes around property names like in JSON. Properties in Bicep are separated by newlines, not commas. In general Bicep will require less typing than the equivalent JSON but they can express similar structures.

While Bicep uses newlines to separate properties and other syntax, it is not sensitive to indention like YAML is. By convention Bicep uses 2 spaces for indentation, but it is just a convention and not required.
{{% /alert %}}

At this point you could deploy the application but it doesn't contain any components and so it won't do anything interesting. 

## Step 2: Deploying a single container

Now that you've defined the shell for an application, you can add components to it.

### Add a container component
We will first add a component representing a container.  
Inside your application definition, add the `nodeapp` component shown below:

```bash
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
    }
  }
}
```

The content you added declares a *component*. If you visualize the structure of an application *as a graph*, then *components* represent the nodes and things to deploy.

Components also include a *kind*. In this case the kind is `radius.dev/Container@v1alpha1` which represents a generic container.

A component can be:

- A resource that runs your code (eg. a container)
- A resource that works with data (eg. a message queue or database)
- A configuration resource (eg. configuration for an API gateway)

#### Kind

The component *kind* specifies the type of resource to deploy. In this case, the kind is `radius.dev/Container@v1alpha1`, which represents a generic container.

The set of properties and settings available inside the body of the component depends on the kind. The `run` section is used to specify how the component runs. In this case `run` specifies the container image to run. 

{{% alert title="💡 Naming" color="primary" %}}
Like the application declaration, components also declare a variable name. The variable name `nodeapplication` could be used in this file to reference the component in other declarations. The value of the `name` property (`nodeapp`) is what will be used to identify the component during management operations.
{{% /alert %}}

{{% alert title="💡 Run" color="primary" %}}
The `run` section is one of several top level sections in a *component*. In general components that run your code will have a `run` section.
{{% /alert %}}

### Add an HTTP service
You could deploy this now and it will run the `radiusteam/nodeapp` image but you would have no way to interact with the running application.

Add the ability to listen for HTTP traffic as depicted in the diagram above. Expand your `nodeapp` as shown below so that it includes a web service definition via a `provides` section:

```bash
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
      provides: [
        {
          kind: 'http'
          name: 'web'
          containerPort: 3000
        }
      ]
    }
  }
}
```

#### Service

What you've added here defines a *service* called `web` and with the kind `http`. Services in Radius are logical connection-points. It's a way that one component can expose functionality for components of the application to bind to. In this case you've defined an HTTP service that others can use to find the URL of `nodeapp` and sent it HTTP traffic. 

There is nothing special about the name `web`, it is just an identifier used for the name of the service.

{{% alert title="💡 HTTP services" color="primary" %}}
HTTP services in Radius are *internal*, meaning that they are not exposed to internet traffic by default.
{{% /alert %}}

### Deploy application

Now you are ready to deploy the application for the first time. 

1. First, double-check that you are logged-in to Azure. Switch to your commandline and run the following command:

   ```sh
   az login
   ```

1. Then after that completes, run:

   ```sh
   rad deploy template.bicep
   ```

   This will deploy the application and launch the container.

1. Confirm that your container was deployed

   View deployed applications:

   ```sh
   rad application list
   ```
   
   Example output with the `dapr-hello` Radius application: 
   ```sh
   {
     "value": [
       {
         "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/dapr-hello",
         "name": "radius/dapr-hello",
         "type": "Microsoft.CustomProviders/resourceProviders/Applications"
       }
     ]
   }
   ```

   View deployments for a specific app: 

   ```sh
   rad deployment list --application-name dapr-hello
   ```

   Your `nodeapp` component has been created into a `default` deployment of the `dapr-hello` application. 
   You should see something like this:

   ```sh
   {
     "value": [
       {
         "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/dapr-hello/Deployments/default",
         "name": "radius/dapr-hello/default",
         "type": "Microsoft.CustomProviders/resourceProviders/Applications/Deployments",
         "properties": {
           "components": [
             {
               "componentName": "nodeapp"
             }
           ]
         }
       }
     ]
   }
   ```

1. View the properties of the deployed `nodeapp` component 

   ```sh
   rad component get --application-name dapr-hello --name nodeapp
   ```
 
   The details of the `nodeapp` component should match its definition from your template.bicep file. Example output:

   ```sh
   Using config file: /Users/{USER}/.rad/config.yaml
   {
     "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/dapr-hello/Components/nodeapp",
     "name": "radius/dapr-hello/nodeapp",
     "type": "Microsoft.CustomProviders/resourceProviders/Applications/Components",
     "kind": "radius.dev/Container@v1alpha1",
     "properties": {
       "provides": [
         {
           "containerPort": 3000,
           "kind": "http",
           "name": "web"
         }
       ],
       "revision": "623b7888a3b779c6579e82ee2f7215a6396dfcef",
       "run": {
         "container": {
           "image": "radiusteam/tutorial-nodeapp"
         }
       }
     }
   }
   ```

1. To test out your `dapr-hello` application, open a local tunnel to your application:

   ```sh
   rad expose dapr-hello nodeapp --port 3000
   ```

   {{% alert title="💡 rad expose" color="primary" %}}
   The `rad expose` command provides the application name, followed by the component name, followed by a port. If you changed any of these names when deploying, update your command to match.
   {{% /alert %}}

1. Visit the URL `http://localhost:3000/order` in your browser. For now you should see a message like:

   ```txt
   {"message":"The container is running, but Dapr has not been configured."}
   ```

   If the message matches, then it means that the container is running as expected.

1. When you are done testing press CTRL+C to terminate the port-forward, and you are ready to move on to the next step.

## Step 3: Adding Dapr and the state store

As the message from the previous step stated, you haven't yet added Dapr. You also haven't configured the Azure Table Storage state store. This step will add both of these things.

### Add a Dapr trait to the nodeapp component
We will first add a *trait* that describes the Dapr configuration. 

Expand your `nodeapp` as shown below so that it includes a `traits` section.

```bash
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
}
```

The `traits` section is used to configure cross-cutting behaviors of components. Since Dapr is not part of the standard definition of a container, it can be added via a trait. Traits have a `kind` so that they can be strongly typed. In this case we're providing some required Dapr configuration: the `app-id` and `app-port`.

{{% alert title="💡 Traits" color="primary" %}}
The `traits` section is one of several top level sections in a *component*. Traits are used to configure the component in a cross-cutting way. Other examples would include handling public traffic (ingress) or scaling.
{{% /alert %}}

### Add statestore component
Now the nodeapp is hooked up to Dapr, but we still need to define a state store to save information about orders.

Add a new component declaration (`statestore`) to your application definition as shown below. Leave your existing declaration for `nodeapp` unchanged.

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

  ...

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

This declaration adds the state store as a component of kind `dapr.io/StateStore@v1alpha1`. You've seen component declarations before, so you can notice some differences with this one. `statestore` has a `config` section instead of `run` and `provides` (services) sections.

{{% alert title="💡 Config" color="primary" %}}
The `config` section is one of several top level sections in a *component*. In general components that represent a data store will have a `config` section
{{% /alert %}}

Inside the `config` section, you specified a `kind` of `state.azure.tablestorage`. This corresponds to the kind of Dapr state store used for [Azure Table Storage](https://docs.dapr.io/operations/components/setup-state-store/supported-state-stores/setup-azure-tablestorage/).

You also specified `managed: true`. This flag tells Radius to manage the lifetime of the Azure Storage account for you. The Azure Storage account will be deleted when you delete the application.

{{% alert title="💡 Resource creation" color="primary" %}}
If you have used Dapr before, you may notice that your Radius template neither created the Azure Storage resource itself nor configured Dapr with details like connection strings. Radius does this for you.
{{% /alert %}}

### Reference statestore from nodeapp
Now that you've created the state store as a component, you can connect them by referecing the `statestore` component from the `nodeapp` component. 

Type or paste the additional content from the following text inside your application definition. What's new is the `dependsOn` section.

```bash
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

  ...
}
```

The `dependsOn` section is used to configure relationships between a component and services provided by other componentss. Since the `statestore` is of kind `dapr.io/StateStore@v1alpha1` it is considered to provide a service of kind `dapr.io/StateStore` implicitly. Configuring a dependency on a service is the other part of specifying a relation. This declares the *intention* from the `nodeapp` component to communicate with the `statestore` using `dapr.io/StateStore` as the protocol.

{{% alert title="💡 Relationships" color="primary" %}}
Radius captures the relationships and intentions behind an application so that they can simplify deployment. Examples of this include wiring up connection strings, or granting permissions, or restarting components when a dependency changes.
{{% /alert %}}

### Deploy application with Dapr

Now you are ready to deploy. 

1. Switch to the command-line and run the following command.

   ```sh
   rad deploy template.bicep
   ```

   This will deploy the application, including the Azure Storage account. This may take a few minutes because of the extra time required to create the Storage Account.

1. Confirm that the state store was deployed.

   ```sh
   rad deployment list --application-name dapr-hello
   ```

   Now you should see both `nodeapp` and `statestore` components in your `dapr-hello` application, similar to:

   ```sh
   Using config file: /Users/{USER}/.rad/config.yaml
   {
     "value": [
       {
         "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/dapr-hello/Deployments/default",
         "name": "radius/dapr-hello/default",
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

1. Visit the the URL `http://localhost:3000/order` in your browser. You should see a message like:

   ```txt
   {"message":"no orders yet"}
   ```

   If your message matches, then it means that the container is able to communicate with the state store. When you are done testing press CTRL+C to terminate the port-forward, and you are ready to move on to the next step.

## Step 4: Adding pythonapp

To complete the application, you need to add another component for the order generating microservice (`pythonapp`).

### Add pythonapp component
Type the new component declaration from the following text inside your application definition. Leave your existing declarations for nodeapp and statestore unchanged.

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

  ...

  resource pythonapplication 'Components' = {
    name: 'pythonapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-pythonapp'
        }
      }
      dependsOn: [
        {
          kind: 'dapr.io/Invoke'
          name: 'nodeapp'
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'pythonapp'
          }
        }
      ]
    }
  }
}
```

The definition for `pythonapp` is simpler than what you have done previously for `nodeapp`. There are few key points that you should notice:

- `pythonapp` doesn't listen for HTTP traffic, so it configures neither a Dapr app-port nor a service for HTTP.
- `pythonapp` needs to communicate with `nodeapp` using the Dapr service invocation protocol.

### Deploy application with pythonapp

Now you are ready to deploy. 

1. Switch to the command-line and run the following command.

   ```sh
   rad deploy template.bicep
   ```

   This will deploy the complete application, including both containers and the Azure Storage account.

1. To test out the pythonapp microservice, open a local tunnel on port 3000 again:

   ```sh
   rad expose dapr-hello nodeapp --port 3000
   ```

1. Visit the URL `http://localhost:3000/order` in your browser.

   Refresh the page multiple times and you should see a message like before. The order number is steadily increasing after refresh. 

   ```txt
   {"orderId":7}
   ```

   If your message matches, then it means that `pythonapp` is able to communicate with `nodeapp`. 

1. When you are done testing press CTRL+C to terminate the port-forward. 

## Step 5: Cleanup

If you'd like to try another tutorial with your existing environment, go back to the [Radius tutorials]({{< ref tutorial >}}) page. 

If you're done with testing, clean up your environment to **prevent additional charges in your subscription**. 

Deleting an environment will delete:

- Your Radius environment
- The related resource group
- The application you just deployed

```sh
rad env delete azure --yes
```

You have completed this tutorial!

## Related links

- View the full template.bicep for this tutorial [here](https://github.com/Azure/radius/blob/main/docs/content/getting-started/tutorial/dapr-microservices/dapr-microservices.bicep)
- [Dapr documentation](https://docs.dapr.io/)
- You can find the source code for this application [here](https://github.com/dapr/quickstarts/tree/v1.0.0/hello-world) as well as additional information and tutorials for Dapr.
