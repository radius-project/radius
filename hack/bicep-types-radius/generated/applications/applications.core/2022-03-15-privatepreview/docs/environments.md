## Resource Applications.Core/environments@2022-03-15-privatepreview

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' |  ReadOnly, DeployTimeConstant | The resource api version |
| **id** | string |  ReadOnly, DeployTimeConstant | The resource id |
| **location** | string |  Required | The geo-location where the resource lives |
| **name** | string |  Required, DeployTimeConstant | The resource name |
| **properties** | [EnvironmentProperties](#environmentproperties) |  | Environment properties |
| **systemData** | [SystemData](#systemdata) |  ReadOnly | Metadata pertaining to creation and last modification of the resource. |
| **tags** | [TrackedResourceTags](#trackedresourcetags) |  | Resource tags. |
| **type** | 'Applications.Core/environments' |  ReadOnly, DeployTimeConstant | The resource type |

## EnvironmentProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **compute** | [EnvironmentCompute](#environmentcompute) |  Required | Represents backing compute resource |
| **extensions** | [Extension](#extension)[] |  | The environment extension. |
| **providers** | [Providers](#providers) |  | The Cloud providers configuration |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' |  ReadOnly | Provisioning state of the portable resource at the time the operation was called |
| **recipes** | [EnvironmentPropertiesRecipes](#environmentpropertiesrecipes) |  | Specifies Recipes linked to the Environment. |

## EnvironmentCompute

* **Discriminator**: kind

### Base Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **identity** | [IdentitySettings](#identitysettings) |  | IdentitySettings is the external identity setting. |
| **resourceId** | string |  | The resource id of the compute resource for application environment. |

### KubernetesCompute

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'kubernetes' |  Required | Discriminator property for EnvironmentCompute. |
| **namespace** | string |  Required | The namespace to use for the environment. |


## IdentitySettings

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'azure.com.workload' | 'undefined' |  Required | IdentitySettingKind is the kind of supported external identity setting |
| **oidcIssuer** | string |  | The URI for your compute platform's OIDC issuer |
| **resource** | string |  | The resource ID of the provisioned identity |

## Extension

* **Discriminator**: kind

### Base Properties

* **none**


### DaprSidecarExtension

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **appId** | string |  Required | The Dapr appId. Specifies the identifier used by Dapr for service invocation. |
| **appPort** | int |  | The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar. |
| **config** | string |  | Specifies the Dapr configuration to use for the resource. |
| **kind** | 'daprSidecar' |  Required | Discriminator property for Extension. |
| **protocol** | 'grpc' | 'http' |  | The Dapr sidecar extension protocol |

### KubernetesMetadataExtension

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **annotations** | [KubernetesMetadataExtensionAnnotations](#kubernetesmetadataextensionannotations) |  | Annotations to be applied to the Kubernetes resources output by the resource |
| **kind** | 'kubernetesMetadata' |  Required | Discriminator property for Extension. |
| **labels** | [KubernetesMetadataExtensionLabels](#kubernetesmetadataextensionlabels) |  | Labels to be applied to the Kubernetes resources output by the resource |

### KubernetesNamespaceExtension

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'kubernetesNamespace' |  Required | Discriminator property for Extension. |
| **namespace** | string |  Required | The namespace of the application environment. |

### ManualScalingExtension

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'manualScaling' |  Required | Discriminator property for Extension. |
| **replicas** | int |  Required | Replica count. |


## KubernetesMetadataExtensionAnnotations

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: string

## KubernetesMetadataExtensionLabels

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: string

## Providers

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **aws** | [ProvidersAws](#providersaws) |  | The AWS cloud provider definition |
| **azure** | [ProvidersAzure](#providersazure) |  | The Azure cloud provider definition |

## ProvidersAws

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **scope** | string |  Required | Target scope for AWS resources to be deployed into.  For example: '/planes/aws/aws/accounts/000000000000/regions/us-west-2' |

## ProvidersAzure

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **scope** | string |  Required | Target scope for Azure resources to be deployed into.  For example: '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup' |

## EnvironmentPropertiesRecipes

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: [DictionaryOfRecipeProperties](#dictionaryofrecipeproperties)

## DictionaryOfRecipeProperties

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: [RecipeProperties](#recipeproperties)

## RecipeProperties

* **Discriminator**: templateKind

### Base Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **parameters** | any |  | Any object |
| **templatePath** | string |  Required | Path to the template provided by the recipe. Currently only link to Azure Container Registry is supported. |

### BicepRecipeProperties

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **templateKind** | 'bicep' |  Required | Discriminator property for RecipeProperties. |

### TerraformRecipeProperties

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **templateKind** | 'terraform' |  Required | Discriminator property for RecipeProperties. |
| **templateVersion** | string |  | Version of the template to deploy. For Terraform recipes using a module registry this is required, but must be omitted for other module sources. |


## SystemData

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **createdAt** | string |  | The timestamp of resource creation (UTC). |
| **createdBy** | string |  | The identity that created the resource. |
| **createdByType** | 'Application' | 'Key' | 'ManagedIdentity' | 'User' |  | The type of identity that created the resource. |
| **lastModifiedAt** | string |  | The timestamp of resource last modification (UTC) |
| **lastModifiedBy** | string |  | The identity that last modified the resource. |
| **lastModifiedByType** | 'Application' | 'Key' | 'ManagedIdentity' | 'User' |  | The type of identity that created the resource. |

## TrackedResourceTags

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: string

