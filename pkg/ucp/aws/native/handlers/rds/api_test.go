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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference      = "https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/"
	commonErrors      = apiReference + "CommonErrors.html"
	tagDoc            = apiReference + "API_Tag.html"
	createInstanceDoc = apiReference + "API_CreateDBInstance.html"
	createSnapshotDoc = apiReference + "API_CreateDBSnapshot.html"
	auroraGuide       = "https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/"
	startStreamGuide  = auroraGuide + "DBActivityStreams.Enabling.html"
	stopStreamGuide   = auroraGuide + "DBActivityStreams.Disabling.html"
	streamStatusGuide = auroraGuide + "DBActivityStreams.Status.html"
	userGuide         = "https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/"
	addTenantGuide    = userGuide + "oracle-cdb-configuring.adding.pdb.html"
	modifyTenantGuide = userGuide + "oracle-cdb-configuring.modifying.pdb.html"
	deleteTenantGuide = userGuide + "oracle-cdb-configuring.deleting.pdb.html"
	secretsGuide      = userGuide + "rds-secrets-manager.html"
	tagGuide          = userGuide + "USER_Tagging.html"

	// codesUnclear is what every call's page leaves unclear: the error code of a missing parameter, a value out of
	// its limits, or parameters that can't be combined.
	codesUnclear = "The page doesn't name the error for a missing parameter, a value out of its limits, or parameters " +
		"that can't be combined; the fake returns MissingParameter, InvalidParameterValue, and " +
		"InvalidParameterCombination, from the Common Errors page."
)

// apiCall is a call against a fake RDS API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns call as a rule's Break or Check, against a fake RDS API backed by store, which has the available
// Aurora cluster app-aurora and the available CDB instance app-cdb.
func against(call apiCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		addCluster(store, "app-aurora")
		addInstance(store, "app-cdb")
		return call(ctx, newFakeAPI(store))
	}
}

// then returns a call that makes each of calls in order, stopping at the first that fails.
func then(calls ...apiCall) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

func start(edit func(*rds.StartActivityStreamInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &rds.StartActivityStreamInput{
			ResourceArn:      aws.String(clusterArn),
			Mode:             types.ActivityStreamModeAsync,
			KmsKeyId:         aws.String(keyID),
			ApplyImmediately: aws.Bool(true),
		}
		if edit != nil {
			edit(in)
		}
		_, err := f.StartActivityStream(ctx, in)
		return err
	}
}

func stop(edit func(*rds.StopActivityStreamInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &rds.StopActivityStreamInput{ResourceArn: aws.String(clusterArn), ApplyImmediately: aws.Bool(true)}
		if edit != nil {
			edit(in)
		}
		_, err := f.StopActivityStream(ctx, in)
		return err
	}
}

// streamIs returns a call that reads the cluster app-aurora and fails unless its stream has the status want.
func streamIs(want types.ActivityStreamStatus) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		out, err := f.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String("app-aurora")})
		if err != nil {
			return err
		}
		if got := out.DBClusters[0].ActivityStreamStatus; got != want {
			return fmt.Errorf("the stream is %s, not %s", got, want)
		}
		return nil
	}
}

func describeClusters(in *rds.DescribeDBClustersInput) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.DescribeDBClusters(ctx, in)
		return err
	}
}

// activityStreamAPI records the RDS calls ActivityStream makes.
var activityStreamAPI = map[string]nativetest.Call{
	"StartActivityStream": {
		Doc: apiReference + "API_StartActivityStream.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "ResourceArn, Mode, and KmsKeyId are required",
				Doc:   commonErrors,
				Break: against(start(func(in *rds.StartActivityStreamInput) { in.KmsKeyId = nil })),
				Code:  "MissingParameter",
			},
			{
				Rule:  "Mode is sync or async",
				Doc:   commonErrors,
				Break: against(start(func(in *rds.StartActivityStreamInput) { in.Mode = "batch" })),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "ResourceArn is the ARN of an existing DB cluster",
				Break: against(start(func(in *rds.StartActivityStreamInput) { in.ResourceArn = aws.String(arnPrefix + "cluster:missing") })),
				Code:  "DBClusterNotFoundFault",
			},
			{
				Rule:  "a started stream is starting, then started",
				Doc:   streamStatusGuide,
				Check: against(then(start(nil), streamIs(types.ActivityStreamStatusStarting), streamIs(types.ActivityStreamStatusStarted))),
			},
			{
				Rule: "without ApplyImmediately, the stream doesn't start until the cluster's next maintenance window",
				Doc:  startStreamGuide,
				Check: against(then(start(func(in *rds.StartActivityStreamInput) { in.ApplyImmediately = nil }),
					streamIs(types.ActivityStreamStatusStopped))),
			},
		},
		Unclear: codesUnclear + " The page lists InvalidDBClusterStateFault without saying which states cause it; the " +
			"fake returns it for a cluster that isn't available, or whose stream isn't stopped. The Aurora guide says " +
			"Aurora MySQL supports only async mode, and some instance classes and engine versions none, without naming " +
			"the error; the fake has one Aurora PostgreSQL cluster and doesn't check them. The page doesn't say what " +
			"ApplyImmediately defaults to; the fake treats it as false. The page says KmsKeyId can be a key ARN, key " +
			"ID, alias ARN, or alias name, but not in which form DescribeDBClusters returns it, and the guide's example " +
			"shows a key ID; the fake returns it as the call sent it, and the suite's other-form cases cover the rest. " +
			"The page also takes a DB instance's ARN, for RDS for Oracle and SQL Server, which the type doesn't serve, " +
			"so the handler rejects it before any call. The response's Status is starting, but the pages don't say " +
			"whether DescribeDBClusters can still show stopped right after the call; the fake shows starting at once. " +
			"If it lagged, the create would fail as not found, and the next deploy would read the stream and wait for it.",
	},
	"StopActivityStream": {
		Doc: apiReference + "API_StopActivityStream.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "ResourceArn is required",
				Doc:   commonErrors,
				Break: against(stop(func(in *rds.StopActivityStreamInput) { in.ResourceArn = nil })),
				Code:  "MissingParameter",
			},
			{
				Rule:  "ResourceArn is the ARN of an existing DB cluster",
				Break: against(stop(func(in *rds.StopActivityStreamInput) { in.ResourceArn = aws.String(arnPrefix + "cluster:missing") })),
				Code:  "DBClusterNotFoundFault",
			},
			{
				Rule: "a stopped stream is stopping, then stopped",
				Doc:  streamStatusGuide,
				Check: against(then(start(nil), streamIs(types.ActivityStreamStatusStarting), stop(nil),
					streamIs(types.ActivityStreamStatusStopping), streamIs(types.ActivityStreamStatusStopped))),
			},
			{
				Rule: "without ApplyImmediately, the stream doesn't stop until the cluster's next maintenance window",
				Doc:  stopStreamGuide,
				Check: against(then(start(nil), streamIs(types.ActivityStreamStatusStarting),
					stop(func(in *rds.StopActivityStreamInput) { in.ApplyImmediately = nil }), streamIs(types.ActivityStreamStatusStarted))),
			},
		},
		Unclear: codesUnclear + " The page lists InvalidDBClusterStateFault without saying which states cause it; the " +
			"fake returns it for a cluster that isn't available, or whose stream isn't started, and the handler reads " +
			"the cluster again when AWS returns it. The Aurora guide says deleting a cluster stops its stream; the " +
			"handler then makes no stop call, and the fake doesn't delete clusters.",
	},
	"DescribeDBClusters": {
		Doc: apiReference + "API_DescribeDBClusters.html",
		Rules: []nativetest.Rule{
			{
				Rule: "DBClusterIdentifier takes the cluster's identifier or ARN, and isn't case-sensitive",
				Check: against(then(
					describeClusters(&rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String("APP-Aurora")}),
					describeClusters(&rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String(strings.ToUpper(clusterArn))}),
				)),
			},
			{
				Rule:  "DBClusterIdentifier must match an existing cluster",
				Break: against(describeClusters(&rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String("missing")})),
				Code:  "DBClusterNotFoundFault",
			},
			{
				Rule:  "MaxRecords is 20 to 100",
				Doc:   commonErrors,
				Break: against(describeClusters(&rds.DescribeDBClustersInput{MaxRecords: aws.Int32(10)})),
				Code:  "InvalidParameterValue",
			},
			{
				Rule: "MaxRecords defaults to 100, and Marker returns the rest",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					for i := range 100 {
						addCluster(f.store, fmt.Sprintf("app-%03d", i))
					}
					first, err := f.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{})
					if err != nil {
						return err
					}
					if len(first.DBClusters) != 100 || first.Marker == nil {
						return fmt.Errorf("the first page has %d clusters and marker %v", len(first.DBClusters), first.Marker)
					}
					rest, err := f.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{Marker: first.Marker})
					if err != nil {
						return err
					}
					if len(rest.DBClusters) != 1 || rest.Marker != nil {
						return fmt.Errorf("the last page has %d clusters and marker %v", len(rest.DBClusters), rest.Marker)
					}
					return nil
				}),
			},
		},
		Unclear: codesUnclear + " The page says the call also returns Neptune and DocumentDB clusters; their activity " +
			"stream fields are the same, so the handler doesn't filter by engine, and the fake has only Aurora clusters.",
	},
}

func createTenant(edit func(*rds.CreateTenantDatabaseInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &rds.CreateTenantDatabaseInput{
			DBInstanceIdentifier: aws.String("app-cdb"),
			TenantDBName:         aws.String("pdb1"),
			MasterUsername:       aws.String("admin"),
			MasterUserPassword:   aws.String("pass-one-1234"),
		}
		if edit != nil {
			edit(in)
		}
		_, err := f.CreateTenantDatabase(ctx, in)
		return err
	}
}

func describeTenant(ctx context.Context, f *fakeAPI, instance string, name string) (*types.TenantDatabase, error) {
	out, err := f.DescribeTenantDatabases(ctx, &rds.DescribeTenantDatabasesInput{DBInstanceIdentifier: aws.String(instance), TenantDBName: aws.String(name)})
	if err != nil {
		return nil, err
	}
	if len(out.TenantDatabases) != 1 {
		return nil, fmt.Errorf("%d tenant databases match %s %s", len(out.TenantDatabases), instance, name)
	}
	return &out.TenantDatabases[0], nil
}

// tenantIs returns a call that reads the tenant database pdb1 and fails unless check accepts it.
func tenantIs(check func(t *types.TenantDatabase) error) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		t, err := describeTenant(ctx, f, "app-cdb", "pdb1")
		if err != nil {
			return err
		}
		return check(t)
	}
}

func tenantStatusIs(want string) apiCall {
	return tenantIs(func(t *types.TenantDatabase) error {
		if got := aws.ToString(t.Status); got != want {
			return fmt.Errorf("the tenant database is %s, not %s", got, want)
		}
		return nil
	})
}

// readyTenant creates the tenant database pdb1, with the changes edit makes to the input, and reads it, after which
// it is available.
func readyTenant(edit func(*rds.CreateTenantDatabaseInput)) apiCall {
	return then(createTenant(edit), tenantStatusIs("creating"), tenantStatusIs("available"))
}

func modifyTenant(edit func(*rds.ModifyTenantDatabaseInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &rds.ModifyTenantDatabaseInput{DBInstanceIdentifier: aws.String("app-cdb"), TenantDBName: aws.String("pdb1")}
		edit(in)
		_, err := f.ModifyTenantDatabase(ctx, in)
		return err
	}
}

func deleteTenant(edit func(*rds.DeleteTenantDatabaseInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &rds.DeleteTenantDatabaseInput{
			DBInstanceIdentifier:      aws.String("app-cdb"),
			TenantDBName:              aws.String("pdb1"),
			FinalDBSnapshotIdentifier: aws.String("app-cdb-pdb1-final"),
		}
		if edit != nil {
			edit(in)
		}
		_, err := f.DeleteTenantDatabase(ctx, in)
		return err
	}
}

func tag(key string, value string) types.Tag {
	return types.Tag{Key: aws.String(key), Value: aws.String(value)}
}

// tagRules returns the rules of the Tag data type, for a call that takes tags. call makes the call with tags.
func tagRules(call func(tags []types.Tag) apiCall) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "a tag key is 1 to 128 characters and a value at most 256, of letters, digits, white space, and _.:/=+-@",
			Doc:   tagDoc,
			Break: against(call([]types.Tag{tag("team!", "a")})),
			Code:  "InvalidParameterValue",
		},
		{
			Rule:  "a tag key or value doesn't start with aws: or rds:",
			Doc:   tagDoc,
			Break: against(call([]types.Tag{tag("rds:team", "a")})),
			Code:  "InvalidParameterValue",
		},
	}
}

// passwordRules returns the rules of a tenant database's master user password. call makes the call with password.
func passwordRules(call func(password string) apiCall) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "MasterUserPassword is 8 to 30 characters",
			Doc:   commonErrors,
			Break: against(call("short")),
			Code:  "InvalidParameterValue",
		},
		{
			Rule:  "MasterUserPassword is printable ASCII other than /, \", @, &, and '",
			Doc:   commonErrors,
			Break: against(call("pass&word-1")),
			Code:  "InvalidParameterValue",
		},
	}
}

func addTags(arn string, tags ...types.Tag) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.AddTagsToResource(ctx, &rds.AddTagsToResourceInput{ResourceName: aws.String(arn), Tags: tags})
		return err
	}
}

func removeTags(arn string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.RemoveTagsFromResource(ctx, &rds.RemoveTagsFromResourceInput{ResourceName: aws.String(arn), TagKeys: []string{"team"}})
		return err
	}
}

const tenantArn = arnPrefix + "db:app-cdb:pdb1"

// tenantDatabaseAPI records the RDS calls TenantDatabase makes.
var tenantDatabaseAPI = map[string]nativetest.Call{
	"CreateTenantDatabase": {
		Doc: apiReference + "API_CreateTenantDatabase.html",
		Rules: append(append([]nativetest.Rule{
			{
				Rule:  "DBInstanceIdentifier, TenantDBName, and MasterUsername are required",
				Doc:   commonErrors,
				Break: against(createTenant(func(in *rds.CreateTenantDatabaseInput) { in.MasterUsername = nil })),
				Code:  "MissingParameter",
			},
			{
				Rule:  "DBInstanceIdentifier names an existing DB instance",
				Break: against(createTenant(func(in *rds.CreateTenantDatabaseInput) { in.DBInstanceIdentifier = aws.String("missing") })),
				Code:  "DBInstanceNotFound",
			},
			{
				Rule:  "DBInstanceIdentifier isn't case-sensitive",
				Check: against(then(createTenant(func(in *rds.CreateTenantDatabaseInput) { in.DBInstanceIdentifier = aws.String("APP-CDB") }), tenantStatusIs("creating"))),
			},
			{
				Rule:  "MasterUsername starts with a letter",
				Doc:   commonErrors,
				Break: against(createTenant(func(in *rds.CreateTenantDatabaseInput) { in.MasterUsername = aws.String("1admin") })),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "MasterUsername is 1 to 16 letters, numbers, or underscores",
				Doc:   commonErrors,
				Break: against(createTenant(func(in *rds.CreateTenantDatabaseInput) { in.MasterUsername = aws.String("admin_user_name_1") })),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "TenantDBName has DBName's rules for RDS for Oracle: at most 8 characters",
				Doc:   createInstanceDoc,
				Break: against(createTenant(func(in *rds.CreateTenantDatabaseInput) { in.TenantDBName = aws.String("pdbnumber") })),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "TenantDBName has DBName's rules for RDS for Oracle: not the string null",
				Doc:   createInstanceDoc,
				Break: against(createTenant(func(in *rds.CreateTenantDatabaseInput) { in.TenantDBName = aws.String("NULL") })),
				Code:  "InvalidParameterValue",
			},
			{
				Rule: "ManageMasterUserPassword can't be set with MasterUserPassword",
				Doc:  commonErrors,
				Break: against(createTenant(func(in *rds.CreateTenantDatabaseInput) {
					in.ManageMasterUserPassword = aws.Bool(true)
				})),
				Code: "InvalidParameterCombination",
			},
			{
				Rule: "MasterUserSecretKmsKeyId applies only when RDS manages the password",
				Doc:  commonErrors,
				Break: against(createTenant(func(in *rds.CreateTenantDatabaseInput) {
					in.MasterUserSecretKmsKeyId = aws.String(keyID)
				})),
				Code: "InvalidParameterCombination",
			},
			{
				Rule:  "the tenant database doesn't exist",
				Break: against(then(createTenant(nil), createTenant(func(in *rds.CreateTenantDatabaseInput) { in.TenantDBName = aws.String("PDB1") }))),
				Code:  "TenantDatabaseAlreadyExists",
			},
			{
				Rule: "CharacterSetName defaults to AL32UTF8, and NcharCharacterSetName to AL16UTF16",
				Doc:  addTenantGuide,
				Check: against(then(createTenant(nil), tenantIs(func(t *types.TenantDatabase) error {
					if aws.ToString(t.CharacterSetName) != "AL32UTF8" || aws.ToString(t.NcharCharacterSetName) != "AL16UTF16" {
						return fmt.Errorf("the character sets are %s and %s", aws.ToString(t.CharacterSetName), aws.ToString(t.NcharCharacterSetName))
					}
					return nil
				}))),
			},
		}, passwordRules(func(password string) apiCall {
			return createTenant(func(in *rds.CreateTenantDatabaseInput) { in.MasterUserPassword = aws.String(password) })
		})...), tagRules(func(tags []types.Tag) apiCall {
			return createTenant(func(in *rds.CreateTenantDatabaseInput) { in.Tags = tags })
		})...),
		Unclear: codesUnclear + " The page lists InvalidDBInstanceState without saying which states cause it; the " +
			"fake returns it for an instance that isn't available. It doesn't say what AWS does when neither " +
			"MasterUserPassword nor ManageMasterUserPassword is set, or for an instance without the multi-tenant " +
			"configuration; the fake accepts the first and has only multi-tenant instances. DBName's rules give no " +
			"character set for an Oracle name; the fake checks only the length and null. Both pages forbid a reserved " +
			"word as MasterUsername or TenantDBName without listing Oracle's reserved words; the fake rejects only null. The page doesn't name the " +
			"tenant database's statuses; the fake reports creating, as the guide's examples report modifying and " +
			"deleting, then available.",
	},
	"DescribeTenantDatabases": {
		Doc: apiReference + "API_DescribeTenantDatabases.html",
		Rules: []nativetest.Rule{
			{
				Rule: "DBInstanceIdentifier must name an existing instance",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeTenantDatabases(ctx, &rds.DescribeTenantDatabasesInput{DBInstanceIdentifier: aws.String("missing")})
					return err
				}),
				Code: "DBInstanceNotFound",
			},
			{
				Rule: "DBInstanceIdentifier and TenantDBName aren't case-sensitive",
				Check: against(then(createTenant(nil), func(ctx context.Context, f *fakeAPI) error {
					_, err := describeTenant(ctx, f, "APP-CDB", "PDB1")
					return err
				})),
			},
			{
				Rule: "a tenant database has the tags it was created with",
				Doc:  apiReference + "API_TenantDatabase.html",
				Check: against(then(createTenant(func(in *rds.CreateTenantDatabaseInput) { in.Tags = []types.Tag{tag("team", "a")} }),
					tenantIs(func(t *types.TenantDatabase) error {
						if len(t.TagList) != 1 || aws.ToString(t.TagList[0].Key) != "team" {
							return fmt.Errorf("the tags are %v", t.TagList)
						}
						return nil
					}))),
			},
		},
		Unclear: "The page says TenantDBName must match an existing tenant database without naming an error; the fake " +
			"returns no tenant databases, and the handler also treats TenantDatabaseNotFound as not found. It gives no " +
			"default or limits for MaxRecords; the fake returns 2 tenant databases a page when the call doesn't set it, " +
			"so the handler's paging runs.",
	},
	"ModifyTenantDatabase": {
		Doc: apiReference + "API_ModifyTenantDatabase.html",
		Rules: append([]nativetest.Rule{
			{
				Rule:  "DBInstanceIdentifier names an existing DB instance",
				Break: against(modifyTenant(func(in *rds.ModifyTenantDatabaseInput) { in.DBInstanceIdentifier = aws.String("missing") })),
				Code:  "DBInstanceNotFound",
			},
			{
				Rule:  "TenantDBName names an existing tenant database",
				Break: against(modifyTenant(func(in *rds.ModifyTenantDatabaseInput) { in.MasterUserPassword = aws.String("pass-two-5678") })),
				Code:  "TenantDatabaseNotFound",
			},
			{
				Rule: "ManageMasterUserPassword can't be set with MasterUserPassword",
				Doc:  commonErrors,
				Break: against(then(readyTenant(nil), modifyTenant(func(in *rds.ModifyTenantDatabaseInput) {
					in.ManageMasterUserPassword, in.MasterUserPassword = aws.Bool(true), aws.String("pass-two-5678")
				}))),
				Code: "InvalidParameterCombination",
			},
			{
				Rule: "turning off RDS's management of the password takes MasterUserPassword",
				Doc:  commonErrors,
				Break: against(then(readyTenant(func(in *rds.CreateTenantDatabaseInput) {
					in.MasterUserPassword, in.ManageMasterUserPassword = nil, aws.Bool(true)
				}), modifyTenant(func(in *rds.ModifyTenantDatabaseInput) { in.ManageMasterUserPassword = aws.Bool(false) }))),
				Code: "MissingParameter",
			},
			{
				Rule: "MasterUserSecretKmsKeyId applies only when the call turns on RDS's management of the password",
				Doc:  commonErrors,
				Break: against(then(readyTenant(nil), modifyTenant(func(in *rds.ModifyTenantDatabaseInput) {
					in.MasterUserSecretKmsKeyId = aws.String(keyID)
				}))),
				Code: "InvalidParameterCombination",
			},
			{
				Rule: "the secret's KMS key can't change once RDS manages the password",
				Doc:  secretsGuide,
				Break: against(then(readyTenant(func(in *rds.CreateTenantDatabaseInput) {
					in.MasterUserPassword, in.ManageMasterUserPassword = nil, aws.Bool(true)
				}), modifyTenant(func(in *rds.ModifyTenantDatabaseInput) {
					in.ManageMasterUserPassword, in.MasterUserSecretKmsKeyId = aws.Bool(true), aws.String(keyID)
				}))),
				Code: "InvalidParameterCombination",
			},
			{
				Rule: "a new password is pending, and the tenant database modifying, until RDS applies it",
				Doc:  modifyTenantGuide,
				Check: against(then(readyTenant(nil), modifyTenant(func(in *rds.ModifyTenantDatabaseInput) {
					in.MasterUserPassword = aws.String("pass-two-5678")
				}), tenantIs(func(t *types.TenantDatabase) error {
					if aws.ToString(t.Status) != "modifying" || !pending(t.PendingModifiedValues) {
						return fmt.Errorf("the tenant database is %s with pending values %v", aws.ToString(t.Status), t.PendingModifiedValues)
					}
					return nil
				}), tenantStatusIs("available"))),
			},
			{
				Rule: "turning on RDS's management of the password makes a secret in Secrets Manager",
				Doc:  secretsGuide,
				Check: against(then(readyTenant(nil), modifyTenant(func(in *rds.ModifyTenantDatabaseInput) {
					in.ManageMasterUserPassword = aws.Bool(true)
				}), tenantIs(func(t *types.TenantDatabase) error {
					if t.MasterUserSecret == nil {
						return fmt.Errorf("the tenant database has no secret")
					}
					return nil
				}))),
			},
		}, passwordRules(func(password string) apiCall {
			return then(readyTenant(nil), modifyTenant(func(in *rds.ModifyTenantDatabaseInput) { in.MasterUserPassword = aws.String(password) }))
		})...),
		Unclear: codesUnclear + " The page lists InvalidDBInstanceState without saying which states cause it; the " +
			"fake returns it for an instance that isn't available, or a tenant database that is changing. The handler " +
			"doesn't rename tenant databases or rotate the secret, so the fake doesn't take NewTenantDBName or " +
			"RotateMasterUserPassword.",
	},
	"DeleteTenantDatabase": {
		Doc: apiReference + "API_DeleteTenantDatabase.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "DBInstanceIdentifier names an existing DB instance",
				Break: against(deleteTenant(func(in *rds.DeleteTenantDatabaseInput) { in.DBInstanceIdentifier = aws.String("missing") })),
				Code:  "DBInstanceNotFound",
			},
			{
				Rule:  "TenantDBName names an existing tenant database",
				Break: against(deleteTenant(nil)),
				Code:  "TenantDatabaseNotFound",
			},
			{
				Rule:  "without SkipFinalSnapshot, FinalDBSnapshotIdentifier is required",
				Doc:   commonErrors,
				Break: against(then(readyTenant(nil), deleteTenant(func(in *rds.DeleteTenantDatabaseInput) { in.FinalDBSnapshotIdentifier = nil }))),
				Code:  "MissingParameter",
			},
			{
				Rule:  "FinalDBSnapshotIdentifier can't be set with SkipFinalSnapshot",
				Doc:   commonErrors,
				Break: against(then(readyTenant(nil), deleteTenant(func(in *rds.DeleteTenantDatabaseInput) { in.SkipFinalSnapshot = aws.Bool(true) }))),
				Code:  "InvalidParameterCombination",
			},
			{
				Rule:  "a snapshot identifier is 1 to 255 letters, numbers, or hyphens, and starts with a letter",
				Doc:   createSnapshotDoc,
				Break: against(then(readyTenant(nil), deleteTenant(func(in *rds.DeleteTenantDatabaseInput) { in.FinalDBSnapshotIdentifier = aws.String("1-final") }))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "a snapshot identifier doesn't end with a hyphen or have two hyphens in a row",
				Doc:   createSnapshotDoc,
				Break: against(then(readyTenant(nil), deleteTenant(func(in *rds.DeleteTenantDatabaseInput) { in.FinalDBSnapshotIdentifier = aws.String("app--final") }))),
				Code:  "InvalidParameterValue",
			},
			{
				Rule: "FinalDBSnapshotIdentifier isn't the identifier of an existing snapshot",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					f.snapshots = append(f.snapshots, "app-cdb-pdb1-final")
					return then(readyTenant(nil), deleteTenant(nil))(ctx, f)
				}),
				Code: "DBSnapshotAlreadyExists",
			},
			{
				Rule: "RDS takes a final snapshot unless the call skips it, and the tenant database is deleting, then gone",
				Doc:  deleteTenantGuide,
				Check: against(then(readyTenant(nil), deleteTenant(nil), tenantStatusIs("deleting"), func(ctx context.Context, f *fakeAPI) error {
					out, err := f.DescribeTenantDatabases(ctx, &rds.DescribeTenantDatabasesInput{DBInstanceIdentifier: aws.String("app-cdb")})
					if err != nil {
						return err
					}
					if len(out.TenantDatabases) != 0 || len(f.snapshots) != 1 {
						return fmt.Errorf("%d tenant databases and %d snapshots are left", len(out.TenantDatabases), len(f.snapshots))
					}
					return nil
				})),
			},
		},
		Unclear: codesUnclear + " The page says a tenant database can't be deleted when it is the instance's only " +
			"one, and the guide that a final snapshot needs the tenant database and instance to be available, " +
			"without naming the errors; the fake returns InvalidDBInstanceState for both, and for a tenant database " +
			"that is already deleting, which the handler checks before it calls.",
	},
	"AddTagsToResource": {
		Doc: apiReference + "API_AddTagsToResource.html",
		Rules: append(tagRules(func(tags []types.Tag) apiCall { return then(readyTenant(nil), addTags(tenantArn, tags...)) }),
			nativetest.Rule{
				Rule:  "the tenant database exists",
				Break: against(addTags(tenantArn, tag("team", "a"))),
				Code:  "TenantDatabaseNotFound",
			},
			nativetest.Rule{
				Rule: "a resource has at most 50 tags",
				Doc:  tagGuide,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					tags := []types.Tag{}
					for i := range maxTags {
						tags = append(tags, tag(fmt.Sprintf("key-%02d", i), "v"))
					}
					if err := readyTenant(func(in *rds.CreateTenantDatabaseInput) { in.Tags = tags })(ctx, f); err != nil {
						return err
					}
					if err := addTags(tenantArn, tag("key-00", "w"))(ctx, f); err != nil {
						return fmt.Errorf("replacing a value of 50 tags failed: %w", err)
					}
					if addTags(tenantArn, tag("extra", "v"))(ctx, f) == nil {
						return fmt.Errorf("the fake accepts a 51st tag")
					}
					return nil
				}),
			}),
		Unclear: codesUnclear + " The Tag page says a value is optional and also 1 to 256 characters; the fake accepts " +
			"an empty value. The tagging guide limits a resource to 50 tags without naming the error; the fake returns " +
			"InvalidParameterValue, for this call and CreateTenantDatabase.",
	},
	"RemoveTagsFromResource": {
		Doc: apiReference + "API_RemoveTagsFromResource.html",
		Rules: []nativetest.Rule{{
			Rule:  "the tenant database exists",
			Break: against(removeTags(tenantArn)),
			Code:  "TenantDatabaseNotFound",
		}},
		Unclear: "The page doesn't say what AWS does with a key the resource doesn't have; the fake ignores it, as the " +
			"handler's retries expect.",
	},
}
