---
type: docs
title: "Microsoft SQL Server database"
linkTitle: "Microsoft SQL"
description: "Sample application running on a user-managed Azure SQL Database"
weight: 100
---

This application showcases how Radius can use a user-manged Azure SQL Database.

## Supported resources

- [Azure SQL Container](https://hub.docker.com/_/microsoft-mssql-server)
- [Azure SQL](https://docs.microsoft.com/en-us/azure/azure-sql/)

## Resource format

{{< rad file="snippets/unmanaged.bicep" embed=true marker="//DATABASE" >}}

| Property | Description | Example |
|----------|-------------|---------|
| resource | The ID of the Azure SQL Database to use for this resource. | `server::sqldb.id` |
| server | The name of the server. | `myserver` |
| database | The fully qualified domain name of the SQL database. | `myserver.database.com` |

## Connections

You can connect your [service]({{< ref services >}}) to your SQL database through the connector.

The username and password used to access your database are not stored as part of the connector. You should provide this via paramaters or variables when building a connection string in Bicep.

{{< rad file="snippets/unmanaged.bicep" embed=true marker="//CONTAINER" >}}

### Injected values

Connections between resources declare environment variables inside the consuming resource as a convenience for service discovery. See [connections]({{< ref "connections-model#injected-values" >}}) for details.

| Environment variable | Example | Description |
|----------------------|---------|-------------|
| `CONNECTION_TODODB_SERVER` | `myserver.database.windows.net` | The fully-qualified hostname of the database server.
| `CONNECTION_ORDERS_DATABASE` | `mydb` | The name of the SQL Server database.

## Starter

You can get up and running quickly with a SQL Database by using a [starter]({{< ref starter-templates >}}):

{{< rad file="snippets/starter.bicep" embed=true >}}

### Container

The SQL Database starter uses a SQL container and can run on any Radius platform.

```
br:radius.azurecr.io/starters/sql:latest
```

#### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the SQL Database | Yes | - |
| adminPassword | The password for the SQL Server administrator | Yes | - |
| serverName | The name of the Azure SQL Server | No | `'sql-${uniqueString(resourceGroup().id, deployment().name)}'` |

#### Output parameters

| Parameter | Description | Type |
|-----------|-------------|------|
| sqlDB | The SQL Database resource | `microsoft.com.SQLDatabase@v1alpha3` |
| sqlUsername | The username for the SQL Server. Always 'sa'. | string |

### Microsoft Azure

The SQL Database Azure starter uses an Azure SQL Server and can run only on Azure.

```
br:radius.azurecr.io/starters/sql-azure:latest
```

#### Input parameters

| Parameter | Description | Required | Default |
|-----------|-------------|:--------:|---------|
| radiusApplication | The application resource to use as the parent of the SQL Database | Yes | - |
| adminLogin | The username for the SQL Server administrator | Yes | - |
| adminPassword | The password for the SQL Server administrator | Yes | - |
| serverName | The name of the Azure SQL Server | No | `'sql-${uniqueString(resourceGroup().id, deployment().name)}'` |
| databaseName | The name for your SQL Database. Defaults to the module name. | No | `deployment().name` |
| location | The Azure region to deploy the Azure SQL Server | No | `resourceGroup().location` |
| skuName | The Azure SQL Server SKU | No | `'Standard'` |
| skuTier | The Azure SQL Server SKU tier | No | `'Standard'` |

### Output parameters

| Resource | Description | Type |
|----------|-------------|------|
| sqlDB | The SQL Database resource | `microsoft.com.SQLDatabase@v1alpha3` |
| sqlUsername | The username for the SQL Server. | string |

