---
type: docs
title: "Add a secret store to the website tutorial app"
linkTitle: "Add a secret store"
description: "Connect an Azure Key Vault resource to the website tutorial application"
weight: 3000
---

In this step you will learn how to add a secret store and connect to it from the application and retrieve secrets.

We'll discuss template.bicep changes and then provide the full, updated file before deployment. 

## Add kv component
A `kv` secret store component is used to specify a few properties about the database: 

- **kind:** `azure.com/KeyVault@v1alpha1` represents an Azure Key Vault. 
- **managed:** `true` tells Radius to manage the lifetime of the component for you. 

```sh
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

## Reference db from todoapp

Radius captures both logical relationships and related operational details. Examples of this include: wiring up connection strings, granting permissions, or restarting components when a dependency changes.

Once the secret store is defined as a component, you can connect to it by referencing the `kv` component from within the `todoapp` component via a `dependsOn` section. 

The `dependsOn` section is used to configure relationships between a component and services provided by other components. The `kv` is of kind `azure.com/AzureKeyVault@v1alpha1`.
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
      ]
      provides: [ ... ]
    }
  }
```

The `setEnv` section declares operations to perform *based on* the relationship. In this case the `kvuri` value will be retrieved from the key vault and set as an environment variable on the component. As a result, `todoapp` will be able to use the `KV_URI` environment variable to access to the key vault.

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

## Next steps

Next, we will add a database to the todoapp and store the connection string for this database in the secret store we just created

<br>{{< button text="Next: Add a database to the app" page="webapp-add-database.md" >}}

