/*
Copyright 2023 The Radius Authors.

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

package aws

import (
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_ConsoleURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "known type with a hash",
			id:   "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.RDS/DBInstance/db",
			want: "https://us-west-2.console.aws.amazon.com/rds/home?region=us-west-2#databases:",
		},
		{
			name: "known type without a hash",
			id:   "/planes/aws/aws/accounts/123456789012/regions/eu-central-1/providers/AWS.S3/Bucket/b",
			want: "https://eu-central-1.console.aws.amazon.com/s3/buckets?region=eu-central-1",
		},
		{
			name: "type matching is case-insensitive",
			id:   "/planes/aws/aws/accounts/123456789012/regions/us-east-1/providers/aws.sqs/queue/q",
			want: "https://us-east-1.console.aws.amazon.com/sqs/v3/home?region=us-east-1#/queues",
		},
		{
			name: "GovCloud region uses the GovCloud console",
			id:   "/planes/aws/aws-us-gov/accounts/123456789012/regions/us-gov-west-1/providers/AWS.SQS/Queue/q",
			want: "https://console.amazonaws-us-gov.com/sqs/v3/home?region=us-gov-west-1#/queues",
		},
		{
			name: "ElastiCache links to its engine-neutral dashboard",
			id:   "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/Terraform.AWS/aws_elasticache_cluster/memcached-1",
			want: "https://us-west-2.console.aws.amazon.com/elasticache/home?region=us-west-2",
		},
		{
			name: "planes root has no link",
			id:   "/planes",
		},
		{
			name: "AWS plane type with no name has no link",
			id:   "/planes/aws",
		},
		{
			name: "AWS plane with no resource has no link",
			id:   "/planes/aws/aws",
		},
		{
			name: "global console page in an unrecognized partition has no link",
			id:   "/planes/aws/other/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/b",
		},
		{
			name: "unknown type has no link",
			id:   "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.Unknown/Thing/t",
		},
		{
			name: "Terraform type with a region",
			id:   "/planes/aws/aws/accounts/123456789012/regions/us-east-2/providers/Terraform.AWS/aws_db_instance/db",
			want: "https://us-east-2.console.aws.amazon.com/rds/home?region=us-east-2#databases:",
		},
		{
			name: "Terraform type in a GovCloud region",
			id:   "/planes/aws/aws-us-gov/accounts/123456789012/regions/us-gov-east-1/providers/Terraform.AWS/aws_lb/my-lb",
			want: "https://console.amazonaws-us-gov.com/ec2/home?region=us-gov-east-1#LoadBalancers:",
		},
		{
			name: "Terraform type with a global console page in the global region",
			id:   "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/my-bucket",
			want: "https://console.aws.amazon.com/s3/buckets",
		},
		{
			name: "global console page in the GovCloud partition",
			id:   "/planes/aws/aws-us-gov/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/my-bucket",
			want: "https://console.amazonaws-us-gov.com/s3/buckets",
		},
		{
			name: "AWS type with a global console page in the global region",
			id:   "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.IAM/AccessKey/k",
			want: "https://console.aws.amazon.com/iam/home#/users",
		},
		{
			name: "global console page in the China partition has no link",
			id:   "/planes/aws/aws-cn/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/my-bucket",
		},
		{
			name: "regional console page in the global region has no link",
			id:   "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_sqs_queue/q",
		},
		{
			name: "unknown Terraform type has no link",
			id:   "/planes/aws/aws/accounts/123456789012/regions/us-east-2/providers/Terraform.AWS/aws_subnet/subnet-1",
		},
		{
			name: "missing region has no link",
			id:   "/planes/aws/aws/accounts/123456789012/providers/AWS.S3/Bucket/b",
		},
		{
			name: "invalid region has no link",
			id:   "/planes/aws/aws/accounts/123456789012/regions/us-west-2.evil.com/providers/AWS.S3/Bucket/b",
		},
		{
			name: "China region has no link",
			id:   "/planes/aws/aws-cn/accounts/123456789012/regions/cn-north-1/providers/AWS.S3/Bucket/b",
		},
		{
			name: "AWS scope has no link",
			id:   "/planes/aws/aws/accounts/123456789012/regions/us-west-2",
		},
		{
			name: "Azure resource has no link",
			id:   "/subscriptions/s/resourceGroups/g/providers/Microsoft.Storage/storageAccounts/a",
		},
		{
			name: "Radius resource has no link",
			id:   "/planes/radius/local/resourceGroups/g/providers/Applications.Core/containers/c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := resources.Parse(tt.id)
			require.NoError(t, err)
			require.Equal(t, tt.want, ConsoleURL(id))
		})
	}
}

func Test_consolePages(t *testing.T) {
	t.Parallel()

	for resourceType, page := range consolePages {
		require.Equal(t, strings.ToLower(resourceType), resourceType, "keys must be lower case")
		require.True(t, strings.HasPrefix(resourceType, "aws.") || strings.HasPrefix(resourceType, "terraform.aws/aws_"),
			"%s must be an AWS or Terraform AWS type", resourceType)
		require.True(t, strings.HasPrefix(page.path, "/"), "%s path must start with /", resourceType)
		require.True(t, page.hash == "" || strings.HasPrefix(page.hash, "#"), "%s hash must start with #", resourceType)
	}
}
