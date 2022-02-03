---
type: docs
title: "Radius application YAML definition reference"
linkTitle: "rad.yaml"
description: "Detailed reference documentation on the rad.yaml application file"
weight: 200
---

The `rad.yaml` file configures application [multi-stage deployment]({{< ref multi-stage-deployments >}}). The commmand [`rad app init`]({{< ref rad_application_init >}}) creates `rad.yaml` for you as part of app initialization. It lives at the root of your application directory/repo and contains the following sections:

| Property | Description | Example |
|----------|-------------|---------|
| name | The name of the application. | `my-app` |
| stages | A list of stages. | See [Stages](#stages) |

## Stages

Stages define an ordered set of steps to take in the deployment of an application.

| Property | Description | Example |
|----------|-------------|---------|
| name | The name of the stage. | `infra` |
| bicep | Details on a [Bicep template](#bicep-templates) to be deployed. | See [Bicep](#bicep) |
| build | Details on a build to run prior to a deployment. | See [Build](#build) |
| profiles | A list of profiles that can be specified for this stage. | See [Profiles](#profiles) |

### Bicep

Bicep defines a Bicep file and optional parameters to deploy to a Radius environment.

| Property | Description | Example |
|----------|-------------|---------|
| template | The name of the Bicep template to deploy. | `iac/infra.bicep` |
| parameters | A list of parameters to pass to the Bicep template. Can be defined inline or a reference to a parameter JSON file. | `param1: value1` |
| parametersFile | A reference to a parameter JSON file. | `iac/parameters.json` |

### Build

Build defines a list of builds to run prior to a deployment.

#### Docker

Docker defines a Docker build to run prior to a deployment.

| Property | Description | Example |
|----------|-------------|---------|
| context | The directory to run the Docker build in. | `serviceA` |
| image | The name of the Docker image to build. | `myregistry/myimage` |
| dockerfile | The name of the Dockerfile to use. | `Dockerfile` |

### Profiles

Profiles define a set of overrides for a stage. Both build and bicep configuration can be overridden in a profile, with the profile merging into the default and overwriting any properties set in both locations. When running a deployment, a profile can be specified as part of the command:

```sh
rad app deploy --profile dev
```

All of the properties of a stage can be overridden by a profile.

| Property | Description | Example |
|----------|-------------|---------|
| bicep | Details on a [Bicep template](#bicep-templates) to be deployed. | See [Bicep](#bicep) |
| build | Details on a build to run prior to a deployment. | See [Build](#build) |
| profiles | A list of profiles that can be specified for this stage. | See [Profiles](#profiles) |

## Example

```yaml
name: my-app
stages:
- name: infra
  bicep:
    template: iac/infra.bicep
  profiles:
    dev:
      bicep:
        template: iac/infra.dev.bicep
- name: app
  build:
    node_service_build:
      docker:
        context: node-service
        image: myregistry/node-service
  bicep:
    template: iac/app.bicep
    parameters:
      param1: value1
  profiles:
    dev:
      build:
        node_service_build:
          docker:
            dockerFile: Dockerfile.dev
```
