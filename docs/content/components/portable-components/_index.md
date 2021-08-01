---
type: docs
title: "Portable components"
linkTitle: "Portable"
description: "Components that integrate Radius with a variety of OSS and other vendor-neutral services"
weight: 100
---

Radius offers components that can work across any hosting model and will be satisfied using the best means available by the host. These are called *portable components* because application descriptions that use them can be *portable* across hosts without any configuration changes. Portable components are generally OSS services that are not tied to any particular SaaS or hosting platform and usually have multiple implementations.

For example the kind `mongodb.com/Mongo@v1alpha1` specifies a generic MongoDB-compatible database. From the point-of-view of application code, it does not matter if the database is hosted using Kubernetes primitives like a `StatefulSet`, or a MongoDB operator, or a cloud-provider hosted offering like Azure CosmosDB. Radius will provision (or connect to) the appropriate implementation depending on the environment where the application is deployed.