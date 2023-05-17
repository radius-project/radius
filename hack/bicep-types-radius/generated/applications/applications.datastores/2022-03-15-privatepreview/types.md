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

## MongoDatabaseProperties
* **Discriminator**: mode

### Base Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **secrets**: [MongoDatabaseSecrets](#mongodatabasesecrets): The secret values for the given MongoDatabase resource
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### RecipeMongoDatabaseProperties
#### Properties
* **database**: string (ReadOnly): Database name of the target Mongo database
* **host**: string: Host name of the target Mongo database
* **mode**: 'recipe' (Required): Discriminator property for MongoDatabaseProperties.
* **port**: int: Port value of the target Mongo database
* **recipe**: [Recipe](#recipe) (Required): The recipe used to automatically deploy underlying infrastructure for a link

### ResourceMongoDatabaseProperties
#### Properties
* **database**: string (ReadOnly): Database name of the target Mongo database
* **host**: string: Host name of the target Mongo database
* **mode**: 'resource' (Required): Discriminator property for MongoDatabaseProperties.
* **port**: int: Port value of the target Mongo database
* **resource**: string (Required): Fully qualified resource ID of a supported resource with Mongo API to use for this portable resource

### ValuesMongoDatabaseProperties
#### Properties
* **database**: string (ReadOnly): Database name of the target Mongo database
* **host**: string (Required): Host name of the target Mongo database
* **mode**: 'values' (Required): Discriminator property for MongoDatabaseProperties.
* **port**: int (Required): Port value of the target Mongo database


## MongoDatabaseSecrets
### Properties
* **connectionString**: string: Connection string used to connect to the target Mongo database
* **password**: string: Password to use when connecting to the target Mongo database
* **username**: string: Username to use when connecting to the target Mongo database

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

## RedisCacheProperties
* **Discriminator**: mode

### Base Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **secrets**: [RedisCacheSecrets](#rediscachesecrets): The secret values for the given RedisCache resource
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### RecipeRedisCacheProperties
#### Properties
* **host**: string: The host name of the target Redis cache
* **mode**: 'recipe' (Required): Discriminator property for RedisCacheProperties.
* **port**: int: The port value of the target Redis cache
* **recipe**: [Recipe](#recipe) (Required): The recipe used to automatically deploy underlying infrastructure for a link
* **username**: string (ReadOnly): The username for Redis cache

### ResourceRedisCacheProperties
#### Properties
* **host**: string: The host name of the target Redis cache
* **mode**: 'resource' (Required): Discriminator property for RedisCacheProperties.
* **port**: int: The port value of the target Redis cache
* **resource**: string (Required): Fully qualified resource ID of a supported resource with Redis API to use for this portable resource
* **username**: string (ReadOnly): The username for Redis cache

### ValuesRedisCacheProperties
#### Properties
* **host**: string (Required): The host name of the target Redis cache
* **mode**: 'values' (Required): Discriminator property for RedisCacheProperties.
* **port**: int (Required): The port value of the target Redis cache
* **username**: string (ReadOnly): The username for Redis cache


## RedisCacheSecrets
### Properties
* **connectionString**: string: The connection string used to connect to the Redis cache
* **password**: string: The password for this Redis cache instance

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## SqlDatabaseProperties
* **Discriminator**: mode

### Base Properties
* **application**: string: Fully qualified resource ID for the application that the link is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the link is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the link at the time the operation was called
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### RecipeSqlDatabaseProperties
#### Properties
* **database**: string: The name of the Sql database.
* **mode**: 'recipe' (Required): Discriminator property for SqlDatabaseProperties.
* **recipe**: [Recipe](#recipe) (Required): The recipe used to automatically deploy underlying infrastructure for a link
* **server**: string: The fully qualified domain name of the Sql database.

### ResourceSqlDatabaseProperties
#### Properties
* **database**: string: The name of the Sql database.
* **mode**: 'resource' (Required): Discriminator property for SqlDatabaseProperties.
* **resource**: string (Required): Fully qualified resource ID of a supported resource with Sql API to use for this portable resource
* **server**: string: The fully qualified domain name of the Sql database.

### ValuesSqlDatabaseProperties
#### Properties
* **database**: string (Required): The name of the Sql database.
* **mode**: 'values' (Required): Discriminator property for SqlDatabaseProperties.
* **server**: string (Required): The fully qualified domain name of the Sql database.


## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## MongoDatabaseListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): Connection string used to connect to the target Mongo database
* **password**: string (ReadOnly): Password to use when connecting to the target Mongo database
* **username**: string (ReadOnly): Username to use when connecting to the target Mongo database

## RedisCacheListSecretsResult
### Properties
* **connectionString**: string (ReadOnly): The connection string used to connect to the Redis cache
* **password**: string (ReadOnly): The password for this Redis cache instance

