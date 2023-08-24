# Applications.Dapr @ 2022-03-15-privatepreview

## Resource Applications.Dapr/pubSubBrokers@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprPubSubBrokerProperties](#daprpubsubbrokerproperties): PubSubBroker link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Dapr/pubSubBrokers' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Dapr/secretStores@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprSecretStoreProperties](#daprsecretstoreproperties): DaprSecretStore link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Dapr/secretStores' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Dapr/stateStores@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprStateStoreProperties](#daprstatestoreproperties): StateStore link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Dapr/stateStores' (ReadOnly, DeployTimeConstant): The resource type

## DaprPubSubBrokerProperties
### Properties
* **application**: string (Required): Fully qualified resource ID for the application that the portable resource is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string: Fully qualified resource ID for the environment that the portable resource is linked to (if applicable)
* **metadata**: any: Any object
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: A collection of references to resources associated with the pubSubBroker
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: string: Dapr component type which must matches the format used by Dapr Kubernetes configuration format
* **version**: string: Dapr component version

## Recipe
### Properties
* **name**: string (Required): The name of the recipe within the environment to use
* **parameters**: any: Any object

## ResourceReference
### Properties
* **id**: string (Required): Resource id of an existing resource

## ResourceStatus
### Properties
* **compute**: [EnvironmentCompute](#environmentcompute): Represents backing compute resource
* **outputResources**: [OutputResource](#outputresource)[]: Properties of an output resource

## EnvironmentCompute
* **Discriminator**: kind

### Base Properties
* **identity**: [IdentitySettings](#identitysettings): IdentitySettings is the external identity setting.
* **resourceId**: string: The resource id of the compute resource for application environment.
### KubernetesCompute
#### Properties
* **kind**: 'kubernetes' (Required): Discriminator property for EnvironmentCompute.
* **namespace**: string (Required): The namespace to use for the environment.


## IdentitySettings
### Properties
* **kind**: 'azure.com.workload' | 'undefined' (Required): IdentitySettingKind is the kind of supported external identity setting
* **oidcIssuer**: string: The URI for your compute platform's OIDC issuer
* **resource**: string: The resource ID of the provisioned identity

## OutputResource
### Properties
* **id**: string: The UCP resource ID of the underlying resource.
* **localId**: string: The logical identifier scoped to the owning Radius resource. This is only needed or used when a resource has a dependency relationship. LocalIDs do not have any particular format or meaning beyond being compared to determine dependency relationships.
* **radiusManaged**: bool: Determines whether Radius manages the lifecycle of the underlying resource.

## SystemData
### Properties
* **createdAt**: string: The timestamp of resource creation (UTC).
* **createdBy**: string: The identity that created the resource.
* **createdByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User': The type of identity that created the resource.
* **lastModifiedAt**: string: The timestamp of resource last modification (UTC)
* **lastModifiedBy**: string: The identity that last modified the resource.
* **lastModifiedByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User': The type of identity that created the resource.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## DaprSecretStoreProperties
### Properties
* **application**: string (Required): Fully qualified resource ID for the application that the portable resource is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string: Fully qualified resource ID for the environment that the portable resource is linked to (if applicable)
* **metadata**: any: Any object
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: string: Dapr component type which must matches the format used by Dapr Kubernetes configuration format
* **version**: string: Dapr component version

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## DaprStateStoreProperties
### Properties
* **application**: string (Required): Fully qualified resource ID for the application that the portable resource is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string: Fully qualified resource ID for the environment that the portable resource is linked to (if applicable)
* **metadata**: any: Any object
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: A collection of references to resources associated with the state store
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: string: Dapr component type which must matches the format used by Dapr Kubernetes configuration format
* **version**: string: Dapr component version

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

