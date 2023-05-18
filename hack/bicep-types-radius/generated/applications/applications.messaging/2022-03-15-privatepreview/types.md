# Applications.Messaging @ 2022-03-15-privatepreview

## Resource Applications.Messaging/rabbitMQQueues@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [RabbitMQQueueProperties](#rabbitmqqueueproperties): RabbitMQQueue portable resource properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Messaging/rabbitMQQueues' (ReadOnly, DeployTimeConstant): The resource type

## Function listSecrets (Applications.Messaging/rabbitMQQueues@2022-03-15-privatepreview)
* **Resource**: Applications.Messaging/rabbitMQQueues
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [RabbitMQListSecretsResult](#rabbitmqlistsecretsresult)

## RabbitMQQueueProperties
* **Discriminator**: mode

### Base Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **secrets**: [RabbitMQSecrets](#rabbitmqsecrets): The secret values for the given RabbitMQQueue resource
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### RecipeRabbitMQQueueProperties
#### Properties
* **mode**: 'recipe' (Required): Discriminator property for RabbitMQQueueProperties.
* **queue**: string: The name of the queue
* **recipe**: [Recipe](#recipe) (Required): The recipe used to automatically deploy underlying infrastructure for a link

### ValuesRabbitMQQueueProperties
#### Properties
* **mode**: 'values' (Required): Discriminator property for RabbitMQQueueProperties.
* **queue**: string (Required): The name of the queue


## RabbitMQSecrets
### Properties
* **connectionString**: string: The connection string used to connect to this RabbitMQ instance

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

## RabbitMQListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): The connection string used to connect to this RabbitMQ instance

