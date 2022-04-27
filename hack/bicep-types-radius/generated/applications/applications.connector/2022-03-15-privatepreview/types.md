# Applications.Connector @ 2022-03-15-privatepreview

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

## MongoDatabaseProperties
### Properties
* **application**: string (ReadOnly): Fully qualified resource ID for the application that the connector is consumed by
* **fromResource**: [FromResource](#fromresource): Target resource that the connector binds to
* **fromValues**: [SecretsValues](#secretsvalues): Secrets values provided for the resource
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' (ReadOnly): Provisioning state of the connector at the time the operation was called

## FromResource
### Properties
* **source**: string (Required, ReadOnly): Fully qualified resource ID for the resource that the connector binds to

## SecretsValues
### Properties
* **connectionString**: string: The connection string used to connect to the target mongo database the connector binds to
* **password**: string: The password to use when connecting to the target mongo database
* **username**: string: The username to use when connecting to the target mongo database

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

