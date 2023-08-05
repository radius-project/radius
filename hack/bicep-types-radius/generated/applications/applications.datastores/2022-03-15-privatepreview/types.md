# Applications.Datastores @ 2022-03-15-privatepreview

## Resource Applications.Datastores/mongoDatabases@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [MongoDatabaseProperties](#mongodatabaseproperties): MongoDatabase portable resource properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Datastores/mongoDatabases' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Datastores/redisCaches@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [RedisCacheProperties](#rediscacheproperties): RedisCache portable resource properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Datastores/redisCaches' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Datastores/sqlDatabases@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [SqlDatabaseProperties](#sqldatabaseproperties): SqlDatabase properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Datastores/sqlDatabases' (ReadOnly, DeployTimeConstant): The resource type

## Function listSecrets (Applications.Datastores/mongoDatabases@2022-03-15-privatepreview)
* **Resource**: Applications.Datastores/mongoDatabases
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [MongoDatabaseListSecretsResult](#mongodatabaselistsecretsresult)

## Function listSecrets (Applications.Datastores/redisCaches@2022-03-15-privatepreview)
* **Resource**: Applications.Datastores/redisCaches
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [RedisCacheListSecretsResult](#rediscachelistsecretsresult)

## Function listSecrets (Applications.Datastores/sqlDatabases@2022-03-15-privatepreview)
* **Resource**: Applications.Datastores/sqlDatabases
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [SqlDatabaseListSecretsResult](#sqldatabaselistsecretsresult)

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

## Recipe
### Properties
* **name**: string (Required): The name of the recipe within the environment to use
* **parameters**: any: Any object

## ResourceReference
### Properties
* **id**: string (Required): Resource id of an existing resource

## MongoDatabaseSecrets
### Properties
* **connectionString**: string: Connection string used to connect to the target Mongo database
* **password**: string: Password to use when connecting to the target Mongo database

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
* **database**: string: The name of the SQL database.
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **port**: int: Port value of the target SQL database
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: List of the resource IDs that support the SQL Database resource
* **secrets**: [SqlDatabaseSecrets](#sqldatabasesecrets): The secret values for the given SQL Database resource
* **server**: string: The fully qualified domain name of the SQL database.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **username**: string: Username to use when connecting to the target SQL database

## SqlDatabaseSecrets
### Properties
* **connectionString**: string: Connection string used to connect to the target SQL database
* **password**: string: Password to use when connecting to the target SQL database

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## MongoDatabaseListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): Connection string used to connect to the target Mongo database
* **password**: string (ReadOnly): Password to use when connecting to the target Mongo database

## RedisCacheListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): The connection string used to connect to the Redis cache
* **password**: string (ReadOnly): The password for this Redis cache instance
* **url**: string (ReadOnly): The URL used to connect to the Redis cache

## SqlDatabaseListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): Connection string used to connect to the target SQL database
* **password**: string (ReadOnly): Password to use when connecting to the target SQL database

