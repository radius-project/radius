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

package docdb

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/aws/aws-sdk-go-v2/service/docdb/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference        = "https://docs.aws.amazon.com/documentdb/latest/APIReference/"
	commonErrors        = apiReference + "CommonErrors.html"
	tagDoc              = apiReference + "API_Tag.html"
	devGuide            = "https://docs.aws.amazon.com/documentdb/latest/devguide/"
	modifyClusterGuide  = devGuide + "db-cluster-modify.html"
	deleteClusterGuide  = devGuide + "db-cluster-delete.html"
	modifyInstanceGuide = devGuide + "db-instance-modify.html"
	deleteInstanceGuide = devGuide + "db-instance-delete.html"
	certificateGuide    = devGuide + "ca_cert_rotation.html"

	// codesUnclear is what every call's page leaves unclear: the error code of a value limit or of parameters that
	// can't be combined.
	codesUnclear = "The page doesn't name the error for a value out of its limits or parameters that can't be " +
		"combined; the fake returns InvalidParameterValue and InvalidParameterCombination, from the Common Errors page."

	clusterArn  = arnPrefix + "cluster:app"
	instanceArn = arnPrefix + "db:app-1"
)

// apiCall is a call against a fake DocumentDB API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns call as a rule's Break or Check, against a fake DocumentDB API backed by store.
func against(call apiCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		return call(ctx, newFakeAPI(store))
	}
}

// createCluster creates the cluster app, with the changes edit makes to the input.
func createCluster(ctx context.Context, f *fakeAPI, edit func(*docdb.CreateDBClusterInput)) error {
	in := &docdb.CreateDBClusterInput{
		DBClusterIdentifier: aws.String("app"),
		Engine:              aws.String(Engine),
		MasterUsername:      aws.String("radius"),
		MasterUserPassword:  aws.String("pass-word-1"),
	}
	if edit != nil {
		edit(in)
	}
	_, err := f.CreateDBCluster(ctx, in)
	return err
}

func describeCluster(ctx context.Context, f *fakeAPI, id string) (types.DBCluster, error) {
	out, err := f.DescribeDBClusters(ctx, &docdb.DescribeDBClustersInput{DBClusterIdentifier: aws.String(id)})
	if err != nil {
		return types.DBCluster{}, err
	}
	if len(out.DBClusters) == 0 {
		return types.DBCluster{}, fmt.Errorf("no cluster %s", id)
	}
	return out.DBClusters[0], nil
}

// readyCluster creates the cluster app and reads it, after which it is available.
func readyCluster(ctx context.Context, f *fakeAPI, edit func(*docdb.CreateDBClusterInput)) error {
	if err := createCluster(ctx, f, edit); err != nil {
		return err
	}
	_, err := describeCluster(ctx, f, "app")
	return err
}

// onReadyCluster returns a call that makes the cluster app available, then calls call.
func onReadyCluster(edit func(*docdb.CreateDBClusterInput), call apiCall) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		if err := readyCluster(ctx, f, edit); err != nil {
			return err
		}
		return call(ctx, f)
	}
}

func modifyCluster(in *docdb.ModifyDBClusterInput) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in.DBClusterIdentifier = aws.String("app")
		_, err := f.ModifyDBCluster(ctx, in)
		return err
	}
}

func deleteCluster(in *docdb.DeleteDBClusterInput) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in.DBClusterIdentifier = aws.String("app")
		_, err := f.DeleteDBCluster(ctx, in)
		return err
	}
}

func restoreFromSnapshot(edit func(*docdb.RestoreDBClusterFromSnapshotInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		f.snapshots = append(f.snapshots, "snap-1")
		in := &docdb.RestoreDBClusterFromSnapshotInput{DBClusterIdentifier: aws.String("copy"), Engine: aws.String(Engine), SnapshotIdentifier: aws.String("snap-1")}
		edit(in)
		_, err := f.RestoreDBClusterFromSnapshot(ctx, in)
		return err
	}
}

func restoreToPointInTime(edit func(*docdb.RestoreDBClusterToPointInTimeInput)) apiCall {
	return onReadyCluster(nil, func(ctx context.Context, f *fakeAPI) error {
		in := &docdb.RestoreDBClusterToPointInTimeInput{DBClusterIdentifier: aws.String("copy"), SourceDBClusterIdentifier: aws.String("app"), UseLatestRestorableTime: aws.Bool(true)}
		edit(in)
		_, err := f.RestoreDBClusterToPointInTime(ctx, in)
		return err
	})
}

// createInstance creates the instance app-1 in the cluster app, with the changes edit makes to the input.
func createInstance(ctx context.Context, f *fakeAPI, edit func(*docdb.CreateDBInstanceInput)) error {
	in := &docdb.CreateDBInstanceInput{
		DBInstanceIdentifier: aws.String("app-1"),
		DBClusterIdentifier:  aws.String("app"),
		DBInstanceClass:      aws.String("db.t3.medium"),
		Engine:               aws.String(Engine),
	}
	if edit != nil {
		edit(in)
	}
	_, err := f.CreateDBInstance(ctx, in)
	return err
}

func describeInstance(ctx context.Context, f *fakeAPI, id string) (types.DBInstance, error) {
	out, err := f.DescribeDBInstances(ctx, &docdb.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(id)})
	if err != nil {
		return types.DBInstance{}, err
	}
	if len(out.DBInstances) == 0 {
		return types.DBInstance{}, fmt.Errorf("no instance %s", id)
	}
	return out.DBInstances[0], nil
}

// newInstance returns a call that creates the instance app-1 in an available cluster, with the changes edit makes
// to its input.
func newInstance(edit func(*docdb.CreateDBInstanceInput)) apiCall {
	return onReadyCluster(nil, func(ctx context.Context, f *fakeAPI) error {
		return createInstance(ctx, f, edit)
	})
}

// onReadyInstance returns a call that makes the instance app-1 available, then calls call.
func onReadyInstance(edit func(*docdb.CreateDBInstanceInput), call apiCall) apiCall {
	return onReadyCluster(nil, func(ctx context.Context, f *fakeAPI) error {
		if err := createInstance(ctx, f, edit); err != nil {
			return err
		}
		if _, err := describeInstance(ctx, f, "app-1"); err != nil {
			return err
		}
		return call(ctx, f)
	})
}

func modifyInstance(in *docdb.ModifyDBInstanceInput) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in.DBInstanceIdentifier = aws.String("app-1")
		_, err := f.ModifyDBInstance(ctx, in)
		return err
	}
}

func tag(key string, value string) types.Tag {
	return types.Tag{Key: aws.String(key), Value: aws.String(value)}
}

// identifierRules returns DocumentDB's rules for the name of a cluster or instance a call creates. create makes the
// call with the name.
func identifierRules(create func(id string) apiCall) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "the identifier is 1 to 63 letters, numbers, or hyphens, and starts with a letter",
			Doc:   commonErrors,
			Break: against(create("a" + strings.Repeat("b", 63))),
			Code:  "InvalidParameterValue",
		},
		{
			Rule:  "the identifier doesn't end with a hyphen or have two hyphens in a row",
			Doc:   commonErrors,
			Break: against(create("app--1")),
			Code:  "InvalidParameterValue",
		},
	}
}

// tagRules returns the rules of the Tag data type, for a call that takes tags. call makes the call with tags.
func tagRules(call func(tags []types.Tag) apiCall) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "a tag key is 1 to 128 characters and a value at most 256, of letters, digits, white space, and _.:/=+-",
			Doc:   tagDoc,
			Break: against(call([]types.Tag{tag("team!", "a")})),
			Code:  "InvalidParameterValue",
		},
		{
			Rule:  "a tag key or value doesn't start with aws: or rds:",
			Doc:   tagDoc,
			Break: against(call([]types.Tag{tag("aws:team", "a")})),
			Code:  "InvalidParameterValue",
		},
	}
}

// tagsUnclear is what the Tag data type's page leaves unclear.
const tagsUnclear = " The Tag page says a value is optional and also 1 to 256 characters; the fake accepts an empty value."

// existsRule returns the rule that the resource arn names exists, for a tag call, which fails with code. call makes
// the call for arn.
func existsRule(code string, arn string, call func(arn string) apiCall) nativetest.Rule {
	return nativetest.Rule{
		Rule:  "the resource exists",
		Break: against(call(arn)),
		Code:  code,
	}
}

func listTags(arn string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.ListTagsForResource(ctx, &docdb.ListTagsForResourceInput{ResourceName: aws.String(arn)})
		return err
	}
}

func addTags(arn string, tags ...types.Tag) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.AddTagsToResource(ctx, &docdb.AddTagsToResourceInput{ResourceName: aws.String(arn), Tags: tags})
		return err
	}
}

func removeTags(arn string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.RemoveTagsFromResource(ctx, &docdb.RemoveTagsFromResourceInput{ResourceName: aws.String(arn), TagKeys: []string{"team"}})
		return err
	}
}

// tagAPI returns the records of the tag calls for the resource arn names, which ready makes available, and code, the
// error DocumentDB returns for a resource it doesn't have.
func tagAPI(arn string, code string, ready func(call apiCall) apiCall) map[string]nativetest.Call {
	return map[string]nativetest.Call{
		"AddTagsToResource": {
			Doc: apiReference + "API_AddTagsToResource.html",
			Rules: append(tagRules(func(tags []types.Tag) apiCall { return ready(addTags(arn, tags...)) }),
				existsRule(code, arn, func(arn string) apiCall { return addTags(arn, tag("team", "a")) })),
			Unclear: codesUnclear + tagsUnclear + " CreateDBInstance's page limits an instance to 10 tags, but this page " +
				"names no limit, so the fake applies none here.",
		},
		"ListTagsForResource": {
			Doc:   apiReference + "API_ListTagsForResource.html",
			Rules: []nativetest.Rule{existsRule(code, arn, listTags)},
			Unclear: "The page says Filters isn't supported, without saying what AWS does when a call sets it. The " +
				"handler doesn't set it, and the fake ignores it.",
		},
		"RemoveTagsFromResource": {
			Doc:   apiReference + "API_RemoveTagsFromResource.html",
			Rules: []nativetest.Rule{existsRule(code, arn, removeTags)},
			Unclear: "The page doesn't say what AWS does with a key the resource doesn't have; the fake ignores it, " +
				"as the handler's retries expect.",
		},
	}
}

// pagingRules returns the rules of a describe call's MaxRecords and pages. put stores n resources, and describe
// makes the call with marker and maxRecords and returns how many resources the page has and its marker.
func pagingRules(put func(store *awsfake.CloudControl, n int), describe func(ctx context.Context, f *fakeAPI, marker *string, maxRecords *int32) (int, *string, error)) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "MaxRecords is 20 to 100",
			Doc:  commonErrors,
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, _, err := describe(ctx, newFakeAPI(store), nil, aws.Int32(10))
				return err
			},
			Code: "InvalidParameterValue",
		},
		{
			Rule: "a call without MaxRecords returns up to 100 records, and a marker for the rest",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				put(store, 101)
				f := newFakeAPI(store)
				n, marker, err := describe(ctx, f, nil, nil)
				if err != nil {
					return err
				}
				if n != 100 || marker == nil {
					return fmt.Errorf("the first page has %d records and marker %v", n, marker)
				}
				if n, marker, err = describe(ctx, f, marker, nil); err != nil || n != 1 || marker != nil {
					return fmt.Errorf("the second page has %d records and marker %v: %v", n, marker, err)
				}
				return nil
			},
		},
	}
}

// putMany stores n resources of the type, of the docdb engine.
func putMany(typeName string, property string) func(store *awsfake.CloudControl, n int) {
	return func(store *awsfake.CloudControl, n int) {
		for i := range n {
			id := "app-" + strconv.Itoa(i)
			store.Put(typeName, id, map[string]any{property: id, "Engine": Engine})
		}
	}
}

// clusterAPI records the DocumentDB calls Cluster makes.
var clusterAPI = merge(tagAPI(clusterArn, "DBClusterNotFoundFault", func(call apiCall) apiCall { return onReadyCluster(nil, call) }), map[string]nativetest.Call{
	"CreateDBCluster": {
		Doc: apiReference + "API_CreateDBCluster.html",
		Rules: append(append(identifierRules(func(id string) apiCall {
			return func(ctx context.Context, f *fakeAPI) error {
				return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.DBClusterIdentifier = aws.String(id) })
			}
		}), tagRules(func(tags []types.Tag) apiCall {
			return func(ctx context.Context, f *fakeAPI) error {
				return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.Tags = tags })
			}
		})...),
			nativetest.Rule{
				Rule: "the identifier is stored in lowercase",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.DBClusterIdentifier = aws.String("App") }); err != nil {
						return err
					}
					c, err := describeCluster(ctx, f, "app")
					if err != nil || aws.ToString(c.DBClusterIdentifier) != "app" {
						return fmt.Errorf("the cluster is %v: %v", aws.ToString(c.DBClusterIdentifier), err)
					}
					return nil
				}),
			},
			nativetest.Rule{
				Rule: "a cluster with the identifier doesn't exist",
				Break: against(onReadyCluster(nil, func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, nil)
				})),
				Code: "DBClusterAlreadyExistsFault",
			},
			nativetest.Rule{
				Rule: "Engine is docdb",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.Engine = aws.String("aurora-postgresql") })
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "BackupRetentionPeriod is 1 to 35",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.BackupRetentionPeriod = aws.Int32(36) })
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "without BackupRetentionPeriod, backups are kept 1 day; without CopyTagsToSnapshot, tags aren't copied; " +
					"without EngineVersion, the cluster runs the newest 8.0 minor version; without the backup and maintenance " +
					"windows, DocumentDB picks 30-minute windows",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createCluster(ctx, f, nil); err != nil {
						return err
					}
					c, err := describeCluster(ctx, f, "app")
					if err != nil {
						return err
					}
					backup, okB := backupWindow(aws.ToString(c.PreferredBackupWindow))
					maintenance, okM := maintenanceWindow(aws.ToString(c.PreferredMaintenanceWindow))
					if aws.ToInt32(c.BackupRetentionPeriod) != 1 || c.CopyTagsToSnapshot == nil || *c.CopyTagsToSnapshot ||
						!strings.HasPrefix(aws.ToString(c.EngineVersion), "8.0.") || !okB || !okM || backup.overlaps(maintenance) {
						return fmt.Errorf("the cluster has retention %d, copy tags %v, version %s, windows %s and %s",
							aws.ToInt32(c.BackupRetentionPeriod), c.CopyTagsToSnapshot, aws.ToString(c.EngineVersion),
							aws.ToString(c.PreferredBackupWindow), aws.ToString(c.PreferredMaintenanceWindow))
					}
					return nil
				}),
			},
			nativetest.Rule{
				Rule: "MasterUsername is 1 to 63 letters or numbers, and starts with a letter",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.MasterUsername = aws.String("1admin") })
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "MasterUserPassword is 8 to 100 printable ASCII characters other than /, \", and @",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.MasterUserPassword = aws.String("p@ssword-1") })
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "ManageMasterUserPassword can't be true with MasterUserPassword",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.ManageMasterUserPassword = aws.Bool(true) })
				}),
				Code: "InvalidParameterCombination",
			},
			nativetest.Rule{
				Rule: "MasterUserSecretKmsKeyId is valid only when ManageMasterUserPassword is true",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.MasterUserSecretKmsKeyId = aws.String("alias/app") })
				}),
				Code: "InvalidParameterCombination",
			},
			nativetest.Rule{
				Rule: "NetworkType is IPV4 or DUAL",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.NetworkType = aws.String("IPV6") })
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "StorageType is standard or iopt1",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.StorageType = aws.String("gp3") })
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "PreferredBackupWindow is hh24:mi-hh24:mi and at least 30 minutes",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.PreferredBackupWindow = aws.String("01:00-01:15") })
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "PreferredMaintenanceWindow is ddd:hh24:mi-ddd:hh24:mi and at least 30 minutes",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) {
						in.PreferredMaintenanceWindow = aws.String("sun:05:00-sun:05:10")
					})
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "PreferredBackupWindow doesn't conflict with PreferredMaintenanceWindow",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) {
						in.PreferredBackupWindow = aws.String("05:00-05:30")
						in.PreferredMaintenanceWindow = aws.String("sun:05:15-sun:05:45")
					})
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "DBSubnetGroupName isn't default",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.DBSubnetGroupName = aws.String("default") })
				}),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "GlobalClusterIdentifier is 1 to 255 characters of [A-Za-z][0-9A-Za-z-:._]*",
				Doc:  commonErrors,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					return createCluster(ctx, f, func(in *docdb.CreateDBClusterInput) { in.GlobalClusterIdentifier = aws.String("1global") })
				}),
				Code: "InvalidParameterValue",
			},
		),
		Unclear: codesUnclear + tagsUnclear + " This page gives no range for Port, while ModifyDBCluster's gives 1150 to " +
			"65535; the fake accepts any port here. It doesn't list MasterUsername's reserved words, and the fake " +
			"rejects none. It doesn't say which 8.0 minor version is newest; the fake reports 8.0.0. It says 5.0 " +
			"resolves to the 5.0 LTS minor version; the fake keeps the version the call sets, and the suite's " +
			"OtherForm case reads it back as 5.0.0. It says StorageType defaults to standard but isn't returned " +
			"then, while recorded responses return it; the fake reports what the call set. The fake doesn't model " +
			"subnet groups, parameter groups, KMS keys, or global clusters.",
	},
	"DeleteDBCluster": {
		Doc: apiReference + "API_DeleteDBCluster.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the cluster exists",
				Break: against(deleteCluster(&docdb.DeleteDBClusterInput{SkipFinalSnapshot: aws.Bool(true)})),
				Code:  "DBClusterNotFoundFault",
			},
			{
				Rule:  "SkipFinalSnapshot is false by default, and then FinalDBSnapshotIdentifier is required",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, deleteCluster(&docdb.DeleteDBClusterInput{}))),
				Code:  "InvalidParameterCombination",
			},
			{
				Rule: "FinalDBSnapshotIdentifier can't be set when SkipFinalSnapshot is true",
				Doc:  commonErrors,
				Break: against(onReadyCluster(nil, deleteCluster(&docdb.DeleteDBClusterInput{
					SkipFinalSnapshot: aws.Bool(true), FinalDBSnapshotIdentifier: aws.String("final"),
				}))),
				Code: "InvalidParameterCombination",
			},
			{
				Rule: "FinalDBSnapshotIdentifier is 1 to 255 letters, numbers, or hyphens, starts with a letter, doesn't end " +
					"with a hyphen, and has no two hyphens in a row",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, deleteCluster(&docdb.DeleteDBClusterInput{FinalDBSnapshotIdentifier: aws.String("final--1")}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule: "a cluster snapshot named FinalDBSnapshotIdentifier doesn't exist",
				Break: against(onReadyCluster(nil, func(ctx context.Context, f *fakeAPI) error {
					f.snapshots = append(f.snapshots, "final")
					return deleteCluster(&docdb.DeleteDBClusterInput{FinalDBSnapshotIdentifier: aws.String("final")})(ctx, f)
				})),
				Code: "DBClusterSnapshotAlreadyExistsFault",
			},
			{
				Rule: "the cluster is available",
				Doc:  deleteClusterGuide,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createCluster(ctx, f, nil); err != nil {
						return err
					}
					return deleteCluster(&docdb.DeleteDBClusterInput{SkipFinalSnapshot: aws.Bool(true)})(ctx, f)
				}),
				Code: "InvalidDBClusterStateFault",
			},
			{
				Rule: "the cluster has no instances",
				Doc:  deleteClusterGuide,
				Break: against(newInstance(nil).then(func(ctx context.Context, f *fakeAPI) error {
					return deleteCluster(&docdb.DeleteDBClusterInput{SkipFinalSnapshot: aws.Bool(true)})(ctx, f)
				})),
				Code: "InvalidDBClusterStateFault",
			},
			{
				Rule: "a cluster with DeletionProtection can't be deleted",
				Doc:  deleteClusterGuide,
				Break: against(onReadyCluster(func(in *docdb.CreateDBClusterInput) { in.DeletionProtection = aws.Bool(true) },
					deleteCluster(&docdb.DeleteDBClusterInput{SkipFinalSnapshot: aws.Bool(true)}))),
				Code: "InvalidParameterCombination",
			},
		},
		Unclear: codesUnclear + " Neither the page nor the guide names the error for a cluster with instances or " +
			"deletion protection; the fake returns InvalidDBClusterStateFault for instances, and " +
			"InvalidParameterCombination for deletion protection. The deletion guide and CreateDBClusterSnapshot's page " +
			"limit a cluster snapshot's name to 63 characters, while this page says 255; the fake allows 255. The " +
			"handler's name, the cluster's identifier, -final-, and the time, is longer than 63 characters for a " +
			"cluster identifier longer than 42.",
	},
	"DescribeDBClusters": {
		Doc: apiReference + "API_DescribeDBClusters.html",
		Rules: append(pagingRules(putMany(clusterType, "DBClusterIdentifier"), func(ctx context.Context, f *fakeAPI, marker *string, maxRecords *int32) (int, *string, error) {
			out, err := f.DescribeDBClusters(ctx, &docdb.DescribeDBClustersInput{Marker: marker, MaxRecords: maxRecords})
			if err != nil {
				return 0, nil, err
			}
			return len(out.DBClusters), out.Marker, nil
		}),
			nativetest.Rule{
				Rule: "the cluster DBClusterIdentifier names exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := describeCluster(ctx, f, "app")
					return err
				}),
				Code: "DBClusterNotFoundFault",
			},
			nativetest.Rule{
				Rule: "DBClusterIdentifier isn't case sensitive",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createCluster(ctx, f, nil); err != nil {
						return err
					}
					_, err := describeCluster(ctx, f, "APP")
					return err
				}),
			},
			nativetest.Rule{
				Rule: "the filter engine=docdb returns only DocumentDB clusters, because RDS and Neptune share the API",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					store.Put(clusterType, "aurora", map[string]any{"DBClusterIdentifier": "aurora", "Engine": "aurora-postgresql"})
					f := newFakeAPI(store)
					if err := createCluster(ctx, f, nil); err != nil {
						return err
					}
					out, err := f.DescribeDBClusters(ctx, &docdb.DescribeDBClustersInput{Filters: []types.Filter{{Name: aws.String("engine"), Values: []string{Engine}}}})
					if err != nil {
						return err
					}
					if len(out.DBClusters) != 1 || aws.ToString(out.DBClusters[0].Engine) != Engine {
						return fmt.Errorf("the filter returned %d clusters", len(out.DBClusters))
					}
					return nil
				},
			},
		),
		Unclear: codesUnclear + " The page lists only the db-cluster-id filter, but tells callers to use the engine " +
			"filter, which List does; the fake applies both and ignores other filters. The handler also leaves out " +
			"clusters of other engines itself.",
	},
	"ModifyDBCluster": {
		Doc: apiReference + "API_ModifyDBCluster.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the cluster exists",
				Break: against(modifyCluster(&docdb.ModifyDBClusterInput{BackupRetentionPeriod: aws.Int32(7)})),
				Code:  "DBClusterNotFoundFault",
			},
			{
				Rule: "the cluster is available",
				Doc:  modifyClusterGuide,
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createCluster(ctx, f, nil); err != nil {
						return err
					}
					return modifyCluster(&docdb.ModifyDBClusterInput{BackupRetentionPeriod: aws.Int32(7)})(ctx, f)
				}),
				Code: "InvalidDBClusterStateFault",
			},
			{
				Rule: "ApplyImmediately is false by default, and then MasterUserPassword and EngineVersion wait for the " +
					"maintenance window, while other changes apply at once",
				Check: against(onReadyCluster(nil, func(ctx context.Context, f *fakeAPI) error {
					if err := modifyCluster(&docdb.ModifyDBClusterInput{BackupRetentionPeriod: aws.Int32(7), EngineVersion: aws.String("8.0.1")})(ctx, f); err != nil {
						return err
					}
					c, err := describeCluster(ctx, f, "app")
					if err != nil {
						return err
					}
					if aws.ToInt32(c.BackupRetentionPeriod) != 7 || aws.ToString(c.EngineVersion) != "8.0.0" {
						return fmt.Errorf("the cluster has retention %d and version %s", aws.ToInt32(c.BackupRetentionPeriod), aws.ToString(c.EngineVersion))
					}
					return nil
				})),
			},
			{
				Rule: "a modify with ApplyImmediately also applies the changes earlier modifies left for the maintenance " +
					"window, and its own values win",
				Check: against(onReadyCluster(nil, func(ctx context.Context, f *fakeAPI) error {
					queued := &docdb.ModifyDBClusterInput{EngineVersion: aws.String("8.0.1"), MasterUserPassword: aws.String("pass-word-2")}
					if err := modifyCluster(queued)(ctx, f); err != nil {
						return err
					}
					if _, err := describeCluster(ctx, f, "app"); err != nil {
						return err
					}
					now := &docdb.ModifyDBClusterInput{EngineVersion: aws.String("8.0.2"), ApplyImmediately: aws.Bool(true)}
					if err := modifyCluster(now)(ctx, f); err != nil {
						return err
					}
					c, err := describeCluster(ctx, f, "app")
					if err != nil {
						return err
					}
					props := f.store.Resource(clusterType, "app")
					if aws.ToString(c.EngineVersion) != "8.0.2" || props["MasterUserPassword"] != "pass-word-2" || len(f.pending) != 0 {
						return fmt.Errorf("the cluster has version %s and %d queued changes", aws.ToString(c.EngineVersion), len(f.pending))
					}
					return nil
				})),
			},
			{
				Rule:  "BackupRetentionPeriod is 1 to 35",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{BackupRetentionPeriod: aws.Int32(0)}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "Port is 1150 to 65535",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{Port: aws.Int32(80)}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "MasterUserPassword is 8 to 100 printable ASCII characters other than /, \", and @",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{MasterUserPassword: aws.String("short"), ApplyImmediately: aws.Bool(true)}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule: "ManageMasterUserPassword can't be true with MasterUserPassword",
				Doc:  commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{
					ManageMasterUserPassword: aws.Bool(true), MasterUserPassword: aws.String("pass-word-2"), ApplyImmediately: aws.Bool(true),
				}))),
				Code: "InvalidParameterCombination",
			},
			{
				Rule: "turning ManageMasterUserPassword off for a cluster that manages it needs MasterUserPassword",
				Doc:  commonErrors,
				Break: against(onReadyCluster(func(in *docdb.CreateDBClusterInput) {
					in.MasterUserPassword = nil
					in.ManageMasterUserPassword = aws.Bool(true)
				}, modifyCluster(&docdb.ModifyDBClusterInput{ManageMasterUserPassword: aws.Bool(false), ApplyImmediately: aws.Bool(true)}))),
				Code: "InvalidParameterCombination",
			},
			{
				Rule:  "MasterUserSecretKmsKeyId is valid only when the call turns ManageMasterUserPassword on",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{MasterUserSecretKmsKeyId: aws.String("alias/app")}))),
				Code:  "InvalidParameterCombination",
			},
			{
				Rule: "RotateMasterUserPassword must apply immediately",
				Doc:  commonErrors,
				Break: against(onReadyCluster(func(in *docdb.CreateDBClusterInput) {
					in.MasterUserPassword = nil
					in.ManageMasterUserPassword = aws.Bool(true)
				}, modifyCluster(&docdb.ModifyDBClusterInput{RotateMasterUserPassword: aws.Bool(true)}))),
				Code: "InvalidParameterCombination",
			},
			{
				Rule:  "RotateMasterUserPassword is valid only when DocumentDB manages the password",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{RotateMasterUserPassword: aws.Bool(true), ApplyImmediately: aws.Bool(true)}))),
				Code:  "InvalidParameterCombination",
			},
			{
				Rule:  "an EngineVersion of another major version needs AllowMajorVersionUpgrade",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{EngineVersion: aws.String("5.0.0"), ApplyImmediately: aws.Bool(true)}))),
				Code:  "InvalidParameterCombination",
			},
			{
				Rule:  "NetworkType is IPV4 or DUAL",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{NetworkType: aws.String("IPV6")}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "StorageType is standard or iopt1",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{StorageType: aws.String("gp3")}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "PreferredBackupWindow is hh24:mi-hh24:mi and at least 30 minutes",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{PreferredBackupWindow: aws.String("1:00-2:00")}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "PreferredMaintenanceWindow is ddd:hh24:mi-ddd:hh24:mi and at least 30 minutes",
				Doc:   commonErrors,
				Break: against(onReadyCluster(nil, modifyCluster(&docdb.ModifyDBClusterInput{PreferredMaintenanceWindow: aws.String("sun:05:00-sun:05:20")}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule: "PreferredBackupWindow doesn't conflict with the cluster's maintenance window",
				Doc:  commonErrors,
				Break: against(onReadyCluster(func(in *docdb.CreateDBClusterInput) {
					in.PreferredMaintenanceWindow = aws.String("mon:03:00-mon:03:30")
				},
					modifyCluster(&docdb.ModifyDBClusterInput{PreferredBackupWindow: aws.String("02:45-03:15")}))),
				Code: "InvalidParameterValue",
			},
		},
		Unclear: codesUnclear + " The page lists InvalidDBClusterStateFault without the states it means; the guide " +
			"says the cluster must be available, which the fake requires. It gives no rules for " +
			"DBClusterParameterGroupName here. The fake doesn't model renames with NewDBClusterIdentifier, which " +
			"the handler never sends.",
	},
	"RestoreDBClusterFromSnapshot": {
		Doc: apiReference + "API_RestoreDBClusterFromSnapshot.html",
		Rules: append(append(identifierRules(func(id string) apiCall {
			return restoreFromSnapshot(func(in *docdb.RestoreDBClusterFromSnapshotInput) { in.DBClusterIdentifier = aws.String(id) })
		}), tagRules(func(tags []types.Tag) apiCall {
			return restoreFromSnapshot(func(in *docdb.RestoreDBClusterFromSnapshotInput) { in.Tags = tags })
		})...),
			nativetest.Rule{
				Rule: "the snapshot exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RestoreDBClusterFromSnapshot(ctx, &docdb.RestoreDBClusterFromSnapshotInput{
						DBClusterIdentifier: aws.String("copy"), Engine: aws.String(Engine), SnapshotIdentifier: aws.String("missing"),
					})
					return err
				}),
				Code: "DBClusterSnapshotNotFoundFault",
			},
			nativetest.Rule{
				Rule: "a cluster with the identifier doesn't exist",
				Break: against(onReadyCluster(nil, restoreFromSnapshot(func(in *docdb.RestoreDBClusterFromSnapshotInput) {
					in.DBClusterIdentifier = aws.String("app")
				}))),
				Code: "DBClusterAlreadyExistsFault",
			},
			nativetest.Rule{
				Rule:  "Port is 1150 to 65535",
				Doc:   commonErrors,
				Break: against(restoreFromSnapshot(func(in *docdb.RestoreDBClusterFromSnapshotInput) { in.Port = aws.Int32(70000) })),
				Code:  "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "DBClusterParameterGroupName is 1 to 255 letters, numbers, or hyphens, starts with a letter, doesn't end " +
					"with a hyphen, and has no two hyphens in a row",
				Doc: commonErrors,
				Break: against(restoreFromSnapshot(func(in *docdb.RestoreDBClusterFromSnapshotInput) {
					in.DBClusterParameterGroupName = aws.String("params-")
				})),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule:  "NetworkType is IPV4 or DUAL",
				Doc:   commonErrors,
				Break: against(restoreFromSnapshot(func(in *docdb.RestoreDBClusterFromSnapshotInput) { in.NetworkType = aws.String("IPV6") })),
				Code:  "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule:  "StorageType is standard or iopt1",
				Doc:   commonErrors,
				Break: against(restoreFromSnapshot(func(in *docdb.RestoreDBClusterFromSnapshotInput) { in.StorageType = aws.String("gp3") })),
				Code:  "InvalidParameterValue",
			},
		),
		Unclear: codesUnclear + tagsUnclear + " The page says Engine must be compatible with the source without saying " +
			"which engines are; the handler sends docdb, and the fake doesn't check it. The fake keeps only the names " +
			"of snapshots, so the restored cluster has only the values the call sets, not the source's configuration.",
	},
	"RestoreDBClusterToPointInTime": {
		Doc: apiReference + "API_RestoreDBClusterToPointInTime.html",
		Rules: append(append(identifierRules(func(id string) apiCall {
			return restoreToPointInTime(func(in *docdb.RestoreDBClusterToPointInTimeInput) { in.DBClusterIdentifier = aws.String(id) })
		}), tagRules(func(tags []types.Tag) apiCall {
			return restoreToPointInTime(func(in *docdb.RestoreDBClusterToPointInTimeInput) { in.Tags = tags })
		})...),
			nativetest.Rule{
				Rule: "the source cluster exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RestoreDBClusterToPointInTime(ctx, &docdb.RestoreDBClusterToPointInTimeInput{
						DBClusterIdentifier: aws.String("copy"), SourceDBClusterIdentifier: aws.String("app"), UseLatestRestorableTime: aws.Bool(true),
					})
					return err
				}),
				Code: "DBClusterNotFoundFault",
			},
			nativetest.Rule{
				Rule:  "RestoreToTime is required unless UseLatestRestorableTime is true",
				Doc:   commonErrors,
				Break: against(restoreToPointInTime(func(in *docdb.RestoreDBClusterToPointInTimeInput) { in.UseLatestRestorableTime = nil })),
				Code:  "InvalidParameterCombination",
			},
			nativetest.Rule{
				Rule:  "RestoreToTime can't be set when UseLatestRestorableTime is true",
				Doc:   commonErrors,
				Break: against(restoreToPointInTime(func(in *docdb.RestoreDBClusterToPointInTimeInput) { in.RestoreToTime = aws.Time(time.Now()) })),
				Code:  "InvalidParameterCombination",
			},
			nativetest.Rule{
				Rule: "RestoreToTime can't be set when RestoreType is copy-on-write",
				Doc:  commonErrors,
				Break: against(restoreToPointInTime(func(in *docdb.RestoreDBClusterToPointInTimeInput) {
					in.UseLatestRestorableTime = nil
					in.RestoreToTime = aws.Time(time.Now())
					in.RestoreType = aws.String("copy-on-write")
				})),
				Code: "InvalidParameterCombination",
			},
			nativetest.Rule{
				Rule:  "RestoreType is full-copy or copy-on-write",
				Doc:   commonErrors,
				Break: against(restoreToPointInTime(func(in *docdb.RestoreDBClusterToPointInTimeInput) { in.RestoreType = aws.String("clone") })),
				Code:  "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule:  "Port is 1150 to 65535",
				Doc:   commonErrors,
				Break: against(restoreToPointInTime(func(in *docdb.RestoreDBClusterToPointInTimeInput) { in.Port = aws.Int32(1000) })),
				Code:  "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule:  "StorageType is standard or iopt1",
				Doc:   commonErrors,
				Break: against(restoreToPointInTime(func(in *docdb.RestoreDBClusterToPointInTimeInput) { in.StorageType = aws.String("gp3") })),
				Code:  "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "the new cluster has the source's configuration, except that it gets the default security group",
				Check: against(onReadyCluster(func(in *docdb.CreateDBClusterInput) { in.VpcSecurityGroupIds = []string{"sg-1"} }, func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RestoreDBClusterToPointInTime(ctx, &docdb.RestoreDBClusterToPointInTimeInput{
						DBClusterIdentifier: aws.String("copy"), SourceDBClusterIdentifier: aws.String("app"), UseLatestRestorableTime: aws.Bool(true),
					})
					if err != nil {
						return err
					}
					c, err := describeCluster(ctx, f, "copy")
					if err != nil {
						return err
					}
					if aws.ToString(c.MasterUsername) != "radius" || len(c.VpcSecurityGroups) != 0 {
						return fmt.Errorf("the copy has master user %q and security groups %v", aws.ToString(c.MasterUsername), c.VpcSecurityGroups)
					}
					return nil
				})),
			},
		),
		Unclear: codesUnclear + tagsUnclear + " The fake doesn't model the latest restorable time a RestoreToTime must " +
			"come before, or the rule that a KmsKeyId restore of an unencrypted cluster is rejected. UseLatestRestorableTime " +
			"reads as not allowed with RestoreToTime even when false, while RestoreToTime's rule names only true; the " +
			"fake rejects only true. The page says the port defaults to the engine's without giving it, so the fake " +
			"doesn't copy the source's port.",
	},
})

// instanceAPI records the DocumentDB calls Instance makes.
var instanceAPI = merge(tagAPI(instanceArn, "DBInstanceNotFound", func(call apiCall) apiCall { return onReadyInstance(nil, call) }), map[string]nativetest.Call{
	"CreateDBInstance": {
		Doc: apiReference + "API_CreateDBInstance.html",
		Rules: append(append(identifierRules(func(id string) apiCall {
			return newInstance(func(in *docdb.CreateDBInstanceInput) { in.DBInstanceIdentifier = aws.String(id) })
		}), tagRules(func(tags []types.Tag) apiCall {
			return newInstance(func(in *docdb.CreateDBInstanceInput) { in.Tags = tags })
		})...),
			nativetest.Rule{
				Rule: "the identifier is stored in lowercase",
				Check: against(newInstance(func(in *docdb.CreateDBInstanceInput) { in.DBInstanceIdentifier = aws.String("App-1") }).then(func(ctx context.Context, f *fakeAPI) error {
					i, err := describeInstance(ctx, f, "app-1")
					if err != nil || aws.ToString(i.DBInstanceIdentifier) != "app-1" {
						return fmt.Errorf("the instance is %v: %v", aws.ToString(i.DBInstanceIdentifier), err)
					}
					return nil
				})),
			},
			nativetest.Rule{
				Rule:  "an instance with the identifier doesn't exist",
				Break: against(onReadyInstance(nil, func(ctx context.Context, f *fakeAPI) error { return createInstance(ctx, f, nil) })),
				Code:  "DBInstanceAlreadyExists",
			},
			nativetest.Rule{
				Rule:  "the cluster exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error { return createInstance(ctx, f, nil) }),
				Code:  "DBClusterNotFoundFault",
			},
			nativetest.Rule{
				Rule:  "Engine is docdb",
				Doc:   commonErrors,
				Break: against(newInstance(func(in *docdb.CreateDBInstanceInput) { in.Engine = aws.String("neptune") })),
				Code:  "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule:  "PromotionTier is 0 to 15",
				Doc:   commonErrors,
				Break: against(newInstance(func(in *docdb.CreateDBInstanceInput) { in.PromotionTier = aws.Int32(16) })),
				Code:  "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule:  "PreferredMaintenanceWindow is ddd:hh24:mi-ddd:hh24:mi and at least 30 minutes",
				Doc:   commonErrors,
				Break: against(newInstance(func(in *docdb.CreateDBInstanceInput) { in.PreferredMaintenanceWindow = aws.String("sun:5:00-sun:6:00") })),
				Code:  "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "an instance has up to 10 tags",
				Doc:  commonErrors,
				Break: against(newInstance(func(in *docdb.CreateDBInstanceInput) {
					for i := range 11 {
						in.Tags = append(in.Tags, tag("tag"+strconv.Itoa(i), "a"))
					}
				})),
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "without PromotionTier, the tier is 1; without AvailabilityZone, DocumentDB picks one in the region; " +
					"without PreferredMaintenanceWindow, DocumentDB picks a 30-minute window",
				Check: against(newInstance(nil).then(func(ctx context.Context, f *fakeAPI) error {
					i, err := describeInstance(ctx, f, "app-1")
					if err != nil {
						return err
					}
					if aws.ToInt32(i.PromotionTier) != 1 || !strings.HasPrefix(aws.ToString(i.AvailabilityZone), "us-west-2") {
						return fmt.Errorf("the instance has tier %d and zone %q", aws.ToInt32(i.PromotionTier), aws.ToString(i.AvailabilityZone))
					}
					if w, ok := maintenanceWindow(aws.ToString(i.PreferredMaintenanceWindow)); !ok || w.length < 30 {
						return fmt.Errorf("the instance has maintenance window %q", aws.ToString(i.PreferredMaintenanceWindow))
					}
					return nil
				})),
			},
		),
		Unclear: codesUnclear + tagsUnclear + " The page says AutoMinorVersionUpgrade defaults to false and doesn't " +
			"apply to DocumentDB, but recorded responses return true for an instance created without it, as the " +
			"handler's removedInstanceDefaults expects; the fake reports what the call set. The page lists " +
			"InvalidDBClusterStateFault without the cluster states it means; the fake doesn't check the cluster's " +
			"state. It doesn't name the error for more than 10 tags; the fake returns InvalidParameterValue.",
	},
	"DeleteDBInstance": {
		Doc: apiReference + "API_DeleteDBInstance.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the instance exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteDBInstance(ctx, &docdb.DeleteDBInstanceInput{DBInstanceIdentifier: aws.String("app-1")})
					return err
				}),
				Code: "DBInstanceNotFound",
			},
			{
				Rule: "the instance is available",
				Doc:  deleteInstanceGuide,
				Break: against(newInstance(nil).then(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteDBInstance(ctx, &docdb.DeleteDBInstanceInput{DBInstanceIdentifier: aws.String("app-1")})
					return err
				})),
				Code: "InvalidDBInstanceState",
			},
		},
		Unclear: "The page lists InvalidDBClusterStateFault without the cluster states it means; the fake doesn't " +
			"check the cluster's state.",
	},
	"DescribeDBInstances": {
		Doc: apiReference + "API_DescribeDBInstances.html",
		Rules: append(pagingRules(putMany(instanceType, "DBInstanceIdentifier"), func(ctx context.Context, f *fakeAPI, marker *string, maxRecords *int32) (int, *string, error) {
			out, err := f.DescribeDBInstances(ctx, &docdb.DescribeDBInstancesInput{Marker: marker, MaxRecords: maxRecords})
			if err != nil {
				return 0, nil, err
			}
			return len(out.DBInstances), out.Marker, nil
		}),
			nativetest.Rule{
				Rule: "the instance DBInstanceIdentifier names exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := describeInstance(ctx, f, "app-1")
					return err
				}),
				Code: "DBInstanceNotFound",
			},
			nativetest.Rule{
				Rule: "DBInstanceIdentifier isn't case sensitive",
				Check: against(newInstance(nil).then(func(ctx context.Context, f *fakeAPI) error {
					_, err := describeInstance(ctx, f, "APP-1")
					return err
				})),
			},
		),
		Unclear: codesUnclear + " The page lists the db-cluster-id and db-instance-id filters, not the engine filter " +
			"List sends, as DescribeDBClusters' page tells callers to; the fake applies it, and the handler also " +
			"leaves out instances of other engines itself.",
	},
	"ModifyDBInstance": {
		Doc: apiReference + "API_ModifyDBInstance.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the instance exists",
				Break: against(modifyInstance(&docdb.ModifyDBInstanceInput{DBInstanceClass: aws.String("db.r6g.large")})),
				Code:  "DBInstanceNotFound",
			},
			{
				Rule:  "the instance is available",
				Doc:   modifyInstanceGuide,
				Break: against(newInstance(nil).then(modifyInstance(&docdb.ModifyDBInstanceInput{DBInstanceClass: aws.String("db.r6g.large")}))),
				Code:  "InvalidDBInstanceState",
			},
			{
				Rule: "ApplyImmediately is false by default, and then a change of DBInstanceClass waits for the maintenance " +
					"window, while a change of PreferredMaintenanceWindow applies at once",
				Check: against(onReadyInstance(nil, func(ctx context.Context, f *fakeAPI) error {
					err := modifyInstance(&docdb.ModifyDBInstanceInput{DBInstanceClass: aws.String("db.r6g.large"), PreferredMaintenanceWindow: aws.String("mon:01:00-mon:01:30")})(ctx, f)
					if err != nil {
						return err
					}
					i, err := describeInstance(ctx, f, "app-1")
					if err != nil {
						return err
					}
					if aws.ToString(i.DBInstanceClass) != "db.t3.medium" || aws.ToString(i.PreferredMaintenanceWindow) != "mon:01:00-mon:01:30" {
						return fmt.Errorf("the instance has class %s and window %s", aws.ToString(i.DBInstanceClass), aws.ToString(i.PreferredMaintenanceWindow))
					}
					return nil
				})),
			},
			{
				Rule: "a modify with ApplyImmediately also applies the changes earlier modifies left for the maintenance " +
					"window, and its own values win",
				Check: against(onReadyInstance(nil, func(ctx context.Context, f *fakeAPI) error {
					if err := modifyInstance(&docdb.ModifyDBInstanceInput{DBInstanceClass: aws.String("db.r6g.large"), PromotionTier: aws.Int32(2)})(ctx, f); err != nil {
						return err
					}
					if _, err := describeInstance(ctx, f, "app-1"); err != nil {
						return err
					}
					if err := modifyInstance(&docdb.ModifyDBInstanceInput{PromotionTier: aws.Int32(3), ApplyImmediately: aws.Bool(true)})(ctx, f); err != nil {
						return err
					}
					i, err := describeInstance(ctx, f, "app-1")
					if err != nil {
						return err
					}
					if aws.ToString(i.DBInstanceClass) != "db.r6g.large" || aws.ToInt32(i.PromotionTier) != 3 || len(f.pending) != 0 {
						return fmt.Errorf("the instance has class %s, tier %d, and %d queued changes", aws.ToString(i.DBInstanceClass), aws.ToInt32(i.PromotionTier), len(f.pending))
					}
					return nil
				})),
			},
			{
				Rule: "a change of CACertificateIdentifier without ApplyImmediately waits for the maintenance window",
				Doc:  certificateGuide,
				Check: against(onReadyInstance(func(in *docdb.CreateDBInstanceInput) { in.CACertificateIdentifier = aws.String("rds-ca-2019") }, func(ctx context.Context, f *fakeAPI) error {
					if err := modifyInstance(&docdb.ModifyDBInstanceInput{CACertificateIdentifier: aws.String("rds-ca-rsa2048-g1")})(ctx, f); err != nil {
						return err
					}
					i, err := describeInstance(ctx, f, "app-1")
					if err != nil || aws.ToString(i.CACertificateIdentifier) != "rds-ca-2019" || f.restarts["app-1"] != 0 {
						return fmt.Errorf("the instance has CA %s after %d restarts: %v", aws.ToString(i.CACertificateIdentifier), f.restarts["app-1"], err)
					}
					return nil
				})),
			},
			{
				Rule: "a CA certificate change restarts the instance unless CertificateRotationRestart is false",
				Check: func(ctx context.Context, _ *awsfake.CloudControl) error {
					for _, restart := range []*bool{nil, aws.Bool(true), aws.Bool(false)} {
						f := newFakeAPI(awsfake.NewCloudControl())
						err := onReadyInstance(func(in *docdb.CreateDBInstanceInput) { in.CACertificateIdentifier = aws.String("rds-ca-2019") }, modifyInstance(&docdb.ModifyDBInstanceInput{
							CACertificateIdentifier: aws.String("rds-ca-rsa2048-g1"), CertificateRotationRestart: restart, ApplyImmediately: aws.Bool(true),
						}))(ctx, f)
						if err != nil {
							return err
						}
						want := 1
						if restart != nil && !*restart {
							want = 0
						}
						if f.restarts["app-1"] != want {
							return fmt.Errorf("CertificateRotationRestart %v restarted the instance %d times", restart, f.restarts["app-1"])
						}
					}
					return nil
				},
			},
			{
				Rule:  "PromotionTier is 0 to 15",
				Doc:   commonErrors,
				Break: against(onReadyInstance(nil, modifyInstance(&docdb.ModifyDBInstanceInput{PromotionTier: aws.Int32(-1)}))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "PreferredMaintenanceWindow is ddd:hh24:mi-ddd:hh24:mi and at least 30 minutes",
				Doc:   commonErrors,
				Break: against(onReadyInstance(nil, modifyInstance(&docdb.ModifyDBInstanceInput{PreferredMaintenanceWindow: aws.String("sun:05:00-sun:05:29")}))),
				Code:  "InvalidParameterValue",
			},
		},
		Unclear: codesUnclear + " The page says changes without ApplyImmediately wait for the maintenance window, " +
			"naming DBInstanceClass, and that a PreferredMaintenanceWindow change applies at once; the fake holds " +
			"every other change too. It doesn't say when the certificate changes for an instance that isn't " +
			"restarted; the fake reports the new CA at once.",
	},
})

// then returns a call that makes call, then next.
func (call apiCall) then(next apiCall) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		if err := call(ctx, f); err != nil {
			return err
		}
		return next(ctx, f)
	}
}

// merge returns the records of a and b.
func merge(a map[string]nativetest.Call, b map[string]nativetest.Call) map[string]nativetest.Call {
	out := maps.Clone(a)
	maps.Copy(out, b)
	return out
}
