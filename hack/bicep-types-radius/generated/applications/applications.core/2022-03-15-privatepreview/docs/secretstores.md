## Resource Applications.Core/secretStores@2022-03-15-privatepreview

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' |  ReadOnly, DeployTimeConstant | The resource api version |
| **id** | string |  ReadOnly, DeployTimeConstant | The resource id |
| **location** | string |  Required | The geo-location where the resource lives |
| **name** | string |  Required, DeployTimeConstant | The resource name |
| **properties** | [SecretStoreProperties](#secretstoreproperties) |  | The properties of SecretStore |
| **systemData** | [SystemData](#systemdata) |  ReadOnly | Metadata pertaining to creation and last modification of the resource. |
| **tags** | [TrackedResourceTags](#trackedresourcetags) |  | Resource tags. |
| **type** | 'Applications.Core/secretStores' |  ReadOnly, DeployTimeConstant | The resource type |

## Function listSecrets (Applications.Core/secretStores@2022-03-15-privatepreview)

* **Resource**: Applications.Core/secretStores
* **ApiVersion**: 2022-03-15-privatepreview
* **Input**: any
* **Output**: [SecretStoreListSecretsResult](#secretstorelistsecretsresult)

## SecretStoreProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **application** | string |  Required | Fully qualified resource ID for the application that the portable resource is consumed by |
| **data** | [SecretStorePropertiesData](#secretstorepropertiesdata) |  Required | An object to represent key-value type secrets |
| **environment** | string |  | Fully qualified resource ID for the environment that the portable resource is linked to (if applicable) |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' |  ReadOnly | Provisioning state of the portable resource at the time the operation was called |
| **resource** | string |  | The resource id of external secret store. |
| **status** | [ResourceStatus](#resourcestatus) |  ReadOnly | Status of a resource. |
| **type** | 'certificate' | 'generic' |  | The type of SecretStore data |

## SecretStorePropertiesData

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: [SecretValueProperties](#secretvalueproperties)

## SecretValueProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **encoding** | 'base64' | 'raw' |  | The type of SecretValue Encoding |
| **value** | string |  | The value of secret. |
| **valueFrom** | [ValueFromProperties](#valuefromproperties) |  | The Secret value source properties |

## ValueFromProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **name** | string |  Required | The name of the referenced secret. |
| **version** | string |  | The version of the referenced secret. |

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

## SecretStoreListSecretsResult

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **data** | [SecretStoreListSecretsResultData](#secretstorelistsecretsresultdata) |  ReadOnly | An object to represent key-value type secrets |
| **type** | 'certificate' | 'generic' |  ReadOnly | The type of SecretStore data |

## SecretStoreListSecretsResultData

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: [SecretValueProperties](#secretvalueproperties)

