# Applications.Dapr @ 2022-03-15-privatepreview

## Resource Applications.Dapr/pubSubBrokers@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprPubSubBrokerProperties](#daprpubsubbrokerproperties): Dapr PubSubBroker portable resource properties
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
* **properties**: [DaprSecretStoreProperties](#daprsecretstoreproperties): Dapr SecretStore portable resource properties
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
* **properties**: [DaprStateStoreProperties](#daprstatestoreproperties): Dapr StateStore portable resource properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Dapr/stateStores' (ReadOnly, DeployTimeConstant): The resource type

## DaprPubSubBrokerProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **metadata**: any: Any object
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: A collection of references to resources associated with the Dapr PubSubBroker
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: string: Dapr PubSubBroker type. These strings match the format used by Dapr Kubernetes configuration format.
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
* **outputResources**: any[]: Properties of an output resource

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
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **metadata**: any: Any object
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: string: Dapr SecretStore type. These strings match the types defined in Dapr Component format: https://docs.dapr.io/reference/components-reference/supported-secret-stores/
* **version**: string: Dapr component version

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## DaprStateStoreProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **metadata**: any: Any object
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: A collection of references to resources associated with the Dapr StateStore
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: string: Dapr StateStore type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string: Dapr component version

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

