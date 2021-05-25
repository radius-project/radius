---
type: docs
title: "Add a database to the website tutorial app"
linkTitle: "Add a database"
description: "Connect a CosmosDB resource to the website tutorial application"
weight: 3000
---

So far you have not yet configured a database, so the todo items you enter will be stored in memory inside the application. If the website restarts then all of your data will be lost!

In this step you will learn how to add a database and connect to it from the application.

We'll discuss template.bicep changes and then provide the full, updated file before deployment. 

## Add db component
A `db` database component is used to specify a few properties about the database: 

- **kind:** `azure.com/CosmosDocumentDb@v1alpha1` represents a Cosmos DB database. 
- **managed:** `true` tells Radius to manage the lifetime of the component for you. 

```sh
  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDocumentDb@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
```

## Reference db from todoapp

Radius captures both logical relationships and related operational details. Examples of this include: wiring up connection strings, granting permissions, or restarting components when a dependency changes.

Once the database is defined as a component, you can connect to it by referencing the `db` component from within the `todoapp` component via a `dependsOn` section. 

The `dependsOn` section is used to configure relationships between a component and services provided by other components. The `db` is of kind `azure.com/CosmosDocumentDb@v1alpha1`, which supports the MongoDB protocol. `db` is considered to provide a service of kind `mongodb.com/Mongo` implicitly. Configuring a dependency on a service is the other part of specifying a relationship. This declares the *intention* from the `todoapp` component to communicate with the `db` using `mongodb.com/Mongo` as the protocol.

Here's what the todoapp component will look like with the `dependsOn` section added within its properties:

```sh
  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: { ... }
      dependsOn: [
        {
          name: 'kv'
          kind: 'azure.com/KeyVault'
          setEnv: {
            KV_URI: 'kvuri'
          }
        }
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
      ]
      provides: [ ... ]
    }
  }
```

The `setSecret` section declares the secrets to be created after creation of the `db` component. In this case the `connectionString` value will be retrieved from the database and set as a secret in the secret store named (`kv` here) with the secret name `DBCONNECTION`.

## Update your template.bicep file 

Update your `template.bicep` file to match the full application definition:

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

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
          name: 'kv'
          kind: 'azure.com/KeyVault'
          setEnv: {
            KV_URI: 'kvuri'
          }
        }
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

  resource kv 'Components' = {
    name: 'kv'
    kind: 'azure.com/KeyVault@v1alpha1'
    properties: {
        config: {
            managed: true
        }
    }
  }
}
```

## Deploy application with database

1. Now you are ready to re-deploy the application, including the Azure CosmosDB database. Switch to the command-line and run: 

   ```sh
   rad deploy template.bicep
   ```

   This may take a few minutes because of the time required to create the database.

1. You can confirm that the new `db` component was deployed by running:

   ```sh
   rad deployment list --application-name webapp
   ```

   You should see `kv`, `db` and `todoapp` components in your `webapp` application. Example output: 

   ```
   Using config file: /Users/{USER}/.rad/config.yaml
   {
     "value": [
       {
         "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/webapp/Deployments/default",
         "name": "default",
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

1. To test the database, open a local tunnel on port 3000 again:

   ```sh
   rad expose webapp todoapp --port 3000
   ```

1. Visit the URL [http://localhost:3000](http://localhost:3000) in your browser. You should see a page like:

   <img src="todoapp-withdb.png" width="400" alt="screenshot of the todo application with a database">

   If your page matches, then it means that the container is able to communicate with the database. Just like before, you can test the features of the todo app. Add a task or two. Now your data is being stored in an actual database.

1. When you're done testing press CTRL+C to terminate the port-forward. 

## Next steps

- To view the website application code used in this tutorial, download the [zipped application code](/tutorial/webapp.zip).
- If you'd like to try another tutorial with your existing environment, go back to the [Radius tutorials]({{< ref tutorial >}}) page. 
- If you're done with testing, use the rad CLI to [delete an environment]({{< ref rad_env_delete.md >}}) to **prevent additional charges in your subscription**. 

You have completed this tutorial!

<br>{{< button text="Try another tutorial" page="tutorial" >}}
