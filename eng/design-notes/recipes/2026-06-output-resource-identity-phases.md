# Output Resource Identity Phases

* **Author**: Will Smith (@willdavsmith)

## Overview

AWS Terraform recipes currently populate output resources by reading Terraform state, extracting each resource ARN, and converting that ARN into a CloudControl-shaped Radius AWS resource ID. That conversion assumes the ARN contains enough information to infer an AWS-plane resource type, account, region, and resource name.

That assumption is not valid for all AWS resources. The immediate bug is S3: an S3 bucket ARN has the shape:

```text
arn:aws:s3:::shared-bucket
```

This ARN does not contain an account ID, a region, or a CloudControl resource type token such as `Bucket`. Radius cannot generically parse this ARN into a valid `AWS.S3/Bucket` resource ID, so Terraform recipes that produce S3 bucket output resources can fail.

The Phase 1 fix is to stop forcing Terraform-discovered AWS resources into CloudControl-shaped IDs. Terraform output resources should instead use Terraform-shaped Radius IDs under `Terraform.AWS`, built from Terraform state and the configured AWS provider scope:

```text
/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket
```

That fixes the malformed-ID failure, but it also makes the broader identity problem explicit. The same physical AWS bucket can be referenced by a Bicep deployment using a CloudControl-shaped ID and by a Terraform recipe using a Terraform-shaped ID:

```text
/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket
/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket
```

Those two Radius IDs should remain different because they represent different lifecycle systems. Radius still needs a consistent provider-native identity for delete warnings, shared-resource detection, app graph grouping, and future clients.

This document tracks a phased solution:

1. Phase 1: Fix AWS Terraform output resource IDs so Terraform resources use `Terraform.AWS` IDs.
2. Phase 2: Add first-class provider resource identity metadata on output resources and use it for shared-resource delete warnings.
3. Phase 3: Update remaining APIs and clients to consistently use provider resource identity for association and display.

## Terms and definitions

| Term | Definition |
|---|---|
| Output resource | A resource reference stored in `status.outputResources` on a Radius resource. |
| Producer ID | The output resource `id` that identifies the Radius resource provider namespace and lifecycle path. |
| Physical identity | A stable identifier for the underlying provider object, independent of which Radius producer referenced it. |
| Provider resource ID | The provider-native identifier for the underlying resource, for example an AWS ARN. |
| Provider resource ID kind | A discriminator for the provider resource ID format, for example `awsArn`. |
| CloudControl-shaped type | A Radius AWS type derived from AWS CloudFormation or CloudControl, for example `AWS.S3/Bucket`. |
| Terraform-shaped type | A Radius type derived from Terraform provider state, for example `Terraform.AWS/aws_s3_bucket`. |
| ARN | Amazon resource name. ARNs uniquely identify many AWS resources, but ARN formats are service-specific and do not always include account, region, or CloudControl type information. |

## Objectives

> **Issue Reference:** [#11838](https://github.com/radius-project/radius/issues/11838), [#12020](https://github.com/radius-project/radius/issues/12020)

### Goals

- Fix AWS Terraform recipe output resources for resources with ARNs that omit account, region, or resource type information.
- Keep `status.outputResources[].id` as a Radius/UCP resource ID that describes producer and lifecycle ownership.
- Avoid emitting malformed or misleading AWS CloudControl-shaped IDs for Terraform-managed resources.
- Allow Radius to detect when AWS Bicep and Terraform output resources reference the same provider resource.
- Add first-class provider resource identity so future clients do not each implement provider-specific `additionalProperties` matching.
- Preserve existing Azure and Kubernetes output resource behavior.

### Non goals

- Manage Terraform-created AWS resources through AWS CloudControl.
- Build or maintain a Terraform AWS provider type to CloudControl type mapper.
- Replace `status.outputResources[].id` with arbitrary provider-native IDs such as ARNs.
- Add server-side delete enforcement in Phase 2. Phase 2 warnings remain advisory.
- Guarantee provider resource identity for every provider resource type. Resources without provider-native identity continue to compare by producer ID.
- Change Terraform state storage or Terraform apply/destroy behavior.

### User scenarios

#### Terraform S3 recipe succeeds

A user deploys a Terraform recipe that creates an S3 bucket. The S3 ARN has the shape:

```text
arn:aws:s3:::shared-bucket
```

Radius should not fail while trying to infer a CloudControl type or account from that ARN. The output resource should use a Terraform-shaped Radius ID:

```text
/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket
```

#### Shared S3 bucket across Bicep and Terraform recipes

Team 1 provisions or references an existing S3 bucket from a Bicep recipe in Environment A. Team 2 references the same S3 bucket from a Terraform recipe in Environment B. When Team 2 deletes its Radius resource, Radius should detect that another app or environment still references the same S3 bucket and show a warning.

The producer IDs differ:

```text
/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket
/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket
```

Both output resources carry the same provider resource ID:

```text
arn:aws:s3:::shared-bucket
```

## User Experience

Phase 1 changes the IDs users see in `status.outputResources` for AWS Terraform recipes. Terraform-managed AWS resources use `Terraform.AWS` provider IDs rather than CloudControl-shaped `AWS.*` IDs.

**Sample Output:**

```json
{
  "status": {
    "outputResources": [
      {
        "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket",
        "radiusManaged": true
      }
    ]
  }
}
```

Phase 2 adds optional provider resource metadata as first-class output resource fields:

```json
{
  "status": {
    "outputResources": [
      {
        "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket",
        "radiusManaged": true,
        "providerResourceId": "arn:aws:s3:::shared-bucket",
        "providerResourceIdKind": "awsArn",
        "additionalProperties": {
          "arn": "arn:aws:s3:::shared-bucket"
        }
      }
    ]
  }
}
```

**Sample Recipe Contract:**

No recipe input contract changes are required. Existing `result.resources` and recipe-derived output resources continue to be supported.

## Design

### High Level Design

The selected design separates producer identity from provider identity.

- `status.outputResources[].id` remains a Radius/UCP resource ID. It identifies the producer namespace, type, scope, and lifecycle path.
- `providerResourceId` identifies the provider-native resource when Radius can determine one.
- `providerResourceIdKind` identifies the provider resource ID format, for example `awsArn`.

Phase 1 fixes the Terraform AWS producer ID. Phase 2 adds first-class provider identity fields to output resources and uses them for shared-resource delete warnings. Phase 3 updates remaining APIs and clients to use those fields consistently for association and display.

### Architecture Diagram

```text
Terraform state                  Bicep / CloudControl response
  type: aws_s3_bucket              id: AWS.S3/Bucket/shared-bucket
  arn: arn:aws:s3:::bucket         properties.Arn: arn:aws:s3:::bucket
        |                                      |
        v                                      v
Terraform driver                     Bicep driver / deployment processor
  id: Terraform.AWS/...              id: AWS.S3/Bucket/...
  providerResourceId: ARN            providerResourceId: ARN
        |                                      |
        +------------------+-------------------+
                           v
              status.outputResources
                           |
                           v
           shared resource comparison / app graph
```

### Detailed Design

#### Phase 1: Terraform AWS output IDs

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

The previous AWS Terraform behavior parsed an ARN and tried to emit a CloudControl-shaped ID. That fails for S3 because `arn:aws:s3:::shared-bucket` has no account ID, no region, and no CloudControl resource type token such as `Bucket`. Phase 1 avoids inventing missing CloudControl information. The account comes from the configured AWS provider scope, the empty ARN region becomes `global`, and the Terraform resource type comes from Terraform state.

Terraform provider `id` values are provider-defined, so Radius should prefer the ARN resource component when it has a usable final physical-name segment. If that is unavailable, Radius can use simple provider state attributes such as `id`. Radius should not use the Terraform block name as the physical name. In `resource "aws_s3_bucket" "storage"`, `storage` is a local Terraform label, not the AWS bucket name.

Adversarial examples:

| Terraform resource type | Example ARN | CloudControl type | Why Terraform IDs are safer |
|---|---|---|---|
| `aws_s3_bucket` | `arn:aws:s3:::shared-bucket` | `AWS::S3::Bucket` | ARN lacks account, region, and a `Bucket` type segment. |
| `aws_sqs_queue` | `arn:aws:sqs:us-west-2:123456789012:orders` | `AWS::SQS::Queue` | ARN has a bare resource name; Terraform `id` is commonly a queue URL. |
| `aws_sns_topic` | `arn:aws:sns:us-east-1:123456789012:orders` | `AWS::SNS::Topic` | ARN has no type token from which to infer `Topic`. |
| `aws_cloudwatch_log_group` | `arn:aws:logs:us-west-1:123456789012:log-group:/app/orders` | `AWS::Logs::LogGroup` | Terraform says `cloudwatch`, ARN says `logs`, ARN path says `log-group`, and CloudControl says `Logs::LogGroup`. |
| `aws_lb` | `arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/web/50dc...` | `AWS::ElasticLoadBalancingV2::LoadBalancer` | Terraform abbreviates the type, ARN service is long, and CloudControl uses another service spelling. |
| `aws_s3_bucket_versioning` | Often bucket-name based rather than a standalone ARN. | Usually modeled as configuration on `AWS::S3::Bucket`. | Not every Terraform resource maps to a standalone CloudControl resource. |

#### Phase 2: First-class provider resource metadata

Phase 2 keeps producer IDs distinct but adds optional first-class provider resource identity fields:

```json
{
  "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket",
  "radiusManaged": true,
  "providerResourceId": "arn:aws:s3:::shared-bucket",
  "providerResourceIdKind": "awsArn",
  "additionalProperties": {
    "arn": "arn:aws:s3:::shared-bucket"
  }
}
```

For AWS resources, `providerResourceId` is the ARN when Radius can read one, and `providerResourceIdKind` is `awsArn`. Radius should not synthesize account or region into an ARN. S3 bucket ARNs omit those fields because the bucket name is already globally unique within the AWS partition.

Phase 2 behavior:

- AWS Terraform output resources store `providerResourceId` and `providerResourceIdKind` when Terraform state has an `arn` attribute. They may also preserve the raw ARN as `additionalProperties.arn`.
- AWS Bicep output resources keep CloudControl-shaped IDs and store the same ARN metadata when CloudControl resource properties expose `ARN`, `Arn`, or `arn`.
- Delete warning logic compares output resources by `providerResourceId` when both sides have it, then falls back to producer ID equality. If both sides provide `providerResourceIdKind`, the kind must also match.
- Application graph output resources preserve `providerResourceId` and `providerResourceIdKind` so clients can inspect the provider identity.
- Warnings remain advisory. Phase 2 does not add server-side delete enforcement.

#### Phase 3: Provider resource identity adoption

Phase 3 extends usage of the Phase 2 provider resource identity fields across Radius APIs and clients:

```json
{
  "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket",
  "radiusManaged": true,
  "providerResourceId": "arn:aws:s3:::shared-bucket",
  "providerResourceIdKind": "awsArn"
}
```

Phase 3 behavior:

- Use `providerResourceId` for server-side and client-facing resource association beyond delete warnings.
- Keep producer IDs unchanged so lifecycle ownership remains explicit.
- Allow APIs such as app graph to group or annotate output resources by provider resource identity.
- Ensure dashboard, CLI, and generated clients use `providerResourceId` and `providerResourceIdKind` directly instead of provider-specific `additionalProperties` conventions.

Phase 2 is sufficient for delete warnings, but it does not solve every client concern. For example, a dashboard app graph should not have to implement its own grouping behavior. Phase 3 makes provider resource identity a platform-wide association contract.

#### Advantages

- Fixes malformed AWS Terraform output resource IDs without an unreliable type map.
- Preserves lifecycle ownership in the producer ID.
- Enables cross-producer matching when a real provider resource identity exists.
- Avoids a heavy Phase 2 to Phase 3 metadata migration by adding the provider resource identity fields in Phase 2.
- Avoids making every future client infer Radius identity rules from raw output resource IDs.

#### Disadvantages

- Producer IDs differ for the same physical object.
- Phase 2 only applies the fields to shared-resource delete warnings; broader API and client behavior waits for Phase 3.
- Resources without reliable provider resource metadata still fall back to ID matching.
- ARN extraction from CloudControl properties starts with common property names. Schema-driven extraction may be needed for broader AWS coverage.

#### Proposed Option

Use Terraform-shaped AWS output resource IDs for Terraform recipe output resources, and add provider resource identity as separate output resource fields in Phase 2. Phase 3 then expands API and client usage of those fields.

### API design

Phase 1 has no public schema change. It changes the value shape of AWS Terraform output resource IDs.

Phase 2 adds first-class fields to `OutputResource`:

```typespec
model OutputResource {
  localId?: string;
  id?: string;
  radiusManaged?: boolean;
  providerResourceId?: string;
  providerResourceIdKind?: string;
  additionalProperties?: Record<string>;
}
```

Phase 3 does not need another output resource schema change for identity. It updates APIs and clients to use these fields consistently.

### CLI Design

No new CLI commands are proposed.

Phase 2 updates existing delete flows to warn when another Radius resource references the same provider resource ID. The warning is advisory and does not change `--force` semantics.

### Implementation Details

#### UCP

No Phase 1 or Phase 2 UCP routing changes are required.

#### Bicep

For AWS Bicep output resources, the Bicep deployment path should read deployed resource properties and set `providerResourceId` and `providerResourceIdKind` when the properties include `ARN`, `Arn`, or `arn`. The raw ARN may also be preserved as `additionalProperties.arn`.

#### Deployment Engine

The deployment processor should preserve output resource `providerResourceId`, `providerResourceIdKind`, and `AdditionalProperties` when persisting deployed output resources. This lets provider resource metadata survive from recipe or Bicep deployment output into resource status.

#### Core RP

The Core RP should preserve `providerResourceId` and `providerResourceIdKind` in app graph output resource data so clients can inspect Phase 2 metadata.

#### Portable Resources / Recipes RP

The portable resource processors should preserve metadata-rich recipe output resources and continue parsing plain `result.resources` string IDs as Radius/UCP resource IDs.

The Terraform driver should build AWS Terraform output resource IDs from Terraform state and AWS provider configuration:

- `resource.ProviderName` identifies AWS provider resources.
- `resource.Type` becomes the Terraform-shaped resource type under `Terraform.AWS`.
- ARN provides partition, region, and a candidate name when possible.
- The configured AWS provider scope provides account.
- Empty ARN region becomes `global`.
- ARN is copied to `providerResourceId` with `providerResourceIdKind: awsArn`. The raw ARN may also be preserved as `additionalProperties.arn`.

### Error Handling

- If a Terraform AWS ARN is malformed, the Terraform driver should return a recipe output error rather than emitting a malformed resource ID.
- If the configured AWS provider scope is missing account information, the Terraform driver should return an error because the Terraform-shaped output resource ID requires an account scope.
- If AWS Bicep output resource properties do not contain an ARN, Radius should still persist the output resource without provider resource identity metadata.
- If either side of a shared-resource comparison lacks provider resource identity, Radius should fall back to producer ID comparison.
- If both sides provide `providerResourceIdKind` and the kinds differ, the output resources should not match.

## Test plan

Phase 1:

- Unit test Terraform AWS S3 state with `arn:aws:s3:::<bucket>` and verify a `Terraform.AWS/aws_s3_bucket` output resource ID.
- Unit test AWS resources whose ARN includes account and region and verify they no longer emit CloudControl-shaped IDs from Terraform state.
- Preserve existing Azure and Kubernetes output resource behavior.

Phase 2:

- Unit test output resource comparison for provider resource ID match, provider resource ID mismatch, provider resource ID kind mismatch, and ID fallback.
- Unit test Terraform output resources with ARN metadata.
- Unit test AWS Bicep output resources with `ARN`, `Arn`, `arn`, and missing ARN properties.
- Unit test resource and application delete warnings for Bicep-shaped and Terraform-shaped output resources that share the same ARN.
- Verify the app graph preserves output resource `AdditionalProperties`.

Phase 3:

- Add app graph tests showing clients can group or inspect output resources by `providerResourceId`.
- Add dashboard and CLI tests, where applicable, showing provider resource identity is used directly instead of inferred from IDs or `additionalProperties`.
- Add API compatibility tests ensuring `providerResourceId` and `providerResourceIdKind` continue to round-trip.

## Security

This design does not change AWS, Azure, or Kubernetes credential handling.

`providerResourceId` is status metadata and will appear in API responses, CLI output, logs, and dashboard data. It must not contain secrets. AWS ARNs are generally identifiers, not credentials. Producers should only populate provider resource IDs from provider-native identity fields, not from secret values or connection strings.

## Compatibility

- Phase 1 changes AWS Terraform output resource IDs. Existing Terraform-created AWS output resources are repopulated with `Terraform.AWS` IDs on the next recipe deployment.
- Phase 2 adds optional output resource fields. Existing resources without provider resource identity continue to compare by ID.
- Phase 3 should use the Phase 2 fields directly, avoiding a metadata migration.
- Existing Azure and Kubernetes output resource behavior is unchanged by Phase 1 and Phase 2.
- Older clients that ignore new output resource fields continue to see output resource IDs as before, except for the intended Phase 1 AWS Terraform ID shape change.

## Monitoring and Logging

The Terraform driver should log when an AWS resource cannot be represented as an output resource because required identity fields are missing or invalid.

Delete warning flows should continue to use existing CLI output. No new metrics are required for Phase 1 or Phase 2.

## Development plan

1. Phase 1: Merge AWS Terraform output resource ID changes for [#11838](https://github.com/radius-project/radius/issues/11838).
2. Phase 2: Merge shared-resource comparison and AWS ARN metadata changes for [#12020](https://github.com/radius-project/radius/issues/12020).
3. Phase 3: Open a follow-up issue or design review for making app graph, dashboard, CLI, and other APIs consistently use provider resource identity.

## Open Questions

- Should `providerResourceId` always be paired with `providerResourceIdKind`, or can a missing kind be treated as an opaque provider ID?
- Should Radius normalize ARN case or preserve provider output exactly?
- Should Radius expose graph grouping directly, or only expose provider resource IDs and let clients group?
- Should server-side delete validation use provider resource identity, and if so, which operations should enforce it?

## Alternatives considered

### Convert Terraform AWS resources to CloudControl IDs

This would make Terraform and Bicep output resources share the same ID when a Terraform type maps cleanly to CloudControl.

Advantages:

- Existing ID comparison would work for mapped resources.
- App graph clients would not need a second identity field for those cases.

Disadvantages:

- Requires maintaining or generating a Terraform-to-CloudControl type map.
- Does not cover Terraform resources that represent configuration, relationships, or provider-specific resources.
- ARN and Terraform type strings do not guarantee enough information to infer CloudControl type.
- Incorrect mappings would imply CloudControl lifecycle semantics for Terraform-managed resources.

### Use ARN as the output resource ID

This would avoid producer-specific Radius IDs for AWS resources with ARNs.

Advantages:

- ARN is the AWS-native identity for many resources.
- Bicep and Terraform would naturally converge for many AWS resources.

Disadvantages:

- `status.outputResources[].id` is a Radius resource ID today, not an arbitrary provider ID.
- ARNs do not identify the Radius producer or lifecycle path.
- Some resources do not expose ARNs, and some Terraform resources are not standalone AWS resources.
- This would be a larger API contract change than Phase 1 or Phase 2.

## Design Review Notes

To be updated during design review.
