# Radius DeploymentTemplate Controller

* **Author**: Will Smith (@willdavsmith)

## Overview

Today, users of Radius and future adopters of Radius use many Kubernetes-specific tools in their production workflows. Most of these tools operate on Kubernetes resources exclusively - which presents a problem when trying to deploy resources defined in Bicep manifests. This design proposes the creation of a new Kubernetes controller in Radius called the DeploymentTemplate Controller that will allow users to deploy resources defined in Bicep manifests using Kubernetes tooling using Radius.

## Today's Status

Today, Radius users are unable to deploy resources defined in Bicep manifests using Kubernetes tooling. This is because Kubernetes tools do not understand Bicep manifests. Users must use the Radius CLI to deploy resources defined in Bicep manifests.

## Terms and definitions

**CRD (Custom Resource Definition)**: A definition for a Kubernetes resource that allows users to define their own resource types.

**CR (Custom Resource)**: An instance of a CRD that represents a custom resource in Kubernetes.

**Kubernetes Tools**: Tools that exist in the Kubernetes ecosystem and operate on kubernetes resources. Examples include `helm`, GitOps tools such as `Flux` and `ArgoCD`, and `kubectl`.

**Bicep**: An infrastructure-as-code language that when used with Radius, can deploy Radius resources, Azure resources, and AWS resources.
 
## Objectives

### Goals

**Goal: Users can use Kubernetes tooling to continuously deploy and manage resources defined in Bicep manifests**
- With this work, users will be able to deploy resources defined in Bicep using only Kubernetes. We will essentially be providing a "translation layer" between Kubernetes resources (that Kubernetes tools can understand) and Radius resources (that Radius can understand).
- With this work, users can rely on the `DeploymentTemplate` controller to repair drift from the desired state, and to perform disaster recovery. These are benefits users can't get from `rad deploy` or `az deployment create`.

**Goal: Users can quickly generate a Kubernetes Custom Resource from Bicep using the Radius CLI**
- We will provide a CLI command that generates the DeploymentTemplate resource from a Bicep manifest to make this feature easy to adopt.

**Goal: The behavior of DeploymentTemplate is consistent with user expectations for a Kubernetes-enabled tool**
- We will follow user expectations for Kubernetes controllers and resources when designing the DeploymentTemplate controller and resource. This includes building in support for retries, status updates, and error handling.

### Non-goals

**Non-goal (out of scope): Full support for GitOps**

- We will not yet be implementing automatic generation of DeploymentTemplate resources from Bicep manifests or querying Git repositories. This design will enable this work, and it will be covered in a future design document.

### User scenarios

#### Jon can deploy cloud (Azure or AWS) resources defined in Bicep manifests using Kubernetes tools

Jon is an infrastructure operator for an enterprise company. His team manages a production application that is deployed on Kubernetes and uses cloud resources. Jon wants to declare all dependencies for his application in Kubernetes manifests, including cloud resources while leveraging Radius for its useful features. He installs Radius on his cluster and uses the rad CLI to generate a custom resource from his Bicep manifest. Jon applies the custom resource to his cluster, and Radius deploys the cloud resources defined in the Bicep manifest. If he wants to update or delete the cloud resources, he can do so by re-applying the updated custom resource or deleting the custom resource.

#### Jon can deploy Radius resources defined in Bicep manifests using Kubernetes tools

Now that he can see that Radius can deploy cloud resources defined in Bicep manifests, Jon wants to take advantage of Radius tooling, such as the App Graph, and fully "Radify" his application. He writes a Bicep manifest that defines a Radius container that connects to the cloud resources, and uses the rad CLI to generate a custom resource from the Bicep manifest. Jon applies the custom resource to his cluster, and Radius deploys the Radius resources defined in the Bicep manifest. Now, Jon can take advantage of Radius tooling, such as the Radius Dashboard and App Graph, to manage his application.

## User Experience

### Radius Container

**Sample Input:**

#### `app.bicep`
```bicep
extension radius

param port int
param tag string

resource demoenv 'Applications.Core/environments@2023-10-01-preview' existing = {
  name: 'demoenv'
}

resource demoapp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'demoapp'
  properties: {
    environment: demoenv.id
  }
}

resource democtnr 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'democtnr'
  properties: {
    application: demoapp.id
    container: {
      image: 'ghcr.io/radius-project/samples/demo:${tag}'
      ports: {
        web: {
          containerPort: port
        }
      }
    }
  }
}
```

#### `app.bicepparam`
```bicep
using 'app.bicep'

param tag = ''
param port = 3000
```

**Sample Output:**
```shell
rad bicep generate-kubernetes-manifest app.bicep --parameters @app.bicepparam --parameters tag=latest --parameters appnetworking.bicepparam --outfile app.yaml

Generating DeploymentTemplate resource...
DeploymentTemplate resource generated at app.yaml

To apply the DeploymentTemplate resource onto your cluster, run:
kubectl apply -f app.yaml
```

#### `app.yaml`
```yaml
kind: DeploymentTemplate
apiVersion: radapp.io/v1alpha3
metadata:
  name: app.bicep
  namespace: radius-system
spec:
  template: |
    {
      "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
      "languageVersion": "2.1-experimental",
      "contentVersion": "1.0.0.0",
      "metadata": {
        "_EXPERIMENTAL_WARNING": "This template uses ARM features that are experimental. Experimental features should be enabled for testing purposes only, as there are no guarantees about the quality or stability of these features. Do not enable these settings for any production usage, or your production environment may be subject to breaking.",
        "_EXPERIMENTAL_FEATURES_ENABLED": [
          "Extensibility"
        ],
        "_generator": {
          "name": "bicep",
          "version": "0.29.47.4906",
          "templateHash": "14156924711842038952"
        }
      },
      "parameters": {
        "tag": {
          "type": "string"
        },
        "port": {
          "type": "int"
        }
      },
      "imports": {
        "Radius": {
          "provider": "Radius",
          "version": "latest"
        }
      },
      "resources": {
        "container": {
          "import": "Radius",
          "type": "Applications.Core/containers@2023-10-01-preview",
          "properties": {
            "name": "container",
            "properties": {
              "application": "[parameters('application')]",
              "container": {
                "image": "[format('ghcr.io/radius-project/samples/demo:{0}', parameters('tag'))]",
                "ports": {
                  "web": {
                    "containerPort": "[parameters('port')]"
                  }
                }
              }
            }
          }
        }
      }
    }
  parameters: |
    {
      "tag": {
        "value": "latest"
      },
      "port": {
        "value": 3000
      }
    }
```

## Design

### High Level Design

This design proposes the creation of two new Kubernetes custom resources and their corresponding reconcilers: `DeploymentTemplate` and `DeploymentResource`. The `DeploymentTemplate` controller will be responsible for deploying and reconciling resources defined in ARM JSON manifests via Radius, while the `DeploymentResource` controller will be responsible for reconciling individual resources. The `DeploymentTemplate` controller will have a control loop that will check the status of an in-progress operation, process deletion, and process creation or update. The `DeploymentResource` controller will have a similar control loop that will check the status of an in-progress operation or process deletion if necessary.

### Architecture Diagram

![Architecture Diagram](2024-10-deploymenttemplate-controller/architecture.png)

### Detailed Design

#### `DeploymentTemplate` Custom Resource Definition

The `DeploymentTemplate` CRD will be a new Kubernetes CRD that will be used to contain the ARM JSON manifest and parameters. The CRD will have the following fields:

```go
// DeploymentTemplateSpec defines the desired state of a DeploymentTemplate
type DeploymentTemplateSpec struct {
  // Template is the ARM JSON manifest that defines the resources to deploy.
  Template string `json:"template"`

  // Parameters is the ARM JSON parameters for the template.
  Parameters string `json:"parameters"`

  // ProviderConfig specifies the scope for resources
  ProviderConfig *ProviderConfig `json:"providerConfig,omitempty"`
}

// From Radius resourcedeploymentsclient (imported)
type ProviderConfig struct {
  Radius      *Radius      `json:"radius,omitempty"`
  Az          *Az          `json:"az,omitempty"`
  AWS         *AWS         `json:"aws,omitempty"`
  Deployments *Deployments `json:"deployments,omitempty"`
}

type Radius struct {
  Type  string `json:"type,omitempty"`
  Value Value  `json:"value,omitempty"`
}

type Value struct {
  Scope string `json:"scope,omitempty"`
}

// ...

// DeploymentTemplateStatus defines the observed state of the Deployment Template.
type DeploymentTemplateStatus struct {
  // ObservedGeneration is the most recent generation observed for this DeploymentTemplate.
  ObservedGeneration int64 `json:"observedGeneration,omitempty"`

  // Template is the ARM JSON manifest that defines the resources to deploy.
  Template string `json:"template"`

  // Parameters is the ARM JSON parameters for the template.
  Parameters string `json:"parameters"`
  
  // ProviderConfig specifies the scope for resources
  ProviderConfig *ProviderConfig `json:"providerConfig,omitempty"`

  // Resource is the resource id of the deployment.
  Resource string `json:"resource,omitempty"`

  // Operation tracks the status of an in-progress provisioning operation.
  Operation *ResourceOperation `json:"operation,omitempty"`

  // Phrase indicates the current status of the Deployment Template.
  Phrase DeploymentTemplatePhrase `json:"phrase,omitempty"`

  // Message is a human-readable description of the status of the Deployment Template.
  Message string `json:"message,omitempty"`
}

// DeploymentTemplatePhrase is a string representation of the current status of a Deployment Template.
type DeploymentTemplatePhrase string

const (
  // DeploymentTemplatePhraseUpdating indicates that the Deployment Template is being updated.
  DeploymentTemplatePhraseUpdating DeploymentTemplatePhrase = "Updating"

  // DeploymentTemplatePhraseReady indicates that the Deployment Template is ready.
  DeploymentTemplatePhraseReady DeploymentTemplatePhrase = "Ready"

  // DeploymentTemplatePhraseFailed indicates that the Deployment Template has failed.
  DeploymentTemplatePhraseFailed DeploymentTemplatePhrase = "Failed"

  // DeploymentTemplatePhraseDeleting indicates that the Deployment Template is being deleted.
  DeploymentTemplatePhraseDeleting DeploymentTemplatePhrase = "Deleting"

  // DeploymentTemplatePhraseDeleted indicates that the Deployment Template has been deleted.
  DeploymentTemplatePhraseDeleted DeploymentTemplatePhrase = "Deleted"
)
```

#### DeploymentResource Custom Resource Definition

The `DeploymentResource` CRD is another CRD that will be responsible for tracking the state of individual resources. The CRD will have the following fields:

```go
// DeploymentResourceSpec defines the desired state of a Deployment Resource.
type DeploymentResourceSpec struct {
  // ProviderConfig specifies the scope for resources
  ProviderConfig *ProviderConfig `json:"providerConfig,omitempty"`

  // ID is the Radius resource ID.
  ID string `json:"id"`
}

type DeploymentResourceStatus struct {
  // ProviderConfig specifies the scope for resources
  ProviderConfig *ProviderConfig `json:"providerConfig,omitempty"`

  // ObservedGeneration is the most recent generation observed for this DeploymentResource.
  ObservedGeneration int64 `json:"observedGeneration,omitempty"`

  // Operation tracks the status of an in-progress provisioning operation.
  Operation *ResourceOperation `json:"operation,omitempty"`

  // Phrase indicates the current status of the Deployment Template Resource.
  Phrase DeploymentResourcePhrase `json:"phrase,omitempty"`

  // Message is a human-readable description of the status of the Deployment Template Resource.
  Message string `json:"description,omitempty"`
}

// DeploymentResourcePhrase is a string representation of the current status of a Deployment Resource.
type DeploymentResourcePhrase string

const (
  // DeploymentResourcePhraseReady indicates that the Deployment Resource is ready.
  DeploymentResourcePhraseReady DeploymentResourcePhrase = "Ready"

  // DeploymentResourcePhraseFailed indicates that the Deployment Resource has failed to delete.
  BicepPhraseFailed DeploymentResourcePhrase = "Failed"

  // DeploymentResourcePhraseDeleting indicates that the Deployment Resource is being deleted.
  BicepPhraseDeleting DeploymentResourcePhrase = "Deleting"

  // DeploymentResourcePhraseDeleted indicates that the Deployment Resource has been deleted.
  BicepPhraseDeleted DeploymentResourcePhrase = "Deleted"
)
```

#### `rad bicep generate-kubernetes-manifest` Command

The `rad bicep generate-kubernetes-manifest` command will be a new command that will generate a Kubernetes custom resource when provided a Bicep template and its parameters. The command will take the following arguments:

- `-p, --parameters`: The parameters for the Bicep template. Can be specified in the same way as [`rad deploy` parameters](https://edge.docs.radapp.io/reference/cli/rad_deploy/).
- `-o, --outfile`: The path to the output file where the `DeploymentTemplate` resource will be written.

#### `DeploymentTemplate` Reconciler

The `radius-controller` will be updated to include a new reconciler that reconciles `DeploymentTemplate` resources. It will have the following control loop:

1. Check if there is an in-progress operation. If so, check its status:
    1. If the operation is still in progress, then queue another reconcile operation and continue processing.
    2. If the operation completed successfully:
        1. Diff the resources in the `properties.outputResources` field returned by the Radius API with the resources in the `status.outputResources` field on the `DeploymentTemplate` resource.
        2. Depending on the diff, create or delete `DeploymentResource` resources on the cluster. In the case of create, add the `DeploymentTemplate` as the owner of the `DeploymentResource` and set the `radapp.io/deployment-resource-finalizer` finalizer on the `DeploymentResource`.
        3. Update the `status.phrase` for the `DeploymentTemplate` to `Ready`.
        4. Continue processing.
    3. If the operation failed, then update the `status.phrase` and `status.message` as `Failed` with the reason for the failure and continue processing.
2. If the `DeploymentTemplate` is being deleted, then process deletion:
    1. Remove the `radapp.io/deployment-template-finalizer` finalizer from the `DeploymentTemplate`.
    1. Since the `DeploymentResources` are owned by the `DeploymentTemplate`, the `DeploymentResource` resources will be deleted first. Once they are deleted, the `DeploymentTemplate` resource will be deleted.
4. If the `DeploymentTemplate` is not being deleted then process this as a create or update:
    1. Add the `radapp.io/deployment-template-finalizer` finalizer onto the `DeploymentTemplate` resource.
    2. Queue a PUT operation against the Radius API to deploy the ARM JSON in the `spec.template` field with the parameters in the `spec.parameters` field.
    3. Set the `status.phrase` for the `DeploymentTemplate` to `Updating` and the `status.operation` to the operation returned by the Radius API.
    4. Continue processing.

#### DeploymentResource Controller

The `radius-controller` will be updated to include a new reconciler that reconciles `DeploymentResource` resources. It will have the following control loop:

1. Check if there is an in-progress deletion. If so, check its status:
    1. If the deletion is still in progress, then queue another reconcile operation and continue processing.
    2. If the deletion completed successfully, then remove the `radapp.io/deployment-resource-finalizer` finalizer from the resource and continue processing.
    3. If the operation failed, then update the `status.phrase` and `status.message` as `Failed`.
2. If the `DeploymentTemplate` is being deleted, then process deletion:
    1. Send a DELETE operation to the Radius API to delete the resource specified in the `spec.resourceId` field.
    2. Continue processing.
3. If the `DeploymentTemplate` is not being deleted then process this as a create or update:
    1. Set the `status.phrase` for the `DeploymentResource` to `Ready`.
    2. Continue processing.

### Alternatives Considered

- Package the Bicep compilation into the Radius controller. We decided against this because it would make the controller more complex and harder to maintain. Instead, we will rely on the CLI to generate the DeploymentTemplate resource.

- We could build logic to use defaults for applications and environments, like the CLI does today. We decided against this because we want to keep the logic of the controller simple and unsurprising. We could consider this as a future enhancement.

### API design (if applicable)

#### API

There will be no changes to the Radius API.

#### CLI

There will be a new CLI command `rad bicep generate-kubernetes-manifest` that will generate a DeploymentTemplate resource from a Bicep template and parameters file.

### Error Handling

The `DeploymentTemplate` controller will handle errors by updating the `status.phrase` and `status.message` fields of the `DeploymentTemplate` resource. The controller will also emit events for the `DeploymentTemplate` resource to track the progress of the deployment.

## Test plan

We will need to write unit tests for the new controllers and integration tests to ensure that the controllers work as expected. We will also need to write tests to ensure that the `rad bicep generate-kubernetes-manifest` command works as expected. We will also need to write functional tests to ensure that the controllers work as expected.

## Security

The new controllers will use the existing security model of other Radius controllers, such as fine-grained Kubernetes RBAC permissions. The controllers will also use the Radius API to deploy resources, which will require the same authentication and authorization as other Radius API calls.

## Compatibility

The changes describes are only additive, so there should be no breaking changes or compatibility issues with other components.

## Monitoring and Logging

The new controllers will emit logs and metrics in the same way as other Radius controllers.

We will be leveraging the Kubernetes Events API to emit events for the DeploymentTemplate and DeploymentResource resources. These events will be used to track the progress of the deployment and to provide feedback to the user.

## Development plan

- Implement `DeploymentTemplate` and `DeploymentResource` CRDs.
- Implement the `rad bicep generate-kubernetes-manifest` command.
- Write tests for the `rad bicep generate-kubernetes-manifest` command.
- Implement the `DeploymentTemplate` controller.
- Implement the `DeploymentResource` controller.
- Write unit tests for the new controllers.
- Write integration and functional tests for the new controllers.

## Open Questions

- Q: What use cases are there for integration with Kubernetes tools that we should consider when designing the DeploymentTemplate controller? Are there any gaps in the design that we should address?
- A: We will see how users adopt this feature set and iterate on the design based on user feedback. For now, we will implement this feature and enable the scenarios that we know about.
