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

package s3tables

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"
	"github.com/aws/aws-sdk-go-v2/service/s3tables/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const apiReference = "https://docs.aws.amazon.com/AmazonS3/latest/API/"

func getReplication(ctx context.Context, store *awsfake.CloudControl, arn string) (*s3tables.GetTableReplicationOutput, error) {
	return newFakeAPI(store).GetTableReplication(ctx, &s3tables.GetTableReplicationInput{TableArn: aws.String(arn)})
}

func putReplication(ctx context.Context, store *awsfake.CloudControl, arn string, configuration *types.TableReplicationConfiguration, version *string) error {
	_, err := newFakeAPI(store).PutTableReplication(ctx, &s3tables.PutTableReplicationInput{TableArn: aws.String(arn), Configuration: configuration, VersionToken: version})
	return err
}

func deleteReplication(ctx context.Context, store *awsfake.CloudControl, arn string, version *string) error {
	_, err := newFakeAPI(store).DeleteTableReplication(ctx, &s3tables.DeleteTableReplicationInput{TableArn: aws.String(arn), VersionToken: version})
	return err
}

// sdkConfiguration returns a configuration with role and a rule with destinations.
func sdkConfiguration(role string, destinations ...string) *types.TableReplicationConfiguration {
	rule := types.TableReplicationRule{}
	for _, d := range destinations {
		rule.Destinations = append(rule.Destinations, types.ReplicationDestination{DestinationTableBucketARN: aws.String(d)})
	}
	return &types.TableReplicationConfiguration{Role: aws.String(role), Rules: []types.TableReplicationRule{rule}}
}

// validPut is a configuration S3 Tables takes.
func validPut() *types.TableReplicationConfiguration {
	return sdkConfiguration(role, destination1)
}

// putThenGet sets the test table's configuration and returns its version token.
func putThenGet(ctx context.Context, store *awsfake.CloudControl) (*string, error) {
	if err := putReplication(ctx, store, tableArn, validPut(), nil); err != nil {
		return nil, err
	}
	out, err := getReplication(ctx, store, tableArn)
	if err != nil {
		return nil, err
	}
	return out.VersionToken, nil
}

// tableArnRules are the rules every call has for TableArn.
func tableArnRules(call func(ctx context.Context, store *awsfake.CloudControl, arn string) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "TableArn matches arn:aws[-a-z0-9]*:[a-z0-9]+:[-a-z0-9]*:[0-9]{12}:bucket/[a-z0-9_-]{3,63}/table/[a-zA-Z0-9-_]{1,255}",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "arn:aws:s3tables:us-west-2:123456789012:bucket/Sales/table/orders")
			},
			Code: badRequestCode,
		},
		{
			Rule: "the table exists",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, tableBucket+"/table/"+missingPrefix+"orders")
			},
			Code: notFoundCode,
		},
	}
}

// tableReplicationAPI records the S3 Tables calls TableReplication makes.
var tableReplicationAPI = map[string]nativetest.Call{
	"GetTableReplication": {
		Doc: apiReference + "API_s3Buckets_GetTableReplication.html",
		Rules: append(tableArnRules(func(ctx context.Context, store *awsfake.CloudControl, arn string) error {
			if err := putReplication(ctx, store, tableArn, validPut(), nil); err != nil {
				return err
			}
			_, err := getReplication(ctx, store, arn)
			return err
		}), nativetest.Rule{
			Rule: "the call returns the configuration and a version token for its current state",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				first, err := putThenGet(ctx, store)
				if err != nil {
					return err
				}
				second, err := putThenGet(ctx, store)
				if err != nil {
					return err
				}
				if aws.ToString(first) == aws.ToString(second) {
					return fmt.Errorf("the version token didn't change after a put")
				}
				return nil
			},
		}),
		Unclear: "The page doesn't say what the call returns for a table with no table-level configuration, or one " +
			"that only its table bucket's configuration replicates; the fake returns NotFoundException, and the handler " +
			"treats it as no configuration. TableArn's pattern has no anchors; the fake applies it to the whole ARN.",
	},
	"PutTableReplication": {
		Doc: apiReference + "API_s3Buckets_PutTableReplication.html",
		Rules: append(tableArnRules(func(ctx context.Context, store *awsfake.CloudControl, arn string) error {
			return putReplication(ctx, store, arn, validPut(), nil)
		}),
			nativetest.Rule{
				Rule: "configuration is required",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putReplication(ctx, store, tableArn, nil, nil)
				},
				Code: badRequestCode,
			},
			nativetest.Rule{
				Rule: "role is 20 to 2048 characters",
				Doc:  apiReference + "API_s3Buckets_TableReplicationConfiguration.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putReplication(ctx, store, tableArn, sdkConfiguration("arn:a:iam::1:role/r", destination1), nil)
				},
				Code: badRequestCode,
			},
			nativetest.Rule{
				Rule: "role matches arn:.+:iam::[0-9]{12}:role/.+",
				Doc:  apiReference + "API_s3Buckets_TableReplicationConfiguration.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putReplication(ctx, store, tableArn, sdkConfiguration("arn:aws:iam::123456789012:user/replicator", destination1), nil)
				},
				Code: badRequestCode,
			},
			nativetest.Rule{
				Rule: "rules has exactly one item",
				Doc:  apiReference + "API_s3Buckets_TableReplicationConfiguration.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					c := validPut()
					c.Rules = append(c.Rules, c.Rules[0])
					return putReplication(ctx, store, tableArn, c, nil)
				},
				Code: badRequestCode,
			},
			nativetest.Rule{
				Rule: "a rule has at least one destination",
				Doc:  apiReference + "API_s3Buckets_TableReplicationRule.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putReplication(ctx, store, tableArn, sdkConfiguration(role), nil)
				},
				Code: badRequestCode,
			},
			nativetest.Rule{
				Rule: "a rule has at most five destinations",
				Doc:  apiReference + "API_s3Buckets_TableReplicationRule.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putReplication(ctx, store, tableArn, sdkConfiguration(role, destination1, destination2, destination1, destination2, destination1, destination2), nil)
				},
				Code: badRequestCode,
			},
			nativetest.Rule{
				Rule: "destinationTableBucketARN matches arn:aws[-a-z0-9]*:[a-z0-9]+:[-a-z0-9]*:[0-9]{12}:bucket/[a-z0-9_-]{3,63}",
				Doc:  apiReference + "API_s3Buckets_ReplicationDestination.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putReplication(ctx, store, tableArn, sdkConfiguration(role, tableArn), nil)
				},
				Code: badRequestCode,
			},
			nativetest.Rule{
				Rule: "a version token from an earlier GetTableReplication call is rejected once the configuration changed",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					stale, err := putThenGet(ctx, store)
					if err != nil {
						return err
					}
					if err := putReplication(ctx, store, tableArn, validPut(), stale); err != nil {
						return err
					}
					return putReplication(ctx, store, tableArn, validPut(), stale)
				},
				Code: conflictCode,
			},
			nativetest.Rule{
				Rule: "the call creates or replaces the whole configuration",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := putReplication(ctx, store, tableArn, sdkConfiguration(role, destination1, destination2), nil); err != nil {
						return err
					}
					if err := putReplication(ctx, store, tableArn, sdkConfiguration(role, destination2), nil); err != nil {
						return err
					}
					out, err := getReplication(ctx, store, tableArn)
					if err != nil {
						return err
					}
					if got := out.Configuration.Rules[0].Destinations; len(got) != 1 || aws.ToString(got[0].DestinationTableBucketARN) != destination2 {
						return fmt.Errorf("the destinations are %+v", got)
					}
					return nil
				},
			},
		),
		Unclear: "The page lists BadRequestException for an invalid request without naming each limit; the fake returns " +
			"it for every limit the data types give. It says the version token makes sure the call updates the expected " +
			"version, without naming the error for an older one; the fake returns ConflictException, which the page " +
			"lists for a conflict with a previous write. It doesn't say what AWS does without a version token when a " +
			"configuration exists; the fake replaces it, and the handler sends one whenever it updates. The page names " +
			"s3tables:GetTableMaintenanceConfig and s3tables:PutTableMaintenanceConfig, which the schema lists by their " +
			"IAM action names, GetTableMaintenanceConfiguration and PutTableMaintenanceConfiguration. The patterns have " +
			"no anchors; the fake applies them to the whole value.",
	},
	"DeleteTableReplication": {
		Doc: apiReference + "API_s3Buckets_DeleteTableReplication.html",
		Rules: append(tableArnRules(func(ctx context.Context, store *awsfake.CloudControl, arn string) error {
			version, err := putThenGet(ctx, store)
			if err != nil {
				return err
			}
			return deleteReplication(ctx, store, arn, version)
		}),
			nativetest.Rule{
				Rule: "versionToken is required",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if _, err := putThenGet(ctx, store); err != nil {
						return err
					}
					return deleteReplication(ctx, store, tableArn, nil)
				},
				Code: badRequestCode,
			},
			nativetest.Rule{
				Rule: "a version token from an earlier GetTableReplication call is rejected once the configuration changed",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					stale, err := putThenGet(ctx, store)
					if err != nil {
						return err
					}
					if err := putReplication(ctx, store, tableArn, validPut(), stale); err != nil {
						return err
					}
					return deleteReplication(ctx, store, tableArn, stale)
				},
				Code: conflictCode,
			},
			nativetest.Rule{
				Rule: "the table has a replication configuration",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return deleteReplication(ctx, store, tableArn, aws.String("token"))
				},
				Code: notFoundCode,
			},
			nativetest.Rule{
				Rule: "after the call, the table has no replication configuration",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					version, err := putThenGet(ctx, store)
					if err != nil {
						return err
					}
					if err := deleteReplication(ctx, store, tableArn, version); err != nil {
						return err
					}
					if _, err := getReplication(ctx, store, tableArn); err == nil || !strings.Contains(err.Error(), notFoundCode) {
						return errors.Join(fmt.Errorf("the configuration is still there"), err)
					}
					return nil
				},
			},
		),
		Unclear: "The page doesn't name the error for a table with no configuration, or for a version token of an " +
			"older configuration; the fake returns NotFoundException and ConflictException. Replicas already made stay " +
			"in their table buckets, so the fake keeps nothing about them.",
	},
}
