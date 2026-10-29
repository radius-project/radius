## AWS Support in UCP

AWS is a combination of heterogeneous services such as Kinesis, VPC, etc. Each of these services provides its own API and CLI commands which are not necessarily consistent with each other. AWS has introduced the Cloud Control API which is a wrapper on top of these services which enables users to use them with a consistent set of APIs.

In UCP, we use AWS Cloud Control APIs

With UCP, we want to support ARM-RPC protocol for clients even for systems such as AWS that do not support this protocol. While adding AWS support in UCP, we were faced with the following challenges:-
* AWS Cloud Control API has separate Create and Update operations instead of an idempotent PUT operation. The Update operation requires UCP to compute a JSON patch with the changes to be applied to the existing resource.
* AWS resources do not have a pre-defined property such as "name" that defines the resource name but is instead defined by fields that are marked as "Primary Identifiers" in the resource schema
* AWS resources can have multiple primary identifiers. In this case, the resource is uniquely identified by a combination of all primary identifiers e.g.: PrimaryIdentifierA | PrimaryIdentifierB | PrimaryIdentifierC.
* AWS resources can have generated names which means that the complete resourceID is not always known before resource creation.
* AWS has read and create only properties. This poses a challenge for computing a JSON patch for translating a PUT request to an Update AWS operation.
* Update call with no updates to the resource hangs indefinitely (operation is always in a “PENDING” state from AWS). This is currently handled by determining if the JSON patch for an update operation is empty and if empty, do not make a call to AWS and treat the operation like a no-op.

<br/><br/>

### Comparing values AWS reports in a different form

AWS often reports a value in a different form than the template set it. `GeneratePatch` (`pkg/aws/operations`) uses the type's schema, already loaded with `DescribeType`, to count these as unchanged, so an unchanged redeploy sends nothing to AWS:

- **Arrays where order does not matter.** The schema marks them `"insertionOrder": false`, such as `SubnetIds` on a DB subnet group and `Tags` on most types. They are compared ignoring order, as multisets, with each item compared by its own rules.
- **Values AWS rewrites.** The schema's `propertyTransform` turns the template's value into a pattern AWS's value must match. For example, an RDS `EngineVersion` of `8.4` matches a running `8.4.11`, and `DBInstanceIdentifier` is compared in lowercase. `$lowercase`, `$uppercase`, `$join`, `$string`, and `$OR` are supported, with string literals, array literals, and property references. A reference resolves against the property's parent object first, then the root. Text from the template is matched literally, and string literals in the expression are regular expressions. A property whose transform uses any other function or operator, or has an alternative that reads no template property (such as a fixed KMS key ARN pattern), is compared as it is, so a real change is still sent.
- **Empty objects.** An empty object AWS reports, such as `DataProtectionPolicy: {}` on a log group, is kept unless the template sets values inside it.

<br/><br/>

### Extension of ARM-RPC protocol in UCP

All Azure resources have a name where the name is used in the resource ID
/subscriptions/…/providers/Microsoft.KeyVault/vaults/{name}. However, AWS resources don’t have a consistent name property but use PrimaryIdentifier(s) for resource name. More than one primary identifier may exist, which would be separated by ‘|’. For example, if the RestApiId = ‘my-id’ and StageName is ‘my-stage’, the name of the resource would be ‘my-id|my-stage’.

<br/><br/>

Fundamentally, as part of the ARM API contract, name is required for GET, PUT, etc. operations. For example,
```
GET /subscriptions/…/providers/Microsoft.KeyVault/vaults/{name}
````

If we don’t have a name, either we need to calculate with additional information in the Deployment Engine, or we need to calculate it in the UCP-AWS provider and determine what it means for the API contract to not know the name prior to these calls.

<br/><br/>

To address this issue, we have extended the ARM-RPC protocol to add POST APIs to the UCP contract with a get/put/delete action and a request body.
```
POST http://127.0.0.1:8001/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-east-2/providers/AWS.ApiGateway/Stage/:get

With Request Body:
{
  "properties": {
    "RestApiId": "lfx3ec07zf",
    "StageName": "canary"
  }
}
```

The pros and cons of this design are:-

Pros:-
* Requires only UCP to have knowledge of AWS resources and primary identifiers
* Same number of network calls as normal Azure resource deployment, no extra hop to calculate resource ID.

Cons:-
* Requires extending the ARM protocol
* Full resource ID is not known prior to first resource operation

<br/><br/>

### Handling non-idempotent AWS resources

Many AWS resources have a generated name and the resource schema does not necessarily have any field which could uniquely identify the resource. As a result, every time the same template is deployed, it could result in creation of a new resource with a new generated name.

To address this issue, we will introduce state storage in UCP. The user will specify a friendly name for the resource in the bicep file that is unique in the deployment scope (which will be the Radius resource group). UCP will create a mapping between the friendly name and the actual AWS resource deployed. After this point, UCP will use this mapping to determine if the resource with the particular friendly name is being created or updated.

The details of this design can be found at: https://github.com/radius-project/design-notes/pull/21

<br/><br/>

### AWS types that Cloud Control cannot fully manage

UCP returns a `400` for these AWS types instead of the error Cloud Control returns. Error messages name the type and properties, never property values.

| Type                                                                                      | How UCP knows                                                                                       | What UCP returns                                                                                                                                                                                                            |
|-------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Legacy type with a supported replacement, for example `AWS::IAM::Policy`                  | List in `pkg/aws/awstypes`                                                                          | `400` naming the replacement, on every operation. Nothing is called in AWS                                                                                                                                                  |
| Deprecated service or CloudFormation template feature, for example `AWS::OpsWorks::Stack` | List in `pkg/aws/awstypes`                                                                          | `400` saying Radius does not support the type, on every operation. Nothing is called in AWS                                                                                                                                 |
| Read-only type created by AWS, for example `AWS::EC2::Image`                              | `DescribeType` reports `NON_PROVISIONABLE`, and the schema has a read handler but no create handler | `400` on a PUT, before UCP reads the resource, saying the type can only be used with `existing`                                                                                                                             |
| Other type Cloud Control cannot create                                                    | `DescribeType` reports `NON_PROVISIONABLE`                                                          | `400` on a PUT, before UCP reads the resource                                                                                                                                                                               |
| Create-and-delete-only type                                                               | `DescribeType` reports `IMMUTABLE`                                                                  | Create works. A change to an existing resource fails with `400` naming the changed properties. AWS never returns write-only properties, so a request that sets only write-only properties also fails with `400` naming them |

UCP reads the provisioning type from CloudFormation at run time, so a type starts working without a Radius release when AWS adds Cloud Control support for it.

The read-only list in `pkg/aws/awstypes` is for callers that have no AWS access, such as the recipe engine. It is generated from a snapshot of the CloudFormation registry. To refresh it with AWS credentials, run `go run ./gen -refresh` from `pkg/aws/awstypes`.
