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

package rds

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

// API is the part of the RDS client the handlers use.
type API interface {
	DescribeDBClusters(ctx context.Context, params *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
	StartActivityStream(ctx context.Context, params *rds.StartActivityStreamInput, optFns ...func(*rds.Options)) (*rds.StartActivityStreamOutput, error)
	StopActivityStream(ctx context.Context, params *rds.StopActivityStreamInput, optFns ...func(*rds.Options)) (*rds.StopActivityStreamOutput, error)
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	CreateTenantDatabase(ctx context.Context, params *rds.CreateTenantDatabaseInput, optFns ...func(*rds.Options)) (*rds.CreateTenantDatabaseOutput, error)
	DescribeTenantDatabases(ctx context.Context, params *rds.DescribeTenantDatabasesInput, optFns ...func(*rds.Options)) (*rds.DescribeTenantDatabasesOutput, error)
	ModifyTenantDatabase(ctx context.Context, params *rds.ModifyTenantDatabaseInput, optFns ...func(*rds.Options)) (*rds.ModifyTenantDatabaseOutput, error)
	DeleteTenantDatabase(ctx context.Context, params *rds.DeleteTenantDatabaseInput, optFns ...func(*rds.Options)) (*rds.DeleteTenantDatabaseOutput, error)
	AddTagsToResource(ctx context.Context, params *rds.AddTagsToResourceInput, optFns ...func(*rds.Options)) (*rds.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(ctx context.Context, params *rds.RemoveTagsFromResourceInput, optFns ...func(*rds.Options)) (*rds.RemoveTagsFromResourceOutput, error)
}

var inRegion = awsapi.InRegion[rds.Options]

// retryThrottlesOnly makes the SDK send a call again only when AWS throttled it, which AWS refuses before acting on. A
// change that takes no idempotency token, such as CreateTenantDatabase, can't be sent again after any other error: when
// the first attempt reached AWS, the retry gets an error such as TenantDatabaseAlreadyExists, which UCP would read as
// rejected, and it would drop the record of a change AWS has. Sent once, an error AWS answers with a 5xx leaves the
// outcome unknown, and the next deploy reads what AWS has.
func retryThrottlesOnly(o *rds.Options) {
	o.Retryer = throttleRetryer{o.Retryer}
}

// throttleRetryer is retryer, but retries only throttled calls.
type throttleRetryer struct {
	aws.Retryer
}

// IsErrorRetryable reports whether err is a throttle.
func (r throttleRetryer) IsErrorRetryable(err error) bool {
	return retry.IsErrorThrottles(retry.DefaultThrottles).IsErrorThrottle(err) == aws.TrueTernary
}
