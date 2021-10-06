---
type: docs
title: "Add a database to the website tutorial app"
linkTitle: "Add a database"
description: "Connect a MongoDB resource to the website tutorial application"
slug: "database"
weight: 3000
---

So far you have not yet configured a database, so the todo items you enter will be stored in memory inside the application. If the website restarts then all of your data will be lost!

In this step you will learn how to add a database and connect to it from the application.

We'll discuss template.bicep changes and then provide the full, updated file before deployment. 

## Add db component
A `db` database component is used to specify a few properties about the database: 

- **kind:** `mongodb.com.MongoDBComponent` represents a MongoDB compatible database.
- **managed:** `true` tells Radius to [manage the lifetime]({{< ref "components-model#radius-managed" >}}) of the component for you.

{{< rad file="snippets/app.bicep" embed=true marker="//MONGO" >}}

{{< tabs "Microsoft Azure" Kubernetes>}}

{{% codetab %}}
When deploying to an Azure environment, a managed [`mongodb.com.MongoDBComponent`]({{< ref mongodb >}}) Component will be bound to an Azure CosmosDB API for MongoDB. By declaring your dependency on a generic *MongoDB-compatible* database, your code is more portable.
{{% /codetab %}}

{{% codetab %}}
When deploying to a Kubernetes environment, a managed [`mongodb.com.MongoDBComponent`]({{< ref mongodb >}}) will be bound to the [`mongo` Docker image](https://hub.docker.com/_/mongo/) running a lightweight developer configuration. 
{{% /codetab %}}

{{< /tabs >}}

## Reference db from todoapp

Radius captures both logical relationships and related operational details. Examples of this include wiring up connection strings, granting permissions, or restarting components when a dependency changes.

Once the database is defined as a Component, you can connect to it by referencing the `db` component from within the `todoapp` Component via the [`connections`]({{< ref connections-model >}}) section. 

[`connections`]({{< ref connections-model >}}) is used to configure relationships between two components. The `db` is of kind `mongodb.com.MongoDBComponent`, which supports the `mongodb.com/Mongo` MongoDB protocol. Configuring a dependency on this protocal is the other part of specifying a relationship. This declares the *intention* from the `todoapp` component to communicate with the `db`.

Once you connect to `db` you can now reference the `db.connectionStrings()` method from within the `todoapp` Component's `env` definition. This places the `db` connection string in the `todoapp` Component's environment

Here's what the `todoapp` component will look like with the `connections` section added within its properties and an environment variable defined in `env`:

{{< rad file="snippets/app.bicep" embed=true marker="//CONTAINER" replace-key-ports="//PORTS" replace-value-ports="ports: {...}" replace-key-bindings="//BINDINGS" replace-value-bindings="bindings: {...}" >}}

## Update your template.bicep file 

Update your `template.bicep` file to match the full application definition:

{{< rad file="snippets/app.bicep" download=true >}}

## Deploy application with database

1. Now you are ready to re-deploy the application, including the Azure CosmosDB database. Switch to the command-line and run: 

   ```sh
   rad deploy template.bicep
   ```

   This may take a few minutes because of the time required to create the database.

1. You can confirm that the new `db` component was deployed by running:

   ```sh
   rad resource list --application webapp
   ```

   You should see both `db` and `todoapp` components in your `webapp` application. Example output:

   ```
   RESOURCE    KIND                          PROVISIONING_STATE   HEALTH_STATE
   todoapp     ContainerComponent            Deployed             Healthy
   db          mongodb.com.MongoDBComponent  Deployed             Healthy
   ```

1. To test the database, open a local tunnel on port 3000 again:

   ```sh
   rad resource expose todoapp --application webapp --port 3000
   ```

1. Visit the URL [http://localhost:3000](http://localhost:3000) in your browser. You should see a page like:

   <img src="todoapp-withdb.png" width="400" alt="screenshot of the todo application with a database">

   If your page matches, then it means that the container is able to communicate with the database. Just like before, you can test the features of the todo app. Add a task or two. Now your data is being stored in an actual database.

1. When you're done testing press CTRL+C to terminate the port-forward. 

### Validate data

After you have deployed the application, you can validate that the data is being stored in the database.

{{< tabs "Microsoft Azure" >}}

{{% codetab %}}
1. Open the Azure resource group where your application is deployed. The URL was output during the `rad deploy` command.

1. Open the CosmosDB resource prefixed with `db-`

   <img src="azure-db.png" width="400px" alt="Screenshot of the db CosmosDB instance">

1. Open the Data Explorer to the `todos` collection. You can now see the entries you added in the todo app.

   <img src="db-entries.png" width="800px" alt="Screenshot of the db CosmosDB Data Explorer with todo items">
{{% /codetab %}}

{{< /tabs >}}

## Cleanup

{{% alert title="Delete application" color="warning" %}} If you're done with testing, you can use the rad CLI to [delete an environment]({{< ref rad_env_delete.md >}}) to prevent additional charges in your subscription. {{% /alert %}}
