### Top-Level Resource

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' | The resource api version <br />_(read-only, deploy-time constant)_ |
| **id** | string | The resource id <br />_(read-only, deploy-time constant)_ |
| **location** | string | The geo-location where the resource lives <br />_(required)_ |
| **name** | string | The resource name <br />_(required, deploy-time constant)_ |
| **properties** | [DaprSecretStoreProperties](#daprsecretstoreproperties) | Dapr SecretStore portable resource properties |
| **systemData** | [SystemData](#systemdata) | Metadata pertaining to creation and last modification of the resource. <br />_(read-only)_ |
| **tags** | [TrackedResourceTags](#trackedresourcetags) | Resource tags. |
| **type** | 'Applications.Dapr/secretStores' | The resource type <br />_(read-only, deploy-time constant)_ |

### DaprSecretStoreProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **application** | string | Fully qualified resource ID for the application that the portable resource is consumed by (if applicable) |
| **componentName** | string | The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component. <br />_(read-only)_ |
| **environment** | string | Fully qualified resource ID for the environment that the portable resource is linked to <br />_(required)_ |
| **metadata** | any | Any object |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | Provisioning state of the portable resource at the time the operation was called <br />_(read-only)_ |
| **recipe** | [Recipe](#recipe) | The recipe used to automatically deploy underlying infrastructure for a portable resource |
| **resourceProvisioning** | 'manual' | 'recipe' | Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values. |
| **status** | [ResourceStatus](#resourcestatus) | Status of a resource. <br />_(read-only)_ |
| **type** | string | Dapr component type which must matches the format used by Dapr Kubernetes configuration format |
| **version** | string | Dapr component version |

### Recipe

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **name** | string | The name of the recipe within the environment to use <br />_(required)_ |
| **parameters** | any | Any object |

### ResourceStatus

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **compute** | [EnvironmentCompute](#environmentcompute) | Represents backing compute resource |
| **outputResources** | [OutputResource](#outputresource)[] | Properties of an output resource |

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

### OutputResource

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **id** | string | The UCP resource ID of the underlying resource. |
| **localId** | string | The logical identifier scoped to the owning Radius resource. This is only needed or used when a resource has a dependency relationship. LocalIDs do not have any particular format or meaning beyond being compared to determine dependency relationships. |
| **radiusManaged** | bool | Determines whether Radius manages the lifecycle of the underlying resource. |

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

