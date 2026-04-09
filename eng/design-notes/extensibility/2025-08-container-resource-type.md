# Container Resource Type Definition

* **Author**: Zach Casper (@zachcasper)

## Overview

The compute extensibility project is implementing Recipe-backed Resource Types for the core Radius Resource Types including Containers, Volumes, Secrets, and Gateways. As part of this effort, Recipes are being developed replacing the imperative code in Applications RP. Because of this, we are taking the opportunity to re-examine the schema and make adjustments as needed. 

## Objectives

The objective of this document is to define version two of the Containers Resource Type. 

### Goals

* Ensure that the Containers Resource Type feels familiar with both developers and platform engineers with experience with Kubernetes. As such, the Containers Resource Type definition is Kubernetes-first.
* Enable developers to use the Containers Resource Type for the vast majority of their use cases without platform engineers needing to modify the Resource Type definition. Concretely, all developer-oriented properties of the Kubernetes Pod and Deployment resources are available in Containers.
* Ensure Containers is modeled such that other container platforms can be implemented via Recipes in the future. This includes AWS ECS, Azure Container Apps, Azure Container Instances, and Google Cloud Run. Containers property names should be similar to other platforms as much as possible.
* Any modifications to the Containers Resource Type should not drive changes within Radius itself. All changes should only affect the implementation of the Containers Recipe.

### Non goals

This document is focused solely on the Containers Resource Type definition. Volumes, Secrets, and Gateways definitions are discussed elsewhere and Recipes are not addressed.

## Limitations of `Applications.Core/containers`

The current version of the Containers Resource Type has several limitations:

### Supports only one container

The Containers Resource Type has a single `container` property. This is in stark contrast to all other container platforms which accept an array of containers. Multiple containers within a larger construct (such as a a Kubernetes Pod or ECS Task) is quite common. Observability tools such as Datadog offer sidecar containers. Container security tools such as Aqua Security are also used as sidecars.  These are platform engineering examples, but the existence of multiple containers in Kubernetes and other platforms allows developers to use this pattern in their applications as well.

### Does not support init containers

Since Containers does not support multiple containers, obviously it does not support any type of sequencing or startup dependencies. Kubernetes, ACA, ACI, and Cloud Run all support init containers. ECS has a more flexible model and offers the ability to specify arbitrary dependencies on containers within a Task.

Init containers are common for container preparation and setup. For example:

* Create a httpconf file based on environment variables
* Validate database schema and apply DML if needed
* Download data such as static assets for a website

### Does not support resource requests and limits

Containers has no properties for specifying CPU and memory requirements. All other container platforms support at least requests and most support setting limits (ECS is the exception). Most other platforms have enhanced the resource types to include GPUs for AI workloads which is a potential future Radius enhancement.

### Does not support autoscaling

Containers only offers the ability to specify a fixed number of replicas. There is no support for developers to define autoscaling metrics which the Recipe could use to configure autoscaling.

## Proposed New Capabilities

### Addition 1: The `containers` property is now a map

The Containers Resource Type will have a `containers` map property instead of a `container`. Multiple containers is supported by all container platforms:

* Kubernetes: `pod.spec.containers[]`
* ECS Task: `TaskDefinition.containerDefinitions[]`
* ACI Container Group: `containerGroups.containers[]`
* ACA Container App: `containerApps.template.container[]`
* Google Cloud Run: `Service.spec.template.spec.containers[]`

A developer, for example, could create a Container via:

```yaml
resource myContainer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'myContainer'
  properties: {
    environment: environment
    application: myApplication.id
    containers: {
      frontend: {
        image: 'frontend:latest'
      }
      sidecar: {
        image: 'sidecar:latest'
      }
    }
  }
}
```

The implications of this are not as impactful as one may think:

* The application graph does not change. Resources are still connected to the parent Containers resource. Environment variables are still created in each of the containers just as they do today in the single container (unless `disableDefaultEnvVars` is true).
* A Kubernetes Service is created for each container that exposes a container port just as today. The only difference is that now, multiple Services may be created, one for each container port.
* Volumes is refactored which will enable storage sharing between containers.

> [!CAUTION]
>
> The inclusion of multiple containers raises the question of naming of Containers. As you can see in this document, great pain has been taken to refer to the Containers Resource Type and the `containers` property distinctly. This is why Kubernetes has the Pod term, ECS has the Task term, ACA has the Application term, ACI has the Container Group term, and Cloud Run has the Service and Job terms. No change is being proposed today, but Radius is an exception amongst its peers which may be a caution sign.

### Addition 2: Addition of init containers

The Containers Resource Type will have an `initContainers` property. This is only a special instance of multiple containers.

Given an init container is just a container, this highlights one of the limitations of today's Resource Type definition YAML implementation. To model both a container and an initContainer, the schema for each of these properties must be duplicated. This makes the YAML file very unwieldy. See [Feature request: Support referring to existing object schemas in Resource Type definition YAML files #10276](https://github.com/radius-project/radius/issues/10276).

A developer could, for example:

```yaml
resource myContainer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'myContainer'
  properties: {
    environment: environment
    application: myApplication.id
    containers: {
      frontend: {
        image: 'frontend:latest'
      }
    }
    initContainers: {
      config: {
        image: 'init:latest'
      }
    }
  }
}
```



### Addition 3: New resource request and limits properties

The Containers Resource Type does not have the ability to set CPU and memory request and limits. These are added on the `containers` property.

The impact of this on the Recipe is minimal. These property are one-to-one match and no special testing is needed.

```yaml
resources:
  type: object
  description: (Optional) Compute resource requirements for the container.
  properties:
    requests:
      type: object
      description: (Optional) Requests define the minimum amount of CPU or memory that is required by the container.
      properties:
        cpu:
          type: float
          description: (Optional) The minimum number of vCPUs required by the container. `0.1` results in one tenth of a vCPU being reserved.
        memoryInMib:
          type: integer
          description: (Optional) The minimum amount of memory required by the container in MiB. `1024` results in 1 GiB of memory being reserved.
      limits:
        cpu:
          type: float
          description: (Optional) The maximum number of vCPUs which can be used by the container.
        memoryInMib:
          type: integer
          description: (Optional) The maximum amount of memory which can be used by the container in MiB.
```

### Addition 4: New autoscaling rules

The Containers Resource Type only has a replicas property (`extensions.manualScaling.replicas`) and no way to specify autoscaling. Containers will have a new `autoscaling` property.

This new property will require the recipe to configure autoscaling. On Kubernetes, this entails creating a horizontal pod autoscaler. The Recipe design will address whether this new functionality is implemented now or in the future.

```yaml
autoScaling:
  type: object
  properties:
    maxReplicas:
      type: integer
      description: (Optional) The maximum number of replicas for the autoscaler.
    metric:
      type: object
      description: (Required) The metric to measure and target used to autoscale. 
      additionalProperties:
        kind:
          type: string
          enum: [cpu, memory, custom]
          description: (Required) The metric to measure. 
        customMetric: 
          type: string
          description: (Optional) The custom metric exposed by the application. Implementation specific. See platform engineer for further guidance.
        target:
          type: object
          description: (Required) When the metric exceeds the target value specified, autoscaling is triggered. Only one target value can be specified dependent upon the type.
          properties:
            averageUtilization:
              type: integer
              description: (Optional) The average CPU or memory utilization across all containers expressed as a percentage. Kind must be CPU or memory.
            averageValue:
              type: integer
              description: (Optional) The average value of the metric as a quantity.
            value:
              type: integer
              description: (Optional) The absolute value of the metric as a quantity.
      required: [kind, target]
    required: [metric]
```

## Proposed Changes

### Change 1: Refactored `volumes`

Volumes on the new Container Resource Type is refactored with these goals:

* Support sharing storage between multiple containers
* Aligning property names (using `emptyDir` for ephemeral storage for example)
* Mounting Radius Secrets not just Azure Key Vaults
* Alignment with other container platforms

The `volumes` property on the container property resource will be replaced with a `volumeMount` property on the container property and a `volumes` at the Container Resource Type level.

```yaml
containers:
    type: object
    additionalProperties:
      ...
      volumeMounts:
        type: object
        additionalProperties:
          type: object
            properties:
              mountPath:
                type: string
              volumeName:
                type: string
                description: (Required) The name of the volume defined in Containers.properties.volumes.
            required: [mountPath, volumeName]
volumes:
  type: object
    additionalProperties:
      type: object
      properties:
        persistentVolumeId: 
          type: string
          description: (Optional) The Radius PersistentVolume resource ID.
        secretId:
          type: string
          description: (Optional) The Radius Secret resource ID.
        emptyDir:
          type: null
          description: (Optional) An empty ephemeral directory.
```

The persistent volume is a separate resource type.

The `volumeMounts` with a separate `volumes` property is the same pattern for:

| Container Platform | Volume Mounts                                                | Volumes                                      |
| ------------------ | ------------------------------------------------------------ | -------------------------------------------- |
| ACA                | `containerApps.properties.configurations.containers.volumeMounts` | `containerApps.properties.templates.volumes` |
| ACI                | `containerGroups.properties.containers.volumeMounts`         | `containerGroups.properties.volumes`         |
| Cloud Run:         | `Service.spec.template.spec.containers.volumeMounts`         | `Service.spec.template.spec.volumes`         |
| ECS                | `TaskDefinition.ContainerDefinitions.mountPoints`            | `TaskDefinition.volumes`                     |
| Kubernetes         | `PodSpec.containers.volumeMounts`                            | `PodSpec.volumes`                            |

In the future, we can enhance these types with:

- An NFS mount point
- A connected resource properties volume type at provides the connected resource properties via the file system, similar to mounting a secret

### Change 2: The `command` and `args` properties are string arrays

This is a small change to make Radius consistent with other container platforms. The `command` and `args` properties should be an array of strings.  

### Change 3: Other small changes

* The `env.valueFrom.secretRef` property is renamed `secretKeyRef` to be consistent with Kubernetes. 
* The `env.valueFrom.secretRef.source` property is renamed `secretId` to be the more descriptive `secretId`.
* There are several small changes to readinessProbe and livenessProbe to be consistent with Kubernetes. 

## Removed Functionality

### Removal 1: Removal of the `imagePullPolicy`

The Container Resource Type has an `imagePullPolicy` today. However, no other container platform other than Kubernetes supports this option. This property is being removed. Platform engineers can set this in a Recipe if needed. A common use case would be to set `imagePullPolicy: Always` in a test environment. 

### Removal 2: Removal of `iam` property on connections

This property is used to specify the required IAM permissions of a connected resource when that resource is an Azure resource. Since all resources are expected to be Recipe-based, this property is no longer needed.

### Removal 3: Removal of the Kubernetes metadata extension

The Kubernetes metadata extension allows developers to set Kubernetes labels and annotations on deployed resources. Setting labels and annotations is primarily a platform engineering function so this capability moves to the Recipe. 

## Appendix 1: Container platform API references

- [Kubernetes Pod](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/)
- [ECS TaskDefinition](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_TaskDefinition.html)
- [ACI Container Group](https://learn.microsoft.com/en-us/azure/templates/microsoft.containerinstance/containergroups)
- [ACA Container App](https://learn.microsoft.com/en-us/azure/templates/microsoft.app/containerapps?pivots=deployment-language-bicep)
- [Google Cloud Run](https://cloud.google.com/run/docs/reference/yaml/v1)
