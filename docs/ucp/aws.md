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

- **PUT** reads the record. With no AWS identifier yet, UCP takes over a resource AWS already has, found by the top-level `name` or by the identifier in the properties, or creates one. Before it creates anything, UCP saves the record with a client token, and sends that token with every attempt to create the resource, so a retry never creates a second one. With a record, UCP compares the template with AWS and with the last-applied properties: a property the last deploy set and the template no longer sets is cleared, and a removed create-only property fails with `400`.
- **GET** reads the record, then AWS by the identifier in the record. With no record, UCP reads AWS by the URL's last segment, and returns `404` when AWS does not have it (transitional, see below).
- **DELETE** reads the record and deletes the resource by its identifier. With no record, UCP deletes the resource in AWS by the URL's last segment, and returns `204` when AWS does not have it (transitional, see below).
- **POST `/:get`** reads a resource Radius may not have created, for an `existing` block that sets `name` or `properties`. It reads AWS by the top-level `name` in the body when it is set, and otherwise by the primary or additional identifiers in the properties. It never saves a record.

PUT, and a DELETE that finds a record, require the `X-Radius-Resource-Owner` header, and fail with `400` without it. A PUT or DELETE from a different owner than the record's fails with `409`, and so does a PUT that would manage an AWS resource another alias's record already has. Owners are compared ignoring case.

UCP owns every create, update, and delete. It saves the new last-applied properties and fingerprints on the record as pending values before it calls AWS, saves the Cloud Control request token, and queues an operation. The backend controller in `pkg/ucp/backend/controller/awsresources` polls Cloud Control, then moves the pending values to last-applied on success, or drops them on failure so the previous ones stay. A failed create removes the record. The operation status and result routes read UCP's data store, and fall back to Cloud Control for an operation UCP does not have (transitional, see below). A second PUT or DELETE while an operation is running gets `409`. One that finds an operation nothing has polled for 10 minutes, or one AWS accepted but UCP never queued, resumes it.

Write-only values are never stored: not in the last-applied or pending properties, and not in operations.

Until the Deployment Engine deploys AWS resources by alias, UCP keeps four transitional behaviors from before records, for Deployment Engine releases that still deploy with POST `/:put` and `/:delete`. They are removed once the Deployment Engine deploys by alias:

- POST `/:put` and `/:delete` work as before: no owner header and no record, so no fingerprints. A write-only value on an existing create-and-delete-only resource fails with `400`, because UCP cannot confirm it is unchanged.
- The operation status and result routes ask Cloud Control by request token when UCP has no operation with that ID.
- A DELETE of a URL with no record deletes the resource in AWS by the URL's last segment, with or without the owner header. A header that is sent must not be blank. Recipe deletes rely on this, because the IDs of resources created with POST `/:put` end in the AWS identifier.
- A GET of a URL with no record reads the resource from AWS by the URL's last segment. The Bicep recipe driver relies on this to read the ARN of each AWS output resource.

POST `/:put`, POST `/:delete`, and a DELETE with no record return `409` for a resource an alias's record manages, so they never skip that record's owner check. Resources with no record behave as before records.

### Write-only values

AWS never returns write-only values, such as passwords, so UCP cannot compare them with AWS. Instead, the record keeps an HMAC-SHA-256 fingerprint of each write-only value of the last successful deploy (`pkg/ucp/frontend/controller/awsproxy/fingerprints.go`). The key is in the `radius-aws-fingerprint-key` Kubernetes Secret, never in the data store, as described in [credentials](../architecture/credentials.md). The record stores the key ID once, apart from the digests. Each fingerprint is bound to the resource ID and the property path, and the value is encoded as JSON, so key order and number forms do not matter. A write-only property inside array items, such as `Users/*/Password`, is covered by a fingerprint of the whole array.

| Write-only value in the template                                                   | What UCP does                                                                                                                                                          |
|------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Matches its fingerprint                                                            | Leaves it out of the update. A create-and-delete-only type succeeds without an update                                                                                  |
| Differs from its fingerprint, or the last deploy did not set it                    | Sends it. A create-and-delete-only type fails with `400` naming the property                                                                                           |
| No usable fingerprint: UCP just took the resource over, or a different key made it | Sends a value that can be updated once. Does not send a value that cannot change after create, and does not fail. Either way, saves a fingerprint with the current key |

If the key is missing, unreadable, or invalid, a PUT whose template sets a write-only value fails with `500` naming the Secret, before UCP saves anything or calls Cloud Control. Other PUTs, reads, and deletes still work.

<br/><br/>

### AWS types that Cloud Control cannot fully manage

UCP returns a `400` for these AWS types instead of the error Cloud Control returns. Error messages name the type and properties, never property values.

| Type                                                                                      | How UCP knows                                                                                       | What UCP returns                                                                                                                                                                                                                       |
|-------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Legacy type with a supported replacement, for example `AWS::IAM::Policy`                  | List in `pkg/aws/awstypes`                                                                          | `400` naming the replacement, on every operation. Nothing is called in AWS                                                                                                                                                             |
| Deprecated service or CloudFormation template feature, for example `AWS::OpsWorks::Stack` | List in `pkg/aws/awstypes`                                                                          | `400` saying Radius does not support the type, on every operation. Nothing is called in AWS                                                                                                                                            |
| Read-only type created by AWS, for example `AWS::EC2::Image`                              | `DescribeType` reports `NON_PROVISIONABLE`, and the schema has a read handler but no create handler | `400` on a PUT, before UCP reads the resource, saying the type can only be used with `existing`                                                                                                                                        |
| Other type Cloud Control cannot create                                                    | `DescribeType` reports `NON_PROVISIONABLE`                                                          | `400` on a PUT, before UCP reads the resource                                                                                                                                                                                          |
| Create-and-delete-only type                                                               | `DescribeType` reports `IMMUTABLE`                                                                  | Create works. A change to an existing resource fails with `400` naming the changed properties. A write-only value counts as changed only when it differs from its fingerprint, as described in [Write-only values](#write-only-values) |

UCP reads the provisioning type from CloudFormation at run time, so a type starts working without a Radius release when AWS adds Cloud Control support for it.

The read-only list in `pkg/aws/awstypes` is for callers that have no AWS access, such as the recipe engine. It is generated from a snapshot of the CloudFormation registry. To refresh it with AWS credentials, run `go run ./gen -refresh` from `pkg/aws/awstypes`.
