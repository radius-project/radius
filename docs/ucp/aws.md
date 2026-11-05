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

### Resource records and alias IDs

UCP keeps a record of each AWS resource it deploys, so redeploying a template updates the same resource. Every AWS resource in Bicep has an `alias`, and the Deployment Engine addresses the resource by it:

```text
/planes/aws/aws/accounts/{account}/regions/{region}/providers/AWS.EC2/VPC/app-vpc
```

The record (`datamodel.AWSResource`, read and written by `pkg/ucp/aws/records`) is stored in UCP's data store under that ID. It holds the AWS identifier, the owner, the properties of the last successful deploy without write-only values, fingerprints of the write-only values, and the state of the operation in progress. The design is in [radius-project/radius#13462](https://github.com/radius-project/radius/pull/13462).

- **PUT** reads the record. With no AWS identifier yet, UCP takes over a resource AWS already has, found by the top-level `name` or by the identifier in the properties, or creates one. Before it creates anything, UCP saves the record with a client token, and sends that token with every attempt to create the resource, so a retry never creates a second one. Cloud Control returns the first request only when the body matches it exactly. When a retry's template differs, for example in a write-only value, Cloud Control returns `ClientTokenConflictException` naming the first request, and UCP reads that request's status: when it succeeded, the PUT continues as an update with the template, comparing its write-only values with the fingerprints of the earlier attempt (when an earlier retry's outcome is unknown, UCP cannot tell which values AWS has, so they have no usable fingerprint); when it failed, UCP removes the record and the PUT returns the failure; while it runs, the PUT gets `409`. With a record, UCP compares the template with AWS and with the last-applied properties: a property the last deploy set and the template no longer sets is cleared, and a removed create-only property fails with `400`.
- **GET** reads the record, then AWS by the identifier in the record. `404` when there is no record. The Bicep recipe driver reads the ARN of each AWS output resource with GET, and on `404` with POST `/:get` by the last segment of its ID, because a resource a recipe references with `existing` has no record and its ID ends in the AWS identifier.
- **DELETE** reads the record, deletes the resource by its identifier, and then removes the record. `204` without calling AWS when there is no record, so a retried DELETE never deletes a different resource whose AWS identifier matches the alias.

  A record with a client token but no identifier has a create that may have reached AWS, so DELETE first sends the create again with the same token to find the resource. Cloud Control returns the original request only when the body matches it exactly. The resend has no write-only values, so for a type with write-only properties it gets `ClientTokenConflictException`, and UCP reads the status of the request it names to find the resource. When the message of `ClientTokenConflictException` has no request token UCP can read, the PUT or DELETE fails with `400` and keeps the record.
- **POST `/:get`** reads a resource Radius may not have created, for an `existing` block that sets `name` or `properties`. It reads AWS by the top-level `name` in the body when it is set, and otherwise by the primary or additional identifiers in the properties. It never saves a record.

PUT and DELETE require the `X-Radius-Resource-Owner` header, and fail with `400` without it. A PUT or DELETE from a different owner than the record's fails with `409`, and so does a PUT that would manage an AWS resource another alias's record already has. Owners are compared ignoring case.

UCP owns every create, update, and delete. It saves the new last-applied properties on the record as pending values before it calls AWS, saves the Cloud Control request token, and queues an operation. The backend controller in `pkg/ucp/backend/controller/awsresources` polls Cloud Control, then moves the pending values to last-applied on success, or drops them on failure so the previous ones stay. A failed create, and a finished delete, remove the record. If the worker stops after it removes the record, the redelivered operation reads its result from Cloud Control by the request token. The operation status and result routes read only UCP's data store, and return `404` for an operation UCP does not have. A second PUT or DELETE while an operation is running gets `409`. One that finds an operation nothing has polled for 10 minutes, or one AWS accepted but UCP never queued, resumes it.

Write-only values are never stored: not in the last-applied or pending properties, and not in operations. They are also removed from every response, including values AWS returns: create, update, read, `existing`, and list. A secret the schema doesn't mark write-only, such as a Cognito user pool client's `ClientSecret` that AWS generates, is only kept out of the echo of the request.

UCP has no POST `/:put` or `/:delete` routes. AWS resources deployed before records have no record, so UCP treats them like resources created outside Radius: a DELETE of their old ID returns `204` and leaves them in AWS.

### Write-only values

AWS never returns write-only values, such as passwords, so UCP cannot compare them with AWS. Instead, the record keeps an HMAC-SHA-256 fingerprint of each write-only value of the last successful deploy (`pkg/ucp/frontend/controller/awsproxy/fingerprints.go`). The key is in the `radius-aws-fingerprint-key` Kubernetes Secret, never in the data store, as described in [credentials](../architecture/credentials.md). The record stores the key ID once, apart from the digests. Each fingerprint is bound to the resource ID and the property path, and the value is encoded as JSON, so key order and number forms do not matter. A write-only property inside array items, such as `Users/*/Password`, is covered by a fingerprint of the whole array. A value in an array item AWS does not have, such as one deleted outside Radius, is always sent with the item.

| Write-only value in the template                                                                                 | What UCP does                                                                                                                                                          |
|------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Matches its fingerprint                                                                                          | Leaves it out of the update. A create-and-delete-only type succeeds without an update                                                                                  |
| Differs from its fingerprint, or the last deploy did not set it                                                  | Sends it. A create-and-delete-only type fails with `400` naming the property                                                                                           |
| No usable fingerprint: a different key made it, or UCP took the resource over and the last deploy did not set it | Sends a value that can be updated once. Does not send a value that cannot change after create, and does not fail. Either way, saves a fingerprint with the current key |

If the key is missing, unreadable, or invalid, a PUT whose template sets a write-only value fails with `500` naming the Secret, before UCP saves anything or calls Cloud Control. Other PUTs, reads, and deletes still work.

<br/><br/>

### AWS types that Cloud Control cannot fully manage

UCP returns a `400` for these AWS types instead of the error Cloud Control returns. Error messages name the type and properties, never property values.

| Type                                                                                      | How UCP knows                                                                                       | What UCP returns                                                                                                                                                                                                                       |
|-------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Legacy type with a supported replacement, for example `AWS::IAM::Policy`                  | List in `pkg/aws/awstypes`                                                                          | `400` naming the replacement, on every operation. Nothing is called in AWS                                                                                                                                                             |
| Deprecated service or CloudFormation template feature, for example `AWS::OpsWorks::Stack` | List in `pkg/aws/awstypes`                                                                          | `400` saying Radius does not support the type, on every operation. Nothing is called in AWS                                                                                                                                            |
| Read-only type created by AWS, for example `AWS::EC2::Image`                              | `DescribeType` reports `NON_PROVISIONABLE`, and the schema has a read handler but no create handler | `400` on a PUT, before UCP reads the resource, saying the type can only be used with `existing`                                                                                                                                        |
| Other type Cloud Control cannot create, with no native handler                            | `DescribeType` reports `NON_PROVISIONABLE`                                                          | `400` on a PUT, before UCP reads the resource. A type with a native handler is served by it instead, as described in [Native handlers](#native-handlers)                                                                               |
| Create-and-delete-only type                                                               | `DescribeType` reports `IMMUTABLE`                                                                  | Create works. A change to an existing resource fails with `400` naming the changed properties. A write-only value counts as changed only when it differs from its fingerprint, as described in [Write-only values](#write-only-values) |

UCP reads the provisioning type from CloudFormation at run time, so a type starts working without a Radius release when AWS adds Cloud Control support for it.

The read-only list in `pkg/aws/awstypes` is for callers that have no AWS access, such as the recipe engine. It is generated from a snapshot of the CloudFormation registry. To refresh it with AWS credentials, run `go run ./gen -refresh` from `pkg/aws/awstypes`.

<br/><br/>

### Native handlers

A native handler serves an AWS type by calling the service's own API through the AWS SDK, for types Cloud Control can't create in a region and types with no CloudFormation type. The interface and registry are in `pkg/ucp/aws/native`, and `pkg/ucp/aws/native/handlers` lists the handlers UCP registers. Each handler's schema, in CloudFormation format, is in `pkg/aws/schemas`: `native/` for types that have a CloudFormation type, and `handwritten/` for types that don't.

UCP picks the handler for each request:

1. A hand-written type always uses its native handler. UCP never asks CloudFormation about it.
2. A type in `native/` uses its native handler only when `DescribeType` reports it `NON_PROVISIONABLE` in the request's region. Otherwise Cloud Control serves it. `DescribeType` results are cached for 15 minutes, so a region that gains Cloud Control support switches without a release. If `DescribeType` fails, the request fails with that error.
3. Every other type uses Cloud Control.

Both handlers use the same record. The native schema replaces CloudFormation's with either handler, so both store the same AWS identifier, use the same property names, and treat the same properties as write-only. Every secret is write-only in the native schema, including ones CloudFormation's doesn't mark, and UCP removes write-only values from what either handler returns, because some service APIs return secrets. The record saves which handler started an operation (`handler`), and that handler finishes it, even if the region has since switched.

When AWS lets the caller choose the name and the template leaves it out, a handler declares the name property (`NameProperty`), and UCP generates `radius-` followed by 12 lowercase hex characters of a SHA-256 hash of the record's client token, such as `radius-3f9a1c0b7d2e`, cut to the property's `maxLength` in the schema. A name that doesn't fit the property's `minLength` or `pattern` fails the PUT with `400` before UCP calls AWS, asking for the name in the template. The client token identifies the create, so every retry of one create sends the same name. UCP saves the name on the record before it calls AWS, and leaves it out of the last-applied properties, so a later deploy that still leaves it out doesn't clear it. The property stays optional, because Cloud Control names the resource itself where it serves the type.

Each handler declares how UCP finds a create it lost after AWS accepted it (`Recovery`). A PUT, and a DELETE of a record whose create may have reached AWS, take the same steps:

| `Recovery`            | When                                                                                              | How UCP finds the resource                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
|-----------------------|---------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `RecoverByIdentifier` | The identifier is known before the create, because the template sets it or UCP generates it       | UCP saves the identifier on the record before the create and reads AWS by it. A create whose template doesn't give the identifier fails with `400` before UCP calls AWS                                                                                                                                                                                                                                                                                                      |
| `RecoverByResend`     | AWS assigns the identifier, and the API takes an idempotency token, or the call is safe to repeat | UCP sends the create again with the record's client token, which the handler passes as the token, so AWS returns the first resource. Two requests can resend at once, so `Create` with the same client token must return the same resource every time and never create a second one. That resource may have the first attempt's values, so UCP reads it and updates it to the template, sending every write-only value. Before a delete, the resend has no write-only values |
| `RecoverBySearch`     | AWS assigns the identifier, and the API takes no token                                            | The handler tags the create with the client token, or relies on values AWS keeps unique, and its `Find` looks for the resource with the first attempt's values, without write-only values, because the template may have changed. UCP sends the create again only when AWS has none, and returns `409` when another alias manages the resource `Find` returns                                                                                                                |

Once UCP has the identifier, it reads AWS: if AWS has no resource, a PUT creates it and a DELETE removes only the record; if AWS is still creating it, UCP waits for it and returns `409`; otherwise the PUT continues as an update and the DELETE deletes it.

A native operation follows the same steps as a Cloud Control one: owner check, compare, save pending values, start call, operation status. The handler's start call returns at once, UCP makes its own operation ID, and the worker calls the handler's `Progress` until the operation finishes or the handler's timeout passes. `Progress` only reads AWS, so a restarted worker waits again safely. UCP queues native operations under their own operation type, `AWSRESOURCENATIVE`, so a worker that finds the record already removed never asks Cloud Control about them.

A handler whose identifier isn't its primary identifier values joined with `|` declares its own encoding. UCP uses it for create, take-over, `existing`, recovery, and delete, with either handler. The identifier stays exactly what AWS uses, such as an S3 ARN with `/`. Only the resource ID that a list or `existing` returns for a type with a native handler escapes it into one segment, with URL path escaping, and POST `/:get` unescapes a `name` for such a type, so a recipe can read the resource again by that segment.

| Scenario                                                                  | What UCP does                                                                                                     |
|---------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|
| AWS rejects the start call                                                | `400` with AWS's message. Pending values are dropped. A rejected first create removes the record                  |
| The start call fails with a server or transport error                     | An update's pending values count as applied, as for Cloud Control. A create keeps its client token and identifier |
| A first create was interrupted                                            | The next PUT or DELETE finds the resource as the handler's `Recovery` says, as described above                    |
| Finding a lost create fails, such as a failed `Find` or a rejected resend | The PUT or DELETE fails with AWS's error and keeps the record, so the next request tries again                    |
| The operation fails in AWS                                                | The operation fails with the handler's message. A failed create keeps the record, because the resource may exist  |
| Waiting passes the handler's timeout                                      | The operation fails, naming the state UCP waited for. The next deploy reads the resource and continues            |
| The type can't be listed, or only under a parent                          | `400` on a list                                                                                                   |

Every native handler runs the shared suite in `pkg/ucp/aws/native/nativetest`, which deploys through UCP's controllers and worker with an in-memory AWS. `pkg/ucp/aws/native/nativefake` has a fake handler for tests.
