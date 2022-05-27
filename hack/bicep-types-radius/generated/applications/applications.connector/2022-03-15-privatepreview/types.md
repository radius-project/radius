# Applications.Connector @ 2022-03-15-privatepreview

## Resource Applications.Connector/daprInvokeHttpRoutes@2022-03-15-privatepreview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprInvokeHttpRouteProperties](#daprinvokehttprouteproperties) (Required): DaprInvokeHttpRoute connector properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Connector/daprInvokeHttpRoutes' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Connector/mongoDatabases@2022-03-15-privatepreview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [MongoDatabaseProperties](#mongodatabaseproperties) (Required): MongoDatabse connector properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Connector/mongoDatabases' (ReadOnly, DeployTimeConstant): The resource type

## Function listSecrets (Applications.Connector/mongoDatabases@2022-03-15-privatepreview)
* **Resource**: Applications.Connector/mongoDatabases
* **ApiVersion**: 2022-03-15-privatepreview
* **Output**: [MongoDatabaseSecrets](#mongodatabasesecrets)

## DaprInvokeHttpRouteProperties
### Properties
* **appId**: string (Required): The Dapr appId used for the route
* **application**: string (ReadOnly): Fully qualified resource ID for the application that the connector is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the connector is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the connector at the time the operation was called
* **status**: [ResourceStatus](#resourcestatus): Status of a resource.

## ResourceStatus
### Properties
* **outputResources**: any[]: Array of AnyObject

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

## MongoDatabaseProperties
### Properties
* **application**: string (ReadOnly): Fully qualified resource ID for the application that the connector is consumed by
* **environment**: string (Required): Fully qualified resource ID for the environment that the connector is linked to
* **host**: string: Host name of the target Mongo database
* **port**: int: Port value of the target Mongo database
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the connector at the time the operation was called
* **resource**: string: Fully qualified resource ID of a supported resource with Mongo API to use for this connector
* **secrets**: [MongoDatabaseSecrets](#mongodatabasesecrets): The secret values for the given MongoDatabase resource
* **status**: [ResourceStatus](#resourcestatus): Status of a resource.

## MongoDatabaseSecrets
### Properties
* **connectionString**: string: Connection string used to connect to the target Mongo database
* **password**: string: Password to use when connecting to the target Mongo database
* **username**: string: Username to use when connecting to the target Mongo database

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## MongoDatabaseSecrets
### Properties
* **connectionString**: string: Connection string used to connect to the target Mongo database
* **password**: string: Password to use when connecting to the target Mongo database
* **username**: string: Username to use when connecting to the target Mongo database

