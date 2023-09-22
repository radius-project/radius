### Top-Level Resource

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' | The resource api version  (_ReadOnly, DeployTimeConstant_) |
| **id** | string | The resource id  (_ReadOnly, DeployTimeConstant_) |
| **location** | string | The geo-location where the resource lives  (_Required_) |
| **name** | string | The resource name  (_Required, DeployTimeConstant_) |
| **properties** | [RedisCacheProperties](#rediscacheproperties) | RedisCache portable resource properties |
| **systemData** | [SystemData](#systemdata) | Metadata pertaining to creation and last modification of the resource.  (_ReadOnly_) |
| **tags** | [TrackedResourceTags](#trackedresourcetags) | Resource tags. |
| **type** | 'Applications.Datastores/redisCaches' | The resource type  (_ReadOnly, DeployTimeConstant_) |

### Function listSecrets (Applications.Datastores/redisCaches@2022-03-15-privatepreview)

* **Resource**: Applications.Datastores/redisCaches
* **ApiVersion**: 2022-03-15-privatepreview
* **Input**: any
* **Output**: [RedisCacheListSecretsResult](#rediscachelistsecretsresult)

### RedisCacheProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **application** | string | Fully qualified resource ID for the application that the portable resource is consumed by (if applicable) |
| **environment** | string | Fully qualified resource ID for the environment that the portable resource is linked to  (_Required_) |
| **host** | string | The host name of the target Redis cache |
| **port** | int | The port value of the target Redis cache |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | Provisioning state of the portable resource at the time the operation was called  (_ReadOnly_) |
| **recipe** | [Recipe](#recipe) | The recipe used to automatically deploy underlying infrastructure for a portable resource |
| **resourceProvisioning** | 'manual' | 'recipe' | Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values. |
| **resources** | [ResourceReference](#resourcereference)[] | List of the resource IDs that support the Redis resource |
| **secrets** | [RedisCacheSecrets](#rediscachesecrets) | The secret values for the given RedisCache resource |
| **status** | [ResourceStatus](#resourcestatus) | Status of a resource.  (_ReadOnly_) |
| **tls** | bool | Specifies whether to enable SSL connections to the Redis cache |
| **username** | string | The username for Redis cache |

### Recipe

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **name** | string | The name of the recipe within the environment to use  (_Required_) |
| **parameters** | any | Any object |

### ResourceReference

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **id** | string | Resource id of an existing resource  (_Required_) |

### RedisCacheSecrets

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **connectionString** | string | The connection string used to connect to the Redis cache |
| **password** | string | The password for this Redis cache instance |
| **url** | string | The URL used to connect to the Redis cache |

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
| **kind** | 'kubernetes' | Discriminator property for EnvironmentCompute.  (_Required_) |
| **namespace** | string | The namespace to use for the environment.  (_Required_) |


### IdentitySettings

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'azure.com.workload' | 'undefined' | IdentitySettingKind is the kind of supported external identity setting  (_Required_) |
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

### RedisCacheListSecretsResult

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **connectionString** | string | The connection string used to connect to the Redis cache  (_ReadOnly_) |
| **password** | string | The password for this Redis cache instance  (_ReadOnly_) |
| **url** | string | The URL used to connect to the Redis cache  (_ReadOnly_) |

