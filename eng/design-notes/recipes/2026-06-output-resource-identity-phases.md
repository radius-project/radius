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
- Populate provider resource identity for AWS, Azure, and Kubernetes output resources when Radius can determine it.
- Preserve existing output resource ID behavior except for the intended AWS Terraform ID shape change.

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

Phase 2 adds an optional provider resource identity field:

```json
{
  "status": {
    "outputResources": [
      {
        "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket",
        "radiusManaged": true,
        "providerResourceId": "arn:aws:s3:::shared-bucket"
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

Phase 1 fixes the Terraform AWS producer ID. Phase 2 adds a first-class provider identity field and uses it for shared-resource delete warnings. Phase 3 uses that field across Radius APIs for normalized association and grouping.

This design populates `providerResourceId` for AWS, Azure, and Kubernetes output resources when Radius can determine the provider-native identity. The field is optional, so resources without provider identity continue to work with existing Radius resource ID comparisons.

#### Provider coverage

AWS output resources should use the resource ARN as `providerResourceId`.

Azure output resources should use the ARM resource ID as `providerResourceId`. For many Azure output resources this may be the same value as `id`, but Radius should still populate `providerResourceId` so control plane APIs can use one equality model across providers.

Kubernetes output resources should use a stable Kubernetes object identity as `providerResourceId`, including enough information to distinguish cluster, API group/version, kind or resource, namespace, and name. The exact serialized format should follow existing Radius Kubernetes identity conventions so the value is stable and comparable.

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

Phase 2 keeps producer IDs distinct but adds an optional first-class provider resource identity field:

```json
{
  "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket",
  "radiusManaged": true,
  "providerResourceId": "arn:aws:s3:::shared-bucket"
}
```

For AWS resources, `providerResourceId` is the ARN when Radius can read one. Radius should not synthesize account or region into an ARN. S3 bucket ARNs omit those values because the bucket name is already globally unique within the AWS partition.

Phase 2 behavior:

- AWS Terraform output resources store provider identity when Terraform state has an `arn` attribute.
- AWS Bicep output resources keep CloudControl-shaped IDs and store the same ARN metadata when CloudControl resource properties expose `ARN`, `Arn`, or `arn`.
- Azure output resources store the ARM resource ID as provider identity when Radius can determine it.
- Kubernetes output resources store stable Kubernetes object identity as provider identity when Radius can determine it.
- Delete warning logic compares output resources by provider identity when both sides have it, then falls back to producer ID equality.
- Application graph output resources preserve provider identity so clients can inspect it.
- Warnings remain advisory. Phase 2 does not add server-side delete enforcement.

#### Phase 3: Provider resource identity adoption

Phase 3 implements provider-resource equality in Radius control plane APIs. The control plane should decide when two output resources represent the same provider resource, then expose that association in API responses. Dashboard, CLI, SDKs, and generated clients should not independently compare ARNs or provider-specific fields.

The equality rule is:

1. If both output resources have `providerResourceId`, compare `providerResourceId`.
2. If either output resource lacks `providerResourceId`, fall back to producer ID equality.

This fallback order is intentionally non-breaking. Existing resources and older producers that only populate `id` continue to compare exactly as they do today. Newer resources get stronger cross-producer matching when `providerResourceId` is present.

For example:

```json
{
  "id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket",
  "radiusManaged": true,
  "providerResourceId": "arn:aws:s3:::shared-bucket"
}
```

Phase 3 behavior:

- Use the provider-resource equality rule for control plane API association beyond delete warnings.
- Keep producer IDs unchanged so lifecycle ownership remains explicit.
- Update APIs such as app graph and resource list to group or annotate output resources that represent the same provider resource.
- Ensure dashboard, CLI, SDKs, and generated clients consume control-plane associations instead of implementing provider-specific matching rules.

Phase 2 is sufficient for delete warnings, but it does not solve every client concern. For example, the dashboard app graph and `rad resource list -a <application>` should not have to group equivalent resources themselves by comparing AWS ARNs. Phase 3 makes provider-resource equality a platform-wide control plane contract.

#### Advantages

- Fixes malformed AWS Terraform output resource IDs without an unreliable type map.
- Preserves lifecycle ownership in the producer ID.
- Enables cross-producer matching when a real provider resource identity exists.
- Avoids a Phase 2 to Phase 3 metadata migration by adding provider identity in Phase 2.
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

Phase 2 adds one first-class field to `OutputResource`:

```typespec
model OutputResource {
  localId?: string;
  id?: string;
  radiusManaged?: boolean;
  providerResourceId?: string;
}
```

Phase 3 does not need another output resource schema change for identity. It updates control plane APIs to apply provider-resource equality consistently and return the resulting associations or groups.

### CLI Design

No new CLI commands are proposed.

Phase 2 updates existing delete flows to warn when another Radius resource references the same provider resource ID. The warning remains advisory and does not change `--force` semantics.

### Implementation Details

#### UCP

Phase 1: no UCP routing changes are required.

Phase 2: no UCP routing changes are required.

Phase 3: no UCP routing changes are expected unless a control plane API endpoint outside Core RP needs to apply provider-resource equality.

#### Bicep

Phase 1: no Bicep changes are required.

Phase 2: for AWS Bicep output resources, the Bicep deployment path should set `providerResourceId` when deployed resource properties include `ARN`, `Arn`, or `arn`. For Azure Bicep output resources, it should set `providerResourceId` to the ARM resource ID when Radius can determine it.

Phase 3: no Bicep-specific changes are expected beyond preserving `providerResourceId`.

#### Deployment Engine

Phase 1: no deployment engine schema changes are required.

Phase 2: the deployment processor should preserve output resource `providerResourceId` when persisting deployed output resources. This lets provider resource metadata survive from recipe or Bicep deployment output into resource status.

Phase 3: the deployment engine continues to preserve `providerResourceId`; control plane APIs own equality behavior.

#### Core RP

Phase 1: no Core RP changes are required.

Phase 2: the Core RP should preserve `providerResourceId` in app graph output resource data so clients can inspect Phase 2 metadata.

Phase 3: Core RP APIs that return app/resource relationships should use the provider-resource equality rule to associate output resources that have different producer IDs but the same provider identity.

#### Portable Resources / Recipes RP

Phase 1: the Terraform driver should build AWS Terraform output resource IDs from Terraform state and AWS provider configuration:

- `resource.ProviderName` identifies AWS provider resources.
- `resource.Type` becomes the Terraform-shaped resource type under `Terraform.AWS`.
- ARN provides partition, region, and a candidate name when possible.
- The configured AWS provider scope provides account.
- Empty ARN region becomes `global`.

Phase 2: the portable resource processors should preserve recipe output resources with provider identity and continue parsing plain `result.resources` string IDs as Radius/UCP resource IDs. The Terraform driver copies ARN to `providerResourceId` when Terraform state includes an `arn` attribute.

Phase 3: no Recipes RP-specific equality behavior is expected. Control plane APIs consume the persisted `providerResourceId`.

#### Kubernetes

Phase 1: no Kubernetes-specific changes are required.

Phase 2: Kubernetes output resource producers should populate `providerResourceId` with the stable Kubernetes object identity when Radius can determine it.

Phase 3: Kubernetes output resources participate in the same control plane provider-resource equality behavior as AWS and Azure output resources.

### Error Handling

- If a Terraform AWS ARN is malformed, the Terraform driver should return a recipe output error rather than emitting a malformed resource ID.
- If the configured AWS provider scope is missing account information, the Terraform driver should return an error because the Terraform-shaped output resource ID requires an account scope.
- If Radius cannot read a provider-native identity from Terraform state, deployment output, or Kubernetes object metadata, Radius should still persist the output resource without provider resource identity metadata.
- If either side of a shared-resource comparison lacks provider resource identity, Radius should fall back to producer ID comparison.

## Test plan

Phase 1:

- Unit test Terraform AWS S3 state with `arn:aws:s3:::<bucket>` and verify a `Terraform.AWS/aws_s3_bucket` output resource ID.
- Unit test AWS resources whose ARN includes account and region and verify they no longer emit CloudControl-shaped IDs from Terraform state.
- Preserve existing Azure and Kubernetes output resource ID behavior in Phase 1.

Phase 2:

- Unit test output resource comparison for provider resource ID match, provider resource ID mismatch, and ID fallback.
- Unit test Terraform output resources with ARN metadata.
- Unit test AWS Bicep output resources with `ARN`, `Arn`, `arn`, and deployment output that does not include an ARN property.
- Unit test Azure output resources populate ARM resource ID as `providerResourceId`.
- Unit test Kubernetes output resources populate stable Kubernetes object identity as `providerResourceId`.
- Unit test resource and application delete warnings for Bicep-shaped and Terraform-shaped output resources that share the same ARN.
- Verify the app graph preserves output resource `providerResourceId`.

Phase 3:

- Add app graph tests showing the control plane groups or annotates equivalent output resources by provider-resource equality.
- Add API tests for provider ID match, provider ID mismatch, and producer ID fallback.
- Add dashboard and CLI tests, where applicable, showing clients consume control-plane associations instead of inferring identity from IDs.
- Add API compatibility tests ensuring `providerResourceId` continues to round-trip.

## Security

This design does not change AWS, Azure, or Kubernetes credential handling.

`providerResourceId` is status metadata and will appear in API responses, CLI output, logs, and dashboard data. It must not contain secrets. AWS ARNs are generally identifiers, not credentials. Producers should only populate provider resource IDs from provider-native identity fields, not from secret values or connection strings.

## Compatibility

- Phase 1 changes AWS Terraform output resource IDs. Existing AWS Terraform output resources are repopulated with `Terraform.AWS` IDs on the next recipe deployment.
- Phase 2 adds an optional output resource field. Existing resources without provider identity continue to compare by ID.
- Phase 3 uses the Phase 2 field directly in control plane APIs, avoiding a metadata migration.
- Older clients that ignore the new field continue to see output resource IDs as before, except for the intended Phase 1 AWS Terraform ID shape change.

### No breaking changes

This design should not require a breaking API change. `providerResourceId` is optional status metadata. Existing clients can ignore it, and existing resources that do not have the field remain valid.

Control plane comparison should use this fallback order:

1. If both output resources have `providerResourceId`, compare `providerResourceId`.
2. Otherwise, compare the Radius output resource `id`.

This preserves current behavior for older resources and producers while enabling stronger matching for resources that provide a provider-native identity.

## Monitoring and Logging

The Terraform driver should log when an AWS resource cannot be represented as an output resource because required identity values are missing or invalid.

Delete warning flows should continue to use existing CLI output. No new metrics are required for Phase 1 or Phase 2.

## Development plan

1. Phase 1: Merge AWS Terraform output resource ID changes for [#11838](https://github.com/radius-project/radius/issues/11838).
2. Phase 2: Merge shared-resource comparison and first-class provider resource identity for [#12020](https://github.com/radius-project/radius/issues/12020).
3. Phase 3: Open a follow-up issue or design review for provider-resource equality in control plane APIs, starting with app graph and the APIs that dashboard, CLI, SDKs, and generated clients consume.

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
- Some Radius output resources may not have provider ARNs available in Terraform state or deployment output, and some Terraform resources are not standalone AWS resources.
- This would be a larger API contract change than Phase 1 or Phase 2.

## Design Review Notes

To be updated during design review.
