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
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	rdsReference    = "https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/"
	rdsCommonErrors = rdsReference + "CommonErrors.html"
	rdsGuide        = "https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/"
	rdsStatusGuide  = rdsGuide + "accessing-monitoring.html"
	rdsStopGuide    = rdsGuide + "USER_StopInstance.html"
	rdsStartGuide   = rdsGuide + "USER_StartInstance.html"

	// testDBInstance is the DB instance the rules and tests use.
	testDBInstance = "app-db"

	// dbRegion is the Region the tests send calls to.
	dbRegion = "us-west-2"
)

var inDBRegion = dbInstanceRegion(dbRegion)

// dbInstanceCall returns a Break that puts testDBInstance in status, with the extra fields set, and makes the call
// against a new fake of the RDS API backed by store.
func dbInstanceCall(status string, extra map[string]any, call func(ctx context.Context, f *fakeDBInstances) error) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		putDBInstance(store, testDBInstance, status, extra)
		return call(ctx, newFakeDBInstances(store))
	}
}

// dbInstanceIs returns an error when f doesn't read the DB instance id with status.
func dbInstanceIs(ctx context.Context, f *fakeDBInstances, id string, status string) error {
	out, err := f.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(id)}, inDBRegion)
	if err != nil {
		return err
	}
	if got := aws.ToString(out.DBInstances[0].DBInstanceStatus); got != status {
		return fmt.Errorf("the DB instance is %s, not %s", got, status)
	}
	return nil
}

func startDBInstance(id string) func(ctx context.Context, f *fakeDBInstances) error {
	return func(ctx context.Context, f *fakeDBInstances) error {
		_, err := f.StartDBInstance(ctx, &rds.StartDBInstanceInput{DBInstanceIdentifier: aws.String(id)}, inDBRegion)
		return err
	}
}

func stopDBInstance(id string) func(ctx context.Context, f *fakeDBInstances) error {
	return func(ctx context.Context, f *fakeDBInstances) error {
		_, err := f.StopDBInstance(ctx, &rds.StopDBInstanceInput{DBInstanceIdentifier: aws.String(id)}, inDBRegion)
		return err
	}
}

// dbInstanceIDRules are the rules for the DBInstanceIdentifier of a start or stop, which call makes with id.
func dbInstanceIDRules(status string, call func(id string) func(ctx context.Context, f *fakeDBInstances) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "DBInstanceIdentifier is required",
			Doc:   rdsCommonErrors,
			Break: dbInstanceCall(status, nil, call("")),
			Code:  "MissingParameter",
		},
		{
			Rule:  "DBInstanceIdentifier names a DB instance RDS has",
			Break: dbInstanceCall(status, nil, call("other-db")),
			Code:  "DBInstanceNotFound",
		},
	}
}

var dbInstanceStateAPI = map[string]nativetest.Call{
	"DescribeDBInstances": {
		Doc: rdsReference + "API_DescribeDBInstances.html",
		Rules: []nativetest.Rule{
			{
				Rule: "DBInstanceIdentifier, when set, must match an existing DB instance",
				Break: dbInstanceCall("available", nil, func(ctx context.Context, f *fakeDBInstances) error {
					return dbInstanceIs(ctx, f, "other-db", "available")
				}),
				Code: "DBInstanceNotFound",
			},
			{
				Rule: "DBInstanceIdentifier isn't case-sensitive",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					putDBInstance(store, testDBInstance, "available", nil)
					return dbInstanceIs(ctx, newFakeDBInstances(store), "App-DB", "available")
				},
			},
		},
		Unclear: "The page doesn't say that it also returns the DB instances of Amazon DocumentDB and Amazon Neptune, which " +
			"share the RDS API, or how soon a read shows the status a start or stop moves a DB instance to. The fake returns " +
			"them with their engine, and shows the new status on the next read.",
	},
	"StartDBInstance": {
		Doc: rdsReference + "API_StartDBInstance.html",
		Rules: append(dbInstanceIDRules("stopped", startDBInstance),
			nativetest.Rule{
				Rule:  "only a DB instance that was stopped can be started",
				Break: dbInstanceCall("available", nil, startDBInstance(testDBInstance)),
				Code:  "InvalidDBInstanceState",
			},
			nativetest.Rule{
				Rule: "a started DB instance is starting, and then available",
				Doc:  rdsStartGuide,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					putDBInstance(store, testDBInstance, "stopped", nil)
					f := newFakeDBInstances(store)
					return errors.Join(startDBInstance(testDBInstance)(ctx, f), dbInstanceIs(ctx, f, testDBInstance, "starting"), dbInstanceIs(ctx, f, testDBInstance, "available"))
				},
			},
			nativetest.Rule{
				Rule: "a DB instance whose KMS key RDS can't reach for now (inaccessible-encryption-credentials-recoverable) can be started to recover it",
				Doc:  rdsStatusGuide,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					putDBInstance(store, testDBInstance, recoverableStatus, nil)
					f := newFakeDBInstances(store)
					return errors.Join(startDBInstance(testDBInstance)(ctx, f), dbInstanceIs(ctx, f, testDBInstance, "starting"))
				},
			},
		),
		Unclear: "The page says the call doesn't apply to Aurora DB instances, and names no error for one. The fake returns " +
			"InvalidDBInstanceState for a DB instance of any cluster.",
	},
	"StopDBInstance": {
		Doc: rdsReference + "API_StopDBInstance.html",
		Rules: append(dbInstanceIDRules("available", stopDBInstance),
			nativetest.Rule{
				Rule: "a stopped DB instance is stopping, and then stopped",
				Doc:  rdsStopGuide,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					putDBInstance(store, testDBInstance, "available", nil)
					f := newFakeDBInstances(store)
					return errors.Join(stopDBInstance(testDBInstance)(ctx, f), dbInstanceIs(ctx, f, testDBInstance, "stopping"), dbInstanceIs(ctx, f, testDBInstance, "stopped"))
				},
			},
		),
		Unclear: "The page names InvalidDBInstanceState for a DB instance that isn't in a valid state, but not which states are " +
			"valid. The fake stops only an available DB instance. The page says the call doesn't apply to Aurora DB instances, " +
			"and the User Guide (" + rdsStopGuide + ") says a DB instance that has a read replica, or is one, can't be " +
			"stopped. Neither names an error, and the fake returns InvalidDBInstanceState for both.",
	},
}
