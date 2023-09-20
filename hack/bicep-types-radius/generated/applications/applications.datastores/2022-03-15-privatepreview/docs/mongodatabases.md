## Resource Applications.Datastores/mongoDatabases@2022-03-15-privatepreview

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' |  ReadOnly, DeployTimeConstant | The resource api version |
| **id** | string |  ReadOnly, DeployTimeConstant | The resource id |
| **location** | string |  Required | The geo-location where the resource lives |
| **name** | string |  Required, DeployTimeConstant | The resource name |
| **properties** | [MongoDatabaseProperties](#mongodatabaseproperties) |  | MongoDatabase portable resource properties |
| **systemData** | [SystemData](#systemdata) |  ReadOnly | Metadata pertaining to creation and last modification of the resource. |
| **tags** | [TrackedResourceTags](#trackedresourcetags) |  | Resource tags. |
| **type** | 'Applications.Datastores/mongoDatabases' |  ReadOnly, DeployTimeConstant | The resource type |

## Function listSecrets (Applications.Datastores/mongoDatabases@2022-03-15-privatepreview)

* **Resource**: Applications.Datastores/mongoDatabases
* **ApiVersion**: 2022-03-15-privatepreview
* **Input**: any
* **Output**: [MongoDatabaseListSecretsResult](#mongodatabaselistsecretsresult)

## MongoDatabaseProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **application** | string |  | Fully qualified resource ID for the application that the portable resource is consumed by (if applicable) |
| **database** | string |  | Database name of the target Mongo database |
| **environment** | string |  Required | Fully qualified resource ID for the environment that the portable resource is linked to |
| **host** | string |  | Host name of the target Mongo database |
| **port** | int |  | Port value of the target Mongo database |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' |  ReadOnly | Provisioning state of the portable resource at the time the operation was called |
| **recipe** | [Recipe](#recipe) |  | The recipe used to automatically deploy underlying infrastructure for a portable resource |
| **resourceProvisioning** | 'manual' | 'recipe' |  | Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values. |
| **resources** | [ResourceReference](#resourcereference)[] |  | List of the resource IDs that support the MongoDB resource |
| **secrets** | [MongoDatabaseSecrets](#mongodatabasesecrets) |  | The secret values for the given MongoDatabase resource |
| **status** | [ResourceStatus](#resourcestatus) |  ReadOnly | Status of a resource. |
| **username** | string |  | Username to use when connecting to the target Mongo database |

## Recipe

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **name** | string |  Required | The name of the recipe within the environment to use |
| **parameters** | any |  | Any object |

## ResourceReference

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **id** | string |  Required | Resource id of an existing resource |

## MongoDatabaseSecrets

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **connectionString** | string |  | Connection string used to connect to the target Mongo database |
| **password** | string |  | Password to use when connecting to the target Mongo database |

## ResourceStatus

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **compute** | [EnvironmentCompute](#environmentcompute) |  | Represents backing compute resource |
| **outputResources** | [OutputResource](#outputresource)[] |  | Properties of an output resource |

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

## OutputResource

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **id** | string |  | The UCP resource ID of the underlying resource. |
| **localId** | string |  | The logical identifier scoped to the owning Radius resource. This is only needed or used when a resource has a dependency relationship. LocalIDs do not have any particular format or meaning beyond being compared to determine dependency relationships. |
| **radiusManaged** | bool |  | Determines whether Radius manages the lifecycle of the underlying resource. |

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

## MongoDatabaseListSecretsResult

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **connectionString** | string |  ReadOnly | Connection string used to connect to the target Mongo database |
| **password** | string |  ReadOnly | Password to use when connecting to the target Mongo database |

