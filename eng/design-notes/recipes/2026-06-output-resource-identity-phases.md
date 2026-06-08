# Output Resource Identity Phases

* **Author**: Will Smith (@willdavsmith)
* **Status**: Draft

## Overview

Radius output resources currently use a single `id` field for two different ideas:

- the Radius resource ID used to describe the producer and lifecycle path; and
- the physical cloud resource identity used to decide whether two Radius resources reference the same object.

Those identities are not always the same. AWS makes this visible because Bicep deployments use CloudControl-shaped resource IDs while Terraform recipes discover Terraform provider resources from state.

For an S3 bucket named `shared-bucket`, the same physical AWS bucket can appear as:

```text
/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket
/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket
```

The two Radius IDs should remain different because they represent different lifecycle systems. Radius still needs a consistent physical identity for delete warnings, shared-resource detection, app graph grouping, and future clients.

This document tracks the planned phased solution:

1. Phase 1: Fix AWS Terraform output resource IDs so Terraform resources use `Terraform.AWS` IDs. This resolves [#11838](https://github.com/radius-project/radius/issues/11838).
2. Phase 2: Add transitional physical identity metadata and use it for shared-resource delete warnings. This resolves [#12020](https://github.com/radius-project/radius/issues/12020).
3. Phase 3: Promote provider resource identity to first-class output resource fields so future clients do not need custom `additionalProperties` logic.

## Terms

| Term | Definition |
| --- | --- |
| Output resource | A resource reference stored in `status.outputResources` on a Radius resource. |
| Producer ID | The output resource `id` that identifies the Radius resource provider namespace and lifecycle path. |
| Physical identity | A stable identifier for the underlying provider object, independent of which Radius producer referenced it. |
| Provider resource ID | The provider-native identifier for the underlying resource, for example an AWS ARN. |
| Provider resource ID kind | A discriminator for the provider resource ID format, for example `awsArn`. |
| CloudControl-shaped type | A Radius AWS type derived from AWS CloudFormation or CloudControl, for example `AWS.S3/Bucket`. |
| Terraform-shaped type | A Radius type derived from Terraform provider state, for example `Terraform.AWS/aws_s3_bucket`. |
| ARN | Amazon resource name. ARNs uniquely identify many AWS resources, but ARN formats are service-specific and do not always include account, region, or CloudControl type information. |

## Design Principles

- The output resource `id` should identify the producer and lifecycle path.
- Radius should not force Terraform resources into CloudControl-shaped IDs when the mapping is not guaranteed.
- Physical-resource comparison should use provider-native identity when available.
- Dashboard and future clients should not each implement custom comparison rules.
- The solution should preserve existing output resource ID behavior for Azure, Kubernetes, and AWS resources without physical identity metadata.

## Phase 1: Terraform AWS Output IDs

Phase 1 changes AWS Terraform output resource IDs to use a Terraform-specific resource provider namespace:

```text
/planes/aws/<partition>/accounts/<account-id>/regions/<region>/providers/Terraform.AWS/<terraform-resource-type>/<resource-name>
```

Examples:

```text
/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket
/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/Terraform.AWS/aws_sqs_queue/orders
/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/Terraform.AWS/aws_cloudwatch_log_group/%2Fapp%2Forders
```

### Why This Fixes #11838

The previous AWS Terraform behavior parsed an ARN and tried to emit a CloudControl-shaped ID. That fails for S3:

```text
arn:aws:s3:::shared-bucket
```

This ARN has no account ID, no region, and no CloudControl resource type token such as `Bucket`. Phase 1 avoids inventing missing CloudControl information. The account comes from the configured AWS provider scope, the empty ARN region becomes `global`, and the Terraform resource type comes from Terraform state.

### Resource Name Selection

Terraform provider `id` values are provider-defined, so Radius should prefer the ARN resource component when it has a usable final physical-name segment. If that is unavailable, Radius can use simple provider state attributes such as `id`.

Radius should not use the Terraform block name as the physical name. In `resource "aws_s3_bucket" "storage"`, `storage` is a local Terraform label, not the AWS bucket name.

### Adversarial Examples

| Terraform resource type | Example ARN | CloudControl type | Why Terraform IDs are safer |
| --- | --- | --- | --- |
| `aws_s3_bucket` | `arn:aws:s3:::shared-bucket` | `AWS::S3::Bucket` | ARN lacks account, region, and a `Bucket` type segment. |
| `aws_sqs_queue` | `arn:aws:sqs:us-west-2:123456789012:orders` | `AWS::SQS::Queue` | ARN has a bare resource name; Terraform `id` is commonly a queue URL. |
| `aws_sns_topic` | `arn:aws:sns:us-east-1:123456789012:orders` | `AWS::SNS::Topic` | ARN has no type token from which to infer `Topic`. |
| `aws_cloudwatch_log_group` | `arn:aws:logs:us-west-1:123456789012:log-group:/app/orders` | `AWS::Logs::LogGroup` | Terraform says `cloudwatch`, ARN says `logs`, ARN path says `log-group`, and CloudControl says `Logs::LogGroup`. |
| `aws_lb` | `arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/web/50dc...` | `AWS::ElasticLoadBalancingV2::LoadBalancer` | Terraform abbreviates the type, ARN service is long, and CloudControl uses another service spelling. |
| `aws_s3_bucket_versioning` | Often bucket-name based rather than a standalone ARN. | Usually modeled as configuration on `AWS::S3::Bucket`. | Not every Terraform resource maps to a standalone CloudControl resource. |

### Phase 1 Tests

- Unit test Terraform AWS S3 state with `arn:aws:s3:::<bucket>` and verify a `Terraform.AWS/aws_s3_bucket` output resource ID.
- Unit test AWS resources whose ARN includes account and region and verify they no longer emit CloudControl-shaped IDs from Terraform state.
- Preserve existing Azure and Kubernetes output resource behavior.

## Phase 2: Shared Physical Identity Metadata

Phase 2 keeps producer IDs distinct but adds an optional physical comparison key:

```json
{
  "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket",
  "radiusManaged": true,
  "additionalProperties": {
    "arn": "arn:aws:s3:::shared-bucket",
    "providerResourceId": "arn:aws:s3:::shared-bucket",
    "providerResourceIdKind": "awsArn"
  }
}
```

For AWS resources, `providerResourceId` is the ARN when Radius can read one, and `providerResourceIdKind` is `awsArn`. Radius should not synthesize account or region into an ARN. S3 bucket ARNs omit those fields because the bucket name is already globally unique within the AWS partition.

### Phase 2 Behavior

- AWS Terraform output resources store `additionalProperties.arn`, `additionalProperties.providerResourceId`, and `additionalProperties.providerResourceIdKind` when Terraform state has an `arn` attribute.
- AWS Bicep output resources keep CloudControl-shaped IDs and store the same ARN metadata when CloudControl resource properties expose `ARN`, `Arn`, or `arn`.
- Delete warning logic compares output resources by `providerResourceId` when both sides have it, then falls back to producer ID equality. If both sides provide `providerResourceIdKind`, the kind must also match.
- Application graph output resources preserve `additionalProperties` so clients can inspect the physical identity.
- Warnings remain advisory. Phase 2 does not add server-side delete enforcement.

### Edge Case: Shared Bucket Across Environments

Phase 2 covers the reviewed shared-resource scenario:

1. Team 1 provisions or references an existing S3 bucket from a Bicep recipe in Environment A.
2. Team 2 references the same S3 bucket from a Terraform recipe in Environment B.
3. Team 2 deletes their Radius resource.
4. Radius compares the Bicep `AWS.S3/Bucket` output resource and Terraform `Terraform.AWS/aws_s3_bucket` output resource by ARN.
5. The CLI warns that another app or environment still references the same physical bucket.

The producer IDs differ, but both output resources carry:

```text
arn:aws:s3:::shared-bucket
```

### Phase 2 Limitations

- This is not a universal client model. Clients that need grouping must understand `additionalProperties.providerResourceId`.
- Resources without ARNs continue to compare by producer ID only.
- `providerResourceId` is status metadata and must not contain secrets.
- ARN extraction from CloudControl properties starts with common property names. Schema-driven extraction may be needed for broader AWS coverage.

### Phase 2 Tests

- Unit test `OutputResourceMatches` for physical ID match, physical ID mismatch, and ID fallback.
- Unit test Terraform output resources with ARN metadata.
- Unit test AWS Bicep output resources with `ARN`, `Arn`, `arn`, and missing ARN properties.
- Unit test resource and application delete warnings for Bicep-shaped and Terraform-shaped output resources that share the same ARN.
- Verify the app graph preserves output resource `additionalProperties`.

## Phase 3: First-Class Provider Resource ID

Phase 3 moves the provider resource identity out of ad hoc metadata and into the output resource contract:

```json
{
  "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket",
  "radiusManaged": true,
  "providerResourceId": "arn:aws:s3:::shared-bucket",
  "providerResourceIdKind": "awsArn"
}
```

The goal is for Radius to expose a first-class provider resource identity so dashboard, CLI, APIs, and future clients can use one contract instead of independently reading `additionalProperties`.

### Phase 3 Behavior

- Persist `providerResourceId` and `providerResourceIdKind` on output resources when a producer can determine provider-native identity.
- Use `providerResourceId` for server-side and client-facing resource association.
- Keep producer IDs unchanged so lifecycle ownership remains explicit.
- Use Phase 2 `additionalProperties.providerResourceId` and `additionalProperties.providerResourceIdKind` as the migration source or compatibility fallback.
- Allow APIs such as app graph to group or annotate output resources by provider resource identity.

### Why Phase 3 Is Needed

Phase 2 is sufficient for delete warnings, but it does not solve every client concern. For example, a dashboard app graph should not have to know that `additionalProperties.providerResourceId` is a Radius comparison key. A first-class field makes provider resource identity a platform contract.

### Phase 3 Open Questions

- Should `providerResourceId` always be paired with `providerResourceIdKind`, or can a missing kind be treated as an opaque provider ID?
- Should Radius normalize ARN case or preserve provider output exactly?
- Should Radius expose graph grouping directly, or only expose provider resource IDs and let clients group?
- Should server-side delete validation use provider resource identity, and if so, which operations should enforce it?

## Options Considered

### Option 1: Convert Terraform AWS Resources To CloudControl IDs

This would make Terraform and Bicep output resources share the same ID when a Terraform type maps cleanly to CloudControl.

Pros:

- Existing ID comparison would work for mapped resources.
- App graph clients would not need a second identity field for those cases.

Cons:

- Requires maintaining or generating a Terraform-to-CloudControl type map.
- Does not cover Terraform resources that represent configuration, relationships, or provider-specific resources.
- ARN and Terraform type strings do not guarantee enough information to infer CloudControl type.
- Incorrect mappings would imply CloudControl lifecycle semantics for Terraform-managed resources.

### Option 2: Terraform Producer IDs Plus Physical Identity

This is the selected phased direction.

Pros:

- Fixes malformed AWS Terraform output resource IDs without an unreliable type map.
- Preserves lifecycle ownership in the producer ID.
- Enables cross-producer matching when a real physical identity exists.
- Can evolve from transitional metadata in Phase 2 to first-class `providerResourceId` in Phase 3.

Cons:

- Producer IDs differ for the same physical object.
- Phase 2 requires clients to understand `providerResourceId` in `additionalProperties` until Phase 3 lands.
- Resources without reliable physical identity metadata still fall back to ID matching.

### Option 3: Use ARN As The Output Resource ID

This would avoid producer-specific Radius IDs for AWS resources with ARNs.

Pros:

- ARN is the AWS-native identity for many resources.
- Bicep and Terraform would naturally converge for many AWS resources.

Cons:

- `status.outputResources[].id` is a Radius resource ID today, not an arbitrary provider ID.
- ARNs do not identify the Radius producer or lifecycle path.
- Some resources do not expose ARNs, and some Terraform resources are not standalone AWS resources.
- This would be a larger API contract change than Phase 1 or Phase 2.

## Migration And Compatibility

- Phase 1 changes AWS Terraform output resource IDs. Existing Terraform-created AWS output resources will be repopulated with `Terraform.AWS` IDs on the next recipe deployment.
- Phase 2 adds optional metadata. Existing resources without metadata continue to compare by ID.
- Phase 3 should treat Phase 2 `additionalProperties.providerResourceId` values as compatibility input during migration.
- Existing Azure and Kubernetes output resource behavior is unchanged by Phase 1 and Phase 2.

## Rollout Plan

1. Merge Phase 1 in the existing #11838 PR.
2. Merge Phase 2 in a separate draft PR after review, stacked conceptually on Phase 1.
3. Open a follow-up issue or design review for Phase 3 before changing the public output resource contract.
