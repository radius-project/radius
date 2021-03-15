---
type: docs
title: "Dapr Microservices Tutorial"
linkTitle: "Dapr Microservices Tutorial"
description: "Learn Project Radius by authoring templates and deploying a working Dapr application"
weight: 20
---

## Prerequisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create an environment]({{< ref create-environment.md >}})
- [Install Kubectl](https://kubernetes.io/docs/tasks/tools/)

No prior knowledge of Radius is needed, this tutorial will walk you through authoring the deployment template and deploying a microservices application from first principles.

If you are using Visual Studio Code with the Project Radius extension you should see syntax highlighting. If you have the offical Bicep extension installed, you should disable it for this tutorial. The instructions will refer to VS Code features like syntax highlighting and the problems windows - however, you can complete this tutorial with just a basic text editor.

## Understanding the application

The application you will be deploying is a microservices order processing application. There are three components:

- An order processing service written in Node.JS
- An order generating service written in Python
- A Dapr state store used to store the orders

You can find the source code for the application [here](https://github.com/dapr/quickstarts/tree/v1.0.0/hello-world) as well as additional information and tutorials for Dapr. You will not need to build the application from source or have Dapr installed to complete this tutorial.

### Order processing service

The order processing service (nodeapp) accepts HTTP requests to create or display orders. Here is a diagram:

<img src="https://raw.githubusercontent.com/dapr/quickstarts/v1.0.0/hello-world/img/Architecture_Diagram.png" alt="The nodeapp order processing service" width=700>

You can see that the nodeapp accepts HTTP requests on two endpoints: `GET /order` and `POST /neworder`.

The nodeapp also uses a [Dapr state store](https://docs.dapr.io/developing-applications/building-blocks/state-management/state-management-overview/) to store information about orders.

### Order generating service

The order generting service (pythonapp) does not accept any incoming traffic, and uses [Dapr service invocation](https://docs.dapr.io/developing-applications/building-blocks/service-invocation/service-invocation-overview/) to send requests to nodeapp. 

### Dapr state store
The state store could be any compatible Dapr state store. In this tutorial we will use Azure Table Storage.

Here is a diagram of the complete application include pythonapp:

<img src="https://raw.githubusercontent.com/dapr/quickstarts/v1.0.0/hello-world/img/Architecture_Diagram_B.png" alt="The complete application" width=800>

## The Radius mindset

To get into the right mindset for Radius, you should think about the application in *logical* terms. The diagrams shown so far document the communication flows of the application, but there are some details that are missing. 

A Radius template includes all of the logical relationships of an application but also the operational details associated with those relationships. Here is an updated diagram that shows what the Radius template needs to capture:

<img src="https://user-images.githubusercontent.com/1430011/111005089-04b3c280-833f-11eb-9ce1-bdd12beef78b.png" alt="The application logical diagram" width=800>

The diagram reflects important details of the Radius model that are different from other deployment technologies you may have used:

- The data components (statestore) are part of the application
- Relationships between components are fully specified with protocols and other strongly-typed information

In addition to this high level information, you will also need typical details like:

- container images
- listening ports
- programmatic identifiers and configuration like a Dapr app-id.

Keep the diagram in mind as you proceed through the following steps. Creating a Radius deployment template is similar to process of understanding a diagram like this one.

## Step 1: Creating the application definition

You can start by creating a new `.bicep` file. Call it `template.bicep`. 

Inside `template.bicep`, type in the following content:

```txt
application app = {
  name: 'dapr-hello'

}
```

This defines the basic structure of an application. This declaration:

- Defines an application with the variable name of `app`
- Assigns the name `dapr-hello` to the application resource that will be created

The syntax of Bicep is similar to JSON with a few differences. Top level declarations like `application` declare a variable, and are followed by an equals-sign `=` an then an object. 

{{% alert title="💡 Declarations" color="primary" %}}
Declarations in Bicep have symbolic (variable) names associated with them. The variable name `app` could be used in this file to reference the application in other declarations. The value of the `name` property (`dapr-hello`) is what will be used to identify the application during management operations.
{{% /alert %}}

An object (after the equals-sign) is similar to a JSON object.  Objects in Bicep don't need quotes around property names like in JSON. Properties in Bicep are separated by newlines, not commas. In general Bicep will require less typing than the equivalent JSON but they can express similar structures.

{{% alert title="💡 Indentation" color="primary" %}}
While Bicep uses newlines to separate properties and other syntax, it is not sensitive to indention like YAML is. By convention Bicep uses 2 spaces for indentation, but it is just a convention and not required.
{{% /alert %}}

At this point you could deploy the application but it doesn't contain any components and so it won't do anything interesting. Move on to the next step where we will begin to fill in components.

## Step 2: Deploying a single container

Now that you've defined the shell for an application, you can add components inside.

### Add an instance
Type the additional content from the following text inside your application definition.

```txt
application app = {
  name: 'dapr-hello'

  instance nodeapp 'radius.dev/Container@v1alpha1 = {
    name: 'nodeapp'
  }
}
```

If you are using VS Code you may see and error squiggle at this point. You will have a chance to address those errors after a brief explanation.

The content you added declares an *instance*. If you visualize the structure of an application *as a graph* then *instances* represent the nodes. *instances* represent the things to deploy.

Instances also include a *type*. In this case the type is `radius.dev/Container@v1alpha1` which represents a generic container.

An instance can be:

- A resource that runs your code (eg. a container)
- A resource that works with data (eg. a message queue or database)
- A configuration resource (eg. configuration for an API gateway)

The *type* specifies the type of resource to create. The set of properties and settings available inside the body of the instance depends on the type.

{{% alert title="💡 Naming" color="primary" %}}
Like the application declaration, instances also declare a variable name. The variable name `nodeapp` could be used in this file to reference the instance in other declarations. The value of the `name` property (also `nodeapp`) is what will be used to identify the instance during management operations.
{{% /alert %}}

### Add instance details

Now you can fill in the details of the instance. First you should specify the container image.

Type the additional content from the following text inside your application definition.

```txt
application app = {
  name: 'dapr-hello'

  instance nodeapp 'radius.dev/Container@v1alpha1 = {
    name: 'nodeapp'
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

You should no longer see errors due to missing properties. The content that you've just added for `nodeapp` specifies the container image to run. 

{{% alert title="💡 Run" color="primary" %}}
The `run` section is one of several top level sections in an *instance*. In general instance types that run your code will have a `run` section.
{{% /alert %}}

You could deploy this now and it will run the `radiusteam/nodeapp` image, however you would have no way to interact with the running application.

You can add the ability to listen for HTTP traffic as depicted in the diagram above.

Type the additional content from the following text inside your application definition.

```txt
application app = {
  name: 'dapr-hello'

  instance nodeapp 'radius.dev/Container@v1alpha1 = {
    name: 'nodeapp'
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

What you've added here defines a *service* called `web` and with the kind `http`. Services in Radius are logical connection-points. It's a way that one instance can expose functionality for components of the application to bind to. In this case you've defined an HTTP service that others can use to find the URL of `nodeapp` and sent it HTTP traffic. There is nothing special about the name `web`, it is just an identifier used for the name of the service.

{{% alert title="💡 HTTP services" color="primary" %}}
HTTP services in Radius are *internal*, meaning that they are not exposed to internet traffic.
{{% /alert %}}```

{{% alert title="💡 Ports" color="primary" %}}
Other technologies you might have used specify `ports` as part of the container definition. Radius supports that style of definition as well, but it is more useful to attach operation details like port numbers to *behavior* and *protocols*. This is how Radius helps you capture the logical meaning behind the details.
{{% /alert %}}

---

Now you are ready to deploy the application for the first time. 

First, double-check that you are logged-in to Azure. Switch to your commandline and run the following command:

```sh
az login
```

Then after that completes, run:

```sh
rad deploy template.bicep
```

This will deploy the application and launch the container.


> ⚠️⚠️⚠️⚠️⚠️ this step is temporary
Run this command at the commandline:

```sh
rad env merge-credentials --name azure 
```
> ⚠️⚠️⚠️⚠️⚠️ end temporary-ness - thanks for your patience

To test it out, you can use the following command from the commandline:

```sh
rad expose dapr-hello nodeapp 3000
```

This will open a local tunnel on port 3000. Then you can visit the URL `http://localhost:3000/order` in the browser. For now you should see a message like:

```txt
{"message":"The container is running, but Dapr has not been configured."}
```

If your message matches, then it means that the container is running. When you are done testing press CTRL+C to terminate the port-forward, and you are ready to move on to the next step.

{{% alert title="💡 rad expose" color="primary" %}}
The `rad expose` command provides the application name, followed by the instance name, followed by a port. If you changed any of these names when deploying, update your command to match.
{{% /alert %}}

## Step 3: Adding Dapr and the state store

As the message from the previous step stated, you haven't yet added Dapr. You also haven't configured the Azure Table Storage state store. This step will add both of these things.

### Add trait
First, you should add a *trait* to the `nodeapp` instance to add Dapr. 

Type the additional content from the following text inside your application definition. What's new this time is the `traits` section.

```txt
application app = {
  name: 'dapr-hello'

  instance nodeapp 'radius.dev/Container@v1alpha1 = {
    name: 'nodeapp'
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
}
```

The `traits` section is used to configure cross-cutting behaviors of instances. Since Dapr is not part of the standard definition of a container, it can be added on via a trait. Traits have a `kind` so that they can be strongly typed. In this case we're providing some required configuration to Dapr, the app-id and app-port.

{{% alert title="💡 Traits" color="primary" %}}
The `traits` section is one of several top level sections in an *instance*. Traits are used to configure the instance in a cross-cutting way. Other examples would include handling public traffic (ingress) or scaling.
{{% /alert %}}

### Add statestore instance
Now the nodeapp is hooked up to Dapr, but we still need to address the topic of the state store.

Type the new instance declaration from the following text inside your application definition. Leave your existing declaration for nodeapp unchanged.

```txt
application app = {
  name: 'dapr-hello'

  ...

  instance statestore 'dapr.io/StateStore@v1alpha1' = {
    name: 'statestore'
    properties: {
      config: {
        kind: 'state.azure.tablestorage'
        managed: true
      }
    }
  }
}
```

This declaration adds the state store as an instance of type `dapr.io/StateStore@v1alpha1`. You've seen instance declarations before, so you can notice some differences with this one. `statestore` has a `config` section instead of a `run` section.

{{% alert title="💡 Config" color="primary" %}}
The `config` section is one of several top level sections in an *instance*. In general instance types that represent a data store will have a `config` section
{{% /alert %}}

Inside the `config` section you specified a `kind` of `state.azure.tablestorage`. This corresponds to the type of Dapr state store used for [Azure Table Storage](https://docs.dapr.io/operations/components/setup-state-store/supported-state-stores/setup-azure-tablestorage/).

Inside the `config` section you specified `managed: true`. This flag tells Radius to manage the lifetime of the Azure Storage account for you. The Azure Storage account will be deleted when you delete the application.

{{% alert title="💡 Resource creation" color="primary" %}}
If you have used Dapr before, you may notice that you neither had to create the Azure Storage resource itself, nor configure Dapr with details like connection strings. Radius does this for you.
{{% /alert %}}

### Reference statestore from application
Now that you've created the state store as an instance, you can reference it from nodeapp to connect them.

Type the additional content from the following text inside your application definition. What's new this time is the `dependsOn` section.

```txt
application app = {
  name: 'dapr-hello'

  instance nodeapp 'radius.dev/Container@v1alpha1 = {
    name: 'nodeapp'
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
}
```

The `dependsOn` section is used to configure relationships between an instance and services provided by other instances. Since the `statestore` is of type `dapr.io/StateStore@v1alpha1` it is considered to provide a service of kind `dapr.io/StateStore` implicitly. Configuring a dependency on a service is the oth part of specifying a relation. This declares the *intention* from the `nodeapp` component to communicate with the `statestore` using `dapr.io/StateStore` as the protocol.

{{% alert title="💡 Relationships" color="primary" %}}
Radius captures the relationships and intentions behind an application so that they can simplify deployment. Examples of this include wiring up connection strings, or granting permissions, or restarting components when a dependency changes.
{{% /alert %}}

### Deploy application with Dapr

Now you are ready to deploy. Switch to the command-line and run the following command.

```sh
rad deploy template.bicep
```

This will deploy the application, including the Azure Storage account. This may take a few minutes because of the extra time required to create the Storage Account.

To test it out, you can use the following command from the commandline:

```sh
rad expose dapr-hello nodeapp 3000
```

This will open a local tunnel on port 3000. Then you can visit the URL `http://localhost:3000/order` in the browser. For now you should see a message like:

```txt
{"message":"no orders yet"}
```

If your message matches, then it means that the container is able to communicate with the state store. When you are done testing press CTRL+C to terminate the port-forward, and you are ready to move on to the next step.

## Step 4: Adding pythonapp

To complete the application, you need to add another instance for the pythonapp.

### Add pythonapp instance
### Add pythonapp instance
Type the new instance declaration from the following text inside your application definition. Leave your existing declarations for nodeapp and statestore unchanged.

```txt
application app = {
  name: 'dapr-hello'

  ...

  instance pythonapp 'radius.dev/Container@v1alpha1' = {
    name: 'pythonapp'
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

The definition for pythonapp is simpler than what you have done so far for node app. There are few key points that you should notice:

- pythonapp doesn't listen for HTTP traffic, so it neither configures a Dapr app-port nor a service for HTTP
- pythonapp needs to communicate with nodeapp using the Dapr service invocation protocol

### Deploy application with pythonapp

Now you are ready to deploy. Switch to the command-line and run the following command.

```sh
rad deploy template.bicep
```

This will deploy the complete application, including both containers and the Azure Storage account.

To test it out, you can use the following command from the commandline:

```sh
rad expose dapr-hello nodeapp 3000
```

This will open a local tunnel on port 3000. Then you can visit the URL `http://localhost:3000/orders` in the browser. 

Refresh the page multiple times, and you should see a message like before, but the order number is steadily increasing.

```txt
{"orderId":28}
```

If your message matches, then it means that pythonapp is able to communicate with nodeapp. When you are done testing press CTRL+C to terminate the port-forward.

You have completed this tutorial!

## Step 3: Cleanup (optional)

When you are ready to clean up and delete the resources you can delete your environment. This will delete:

- The resource group
- Your Radius environment
- The application you just deployed

```sh
rad env delete --yes
```

## Related links

- [Dapr documentation](https://docs.dapr.io/)
- [Dapr Hello World](https://github.com/dapr/quickstarts/tree/v1.0.0/hello-world)
