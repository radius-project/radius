# Test.Rp1 @ 2021-10-31

## Resource Test.Rp1/testType1@2021-10-31
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [TestType1Properties](#testtype1properties)
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Test.Rp1/testType1' (ReadOnly, DeployTimeConstant): The resource type

## Function listArrayOfFoos (Test.Rp1/testType1@2021-10-31)
* **Resource**: Test.Rp1/testType1
* **ApiVersion**: 2021-10-31
* **Output**: [FoosResponse](#foosresponse)[]

## Function listFoos (Test.Rp1/testType1@2021-10-31)
* **Resource**: Test.Rp1/testType1
* **ApiVersion**: 2021-10-31
* **Input**: [FoosRequest](#foosrequest)
* **Output**: [FoosResponse](#foosresponse)

## TestType1Properties
### Properties
* **basicString**: string: Description for a basic string property.
* **stringEnum**: 'Bar' | 'Foo': Description for a basic enum property.

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

## FoosResponse
### Properties
* **someString**: string (ReadOnly): The foo response string

## FoosRequest
### Properties
* **someString**: string (Required, WriteOnly): The foo request string

## FoosResponse
### Properties
* **someString**: string (ReadOnly): The foo response string

