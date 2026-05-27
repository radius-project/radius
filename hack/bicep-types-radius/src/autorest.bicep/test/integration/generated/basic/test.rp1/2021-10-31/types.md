# Test.Rp1 @ 2021-10-31

## Resource Test.Rp1/testType1@2021-10-31
* **Readable Scope(s)**: ResourceGroup
* **Writable Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **basicString**: string: Description for a basic string property.
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string: The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant, Identifier): The resource name
* **stringEnum**: 'Bar' | 'Foo': Description for a basic enum property.
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Test.Rp1/testType1' (ReadOnly, DeployTimeConstant): The resource type

### Function listFoos
* **Output**: [FoosResponse](#foosresponse)
#### Parameters
0. **someString**: string

### Function listArrayOfFoos
* **Output**: [FoosResponse](#foosresponse)[]
#### Parameters

## Resource Test.Rp1/testType2@2021-10-31
* **Readable Scope(s)**: ResourceGroup
* **Writable Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string: The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant, Identifier): The resource name
* **properties**: [TestType2Properties](#testtype2properties): Polymorphic properties body.
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Test.Rp1/testType2' (ReadOnly, DeployTimeConstant): The resource type

## Resource Test.Rp1/testType3@2021-10-31
* **Readable Scope(s)**: ResourceGroup
* **Writable Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string: The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant, Identifier): The resource name
* **properties**: [TestType3Properties](#testtype3properties): Properties bag containing a 'name' child which would collide.
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Test.Rp1/testType3' (ReadOnly, DeployTimeConstant): The resource type

## SystemData
### Properties
* **createdAt**: string: The timestamp of resource creation (UTC).
* **createdBy**: string: The identity that created the resource.
* **createdByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User': The type of identity that created the resource.
* **lastModifiedAt**: string: The timestamp of resource last modification (UTC)
* **lastModifiedBy**: string: The identity that last modified the resource.
* **lastModifiedByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User': The type of identity that created the resource.

## TestType2Properties
* **Discriminator**: kind

### Base Properties

### TestType2VariantA
#### Properties
* **kind**: 'VariantA' (Required): The polymorphic discriminator.
* **valueA**: string: Value for variant A.


## TestType3Properties
### Properties
* **extra**: string: A non-conflicting sibling.
* **name**: string: Conflicts with the standardized resource 'name' property.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

