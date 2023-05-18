# Applications.Dapr @ 2022-03-15-privatepreview

## Resource Applications.Dapr/daprPubSubBrokers@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprPubSubBrokerProperties](#daprpubsubbrokerproperties): Dapr PubSubBroker portable resource properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Dapr/daprPubSubBrokers' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Dapr/daprSecretStores@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprSecretStoreProperties](#daprsecretstoreproperties): Dapr SecretStore portable resource properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Dapr/daprSecretStores' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Dapr/daprStateStores@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprStateStoreProperties](#daprstatestoreproperties): Dapr StateStore link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Dapr/daprStateStores' (ReadOnly, DeployTimeConstant): The resource type

## DaprPubSubBrokerProperties
* **Discriminator**: mode

### Base Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **topic**: string: Topic name of the Azure ServiceBus resource
### RecipeDaprPubSubProperties
#### Properties
* **metadata**: any: Any object
* **mode**: 'recipe' (Required): Discriminator property for DaprPubSubBrokerProperties.
* **recipe**: [Recipe](#recipe) (Required): The recipe used to automatically deploy underlying infrastructure for a link
* **type**: string: Dapr PubSub type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string: Dapr component version

### ResourceDaprPubSubProperties
#### Properties
* **metadata**: any: Any object
* **mode**: 'resource' (Required): Discriminator property for DaprPubSubBrokerProperties.
* **resource**: string (Required): PubSub resource
* **type**: string: Dapr PubSub type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string: Dapr component version

### ValuesDaprPubSubProperties
#### Properties
* **metadata**: any (Required): Any object
* **mode**: 'values' (Required): Discriminator property for DaprPubSubBrokerProperties.
* **type**: string (Required): Dapr PubSub type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string (Required): Dapr component version


## ResourceStatus
### Properties
* **outputResources**: any[]: Properties of an output resource

## Recipe
### Properties
* **name**: string (Required): The name of the recipe within the environment to use
* **parameters**: any: Any object

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
* **Discriminator**: mode

### Base Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### RecipeDaprSecretStoreProperties
#### Properties
* **metadata**: any: Any object
* **mode**: 'recipe' (Required): Discriminator property for DaprSecretStoreProperties.
* **recipe**: [Recipe](#recipe) (Required): The recipe used to automatically deploy underlying infrastructure for a link
* **type**: string: Dapr Secret Store type. These strings match the types defined in Dapr Component format: https://docs.dapr.io/reference/components-reference/supported-secret-stores/
* **version**: string: Dapr component version

### ValuesDaprSecretStoreProperties
#### Properties
* **metadata**: any (Required): Any object
* **mode**: 'values' (Required): Discriminator property for DaprSecretStoreProperties.
* **type**: string (Required): Dapr Secret Store type. These strings match the types defined in Dapr Component format: https://docs.dapr.io/reference/components-reference/supported-secret-stores/
* **version**: string (Required): Dapr component version


## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## DaprStateStoreProperties
* **Discriminator**: mode

### Base Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### RecipeDaprStateStoreProperties
#### Properties
* **metadata**: any: Any object
* **mode**: 'recipe' (Required): Discriminator property for DaprStateStoreProperties.
* **recipe**: [Recipe](#recipe) (Required): The recipe used to automatically deploy underlying infrastructure for a link
* **type**: string: Dapr StateStore type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string: Dapr component version

### ResourceDaprStateStoreProperties
#### Properties
* **metadata**: any: Any object
* **mode**: 'resource' (Required): Discriminator property for DaprStateStoreProperties.
* **resource**: string (Required): The resource id of the Azure SQL Database or Azure Table Storage the Dapr StateStore resource is connected to.
* **type**: string: Dapr StateStore type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string: Dapr component version

### ValuesDaprStateStoreProperties
#### Properties
* **metadata**: any (Required): Any object
* **mode**: 'values' (Required): Discriminator property for DaprStateStoreProperties.
* **type**: string (Required): Dapr StateStore type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string (Required): Dapr component version


## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

