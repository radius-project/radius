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

The details of this design can be found at: https://microsoft.sharepoint.com/:w:/t/radiuscoreteam/Ef0J0DM89-1Foyb36i4_a_EBn4zW61Dk8paVfJ9p9RUDOg?e=9tnaV1
