# Fix AWS functional test failure (`Test_AWSRedeployWithUpdatedResourceUpdatesResource`)

## Problem

The AWS mechanics test that creates a CloudWatch LogGroup and then updates its `RetentionInDays` property started failing around March 23, 2026 with:

> "The security token included in the request is invalid (Service: CloudWatchLogs, Status Code: 400)"

The CREATE step succeeded, but the UPDATE step always failed.

## Root Cause

Around March 23, AWS updated the CloudControl `GetResource` response for `AWS::Logs::LogGroup` to include new properties (`DeletionProtectionEnabled`, `FieldIndexPolicies`, etc.) that were added to the LogGroup API in late 2025. Our `GeneratePatch` function was generating `"remove"` operations in the JSON Patch for any property present in the AWS current state but absent from the user's desired state — unless it was a read-only or create-only property. These new updatable properties triggered `"remove"` operations in the patch, which caused CloudControl's async handler to fail.

This was a latent bug in our patch generation logic. The trigger was an AWS-side change to the `GetResource` response shape.

## Changes

### Bug fix: Preserve unspecified properties during update (`pkg/aws/operations/operations.go`)

Changed `GeneratePatch` to preserve all properties from the current state that the user didn't explicitly specify, not just read-only/create-only ones. This prevents generating `"remove"` operations for properties the user doesn't mention, matching the standard behavior of Terraform and CloudFormation where unmentioned properties are left unchanged.

### Credential improvement: IRSA role chaining (`pkg/ucp/aws/ucpcredentialprovider.go`)

The IRSA credential path now performs a two-step credential retrieval: `AssumeRoleWithWebIdentity` followed by `AssumeRole` on the same role. This produces standard IAM session credentials instead of web identity federation tokens, which are more broadly compatible with AWS services. While this wasn't the root cause of the test failure, it is a robustness improvement.

### Regional STS endpoint support

- `pkg/ucp/frontend/aws/routes.go`: Reads `AWS_REGION`/`AWS_DEFAULT_REGION` env vars for STS configuration
- `deploy/Chart/templates/ucp/deployment.yaml`: Added `AWS_REGION` env var to UCP container
- `deploy/Chart/values.yaml`: Added `global.aws.region` config value
- `.github/workflows/functional-test-cloud.yaml`: Passes `AWS_REGION` to Helm install

### Test unskip

- Removed the `t.Skip` that was added to unblock the 0.56.0 release

## IAM Role Change (manual, not in this PR)

The AWS IAM role trust policy (`radius-func-test-5lyb`) was updated to allow `sts:AssumeRole` from `cloudformation.amazonaws.com` and self-assumption, which is required for the role chaining approach:

```json
{
    "Effect": "Allow",
    "Principal": {
        "Service": "cloudformation.amazonaws.com"
    },
    "Action": "sts:AssumeRole"
}
```

This trust policy statement must remain in place for the functional tests to pass.
