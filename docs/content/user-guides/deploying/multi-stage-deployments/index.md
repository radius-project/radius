---
type: docs
title: "Multi-stage deployments"
linkTitle: "Multi-stage deployments"
description: "Learn how to deploy Radius applications in multiple stages with the rad CLI"
weight: 100
---

Radius has built-in support for multi-stage application deployments. Configure stages, profiles, and parameters to control the deployment process of your application.

## Initialize a new application

Run [`rad application init`]({{< ref rad_application_init >}}) to initialize a new application in your current directory. This will create [`rad.yaml`]({{< ref rad-yaml >}}), `iac/infra.bicep`, and `iac/app.bicep` files:

```sh
> rad app init -a my-app
Initializing Application my-app...

        Created rad.yaml
        Created iac/infra.bicep
        Created iac/app.bicep

Have a RAD time 😎
```

Open `rad.yaml` to see a a basic application definition:

```yaml
name: my-app
stages:
- name: infra
  bicep:
    template: iac/infra.bicep
- name: app
  bicep:
    template: iac/app.bicep
```

## Deploy individual stages

### Deploy infra

[Stages]({{< ref "rad-yaml#stages" >}}) define individual steps in your application deployment. Each stage can have a [`bicep`]({{< ref bicep >}}) template with the resources to be deployed.

Once you have an [environment initialized]({{< ref create-environment >}}) and an application created from above, you can deploy each stage of your application to the environment:

```sh
> rad application deploy infra
Reading rad.yaml...
Using environment my-env

Processing stage infra: 1 of 1
Building iac/infra.bicep...
Deploying iac/infra.bicep...


Deployed stage infra: 1 of 1
```

Note that there are no resources within `iac/infra.bicep` yet so nothing is deployed.

### Deploy app

You can run the full deployment of infra and app with the `rad application deploy` command. This will run all stages in the order they are defined in `rad.yaml`.

```sh
> rad application deploy
Reading rad.yaml...
Using environment my-env

Processing stage infra: 1 of 2
Building iac/infra.bicep...
Deploying iac/infra.bicep...


Deployed stage infra: 1 of 2


Processing stage app: 2 of 2
Building iac/app.bicep...
Deploying iac/app.bicep...


Completed            my-app          Application
Completed            web             HttpRoute
Completed            demo            Container

Deployed stage app: 2 of 2

Resources:
    my-app          Application
    demo            Container
    web             HttpRoute

Public Endpoints:
    web             HttpRoute            http://IP-ADDRESS
```

The `iac/app.bicep` file contained a single container and gateway, resulting in the [todo app]({{< ref webapp >}}) being deployed.

## Add a prod profile

[Profiles]({{< ref "rad-yaml#profiles" >}}) define parameters and Bicep files that can override the default behavior of a stage. For example, you have have separate profiles for dev, pre-prod, and prod environments.

Create a new file named `iac/app.prod.bicep`. Add update it to the following:

{{< rad file="snippets/app.prod.bicep" embed=true >}}

This adds a Mongo Database to the application using a [starter](#TODO), and defines a connection from the container to the database.

Update your `rad.yaml` file to include a new profile under the infra stage named 'prod':

```yaml
name: my-app
stages:
- name: infra
  bicep:
    template: iac/infra.bicep
- name: app
  bicep:
    template: iac/app.bicep
  profiles:
    prod:
      bicep:
        template: iac/app.prod.bicep
```

Now re-run your application deployment with the new profile:

```sh
> rad application deploy --profile prod
Reading rad.yaml...
Using environment radius-rg-nvmf2

Processing stage infra: 1 of 2
Building iac/infra.bicep...
Deploying iac/infra.bicep...


Deployed stage infra: 1 of 2


Processing stage app: 2 of 2
Building iac/app.prod.bicep...
Deploying iac/app.prod.bicep...

Resources:
    my-app                  Application
    container-mongo-UNIQUE Container
    demo                    Container
    mongo-route             HttpRoute
    web                     HttpRoute
    mongo-UNIQUE            mongo.com.MongoDatabase

Public Endpoints:
    web                     HttpRoute            http://IP-ADDRESS
```

You can see that the `iac/app.prod.bicep` file has been deployed instead of `iac/app.bicep`, resulting in a Mongo container being deployed in addition to the demo container.

## Configure a build

A `rad.yaml` file can also [build and push Docker containers]({{< ref "rad.yaml#Docker" >}}) as part of the deployment process.

Update your `rad.yaml` file to include a new build step:

```yaml
name: my-app
stages:
- name: infra
  bicep:
    template: iac/infra.bicep
- name: app
  build:
    tutorial_build:
      docker:
        context: todoapp
        image: myregistry/webapptutorial-todoapp:latest
  bicep:
    template: iac/app.bicep
  profiles:
    prod:
      bicep:
        template: iac/app.prod.bicep
```

The next time you run `rad application deploy`, the `tutorial_build` step will be run. This will build the Docker image from the `todoapp` directory and push it to your registry (code samples coming soon).

## Additional resources

- [rad.yaml reference]({{< ref rad-yaml >}})
- [rad CLI overview]({{< ref rad-cli >}})
- [rad CLI reference]({{< ref cli >}})
