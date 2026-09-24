/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resource_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
)

func newS3BackendFixture(ctx context.Context, t *testing.T, name string) cloudBackendFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, cloudBackendTimeout)
	defer cancel()
	region := requiredCloudEnv(t, "AWS_REGION")
	account := requiredCloudEnv(t, "AWS_ACCOUNT_ID")
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	require.NoError(t, err)
	client := s3.NewFromConfig(config)
	require.Nil(t, client.Options().BaseEndpoint, "this test requires real S3, not an endpoint override")
	identity := sts.NewFromConfig(config)
	require.Nil(t, identity.Options().BaseEndpoint, "this test requires real STS, not an endpoint override")
	caller, err := identity.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	require.NoError(t, err)
	require.Equal(t, account, aws.ToString(caller.Account), "AWS_ACCOUNT_ID must match the runner credential")
	bucket := name + "-state"
	input := &s3.CreateBucketInput{Bucket: new(bucket)}
	if region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(region)}
	}
	_, err = client.CreateBucket(ctx, input)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), cloudBackendTimeout)
		defer cancel()
		// Only this newly allocated bucket is traversed. It has no versioning.
		pager := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: new(bucket), ExpectedBucketOwner: new(account)})
		for pager.HasMorePages() {
			page, err := pager.NextPage(cleanupCtx)
			if err != nil {
				t.Errorf("list test backend bucket for cleanup: %v", err)
				return
			}
			for _, object := range page.Contents {
				_, err := client.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{
					Bucket: new(bucket), Key: object.Key, ExpectedBucketOwner: new(account),
				})
				if err != nil {
					t.Errorf("delete test backend object: %v", err)
				}
			}
		}
		_, err := client.DeleteBucket(cleanupCtx, &s3.DeleteBucketInput{Bucket: new(bucket), ExpectedBucketOwner: new(account)})
		if err != nil {
			t.Errorf("delete test backend bucket: %v", err)
		}
	})
	t.Logf("Test-owned Terraform state bucket: %s", bucket)
	_, err = client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: new(bucket), ExpectedBucketOwner: new(account),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls: new(true), BlockPublicPolicy: new(true),
			IgnorePublicAcls: new(true), RestrictPublicBuckets: new(true),
		},
	})
	require.NoError(t, err)
	_, err = client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{
		Bucket: new(bucket), ExpectedBucketOwner: new(account),
		Tagging: &types.Tagging{TagSet: []types.Tag{{Key: new("radiustest"), Value: new("terraform-cloud-backend")}}},
	})
	require.NoError(t, err)

	return cloudBackendFixture{
		settings:     map[string]any{"type": "s3", "bucket": bucket, "region": region},
		prefix:       "radius", // Omit keyPrefix to exercise the default.
		providers:    map[string]any{"aws": map[string]any{"accountId": account, "region": region}},
		resourceType: "aws_s3_bucket",
		resourceID:   func(name string) string { return name },
		read: func(ctx context.Context, key string) ([]byte, error) {
			response, err := client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: new(bucket), Key: new(key), ExpectedBucketOwner: new(account),
			})
			if err != nil {
				return nil, err
			}
			return readCloudStateBody(response.Body)
		},
		keys: func(ctx context.Context) ([]string, error) {
			var keys []string
			pager := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: new(bucket), ExpectedBucketOwner: new(account)})
			for pager.HasMorePages() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, object := range page.Contents {
					keys = append(keys, aws.ToString(object.Key))
				}
			}
			return keys, nil
		},
		verifyObject: func(ctx context.Context, name, revision string) (bool, error) {
			response, err := client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: new(name), ExpectedBucketOwner: new(account)})
			if s3ErrorCode(err, "NoSuchBucket") {
				return revision == "", nil
			}
			if s3ErrorCode(err, "NoSuchTagSet") {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			for _, tag := range response.TagSet {
				if aws.ToString(tag.Key) == "revision" {
					return revision != "" && aws.ToString(tag.Value) == revision, nil
				}
			}
			return false, nil
		},
		cleanupObject: func(ctx context.Context, name string) error {
			_, err := client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: new(name), ExpectedBucketOwner: new(account)})
			if s3ErrorCode(err, "NoSuchBucket") {
				return nil
			}
			return err
		},
	}
}

func s3ErrorCode(err error, code string) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && apiError.ErrorCode() == code
}
