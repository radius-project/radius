# Test.Rp1 @ 2021-10-31

## Resource Test.Rp1/discriminatedUnionTestType@2021-10-31
* **Valid Scope(s)**: ResourceGroup
* **Discriminator**: type

### Base Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **bar**: string: The bar property
* **foo**: string: The foo property
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **type**: 'Test.Rp1/discriminatedUnionTestType' (ReadOnly, DeployTimeConstant): The resource type

### DiscriminatedUnionTestTypeBranchWithInheritedProps
#### Properties
* **baz**: string: The baz property
* **quux**: string: A property defined inline
* **type**: 'inherited' (Required): The variant of this type

### DiscriminatedUnionTestTypeBranchWithAllInlineProps
#### Properties
* **buzz**: string: The buzz property
* **fizz**: string: The fizz property
* **pop**: string: The pop property
* **type**: 'inline' (Required): The variant of this type

### DiscriminatedUnionTestTypeBranchWithOverride
#### Properties
* **foo**: int: The overridden foo property
* **type**: 'override' (Required): The variant of this type


## Resource Test.Rp1/partlyReadonlyType@2021-10-31
* **Valid Scope(s)**: Tenant (ReadOnly), ResourceGroup
### Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [TestType1Properties](#testtype1properties)
* **systemData**: [SystemData](#systemdata) (ReadOnly): Azure Resource Manager metadata containing createdBy and modifiedBy information.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Test.Rp1/partlyReadonlyType' (ReadOnly, DeployTimeConstant): The resource type

## Resource Test.Rp1/readOnlyTestType@2021-10-31 (ReadOnly)
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (ReadOnly): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ReadOnlyTestTypeProperties](#readonlytesttypeproperties) (ReadOnly)
* **systemData**: [SystemData](#systemdata) (ReadOnly): Azure Resource Manager metadata containing createdBy and modifiedBy information.
* **tags**: [TrackedResourceTags](#trackedresourcetags) (ReadOnly): Resource tags.
* **type**: 'Test.Rp1/readOnlyTestType' (ReadOnly, DeployTimeConstant): The resource type

## Resource Test.Rp1/splitPutAndGetType@2021-10-31
* **Valid Scope(s)**: Subscription
### Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: 'constantName' | 'yetAnotherName' | string (Required, DeployTimeConstant): The resource name
* **properties**: [TestType1CreateOrUpdatePropertiesOrTestType1Properties](#testtype1createorupdatepropertiesortesttype1properties): The resource properties.
* **systemData**: [SystemData](#systemdata) (ReadOnly): Azure Resource Manager metadata containing createdBy and modifiedBy information.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Test.Rp1/splitPutAndGetType' (ReadOnly, DeployTimeConstant): The resource type

## Resource Test.Rp1/testType1@2021-10-31
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2021-10-31' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [TestType1CreateOrUpdatePropertiesOrTestType1Properties](#testtype1createorupdatepropertiesortesttype1properties): The resource properties.
* **systemData**: [SystemData](#systemdata) (ReadOnly): Azure Resource Manager metadata containing createdBy and modifiedBy information.
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

## EncryptionProperties
### Properties
* **keyVaultProperties**: [KeyVaultProperties](#keyvaultproperties): Key vault properties.
* **status**: 'disabled' | 'enabled' | string: Indicates whether or not the encryption is enabled for container registry.

## FoosRequest
### Properties
* **locationData**: [LocationData](#locationdata): Metadata pertaining to the geographic location of the resource.
* **someString**: string (Required): The foo request string

## FoosResponse
### Properties
* **someString**: string: The foo response string

## FoosResponse
### Properties
* **someString**: string: The foo response string

## KeyVaultProperties
### Properties
* **identity**: string: The client ID of the identity which will be used to access key vault.
* **keyIdentifier**: string: Key vault uri to access the encryption key.

## LocationData
### Properties
* **city**: string: The city or locality where the resource is located.
* **countryOrRegion**: string: The country or region where the resource is located
* **district**: string: The district, state, or province where the resource is located.
* **name**: string (Required): A canonical name for the geographic or physical location.

## Plan
### Properties
* **name**: string (Required): A user defined name of the 3rd Party Artifact that is being procured.
* **product**: string (Required): The 3rd Party artifact that is being procured. E.g. NewRelic. Product maps to the OfferID specified for the artifact at the time of Data Market onboarding.
* **promotionCode**: string: A publisher provided promotion code as provisioned in Data Market for the said product/artifact.
* **publisher**: string (Required): The publisher of the 3rd Party Artifact that is being bought. E.g. NewRelic
* **version**: string: The version of the desired product/artifact.

## ReadOnlyTestTypeProperties
### Properties
* **plan**: [Plan](#plan): Plan for the resource.

## SystemData
### Properties
* **createdAt**: string: The timestamp of resource creation (UTC).
* **createdBy**: string: The identity that created the resource.
* **createdByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User' | string: The type of identity that created the resource.
* **lastModifiedAt**: string: The timestamp of resource last modification (UTC)
* **lastModifiedBy**: string: The identity that last modified the resource.
* **lastModifiedByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User' | string: The type of identity that last modified the resource.

## TestType1CreateOrUpdatePropertiesOrTestType1Properties
### Properties
* **base64EncodedBytes**: string (ReadOnly)
* **basicString**: string: Description for a basic string property.
* **binaryBuffer**: any (ReadOnly)
* **encryptionProperties**: [EncryptionProperties](#encryptionproperties): TestType1 encryption properties
* **locationData**: [LocationData](#locationdata) (ReadOnly): Metadata pertaining to the geographic location of the resource.
* **skuTier**: 'Basic' | 'Free' | 'Premium' | 'Standard': This field is required to be implemented by the Resource Provider if the service has more than one tier, but is not required on a PUT.
* **stringEnum**: 'Bar' | 'Foo' | string: Description for a basic enum property.
* **subnetId**: string (ReadOnly): A fully-qualified resource ID

## TestType1Properties
### Properties
* **base64EncodedBytes**: string
* **basicString**: string: Description for a basic string property.
* **binaryBuffer**: any
* **encryptionProperties**: [EncryptionProperties](#encryptionproperties): TestType1 encryption properties
* **locationData**: [LocationData](#locationdata): Metadata pertaining to the geographic location of the resource.
* **skuTier**: 'Basic' | 'Free' | 'Premium' | 'Standard': This field is required to be implemented by the Resource Provider if the service has more than one tier, but is not required on a PUT.
* **stringEnum**: 'Bar' | 'Foo' | string: Description for a basic enum property.
* **subnetId**: string: A fully-qualified resource ID

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

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

