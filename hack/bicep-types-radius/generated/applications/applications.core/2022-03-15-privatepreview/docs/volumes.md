### Top-Level Resource

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' | The resource api version <br />_(read-only, deploy-time constant)_ |
| **id** | string | The resource id <br />_(read-only, deploy-time constant)_ |
| **location** | string | The geo-location where the resource lives <br />_(required)_ |
| **name** | string | The resource name <br />_(required, deploy-time constant)_ |
| **properties** | [VolumeProperties](#volumeproperties) | Volume properties |
| **systemData** | [SystemData](#systemdata) | Metadata pertaining to creation and last modification of the resource. <br />_(read-only)_ |
| **tags** | [TrackedResourceTags](#trackedresourcetags) | Resource tags. |
| **type** | 'Applications.Core/volumes' | The resource type <br />_(read-only, deploy-time constant)_ |

### VolumeProperties

* **Discriminator**: kind

#### Base Properties

| Property | Type | Description |
|----------|------|-------------|
| **application** | string | Fully qualified resource ID for the application that the portable resource is consumed by <br />_(required)_ |
| **environment** | string | Fully qualified resource ID for the environment that the portable resource is linked to (if applicable) |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | Provisioning state of the portable resource at the time the operation was called <br />_(read-only)_ |
| **status** | [ResourceStatus](#resourcestatus) | Status of a resource. <br />_(read-only)_ |

#### AzureKeyVaultVolumeProperties

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **certificates** | [AzureKeyVaultVolumePropertiesCertificates](#azurekeyvaultvolumepropertiescertificates) | The KeyVault certificates that this volume exposes |
| **keys** | [AzureKeyVaultVolumePropertiesKeys](#azurekeyvaultvolumepropertieskeys) | The KeyVault keys that this volume exposes |
| **kind** | 'azure.com.keyvault' | Discriminator property for VolumeProperties. <br />_(required)_ |
| **resource** | string | The ID of the keyvault to use for this volume resource <br />_(required)_ |
| **secrets** | [AzureKeyVaultVolumePropertiesSecrets](#azurekeyvaultvolumepropertiessecrets) | The KeyVault secrets that this volume exposes |


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

### AzureKeyVaultVolumePropertiesCertificates

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: [CertificateObjectProperties](#certificateobjectproperties)

### CertificateObjectProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **alias** | string | File name when written to disk |
| **certType** | 'certificate' | 'privatekey' | 'publickey' | Represents certificate types |
| **encoding** | 'base64' | 'hex' | 'utf-8' | Represents secret encodings |
| **format** | 'pem' | 'pfx' | Represents certificate formats |
| **name** | string | The name of the certificate <br />_(required)_ |
| **version** | string | Certificate version |

### AzureKeyVaultVolumePropertiesKeys

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: [KeyObjectProperties](#keyobjectproperties)

### KeyObjectProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **alias** | string | File name when written to disk |
| **name** | string | The name of the key <br />_(required)_ |
| **version** | string | Key version |

### AzureKeyVaultVolumePropertiesSecrets

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: [SecretObjectProperties](#secretobjectproperties)

### SecretObjectProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **alias** | string | File name when written to disk |
| **encoding** | 'base64' | 'hex' | 'utf-8' | Represents secret encodings |
| **name** | string | The name of the secret <br />_(required)_ |
| **version** | string | secret version |

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

