---
type: docs
title: "Web App With Key Vault Tutorial"
linkTitle: "Web app + database + key vault"
description: "Learn Project Radius by authoring templates and deploying a working web application with a database and keyvault."
weight: 100
---

## Before you begin

This tutorial can be treated as an extension to the Web App tutorial which shows how to deploy a working web application that accesses a database. In this tutorial, you will learn how to add a Key Vault in this application.

It is recommended to walk through the [Web App Tutorial](../webapp/index.md) before you begin.

## Prerequisites

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create a Radius environment]({{< ref create-environment.md >}})
- [(recommended) Install Visual Studio Code](https://code.visualstudio.com/)
   - The [Radius VSCode extension]({{< ref "install-cli.md#2-install-custom-vscode-extension" >}}) provides syntax highlighting, completion, and linting.
   - You can also complete this tutorial with any basic text editor.

## Overview of the tutorial application

You will be deploying a *To-Do List* web application. It will have three Radius *components*:

- A containerized web application written in Node.JS ("webapp")
- An Azure CosmosDB database ("db")
- An Azure KeyVault ("kv")

We will be storing the connection string to the database as a secret in the Key Vault. The web application will then access this secret and use the value to connect to the database.

Let's start with the bicep template that we used in the web app tutorial:-
```
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp-kv'

  resource todoapplication 'Components' = {
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

## Step 1: Adding a Key Vault

Let's add the Key Vault component to this bicep file:-
```
resource kv 'Components' = {
    name: 'kv'
    kind: 'azure.com/KeyVault@v1alpha1'
    properties: {
        config: {
            managed: true
        }
    }
  }
```

## Step 2: Adding a dependency in the container component on the KeyVault

The web application needs to access the KeyVault and we therefore add a dependency as below in the container component:-
```
{
  name: 'kv'
  kind: 'azure.com/KeyVault'
  setEnv: {
    KV_URI: 'kvuri'
  }
}
```

## Step 3: Adding database connection string as a secret to the KeyVault

In the web app tutorial, the container accessed the database connection string in clear text as an environment variable and we therefore defined the database dependency for the container as below:-
```
dependsOn: [
{
  kind: 'mongodb.com/Mongo'
  name: 'db'
  setEnv: {
    DB_CONNECTION: 'connectionString'
  }
}
```

We no longer want the container application to use the database connection string in clear text and need to access it as a secret from the KeyVault.  For this, we will change the database dependency section in the container component as below:-
```
{
  kind: 'mongodb.com/Mongo'
  name: 'db'
  setSecret: {
    store: kv.name
    keys: {
      DBCONNECTION: 'connectionString'
    }
  }
}
```

Note the "setSecret" section which tells Radius to use the secret store specified and the secret to create is `DBCONNECTION` with its value set to the database connection string.

## Step 4: Modify the application to use the KeyVault

Now modify the container application code to access the KeyVault to fetch the database connection string. You can use the new docker image: radiusteam/tutorial-webappkv which has the modified code.


### Deploy application with database and keyvault

Now you are ready to deploy.

1. Switch to the command-line and run the following command.

   ```sh
   rad deploy template.bicep
   ```

   This will deploy the application, including the Azure CosmosDB database and Azure KeyVault. This may take a few minutes because of the extra time required to create the database.

1. Confirm that the database was deployed.

   ```sh
   rad deployment list --application-name webapp-kv
   ```

   Now you should see `db`, `kv` and `todoapp` components in your `webapp` application, similar to:

   ```sh
   Using config file: /Users/{USER}/.rad/config.yaml
   {
     "value": [
       {
         "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/webapp/Deployments/default",
         "name": "radius/webapp/default",
         "type": "Microsoft.CustomProviders/resourceProviders/Applications/Deployments",
         "properties": {
           "components": [
             {
               "componentName": "db"
             },
             {
               "componentName": "kv"
             },
             {
               "componentName": "todoapp"
             }
           ]
         }
       }
     ]
   }
   ```

1. To test out the database, open a local tunnel on port 3000 again:

   ```sh
   rad expose webapp todoapp --port 3000
   ```

1. Visit the URL `http://localhost:3000` in your browser. You should see a page like:

   <img src="todoapp-withdb.png" width="400" alt="screenshot of the todo application with a database">

   If your page matches, then it means that the container is able to communicate with the database. Just like before, you can test the features of the todo app. Add a task or two. Now your data is being stored in an actual database. 

1. When you are done testing press CTRL+C to terminate the port-forward. 

## Step 5: Cleanup

If you'd like to try another tutorial with your existing environment, go back to the [Radius tutorials]({{< ref tutorial >}}) page. 

If you're done with testing, clean up your environment to **prevent additional charges in your subscription**. 

Deleting an environment will delete:

- Your Radius environment
- The related resource group
- The application you just deployed

```sh
rad env delete --name azure --yes
```

You have completed this tutorial!

## Related links

- View the full template.bicep for this tutorial [here](https://github.com/Azure/radius/blob/main/docs/content/getting-started/tutorial/webapp-kv/template.bicep)