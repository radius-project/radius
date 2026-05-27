# AWS Terraform Output Resource Identity

* **Author**: Will Smith (@willdavsmith)
* **Status**: Draft

## Overview

Radius Terraform recipes populate `status.outputResources` by reading resources from Terraform state. For AWS resources, the current implementation reads the resource ARN and converts it to a UCP AWS resource ID.

This works for ARNs whose resource component includes a resource type, such as EC2 subnet ARNs. It does not work for all AWS resources. For example, an S3 bucket ARN has the shape `arn:aws:s3:::bucket-name`. The ARN does not include an account ID, region, or resource type token such as `Bucket`. Radius cannot infer a CloudControl-style resource type from the ARN in a generic way, and as a result, the deployment fails.

This document evaluates options for representing Terraform-created AWS output resources in Radius. The central decision is whether Terraform output resources should use the same AWS-plane, CloudControl-shaped identity as AWS Bicep resources, or whether they should use a Terraform-shaped identity that reflects their actual lifecycle owner.

## Terms and definitions

| Term | Definition |
| --- | --- |
| Output resource | A resource reference stored in `status.outputResources` on a Radius resource. |
| AWS-plane resource ID | A UCP resource ID under `/planes/aws/.../providers/...`. |
| CloudControl-shaped type | A Radius AWS type derived from an AWS CloudFormation or CloudControl type, for example `AWS.S3/Bucket` for `AWS::S3::Bucket`. |
| Terraform-shaped type | A Radius type derived from Terraform provider state, for example `Terraform.AWS/aws_s3_bucket` or `AWS.terraform/aws_s3_bucket`. |
| Terraform resource type | The Terraform provider schema type stored in state, for example `aws_s3_bucket` or `aws_sqs_queue`. |
| Terraform resource ID | The provider-defined `id` attribute in Terraform state. This is not standardized across AWS resource types. |
| ARN | Amazon resource name. ARN formats are service-specific and are not a complete resource type system. |

## Objectives

### Goals

- Fix AWS Terraform recipe output resources for resources with ARNs that omit account, region, or resource type information.
- Avoid emitting malformed or misleading UCP resource IDs.
- Preserve Terraform as the lifecycle owner for Terraform-created resources.
- Define a clear identity contract for Terraform-created AWS output resources.

### Non goals

- Manage Terraform-created AWS resources through AWS CloudControl.
- Build a fully authoritative Terraform AWS provider type to CloudControl type mapper as part of the bug fix.
- Change the recipe output contract for Azure or Kubernetes resources.
- Change Terraform state storage or Terraform apply/destroy behavior.

## Background

The original Terraform recipe output resource design is described in [Populate Terraform Resource IDs](./2023-09-populate-terraform-resourcs-ids.md).

The current AWS behavior assumes this ARN shape:

```text
arn:partition:service:region:account-id:resource-type/resource-id
```

and converts it to:

```text
/planes/aws/<partition>/accounts/<account-id>/regions/<region>/providers/AWS.<service>/<resource-type>/<resource-id>
```

This is not valid for all AWS ARNs. Examples:

```text
arn:aws:s3:::my-bucket
arn:aws:sqs:us-west-2:123456789012:my-queue
arn:aws:sns:us-west-2:123456789012:my-topic
```

These ARNs include a physical name, but not a CloudControl resource type segment. Terraform state provides additional context, including:

```json
{
  "type": "aws_s3_bucket",
  "name": "storage",
  "provider_name": "registry.terraform.io/hashicorp/aws",
  "values": {
    "id": "my-bucket",
    "arn": "arn:aws:s3:::my-bucket",
    "bucket": "my-bucket"
  }
}
```

Terraform state does not provide a generic CloudControl type. Terraform resource types, Terraform `id` attributes, and AWS CloudControl types are separate naming systems.

## Real-world examples

The following examples show why there is no single parsing rule that converts Terraform state or ARN text into a CloudControl-shaped AWS resource ID.

| Terraform resource type | Terraform attributes / import identity | Example ARN shape | CloudControl type | Why this is adversarial |
| --- | --- | --- | --- | --- |
| `aws_s3_bucket` | Terraform manages an S3 bucket resource and exports `arn`. | `arn:aws:s3:::my-bucket` | `AWS::S3::Bucket` | ARN has no account, no region, and no resource type token. The bucket name is present, but `Bucket` is not. |
| `aws_sqs_queue` | Terraform exports `arn`; Terraform `id` and `url` are the queue URL. | `arn:aws:sqs:us-west-2:123456789012:my-queue` | `AWS::SQS::Queue` | ARN has account and region, but the resource component is only `my-queue`. A parser cannot infer `Queue` from the ARN alone, and Terraform `id` is a URL rather than an ARN. |
| `aws_sns_topic` | Terraform exports `arn`; Terraform `id` is also the topic ARN. | `arn:aws:sns:us-east-1:123456789012:my-topic` | `AWS::SNS::Topic` | Same bare resource component shape as SQS. A generic ARN parser would emit a type-less or malformed UCP resource ID. |
| `aws_cloudwatch_log_group` | Terraform resource type is `aws_cloudwatch_log_group`; Terraform documents an `arn` attribute. | `arn:aws:logs:us-west-1:123456789012:log-group:/mystack-testgroup` | `AWS::Logs::LogGroup` | Terraform says `cloudwatch`, ARN says `logs`, ARN path says `log-group`, and CloudControl says `Logs::LogGroup`. None of these match mechanically. |
| `aws_db_instance` | Terraform resource type is `aws_db_instance`; Terraform exports `arn`. | `arn:aws:rds:us-west-2:123456789012:db:mydb` | `AWS::RDS::DBInstance` | Terraform omits `rds` from the type name and uses `db_instance`; ARN uses `rds` service and `db`; CloudControl uses `RDS::DBInstance`. |
| `aws_iam_role` | Terraform exports `arn`; `id` is the role name. | `arn:aws:iam::123456789012:role/developer` | `AWS::IAM::Role` | Region is empty, `id` is not ARN-like, and ARN casing/path tokens do not directly match the CloudControl type casing. |
| `aws_lb` | Terraform imports by load balancer ARN. | `arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb/50dc...` | `AWS::ElasticLoadBalancingV2::LoadBalancer` | Terraform type is abbreviated, ARN service is long, ARN path includes `app` or `net`, and CloudControl uses `ElasticLoadBalancingV2`. |
| `aws_s3_bucket_versioning` | Terraform manages versioning for a bucket via a separate resource using a `bucket` argument. | Often bucket-name based rather than a standalone ARN. | Usually modeled as configuration on `AWS::S3::Bucket`, not a separate bucket. | Not every Terraform resource maps to a standalone CloudControl resource. Emitting a provider resource ID can be misleading. |

These examples are intentionally not all failure cases for the same reason. They show different failure modes:

- missing account or region in ARN;
- missing resource type token in ARN;
- Terraform service names that do not match ARN service names;
- ARN resource tokens that do not match CloudControl type names;
- Terraform resources that represent configuration or relationships rather than standalone AWS resources.

## Constraints

- `status.outputResources[].id` must be parseable by Radius as a resource ID.
- The same Terraform resource can expose `id`, `arn`, and name-like attributes with different semantics.
- Terraform `id` is provider-defined and is not guaranteed to have a consistent delimiter or part count.
- ARN formats are service-specific and are not guaranteed to include CloudControl resource type information.
- AWS CloudControl type names are the existing type vocabulary for AWS-plane resources in Radius.
- Terraform-created AWS resources are created, updated, and deleted by Terraform, not by AWS CloudControl.

## Options

### Option 1: CloudControl-shaped AWS-plane IDs with curated or generated mapping

Terraform-created AWS output resources continue to use AWS-plane IDs with CloudControl-shaped types:

```text
/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket
```

Radius would use (and maintain) a mapping from Terraform resource type to CloudControl-shaped Radius type:

```text
aws_s3_bucket -> AWS.S3/Bucket
aws_sqs_queue -> AWS.SQS/Queue
aws_sns_topic -> AWS.SNS/Topic
aws_cloudwatch_log_group -> AWS.Logs/LogGroup
aws_db_instance -> AWS.RDS/DBInstance
```

Unknown or ambiguous Terraform AWS resources would be skipped or logged rather than emitted incorrectly.

The mapping can start as a small curated table for high-confidence resources. It can later be generated from Terraform AWS provider schema, AWS CloudFormation or CloudControl schemas, and AWS service authorization ARN formats. Generated output should still be checked into the repo and reviewed. Runtime lookup against external metadata is not recommended during recipe execution.

#### Pros

- Keeps AWS Bicep and AWS Terraform output resources in the same AWS-plane identity vocabulary.
- Graph, status, filtering, and future tooling can treat the same AWS object as the same kind of provider resource.
- Preserves the existing `status.outputResources` shape for AWS resources.

#### Cons

- Requires a mapping from Terraform resource types to CloudControl-shaped AWS types.
- The mapping cannot be derived perfectly from Terraform type names, ARNs, or Terraform `id`.
- Manual mapping creates ongoing maintenance cost; generated mapping still needs heuristics and overrides.
- Some Terraform resources do not represent standalone CloudControl resources.
- Incorrect mappings are worse than omitted output resources because they create misleading provider resource IDs.
- The mapping adds version drift risk as both Terraform AWS provider and AWS CloudControl schemas evolve.

#### Implementation difficulty

Small for a targeted curated fix, medium to high for broad generated coverage.

A targeted implementation can add a small mapping for `aws_s3_bucket` and any other confirmed high-confidence resources. A broader implementation would need metadata inputs, review tooling, and CI checks that fail when generated output changes unexpectedly.

### Option 2: Terraform-shaped output resource IDs

Terraform-created AWS output resources use a Terraform-specific type vocabulary instead of CloudControl-shaped AWS types.

Example:

```text
/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/Terraform.AWS/aws_sqs_queue/my-queue
```

or:

```text
/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.terraform/aws_sqs_queue/my-queue
```

The ID uses Terraform state as the source of truth:

- `resource.ProviderName` identifies the Terraform AWS provider.
- `resource.Type` identifies the Terraform resource type.
- The resource name comes from ARN resource text, a name-like Terraform attribute, or `id`.
- Account and region come from ARN when available, with provider configuration as the authoritative fallback for account.

#### Pros

- Avoids pretending every Terraform AWS resource maps to a CloudControl resource.
- Works for present and future Terraform AWS resource types without a CloudControl mapping.
- Represents lifecycle ownership more honestly: Terraform owns these resources.
- Avoids malformed CloudControl-shaped IDs for resources with bare or service-specific ARNs.
- Removes the need for a static Terraform-to-CloudControl map for recipe output resources.

#### Cons

- Introduces a second AWS output resource type vocabulary.
- AWS Bicep and Terraform output resources for the same underlying AWS object would not converge to the same ID.
- Existing users and tooling may expect AWS output resources to be CloudControl-shaped.
- `Terraform.AWS` under the AWS plane is a new convention that needs API review.
- It may be unclear whether `/planes/aws/.../providers/Terraform.AWS/...` represents an AWS provider resource, a Terraform state resource, or both.
- The physical name is still not perfectly generic. Some Terraform resources have composite IDs or no stable simple name.

### Option 3: Strict CloudControl-shaped IDs with explicit recipe escape hatch

Radius keeps the existing CloudControl-shaped AWS-plane identity contract, but becomes stricter. If an AWS ARN can be converted safely into a valid CloudControl-shaped AWS-plane ID, Radius emits it. If the ARN lacks enough information, Radius logs and skips the output resource. Recipe authors can explicitly return `result.resources` for cases where they know the desired Radius ID.

For example, EC2 subnet output resources would continue to be emitted:

```text
arn:aws:ec2:us-east-2:123456789012:subnet/subnet-abc
```

S3 bucket output resources would be skipped unless the recipe explicitly returns a valid output resource ID:

```hcl
output "result" {
  value = {
    resources = [
      "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket"
    ]
  }
}
```

#### Pros

- Avoids false or misleading output resource IDs.
- Requires no Terraform-to-CloudControl mapping.
- Keeps the existing AWS-plane identity contract.
- Smallest implementation surface.
- Gives advanced recipe authors an escape hatch through explicit `result.resources`.

#### Cons

- Users lose useful output resource visibility for common resources like S3 buckets, SQS queues, and SNS topics.
- Behavior depends on AWS ARN format, which is not intuitive to Radius users.
- Recipes may need to explicitly return output resource IDs to compensate.
- Burdens recipe authors with Radius UCP ID construction for ambiguous resources.

## Open questions

<TODO>

## References

- [Terraform AWS provider `aws_s3_bucket`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket)
- [Terraform AWS provider `aws_sqs_queue`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sqs_queue)
- [Terraform AWS provider `aws_sns_topic`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic)
- [Terraform AWS provider `aws_cloudwatch_log_group`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_log_group)
- [Terraform AWS provider `aws_db_instance`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/db_instance)
- [Terraform AWS provider `aws_iam_role`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role)
- [Terraform AWS provider `aws_lb`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lb)
- [AWS CloudFormation `AWS::S3::Bucket`](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-s3-bucket.html)
- [AWS CloudFormation `AWS::SQS::Queue`](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-sqs-queue.html)
- [AWS CloudFormation `AWS::SNS::Topic`](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-sns-topic.html)
- [AWS CloudFormation `AWS::Logs::LogGroup`](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-resource-logs-loggroup.html)
- [AWS CloudFormation `AWS::RDS::DBInstance`](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-rds-dbinstance.html)
- [AWS CloudFormation `AWS::IAM::Role`](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-iam-role.html)
- [AWS CloudFormation `AWS::ElasticLoadBalancingV2::LoadBalancer`](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-elasticloadbalancingv2-loadbalancer.html)
