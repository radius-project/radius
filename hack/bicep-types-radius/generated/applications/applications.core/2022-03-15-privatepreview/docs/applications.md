### Top-Level Resource

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' | The resource api version  (_ReadOnly, DeployTimeConstant_) |
| **id** | string | The resource id  (_ReadOnly, DeployTimeConstant_) |
| **location** | string | The geo-location where the resource lives  (_Required_) |
| **name** | string | The resource name  (_Required, DeployTimeConstant_) |
| **properties** | [ApplicationProperties](#applicationproperties) | Application properties |
| **systemData** | [SystemData](#systemdata) | Metadata pertaining to creation and last modification of the resource.  (_ReadOnly_) |
| **tags** | [TrackedResourceTags](#trackedresourcetags) | Resource tags. |
| **type** | 'Applications.Core/applications' | The resource type  (_ReadOnly, DeployTimeConstant_) |

### ApplicationProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **environment** | string | Fully qualified resource ID for the environment that the portable resource is linked to  (_Required_) |
| **extensions** | [Extension](#extension)[] | The application extension. |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | Provisioning state of the portable resource at the time the operation was called  (_ReadOnly_) |
| **status** | [ResourceStatus](#resourcestatus) | Status of a resource.  (_ReadOnly_) |

### Extension

* **Discriminator**: kind

#### Base Properties

* **none**


#### DaprSidecarExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **appId** | string | The Dapr appId. Specifies the identifier used by Dapr for service invocation.  (_Required_) |
| **appPort** | int | The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar. |
| **config** | string | Specifies the Dapr configuration to use for the resource. |
| **kind** | 'daprSidecar' | Discriminator property for Extension.  (_Required_) |
| **protocol** | 'grpc' | 'http' | The Dapr sidecar extension protocol |

#### KubernetesMetadataExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **annotations** | [KubernetesMetadataExtensionAnnotations](#kubernetesmetadataextensionannotations) | Annotations to be applied to the Kubernetes resources output by the resource |
| **kind** | 'kubernetesMetadata' | Discriminator property for Extension.  (_Required_) |
| **labels** | [KubernetesMetadataExtensionLabels](#kubernetesmetadataextensionlabels) | Labels to be applied to the Kubernetes resources output by the resource |

#### KubernetesNamespaceExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'kubernetesNamespace' | Discriminator property for Extension.  (_Required_) |
| **namespace** | string | The namespace of the application environment.  (_Required_) |

#### ManualScalingExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'manualScaling' | Discriminator property for Extension.  (_Required_) |
| **replicas** | int | Replica count.  (_Required_) |


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

