# Applications.Link @ 2022-03-15-privatepreview

## Resource Applications.Link/daprPubSubBrokers@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprPubSubBrokerProperties](#daprpubsubbrokerproperties): DaprPubSubBroker link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Link/daprPubSubBrokers' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Link/daprSecretStores@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprSecretStoreProperties](#daprsecretstoreproperties): DaprSecretStore link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Link/daprSecretStores' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Link/daprStateStores@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprStateStoreProperties](#daprstatestoreproperties): DaprStateStore link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Link/daprStateStores' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Link/extenders@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ExtenderProperties](#extenderproperties) (Required): Extender link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Link/extenders' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Link/mongoDatabases@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [MongoDatabaseProperties](#mongodatabaseproperties): MongoDatabase link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Link/mongoDatabases' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Link/rabbitMQMessageQueues@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [RabbitMQMessageQueueProperties](#rabbitmqmessagequeueproperties): RabbitMQMessageQueue link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Link/rabbitMQMessageQueues' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Link/redisCaches@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [RedisCacheProperties](#rediscacheproperties): RedisCache link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Link/redisCaches' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Link/sqlDatabases@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [SqlDatabaseProperties](#sqldatabaseproperties): SqlDatabase properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Link/sqlDatabases' (ReadOnly, DeployTimeConstant): The resource type

## Function listSecrets (Applications.Link/mongoDatabases@2022-03-15-privatepreview)
* **Resource**: Applications.Link/mongoDatabases
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [MongoDatabaseListSecretsResult](#mongodatabaselistsecretsresult)

## Function listSecrets (Applications.Link/rabbitMQMessageQueues@2022-03-15-privatepreview)
* **Resource**: Applications.Link/rabbitMQMessageQueues
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [RabbitMQListSecretsResult](#rabbitmqlistsecretsresult)

## Function listSecrets (Applications.Link/redisCaches@2022-03-15-privatepreview)
* **Resource**: Applications.Link/redisCaches
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [RedisCacheListSecretsResult](#rediscachelistsecretsresult)

## Function listSecrets (Applications.Link/sqlDatabases@2022-03-15-privatepreview)
* **Resource**: Applications.Link/sqlDatabases
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [SqlDatabaseListSecretsResult](#sqldatabaselistsecretsresult)

## Function listSecrets (Applications.Link/extenders@2022-03-15-privatepreview)
* **Resource**: Applications.Link/extenders
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [ExtenderSecrets](#extendersecrets)

## DaprPubSubBrokerProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **componentName**: string (ReadOnly): The name of the Dapr component object. Use this value in your code when interacting with the Dapr client to use the Dapr component.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **metadata**: any: Any object
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: A collection of references to resources associated with the daprPubSubBroker
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: string: DaprPubSubBroker type. These strings match the format used by Dapr Kubernetes configuration format.
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
* **type**: string: Dapr Secret Store type. These strings match the types defined in Dapr Component format: https://docs.dapr.io/reference/components-reference/supported-secret-stores/
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
* **resources**: [ResourceReference](#resourcereference)[]: A collection of references to resources associated with the state store
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: string: Dapr StateStore type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string: Dapr component version

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ExtenderProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **secrets**: [ExtenderSecrets](#extendersecrets): The secret values for the given Extender resource
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### Additional Properties
* **Additional Properties Type**: any

## ExtenderSecrets
### Properties
### Additional Properties
* **Additional Properties Type**: any

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## MongoDatabaseProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **database**: string: Database name of the target Mongo database
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **host**: string: Host name of the target Mongo database
* **port**: int: Port value of the target Mongo database
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: List of the resource IDs that support the MongoDB resource
* **secrets**: [MongoDatabaseSecrets](#mongodatabasesecrets): The secret values for the given MongoDatabase resource
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **username**: string: Username to use when connecting to the target Mongo database

## MongoDatabaseSecrets
### Properties
* **connectionString**: string: Connection string used to connect to the target Mongo database
* **password**: string: Password to use when connecting to the target Mongo database

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## RabbitMQMessageQueueProperties
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
* **secrets**: [RabbitMQSecrets](#rabbitmqsecrets): The secret values for the given RabbitMQMessageQueue resource
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **username**: string: Username to use when connecting to the target rabbitMQ
* **vHost**: string: The vHost of the RabbitMQ instance

## RabbitMQSecrets
### Properties
* **password**: string: The password used to connect to this RabbitMQ instance
* **uri**: string: The connection URI of the RabbitMQ instance

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## RedisCacheProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **host**: string: The host name of the target Redis cache
* **port**: int: The port value of the target Redis cache
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: List of the resource IDs that support the Redis resource
* **secrets**: [RedisCacheSecrets](#rediscachesecrets): The secret values for the given RedisCache resource
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **tls**: bool: Specifies whether to enable SSL connections to the Redis cache
* **username**: string: The username for Redis cache

## RedisCacheSecrets
### Properties
* **connectionString**: string: The connection string used to connect to the Redis cache
* **password**: string: The password for this Redis cache instance
* **url**: string: The URL used to connect to the Redis cache

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## SqlDatabaseProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **database**: string: The name of the Sql database.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **port**: int: Port value of the target Sql database
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: List of the resource IDs that support the SqlDatabase resource
* **secrets**: [SqlDatabaseSecrets](#sqldatabasesecrets): The secret values for the given SqlDatabase resource
* **server**: string: The fully qualified domain name of the Sql database.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **username**: string: Username to use when connecting to the target Sql database

## SqlDatabaseSecrets
### Properties
* **connectionString**: string: Connection string used to connect to the target Sql database
* **password**: string: Password to use when connecting to the target Sql database

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## MongoDatabaseListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): Connection string used to connect to the target Mongo database
* **password**: string (ReadOnly): Password to use when connecting to the target Mongo database

## RabbitMQListSecretsResult
### Properties
* **password**: string (ReadOnly): The password used to connect to this RabbitMQ instance
* **uri**: string (ReadOnly): The connection URI of the RabbitMQ instance

## RedisCacheListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): The connection string used to connect to the Redis cache
* **password**: string (ReadOnly): The password for this Redis cache instance
* **url**: string (ReadOnly): The URL used to connect to the Redis cache

## SqlDatabaseListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): Connection string used to connect to the target Sql database
* **password**: string (ReadOnly): Password to use when connecting to the target Sql database

## ExtenderSecrets
### Properties
### Additional Properties
* **Additional Properties Type**: any

