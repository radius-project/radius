---
type: docs
title: "Dapr Hello World application"
linkTitle: "Dapr"
description: "Sample application running with Dapr"
weight: 50
---

Based on: https://github.com/dapr/samples/tree/master/hello-docker-compose

## Deploying to "local" RP

You need to set some environment variables for the hostname and password to use with redis.

- `REDIS_HOST`
- `REDIS_PASSWORD`

Use the HTTPS URL - this configuration is set up for an Azure-hosted redis instance with HTTPS.

## Bicep file

{{< rad file="azure-bicep/template.bicep">}}