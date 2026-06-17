# Output Resource Identity Phases

* **Author**: Will Smith (@willdavsmith)

## Overview

AWS Terraform recipes currently populate output resources by reading Terraform state, extracting each resource ARN, and converting that ARN into a CloudControl-shaped Radius AWS resource ID. That conversion assumes the ARN contains enough information to infer an AWS resource type, account, region, and resource name.

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

This design separates two concepts that are currently overloaded into one ID:

- Producer identity: the Radius/UCP ID for the system that produced and owns the output resource lifecycle.
- Provider resource identity: the provider-native ID for the underlying resource, such as an AWS ARN.

It proposes three phases:

1. Phase 1: Fix AWS Terraform output resource IDs so Terraform resources use `Terraform.AWS` IDs.
2. Phase 2: Add first-class provider resource identity metadata on output resources and use it for shared-resource delete warnings.
3. Phase 3: Update Radius APIs to consistently use provider resource identity for association and grouping, so dashboard, CLI, and generated clients can consume normalized results without provider-specific matching logic.

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

- Fix AWS Terraform recipe output resources for ARNs that omit account, region, or resource type information.
- Keep `status.outputResources[].id` as a Radius/UCP producer and lifecycle ID.
- Stop emitting malformed or misleading AWS CloudControl-shaped IDs for Terraform-managed resources.
- Detect when AWS Bicep and Terraform output resources reference the same provider resource.
- Add first-class provider resource identity so future clients do not each implement provider-specific matching rules.
- Preserve existing Azure and Kubernetes output resource behavior.

### Non goals

- Manage Terraform-created AWS resources through AWS CloudControl.
- Build or maintain a Terraform AWS provider type to CloudControl type mapper.
- Replace `status.outputResources[].id` with arbitrary provider-native IDs such as ARNs.
- Guarantee provider resource identity for every provider resource type. Resources without provider-native identity continue to compare by producer ID.
- Add server-side delete enforcement in Phase 2. Phase 2 warnings remain advisory.
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

Phase 1 changes the IDs users see in `status.outputResources` for AWS Terraform recipes. Terraform-managed AWS resources use `Terraform.AWS` IDs rather than CloudControl-shaped `AWS.*` IDs, so S3 recipe output no longer fails while Radius tries to parse `arn:aws:s3:::<bucket>` into a CloudControl-shaped ID.

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

Phase 2 adds optional provider resource identity fields:

```json
{
  "status": {
    "outputResources": [
      {
        "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket",
        "radiusManaged": true,
        "providerResourceId": "arn:aws:s3:::shared-bucket",
        "providerResourceIdKind": "awsArn"
      }
    ]
  }
}
```

No recipe input contract changes are required. Existing `result.resources` and recipe-derived output resources continue to be supported.

## Design

### High Level Design

The selected design keeps `status.outputResources[].id` as the producer ID and adds provider identity beside it:

- `id` remains a Radius/UCP resource ID that identifies producer namespace, type, scope, and lifecycle path.
- `providerResourceId` identifies the provider-native resource when Radius can determine one.
- `providerResourceIdKind` identifies the provider resource ID format, for example `awsArn`.

Phase 1 fixes the Terraform AWS producer ID. Phase 2 adds first-class provider identity fields and uses them for shared-resource delete warnings. Phase 3 uses those fields across Radius APIs for normalized association and grouping.

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

The previous AWS Terraform behavior parsed an ARN and tried to emit a CloudControl-shaped ID. That fails for S3 because `arn:aws:s3:::shared-bucket` has no account ID, no region, and no CloudControl resource type token such as `Bucket`. Phase 1 avoids inventing missing CloudControl information. The account comes from the configured AWS provider scope, an empty ARN region becomes `global`, and the resource type comes from Terraform state.

Terraform provider `id` values are provider-defined, so Radius should prefer the ARN resource component when it has a usable final physical-name segment. If that is unavailable, Radius can use simple provider state attributes such as `id`. Radius should not use the Terraform block name as the physical name. In `resource "aws_s3_bucket" "storage"`, `storage` is a local Terraform label, not the AWS bucket name.

#### Phase 2: First-class provider resource metadata

Phase 2 keeps producer IDs distinct but adds optional first-class provider resource identity fields:

```json
{
  "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket",
  "radiusManaged": true,
  "providerResourceId": "arn:aws:s3:::shared-bucket",
  "providerResourceIdKind": "awsArn"
}
```

For AWS resources, `providerResourceId` is the ARN when Radius can read one, and `providerResourceIdKind` is `awsArn`. Radius should not synthesize account or region into an ARN. S3 bucket ARNs omit those fields because the bucket name is already globally unique within the AWS partition.

Phase 2 behavior:

- AWS Terraform output resources store provider identity when Terraform state has an `arn` attribute.
- AWS Bicep output resources keep CloudControl-shaped IDs and store the same ARN metadata when CloudControl resource properties expose `ARN`, `Arn`, or `arn`.
- Delete warning logic compares output resources by provider identity when both sides have it, then falls back to producer ID equality. If both sides provide `providerResourceIdKind`, the kind must also match.
- Application graph output resources preserve provider identity fields so clients can inspect them.
- Warnings remain advisory. Phase 2 does not add server-side delete enforcement.

#### Phase 3: Provider resource identity adoption

Phase 3 implements provider-resource equality in Radius control plane APIs. The control plane should decide when two output resources represent the same provider resource, then expose that association in API responses. Dashboard, CLI, SDKs, and generated clients should not independently compare ARNs or provider-specific fields.

The equality rule is:

1. If both output resources have `providerResourceId`, compare `providerResourceId`.
2. If either output resource lacks `providerResourceId`, fall back to producer ID equality.
3. If both output resources also have `providerResourceIdKind`, the kinds must match.

For example:

```json
{
  "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket",
  "radiusManaged": true,
  "providerResourceId": "arn:aws:s3:::shared-bucket",
  "providerResourceIdKind": "awsArn"
}
```

Phase 3 behavior:

- Use the provider-resource equality rule for control plane API association beyond delete warnings.
- Keep producer IDs unchanged so lifecycle ownership remains explicit.
- Update APIs such as app graph to group or annotate output resources that represent the same provider resource.
- Ensure dashboard, CLI, SDKs, and generated clients consume control-plane associations instead of implementing provider-specific matching rules.

Phase 2 is sufficient for delete warnings, but it does not solve every client concern. For example, the dashboard app graph should not have to group equivalent resources itself by comparing AWS ARNs. Phase 3 makes provider-resource equality a platform-wide control plane contract.

#### Advantages

- Fixes malformed AWS Terraform output resource IDs without an unreliable type map.
- Preserves lifecycle ownership in the producer ID.
- Enables cross-producer matching when a real provider resource identity exists.
- Avoids a Phase 2 to Phase 3 metadata migration by adding provider identity fields in Phase 2.
- Avoids making every future client infer Radius identity rules from raw output resource IDs.

#### Disadvantages

- Producer IDs differ for the same physical object.
- Phase 2 only applies the equality rule to shared-resource delete warnings; broader control-plane API association and grouping behavior waits for Phase 3.
- Resources without reliable provider resource metadata still fall back to ID matching.
- ARN extraction from CloudControl properties starts with common property names. Schema-driven extraction may be needed for broader AWS coverage.

#### Proposed Option

Proceed with the phased design above.

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
}
```

Phase 3 does not need another output resource schema change for identity. It updates control plane APIs to apply provider-resource equality consistently and return the resulting associations or groups.

### CLI Design

No new CLI commands are proposed.

Phase 2 updates existing delete flows to warn when another Radius resource references the same provider resource ID. The warning remains advisory and does not change `--force` semantics.

### Implementation Details

#### UCP

No Phase 1 or Phase 2 UCP routing changes are required.

#### Bicep

For AWS Bicep output resources, the Bicep deployment path should set `providerResourceId` and `providerResourceIdKind` when deployed resource properties include `ARN`, `Arn`, or `arn`.

#### Deployment Engine

The deployment processor should preserve output resource `providerResourceId` and `providerResourceIdKind` when persisting deployed output resources. This lets provider resource metadata survive from recipe or Bicep deployment output into resource status.

#### Core RP

The Core RP should preserve `providerResourceId` and `providerResourceIdKind` in app graph output resource data so clients can inspect Phase 2 metadata.

In Phase 3, Core RP APIs that return app/resource relationships should use the provider-resource equality rule to associate output resources that have different producer IDs but the same provider identity.

#### Portable Resources / Recipes RP

The portable resource processors should preserve recipe output resources with provider identity and continue parsing plain `result.resources` string IDs as Radius/UCP resource IDs.

The Terraform driver should build AWS Terraform output resource IDs from Terraform state and AWS provider configuration:

- `resource.ProviderName` identifies AWS provider resources.
- `resource.Type` becomes the Terraform-shaped resource type under `Terraform.AWS`.
- ARN provides partition, region, and a candidate name when possible.
- The configured AWS provider scope provides account.
- Empty ARN region becomes `global`.
- ARN is copied to `providerResourceId` with `providerResourceIdKind: awsArn`.

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
- Verify the app graph preserves output resource `providerResourceId` and `providerResourceIdKind`.

Phase 3:

- Add app graph tests showing the control plane groups or annotates equivalent output resources by provider-resource equality.
- Add API tests for provider ID match, provider ID mismatch, provider ID kind mismatch, and producer ID fallback.
- Add dashboard and CLI tests, where applicable, showing clients consume control-plane associations instead of inferring identity from IDs.
- Add API compatibility tests ensuring `providerResourceId` and `providerResourceIdKind` continue to round-trip.

## Security

This design does not change AWS, Azure, or Kubernetes credential handling.

`providerResourceId` is status metadata and will appear in API responses, CLI output, logs, and dashboard data. It must not contain secrets. AWS ARNs are generally identifiers, not credentials. Producers should only populate provider resource IDs from provider-native identity fields, not from secret values or connection strings.

## Compatibility

- Phase 1 changes AWS Terraform output resource IDs. Existing AWS Terraform output resources are repopulated with `Terraform.AWS` IDs on the next recipe deployment.
- Phase 2 adds optional output resource fields. Existing resources without provider identity continue to compare by ID.
- Phase 3 uses the Phase 2 fields directly in control plane APIs, avoiding a metadata migration.
- Azure and Kubernetes output resource behavior is unchanged by Phase 1 and Phase 2.
- Older clients that ignore new fields continue to see output resource IDs as before, except for the intended Phase 1 AWS Terraform ID shape change.

## Monitoring and Logging

The Terraform driver should log when an AWS resource cannot be represented as an output resource because required identity fields are missing or invalid.

Delete warning flows should continue to use existing CLI output. No new metrics are required for Phase 1 or Phase 2.

## Development plan

1. Phase 1: Merge AWS Terraform output resource ID changes for [#11838](https://github.com/radius-project/radius/issues/11838).
2. Phase 2: Merge shared-resource comparison and first-class provider resource identity fields for [#12020](https://github.com/radius-project/radius/issues/12020).
3. Phase 3: Open a follow-up issue or design review for provider-resource equality in control plane APIs, starting with app graph and the APIs that dashboard, CLI, SDKs, and generated clients consume.

## Open Questions

- Should `providerResourceId` always be paired with `providerResourceIdKind`, or can a missing kind be treated as an opaque provider ID?
- Should Radius normalize ARN case or preserve provider output exactly?
- What normalized grouping or association shape should control plane APIs expose when multiple producer IDs reference the same provider resource?
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
