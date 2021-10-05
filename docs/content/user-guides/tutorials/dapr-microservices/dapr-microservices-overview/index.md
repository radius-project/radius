---
type: docs
title: "Dapr microservices tutorial app overview"
linkTitle: "App overview"
slug: "overview"
description: "Overview of the Dapr microservices tutorial application"
weight: 1000
---

In this tutorial you will be deploying an online store where you an order items:

<img src="./store.png" alt="A screenshot of the store application" width=500 />

## Components

This Radius application will have three [Components]({{< ref components-model.md >}}):

- A UI for users to place orders written with .NET Blazor (`frontend`)
- A backend order processing microservice written in Node.JS (`backend`)
- A Dapr state store used to store the orders (`statestore`)

### Frontend

The user-facing UI app (`frontend`) offers a portal for users to place orders. Upon creating an order, `frontend` uses [Dapr service invocation](https://docs.dapr.io/developing-applications/building-blocks/service-invocation/service-invocation-overview/) to send requests to `nodeapp`.

<img src="./frontend.png" alt="A diagram of the complete application" width=400 />

### Backend

The order processing microservice (`backend`) accepts HTTP requests to create or display orders. It accepts HTTP requests on two endpoints: `GET /order` and `POST /neworder`.

<img src="./backend.png" alt="A diagram of the backend order processing service" width=600 />

### Statestore

The Dapr state store (`statestore`) stores information about orders. It could be any compatible [Dapr state store](https://docs.dapr.io/developing-applications/building-blocks/state-management/state-management-overview/). In this tutorial we will use Azure Table Storage for Azure environments and Redis for Kubernetes environments.

<img src="./statestore.png" alt="A diagram of the Dapr state store" width=400 />

## Routes

Radius offers communication between runnable Components via [Communication Routes]({{< ref "connections-model#routes" >}}).

### Dapr service invocation

In this tutorial, we will be using `dapr.io.DaprHttpRoute` to model the Dapr sidecar running alongside `frontend` and `backend`. This allows `frontend` to use Dapr service invocation to interact with `backend`.

<img src="./invoke.png" alt="A diagram of the Dapr service invocation" width=500 />

## Summary

In this tutorial, you will learn how Radius offers:

- Services like data components (`statestore` here) as part of the application
- Relationships between components that are fully specified with protocols and other strongly-typed information

In addition to this high level information, the Radius model also uses typical details like:

- Container images
- Listening ports
- Configuration like connection strings

Keep the diagram in mind as you proceed through the following steps. Your Radius deployment template will aim to match it.

<br>{{< button text="Next: Deploy the application frontend" page="dapr-microservices-initial-deployment.md" >}}
