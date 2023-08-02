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
### Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **host**: string: The hostname of the RabbitMQ instance
* **port**: int: The port of the RabbitMQ instance. Defaults to 5672
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **queue**: string: The name of the queue
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: List of the resource IDs that support the rabbitMQ resource
* **secrets**: [RabbitMQSecrets](#rabbitmqsecrets): The secret values for the given RabbitMQQueue resource
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **tls**: bool: Specifies whether to use SSL when connecting to the RabbitMQ instance
* **username**: string: The username to use when connecting to the RabbitMQ instance
* **vHost**: string: The RabbitMQ virtual host (vHost) the client will connect to. Defaults to no vHost.

## Recipe
### Properties
* **name**: string (Required): The name of the recipe within the environment to use
* **parameters**: any: Any object

## ResourceReference
### Properties
* **id**: string (Required): Resource id of an existing resource

## RabbitMQSecrets
### Properties
* **password**: string: The password used to connect to the RabbitMQ instance
* **uri**: string: The connection URI of the RabbitMQ instance. Generated automatically from host, port, SSL, username, password, and vhost. Can be overridden with a custom value

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

## RabbitMQListSecretsResult
### Properties
* **password**: string (ReadOnly): The password used to connect to the RabbitMQ instance
* **uri**: string (ReadOnly): The connection URI of the RabbitMQ instance. Generated automatically from host, port, SSL, username, password, and vhost. Can be overridden with a custom value

