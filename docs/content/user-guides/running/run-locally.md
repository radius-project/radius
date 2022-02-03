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
> docker ps
CONTAINER ID   IMAGE                      COMMAND                  CREATED         STATUS         PORTS                                                                    NAMES
6eebee6b7f68   rancher/k3d-proxy:5.3.0    "/bin/sh -c nginx-pr…"   3 minutes ago   Up 3 minutes   0.0.0.0:58254->80/tcp, 0.0.0.0:58253->443/tcp, 0.0.0.0:58175->6443/tcp   k3d-radius-dev-serverlb
761f8aa83422   rancher/k3s:v1.22.6-k3s1   "/bin/k3s server --d…"   3 minutes ago   Up 3 minutes                                                                            k3d-radius-dev-server-0
da320bb45081   registry:2                 "/entrypoint.sh /etc…"   4 minutes ago   Up 3 minutes   0.0.0.0:58176->5000/tcp                                                  radius-dev-registry

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

Visit the IP address provided to visit the example application.

## Add a build step

Coming soon!
