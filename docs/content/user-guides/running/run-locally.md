---
type: docs
title: "Run a Radius application locally"
linkTitle: "Run app locally"
description: "How to run a Radius application locally"
weight: 100
---

This guide will get you up and running with a local Radius environment and sample application.

## Pre-requisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop)
- [k3d](https://k3d.io/)
- [rad CLI]({{< ref rad-cli >}})

## Initialize a local environment

Begin by initializing a local environment with the [`rad env init dev` command]({{< ref rad_env_init_dev >}}):

```sh
> rad env init dev
Creating Cluster...
Installing Radius...
Installing new Radius Kubernetes environment to namespace: radius-system
Successfully wrote configuration to C:\Users\USER\.rad\config.yaml
```

Validate that the k3d cluster and registry were created:

```sh
> rad env status
NODES        REGISTRY         INGRESS (HTTP)          INGRESS (HTTPS)
Ready (2/2)  localhost:62285  http://localhost:62287  https://localhost:62288
```

## Initialize an application

Create a new Radius application with the [`rad app init` command]({{< ref rad_application_init >}}):

```sh
> rad app init -a myapp
Initializing Application myapp...

        Created rad.yaml
        Created iac/infra.bicep
        Created iac/app.bicep

Have a RAD time 😎
```

For more information on this app refer to the [multi-stage deployments guide]({{< ref multi-stage-deployments >}}).

## Run your application

Run your app locally with the [`rad app run` command]({{< ref rad_application_run >}}):

```sh
rad app run
```

{{% alert title="Temporary" color="warning" %}}
Visit the IP address for INGRESS (HTTP) that is output from `rad env status`. The IP address output from `rad app run` is incorrect.
{{% /alert %}}

## Add a build step

Coming soon!
