---
type: docs
title: "Radius components"
linkTitle: "Components"
description: "Learn how to model your application pieces with Radius components."
weight: 200
---

## dependsOn

The `dependsOn` property tells Radius what relationships exist between the different components in your application. Without any supplemental information, a `dependsOn` relationship tells Radius in what order to deploy the resources. With additional configuration, Radis can set environment variables, place secrets within secret stores, and add additional intelligence to your application.

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of the other component to depend on. | `kv.name`
| kind | y | The service on which you depend. Can be the same as the component kind, or an abstract service kind. | `mongodb.com/Mongo`
| setEnv | | List of key/value pairs which Radius will inject into the compute component runtime.  | `KV_URI: 'keyvaulturi'`
| setSecret | | List of key/value pairs which Radius will inject into the secret store component. | `DBCONNECTION: 'connectionString'`
