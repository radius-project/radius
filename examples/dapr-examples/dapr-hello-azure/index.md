---
type: docs
title: "Dapr Hello World on Azure"
linkTitle: "Dapr on Azure"
description: "Sample application running with Dapr on Azure"
weight: 50
---

Based on: https://github.com/dapr/samples/tree/master/hello-docker-compose

## Deploying to "local" RP

You'll need to set `ARM_SUBSCRIPTION_ID` and `ARM_RESOURCE_GROUP` - this example uses Azure resources.

## Bicep file

{{< rad file="template.bicep">}}