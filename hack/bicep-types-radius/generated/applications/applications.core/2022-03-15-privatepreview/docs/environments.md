### Top-Level Resource

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' | The resource api version <br />_(read-only, deploy-time constant)_ |
| **id** | string | The resource id <br />_(read-only, deploy-time constant)_ |
| **location** | string | The geo-location where the resource lives <br />_(required)_ |
| **name** | string | The resource name <br />_(required, deploy-time constant)_ |
| **properties** | [EnvironmentProperties](#environmentproperties) | Environment properties |
| **systemData** | [SystemData](#systemdata) | Metadata pertaining to creation and last modification of the resource. <br />_(read-only)_ |
| **tags** | [TrackedResourceTags](#trackedresourcetags) | Resource tags. |
| **type** | 'Applications.Core/environments' | The resource type <br />_(read-only, deploy-time constant)_ |

### EnvironmentProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **compute** | [EnvironmentCompute](#environmentcompute) | Represents backing compute resource <br />_(required)_ |
| **extensions** | [Extension](#extension)[] | The environment extension. |
| **providers** | [Providers](#providers) | The Cloud providers configuration |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | Provisioning state of the portable resource at the time the operation was called <br />_(read-only)_ |
| **recipes** | [EnvironmentPropertiesRecipes](#environmentpropertiesrecipes) | Specifies Recipes linked to the Environment. |

### EnvironmentCompute

* **Discriminator**: kind

#### Base Properties

| Property | Type | Description |
|----------|------|-------------|
| **identity** | [IdentitySettings](#identitysettings) | IdentitySettings is the external identity setting. |
| **resourceId** | string | The resource id of the compute resource for application environment. |

#### KubernetesCompute

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'kubernetes' | Discriminator property for EnvironmentCompute. <br />_(required)_ |
| **namespace** | string | The namespace to use for the environment. <br />_(required)_ |


### IdentitySettings

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'azure.com.workload' | 'undefined' | IdentitySettingKind is the kind of supported external identity setting <br />_(required)_ |
| **oidcIssuer** | string | The URI for your compute platform's OIDC issuer |
| **resource** | string | The resource ID of the provisioned identity |

### Extension

* **Discriminator**: kind

#### Base Properties

* **none**


#### DaprSidecarExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **appId** | string | The Dapr appId. Specifies the identifier used by Dapr for service invocation. <br />_(required)_ |
| **appPort** | int | The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar. |
| **config** | string | Specifies the Dapr configuration to use for the resource. |
| **kind** | 'daprSidecar' | Discriminator property for Extension. <br />_(required)_ |
| **protocol** | 'grpc' | 'http' | The Dapr sidecar extension protocol |

#### KubernetesMetadataExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **annotations** | [KubernetesMetadataExtensionAnnotations](#kubernetesmetadataextensionannotations) | Annotations to be applied to the Kubernetes resources output by the resource |
| **kind** | 'kubernetesMetadata' | Discriminator property for Extension. <br />_(required)_ |
| **labels** | [KubernetesMetadataExtensionLabels](#kubernetesmetadataextensionlabels) | Labels to be applied to the Kubernetes resources output by the resource |

#### KubernetesNamespaceExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'kubernetesNamespace' | Discriminator property for Extension. <br />_(required)_ |
| **namespace** | string | The namespace of the application environment. <br />_(required)_ |

#### ManualScalingExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'manualScaling' | Discriminator property for Extension. <br />_(required)_ |
| **replicas** | int | Replica count. <br />_(required)_ |


### KubernetesMetadataExtensionAnnotations

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: string

### KubernetesMetadataExtensionLabels

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: string

### Providers

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **aws** | [ProvidersAws](#providersaws) | The AWS cloud provider definition |
| **azure** | [ProvidersAzure](#providersazure) | The Azure cloud provider definition |

### ProvidersAws

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **scope** | string | Target scope for AWS resources to be deployed into.  For example: '/planes/aws/aws/accounts/000000000000/regions/us-west-2' <br />_(required)_ |

### ProvidersAzure

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **scope** | string | Target scope for Azure resources to be deployed into.  For example: '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup' <br />_(required)_ |

### EnvironmentPropertiesRecipes

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: [DictionaryOfRecipeProperties](#dictionaryofrecipeproperties)

### DictionaryOfRecipeProperties

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: [RecipeProperties](#recipeproperties)

### RecipeProperties

* **Discriminator**: templateKind

#### Base Properties

| Property | Type | Description |
|----------|------|-------------|
| **parameters** | any | Any object |
| **templatePath** | string | Path to the template provided by the recipe. Currently only link to Azure Container Registry is supported. <br />_(required)_ |

#### BicepRecipeProperties

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **templateKind** | 'bicep' | Discriminator property for RecipeProperties. <br />_(required)_ |

#### TerraformRecipeProperties

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **templateKind** | 'terraform' | Discriminator property for RecipeProperties. <br />_(required)_ |
| **templateVersion** | string | Version of the template to deploy. For Terraform recipes using a module registry this is required, but must be omitted for other module sources. |


### SystemData

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **createdAt** | string | The timestamp of resource creation (UTC). |
| **createdBy** | string | The identity that created the resource. |
| **createdByType** | 'Application' | 'Key' | 'ManagedIdentity' | 'User' | The type of identity that created the resource. |
| **lastModifiedAt** | string | The timestamp of resource last modification (UTC) |
| **lastModifiedBy** | string | The identity that last modified the resource. |
| **lastModifiedByType** | 'Application' | 'Key' | 'ManagedIdentity' | 'User' | The type of identity that created the resource. |

### TrackedResourceTags

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: string

