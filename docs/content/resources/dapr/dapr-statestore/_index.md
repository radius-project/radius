---
type: docs
title: "Use Dapr State Store with Radius"
linkTitle: "Dapr StateStore"
description: "Learn how to use Dapr State Store components in Radius"
weight: 300
slug: "statestore"
---

## Overview

The `dapr.io/StateStore` component represents a [Dapr state store](https://docs.dapr.io/developing-applications/building-blocks/state-management/) topic.

This component will automatically:
- Ensure the Dapr control plane is initialized
- Deploy and manage the underlying resource
- Create and deploy the Dapr component spec

## Platform resources

The following resources can act as a `dapr.io.StateStore` resource:

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | [Azure Table Storage](https://docs.microsoft.com/en-us/azure/storage/tables/table-storage-overview)
| [Microsoft Azure]({{< ref azure>}}) | SQL Server
| [Microsoft Azure]({{< ref azure>}}) | Generic
| [Kubernetes]({{< ref kubernetes >}}) | Redis
| [Kubernetes]({{< ref kubernetes >}}) | Generic