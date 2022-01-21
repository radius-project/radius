---
type: docs
title: "Add cross-platform connectors to your Radius application"
linkTitle: "Connectors"
description: "Learn how to model and deploy portable resources with Radius connectors"
weight: 300
---

Connectors provide **abstraction** and **portability** to Radius applications. This allows developement teams to depend on high level resource types and APIs, and let infra teams swap out the underlying resource and configuration.

<img src="connectors.png" alt="Diagram of a connector connecting from a container to either an Azure Redis Cache or a Kubernetes Deployment" width=700px />

For example, instead of developers writing two definitions of their service, one depending on Azure Redis Cache and one depending on a Redis container, they can instead depend on a Redis connector and swap out the supporting infrastructure between the two types of Redis.

## Connector categories

Check out the Radius connector library to begin authoring portable applications:
