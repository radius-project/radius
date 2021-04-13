---
type: docs
title: "Web App Tutorial"
linkTitle: "Web app + database"
description: "Learn Project Radius by authoring templates and deploying a working web application with a database."
weight: 100
---

## Before you begin

This is a tutorial that will teach you how to use Radius to deploy a web application from first principles. As part of this tutorial you will learn the basic syntax of the Bicep language as well as the concepts of the Radius application model. No prior knowledge of Radius is needed, this tutorial will cover the basics.

## Prerequisites

To begin this tutorial you should have already completed the following steps:

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create a Radius environment]({{< ref create-environment.md >}})
- [Install Kubectl](https://kubernetes.io/docs/tasks/tools/)
- [(recommended) Install Visual Studio Code](https://code.visualstudio.com/)
   - The [Radius VSCode extension]({{< ref "install-cli.md#2-install-custom-vscode-extension" >}}) provides syntax highlighting, completion, and linting.

## Understanding the application

The application you will be deploying is a web application with a database. There are two components:

- A *To-Do List* containerized web application written in Node.JS
- An Azure CosmosDB database

Here is a diagram of the complete application:

<img src="./todoapp-diagram.png" width=400 alt="Simple app diagram">

### Web appliation

The web application (`todoapp`) is a single-page-application (SPA) with a Node.JS backend. The SPA sends requests HTTP requests to the Node.JS backend to read and store a lost of *todo* items.

The web application listens on port 3000 for HTTP requests. 

The web application uses the MongoDB protocol to read and store data in a database. The web application reads the environment variable `DB_CONNECTION` to discover the database connection string.

### Database

The database (`db`) is an Azure Cosmos MongoDB database.

## The Radius mindset

To get into the right mindset for Radius, you should think about the application in *logical* terms. The diagrams shown so far document the communication flows of the application, but there are some details that are missing. 

A Radius template includes all of the logical relationships of an application but also the operational details associated with those relationships. Here is an updated diagram that shows what the Radius template needs to capture:

<img src="./todoapp-appdiagram.png" width=600 alt="App diagram with descriptions of all the details and relationships."><br />

This diagram reflects important details of the Radius model that are different from other deployment technologies you may have used:

- The data component (`db`) are part of the application
- Relationships between components are fully specified with protocols and other strongly-typed information

In addition to this high level information, you will also need typical details like:

- Container images
- Listening ports
- Configuration like connection strings

Keep the diagram in mind as you proceed through the following steps. Creating a Radius deployment template is similar to process of understanding a diagram like this one.

## Step 1: Creating the application definition

You can start by creating a new `template.bicep` file.

Inside `template.bicep`, type or paste in the following content:

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

}
```

This defines the basic structure of an application. This declaration:

- Defines an application resource with the variable name of `app`
- Assigns the name `webapp` to the application resource that will be created

Declarations in Bicep start with `resource`. They also declare a variable, and assign a resource type, and then are followed by an equals-sign `=` and then an object. 

{{% alert title="💡 Declarations" color="primary" %}}
Declarations in Bicep have symbolic (variable) names associated with them. The variable name `app` could be used in this file to reference the application in other declarations. The value of the `name` property (`webapp`) is what will be used to identify the application during management operations.
{{% /alert %}}

{{% alert title="💡 Bicep Syntax" color="primary" %}}
Objects in Bicep don't need quotes around property names like in JSON. Properties in Bicep are separated by newlines, not commas. In general Bicep will require less typing than the equivalent JSON but they can express similar structures.

While Bicep uses newlines to separate properties and other syntax, it is not sensitive to indention like YAML is. By convention Bicep uses 2 spaces for indentation, but it is just a convention and not required.
{{% /alert %}}

At this point you could deploy the application but it doesn't contain any components and so it won't do anything interesting. Move on to the next step where we will begin to fill in components.

## Step 2: Deploying a single container

Now that you've defined the shell for an application, you can add components inside.

### Add a container component

Type or paste the additional content from the following text inside your application definition. What's new is the `todoapp` component.

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

  resource todoapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-todoapp'
        }
      }
    }
  }
}
```

The content you added declares a *component*. If you visualize the structure of an application *as a graph*, then *components* represent the nodes and things to deploy.

A component can be:

- A resource that runs your code *(eg. a container)*
- A resource that works with data *(eg. a message queue or database)*
- A configuration resource *(eg. configuration for an API gateway)*

#### Kind

The specific type of resource to deploy is specified by the component *kind*. In this case the kind is `radius.dev/Container@v1alpha1`, which represents a generic container.

The set of properties and settings available inside the body of the component depends on the kind. The `run` section is used to specify how the component runs. In this case `run` specifies the container image to run. 

{{% alert title="💡 Naming" color="primary" %}}
Like the application declaration, components also declare a variable name. The variable name `todoapp` could be used in this file to reference the component in other declarations. The value of the `name` property (also `todoapp`) is what will be used to identify the component during management operations.
{{% /alert %}}

{{% alert title="💡 Run" color="primary" %}}
The `run` section is one of several top level sections in a *component*. In general components that run your code will have a `run` section.
{{% /alert %}}

### Add HTTP

If you were to deploy this application now it will run the `radiusteam/todoapp` image. However, you would have no way to interact with the running application.

You now need to add the ability to listen for HTTP traffic as depicted in the diagram above.

Type or paste the additional content from the following text inside your application definition. What's new this time is the `provides` section of `todoapp`:

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

  resource todoapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-todoapp'
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

What you've added here defines a *service* called `web` and with the kind `http`. Services in Radius are logical connection-points. It's a way that one component can expose functionality for components of the application to bind to. In this case you've defined an HTTP service that others can use to find the URL of `todoapp` and send it HTTP traffic.

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

3. Merge credentials
   
   {{% alert title="⚠️ Temporary" color="warning" %}}
   Run this command at the commandline to gain access to the underlying AKS cluster in your Radius environment. This is temporary pending additions to the rad CLI:

   ```sh
   rad env merge-credentials --name azure 
   ```
   {{% /alert %}}

1. Open a local tunnel to your application:

   ```sh
   rad expose webapp todoapp 3000
   ```
   {{% alert title="💡 rad expose" color="primary" %}}
   The `rad expose` command provides the application name, followed by the component name, followed by a port. If you changed any of these names when deploying, update your command to match.
   {{% /alert %}}

1. Visit the URL `http://localhost:3000` in the browser. For now you should see a page like:

   <img src="todoapp-nodb.png" width="400" alt="screenshot of the todo application with no database">

   If the page you are seeing matches the screenshot hat means that the container is running. As the message indicates no database has been configured yet.

   You can play around with the application's features features:
   - Add a todo item
   - Mark a todo item as complete
   - Delete a todo item

1. When you are done testing press CTRL+C to terminate the port-forward, and you are ready to move on to the next step.

## Step 3: Adding a database

As the message from the previous step stated, you have not yet added a database. This means that the todo items you enter will be stored in memory inside the application. If the web application restarts then all of your data will be lost!

In this step you will learn how to add a database and connect to it from the application.

### Add db component
Type or paste the new component declaration (`db`) from the following text inside your application definition. Leave your existing declaration for todoapp unchanged.

```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

  ...

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDocumentDb@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
}
```

This declaration adds the database as a component of kind `azure.com/CosmosDocumentDb@v1alpha1`. You've seen component declarations before, so you can notice some differences with this one. `db` has a `config` section instead of a `run` section.

{{% alert title="💡 Config" color="primary" %}}
The `config` section is one of several top level sections in a *component*. In general component that represent a data store will have a `config` section
{{% /alert %}}

Inside the `config` section you specified `managed: true`. This flag tells Radius to manage the lifetime of the database for you. The database will be deleted when you delete the application.

### Reference db from todoapp

Now that you've created the database as an component, you can reference it from todoapp to connect them.

Type or paste the additional content from the following text inside your application definition. What's new this time is the `dependsOn` section:
```bash
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

  resource todoapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-todoapp'
        }
      }
      dependsOn: [
        {
          kind: 'mongodb.com/Mongo'
          name: 'db'
          setEnv: {
            DB_CONNECTION: 'connectionString'
          }
        }
      ]
      provides: [
        {
          kind: 'http'
          name: 'web'
          containerPort: 3000
        }
      ]
    }
  }

  ...
}
```

The `dependsOn` section is used to configure relationships between a component and services provided by other components. The `db` is of kind `azure.com/CosmosDocumentDb@v1alpha1`, which supports the MongoDB protocol. `db` is considered to provide a service of kind `mongodb.com/Mongo` implicitly. Configuring a dependency on a service is the other part of specifying a relationship. This declares the *intention* from the `todoapp` component to communicate with the `db` using `mongodb.com/Mongo` as the protocol.

{{% alert title="💡 Protocols" color="primary" %}}
Relations in Radius are based on protocols and services as a form of *loose-coupling*. The definition of `todoapp` documents its dependency on a *MongoDB-compatible database* rather than a dependency on a *cloud-provider-specific service (Azure CosmosDB)*.

`db` could be replaced with another MongoDB-compatible database (eg. MongoDB Atlas) and the defintion of `todoapp` would not need to change.
{{% /alert %}}


The `setEnv` section declares operations to perform *based on* the relationship. In this case the `connectionString` value will be retrieved from the database and set as an environment variable on the component. As a result of this, `todoapp` will be able to use the `DB_CONNECTION` environment variable to access to the database connection string.

{{% alert title="💡 Relationships" color="primary" %}}
Radius captures the relationships and intentions behind an application so that they can simplify deployment. Examples of this include wiring up connection strings, or granting permissions, or restarting components when a dependency changes.
{{% /alert %}}

### Deploy application with database

Now you are ready to deploy.

1. Switch to the command-line and run the following command.

   ```sh
   rad deploy template.bicep
   ```

   This will deploy the application, including the Azure CosmosDB database. This may take a few minutes because of the extra time required to create the database.

1. To test it out, you can use the following command from the commandline:

   ```sh
   rad expose webapp todoapp 3000
   ```

   This will open a local tunnel on port 3000.

1. Visit the URL `http://localhost:3000` in the browser. For now you should see a page like:

   <img src="todoapp-withdb.png" width="400" alt="screenshot of the todo application with a database">

   If your page matches, then it means that the container is able to communicate with the database. Just like before you can test the features of the todo app. Now your data is being stored in an actual database. Add a task or two. 

1. Now that the app is using a database, we can confirm that the task items persist across application restarts. 
   1. Press CTRL+C to terminate the port-forward
   1. Redeploy the application via `rad deploy template.bicep`
   1. Restart port-forwarding via `rad expose webapp todoapp 3000`. 
   1. Visit the URL `http://localhost:3000` in the browser again to see the tasks you previously created. 

1. When you are done testing press CTRL+C to terminate the port-forward. You have completed this tutorial!

## Step 4: Cleanup

If you'd like to try another tutorial with your existing environment, go back to the [Radius tutorials]({{< ref tutorial >}}). 

If you're done with testing, clean up your environment to **prevent additional charges in your subscription**. 


Deleting an environment will delete:

- Your Radius environment
- The related resource group
- The application you just deployed

```sh
rad env delete azure --yes
```
