---
type: docs
title: "Azure CosmosDB Mongo"
linkTitle: "Azure CosmosDB Mongo"
description: "Sample application running MongoDB through Azure CosmosDB API"
---

This application showcases how Radius can use an Azure CosmosDB API for MongoDB in two different scenarios.

## Using a Radius-managed CosmosDB

This example sets the property `managed: true` for the CosmosDB component. When `managed` is set to true, Radius will manage the lifecycle of the underlying database account and database.

{{< rad file="managed.bicep">}}

## Using a user-managed CosmosDB

This example sets the `resource` property to a CosmosDB Mongo database. Setting `managed: false` or using the default value allows you to explicitly specify a link to an Azure resource that you manage. When you supply your own `resource` value, Radius will not change or delete the resource you provide. 

In this example the CosmosDB resources are configured as part of the same `.bicep` template.

{{< rad file="unmanaged.bicep">}}