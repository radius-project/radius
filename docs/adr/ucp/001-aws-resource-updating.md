# 001 ADR: Update strategy for AWS resources

## Stakeholders

Radius Core Team

## Status

proposed

## Context

Radius (UCP) updates AWS Resources using the [AWS CloudControl update-resource API](https://docs.aws.amazon.com/cloudcontrolapi/latest/userguide/resource-operations-update.html). This API expects a [JSON Patch](https://jsonpatch.com/) document to know what updates to make to the resource.

Each resource type in AWS has a schema, which lists its properties and describes which properties on the resource are read-only, write-only, create-only, and so on.

### Example: AWS MemoryDB
Here is a snippet of the resource type schema of an AWS MemoryDB resource. 
```json
{
    "definitions": {
        "Endpoint": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "Address": {
                    "description": "The DNS address of the primary read-write node.",
                    "type": "string"
                },
                "Port": {
                    "description": "The port number that the engine is listening on. ",
                    "type": "integer"
                }
            }
        },
    },
    "properties": {
        "ClusterName": {
            "description": "The name of the cluster. This value must be unique as it also serves as the cluster identifier.",
            "pattern": "[a-z][a-z0-9\\-]*",
            "type": "string"
        },
        "ARN": {
            "description": "The Amazon Resource Name (ARN) of the cluster.",
            "type": "string"
        },
        "ClusterEndpoint": {
            "description": "The cluster endpoint.",
            "$ref": "#/definitions/Endpoint"
        },
        "NumShards": {
            "description": "The number of shards the cluster will contain.",
            "type": "integer"
        },
    },
    "readOnlyProperties": [
        "/properties/ClusterEndpoint/Address",
        "/properties/ClusterEndpoint/Port",
        "/properties/ARN",
    ],
    "createOnlyProperties": [
        "/properties/ClusterName",
    ],
}
```

Note that AWS properties can be nested (such as `/properties/ClusterEndpoint/Address`). Since there are nested properties in AWS resource type schemas, there needs to be special handling for them to ensure that the generated patch sent to `update-resource` is correct.

### Property Types

[Reference](https://docs.aws.amazon.com/cloudformation-cli/latest/userguide/resource-type-schema.html)

There are different types of properties that can exist on an AWS resource type:

#### readOnlyProperties
_Resource properties that can be returned by a read or list request, but can't be set by the user._

#### writeOnlyProperties
_Resource properties that can be specified by the user, but can't be returned by a read or list request._

#### conditionalCreateOnlyProperties
_A list of JSON pointers for properties that can only be updated under certain conditions. For example, you can upgrade the engine version of an RDS DBInstance but you cannot downgrade it. When updating this property for a resource in a CloudFormation stack, the resource will be replaced if it cannot be updated._

If a `conditionalCreateOnlyProperty` is specified and fulfills the condition to be updatable, then it will successfully be updated. Otherwise, it will throw an error to the user. e.g.:

```
"code": "ResourceConflict",
"message": "VPC Multi-AZ DB Instances are not available for engine: sqlserver-ex (Service: Rds, Status Code: 400, Request ID: c6f2ad6b-29cc-4024-bbe5-7ce3e8a4b5ad)"
```

#### nonPublicProperties
_A list of JSON pointers for properties that are hidden. These properties will still be used but will not be visible._

#### createOnlyProperties
_Resource properties that can be specified by the user only during resource creation._

#### deprecatedProperties
_Resource properties that have been deprecated by the underlying service provider. These properties are still accepted in create and update operations. However they may be ignored, or converted to a consistent model on application. Deprecated properties are not guaranteed to be returned by read operations._


### How CloudControl updates resources

[Reference](https://docs.aws.amazon.com/cloudcontrolapi/latest/userguide/resource-operations-update.html#resource-operations-update-patch)

The update request to CloudControl executes in two steps:

1. CloudControl will sequentially apply the operations in the patch document, using the output of the previous operation as the input of the next. Note that this process will not result in any actual changes to the resource in AWS yet. If an error occurs during (for example, specifying a `readOnlyProperty`, CloudControl will stop the patching and report the error back.

2. If validation from each of the patch operations succeeds, then CloudControl will update the resource to the DesiredState specified by the patch document.

This means that update requests via CloudControl will occur in an idempotent way. Any invalid properties sent to CloudControl will not result in the resource being edited.


## Decision

### Strategy

The strategy for handling these updates with nested resources is the following:

1. Retrieve the desired state of the resource (from user-specified bicep manifest)
1. Retrieve the current state of the resource (from CloudControl `get-resource` API)
1. Retrieve the resource type schema of the resource (from CloudFormation `describe-type` API)
1. "Flatten" the current and desired states (details below)
1. Add read-only and create-only properties from the current state to the desired state
1. "Unflatten" the current and desired states (details below)
1. Compare the current state and the updated desired state to produce the patch document

### Flatten/Unflatten

#### Flatten

Flatten takes an object that may have nested properties and converts it to an object with only top-level properties.

```go
in := map[string]interface{}{
    "NumShards": 1,
    "ClusterEndpoint": map[string]interface{}{
        "Address": "test-address",
        "Port": 3000,
    },
}

out := Flatten(in)
// out = map[string]interface{}{
//     "NumShards": 1,
//     "ClusterEndpoint/Address": "test-address",
//     "ClusterEndpoint/Port": 3000,
// }
```

#### Unflatten

Unflatten takes an object with top-level properties (specified in paths, such as `ClusterEndpoint/Address`) and converts it to an object that can have nested properties.

```go
in := map[string]interface{}{
    "NumShards": 1,
    "ClusterEndpoint/Address": "test-address",
    "ClusterEndpoint/Port": 3000,
}

out := Unflatten(in)
// out = map[string]interface{}{
//     "NumShards": 1,
//     "ClusterEndpoint": map[string]interface{}{
//         "Address": "test-address",
//         "Port": 3000,
//     },
// }
```

### Property Handling

#### Create-Only Properties
Create-only properties from the current state are added (if not present) to the desired state so that when the desired state is compared to the current state, no patch is generated.

#### Read-Only Properties
Read-only properties from the current state are added (if not present) to the desired state so that when the desired state is compared to the current state, no patch is generated.

#### Conditional Create-Only Properties
We will intentionally not do any validation on conditional create-only properties because the value may be valid as an update. If it is invalid, CloudControl will send back an error to the user and perform no updates to the resource.

#### Write-Only Properties
Write-only properties are never returned as part of the GetResource response. This means that we cannot create a diff against write-only properties.


## Consequences

### Update logic can now handle nested properties
Flattening the current and desired states allows for each of the special property types to be considered during the comparison. For example, since the `ClusterEndpoint` property itself is not read-only and all of its sub-properties (`Address`, `Port`) are, the user would not be allowed to provide `ClusterEndpoint` in their desired state for a MemoryDB resource. Without performing special handling (i.e. just determining the patch from the top-level fields), an invalid patch would be generated.

### Update logic is more complicated
Introducing the flatten/unflatten code makes the update code more complicated and prone to bugs. Extra unit tests must also be run to validate that this behavior works correctly.

### Create-And-Write-Only properties cannot be updated
Since write-only properties cannot come back as a response from AWS, they cannot be compared to the user's desired state. Therefore, on update requests, we ignore create-and-write-only properties. This means that a user could create a resource, update a create-and-write-only property, try to update the resource, and see that this property update is not reflected in the resource state.

### Array properties will currently be unsupported
Some properties can be specified under an array entry. For example, `/properties/DefaultActions/*/TargetGroupArn` under the AWS::ElasticLoadBalancingV2::Listener resource. This behavior will currently be unsupported by this design.
