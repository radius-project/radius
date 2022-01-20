---
type: docs
title: "Microsoft SQL Server database"
linkTitle: "Microsoft SQL"
description: "Sample application running on a user-managed Azure SQL Database"
weight: 100
---

This application showcases how Radius can use a user-manged Azure SQL Database. 


## Platform resources

| Platform                             | Resource                                                       |
| ------------------------------------ | -------------------------------------------------------------- |
| [Microsoft Azure]({{< ref azure>}})  | [Azure SQL](https://docs.microsoft.com/en-us/azure/azure-sql/) |
| [Kubernetes]({{< ref kubernetes >}}) | Not compatible                                                 |

## Configuration

| Property | Description                                                                         | Example(s)         |
| -------- | ----------------------------------------------------------------------------------- | ------------------ |
| managed  | Indicates if the resource is Radius-managed. If no, a `Resource` must be specified. | `false`            |
| resource | The ID of the user-managed SQL Database to use for this Component.                  | `server::sqldb.id` |

## Resource lifecycle

An `microsoft.com/SQLComponent` can be Radius-managed. At this Radius' support for Microsoft SQL Server compatible database is limited to user-managed resources. For more information read the [Components docs]({{< ref "components-model#resource-lifecycle" >}}).

This sample uses Bicep parameters to pass the resource ID of the database as well as the username and password.

{{< rad file="snippets/unmanaged.bicep" embed=true marker="//PARAMETERS" >}}

Pass the ID of the database into the component:

{{< rad file="snippets/unmanaged.bicep" embed=true marker="//DATABASE" >}}

Radius does not have access to the username and password used to access your database. You should provide this when building a connection string in Bicep.

{{< rad file="snippets/unmanaged.bicep" embed=true marker="//CONTAINER" >}}

## Injected Values

Connections between components declare environment variables inside the consuming component as a convenience for service discovery. See [connections]({{< ref "connections-model#injected-values" >}}) for details.

In the following example, a `todoapp` service connects to a database `db`. The connection is defined as part of `todoapp` and is named `tododb`.

{{< rad file="snippets/unmanaged.bicep" embed=true marker="//CONTAINER" >}}

This example would define the following injected environment variables for use inside `todoapp`:

| Environment Variable         | Example Value                   | Description                                          |
| ---------------------------- | ------------------------------- | ---------------------------------------------------- |
| `CONNECTION_TODODB_SERVER`   | `myserver.database.windows.net` | The fully-qualified hostname of the database server. |
| `CONNECTION_ORDERS_DATABASE` | `todos`                         | The name of the SQL Server database.                 |
