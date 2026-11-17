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
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func newCluster(t *testing.T) (*Cluster, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewCluster(api), api
}

func Test_Options(t *testing.T) {
	cluster, instance := NewCluster(nil), NewInstance(nil)
	require.Equal(t, clusterType, cluster.TypeName())
	require.Equal(t, instanceType, instance.TypeName())
	require.Equal(t, "DBClusterIdentifier", cluster.Options().NameProperty)
	require.Equal(t, "DBInstanceIdentifier", instance.Options().NameProperty)
	for _, o := range []native.Options{cluster.Options(), instance.Options()} {
		require.Zero(t, o.CreateTimeout+o.UpdateTimeout+o.DeleteTimeout, "UCP waits native.DefaultTimeout, because a restore from a large snapshot can take hours")
	}

	options := &docdb.Options{}
	inRegion("eu-west-2")(options)
	require.Equal(t, "eu-west-2", options.Region)

	id, err := cluster.Identifier(map[string]any{"DBClusterIdentifier": "App-DocDB"})
	require.NoError(t, err)
	require.Equal(t, "app-docdb", id)
}

func Test_Cluster_Restore(t *testing.T) {
	t.Run("from a snapshot", func(t *testing.T) {
		h, api := newCluster(t)
		id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{
			"DBClusterIdentifier": "Restored",
			"SnapshotIdentifier":  "snap-1",
			"Port":                27018,
			"Tags":                tags("team", "a"),
		}})
		require.NoError(t, err)
		require.Equal(t, "restored", id)
		require.Contains(t, api.calls, "RestoreDBClusterFromSnapshot")
		require.Equal(t, "snap-1", api.store.Resource(clusterType, "restored")["SnapshotIdentifier"])
	})

	t.Run("to a point in time", func(t *testing.T) {
		h, api := newCluster(t)
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{
			"DBClusterIdentifier":       "copy",
			"SourceDBClusterIdentifier": "source",
			"UseLatestRestorableTime":   true,
		}})
		require.NoError(t, err)
		require.Contains(t, api.calls, "RestoreDBClusterToPointInTime")
	})

	t.Run("a restore that sets what the restore can't take", func(t *testing.T) {
		h, api := newCluster(t)
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{
			"DBClusterIdentifier":   "restored",
			"SnapshotIdentifier":    "snap-1",
			"MasterUserPassword":    "secret-value",
			"BackupRetentionPeriod": 7,
		}})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
		require.Contains(t, failure.Message, "BackupRetentionPeriod, MasterUserPassword")
		require.NotContains(t, failure.Message, "secret-value")
		require.Empty(t, api.calls)

		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{
			"DBClusterIdentifier":       "copy",
			"SourceDBClusterIdentifier": "source",
			"EngineVersion":             "5.0",
		}})
		require.ErrorAs(t, err, &failure)
		require.Contains(t, failure.Message, "EngineVersion")
	})

	t.Run("AWS rejects the create", func(t *testing.T) {
		h, api := newCluster(t)
		api.errs["CreateDBCluster"] = fault("InvalidParameterValue")
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"DBClusterIdentifier": "app"}})
		require.ErrorContains(t, err, "InvalidParameterValue")
	})

	t.Run("a property of the wrong type", func(t *testing.T) {
		h, _ := newCluster(t)
		for _, properties := range []map[string]any{
			{"DBClusterIdentifier": "app", "Port": "not a number"},
			{"DBClusterIdentifier": "app", "SnapshotIdentifier": "s", "Port": "not a number"},
			{"DBClusterIdentifier": "app", "SourceDBClusterIdentifier": "s", "Port": "not a number"},
		} {
			_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
			require.ErrorContains(t, err, "failed to read the properties")
		}
	})
}

func deployed(t *testing.T, h *Cluster, properties map[string]any) {
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
}

func Test_Cluster_Update(t *testing.T) {
	t.Run("logs and removed values", func(t *testing.T) {
		h, api := newCluster(t)
		deployed(t, h, map[string]any{"DBClusterIdentifier": "app", "DeletionProtection": true, "EnableCloudwatchLogsExports": []any{"audit"}})
		current, err := h.Read(t.Context(), region, "app")
		require.NoError(t, err)

		desired := map[string]any{"DBClusterIdentifier": "app", "EnableCloudwatchLogsExports": []any{"profiler"}, "EngineVersion": "5.0"}
		err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "app", Current: current, Desired: desired, Patch: jsondiff.Patch{
			{Type: jsondiff.OperationRemove, Path: "/DeletionProtection"},
			{Type: jsondiff.OperationReplace, Path: "/EnableCloudwatchLogsExports/0", Value: "profiler"},
			{Type: jsondiff.OperationRemove, Path: "/PreferredBackupWindow"},
			{Type: jsondiff.OperationReplace, Path: "/EngineVersion", Value: "5.0"},
		}})
		require.NoError(t, err)
		stored := api.store.Resource(clusterType, "app")
		require.Equal(t, false, stored["DeletionProtection"], "a removed value AWS can't clear goes back to its default")
		require.Equal(t, []any{"profiler"}, stored["EnableCloudwatchLogsExports"])
		require.Equal(t, "5.0", stored["EngineVersion"])
	})

	t.Run("a cluster AWS no longer has", func(t *testing.T) {
		h, _ := newCluster(t)
		err := h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "gone", Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/Port", Value: 1}}})
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("a value of the wrong type", func(t *testing.T) {
		h, _ := newCluster(t)
		deployed(t, h, map[string]any{"DBClusterIdentifier": "app"})
		err := h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "app", Desired: map[string]any{"Port": "x"}, Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/Port", Value: "x"}}})
		require.ErrorContains(t, err, "failed to read the properties")
	})
}

func Test_LogChanges(t *testing.T) {
	changes := logChanges([]string{"audit", "profiler"}, []any{"profiler", "slow"})
	require.Equal(t, []string{"slow"}, changes.EnableLogTypes)
	require.Equal(t, []string{"audit"}, changes.DisableLogTypes)
	none := logChanges([]string{"audit"}, nil)
	require.Empty(t, none.EnableLogTypes)
	require.Equal(t, []string{"audit"}, none.DisableLogTypes)
}

func Test_Cluster_Delete(t *testing.T) {
	t.Run("Delete takes a final snapshot", func(t *testing.T) {
		h, api := newCluster(t)
		deployed(t, h, map[string]any{"DBClusterIdentifier": "app"})
		require.NoError(t, h.Delete(t.Context(), region, "app"))
		require.Len(t, api.snapshots, 1)
		require.Contains(t, api.snapshots[0], "app-final-")
	})

	t.Run("a 63-character identifier gets a snapshot name DocumentDB accepts", func(t *testing.T) {
		h, api := newCluster(t)
		identifier := "a" + strings.Repeat("b", 62)
		deployed(t, h, map[string]any{"DBClusterIdentifier": identifier})
		require.NoError(t, h.Delete(t.Context(), region, identifier))
		require.Len(t, api.snapshots, 1)
		require.Len(t, api.snapshots[0], 63)
	})

	t.Run("a snapshot name that exists fails the delete and keeps the cluster", func(t *testing.T) {
		h, api := newCluster(t)
		deployed(t, h, map[string]any{"DBClusterIdentifier": "app"})
		api.snapshots = []string{"taken"}
		err := h.DeleteWithToken(t.Context(), region, "app", "taken")
		require.ErrorContains(t, err, "DBClusterSnapshotAlreadyExistsFault")
		require.NotNil(t, api.store.Resource(clusterType, "app"))
	})

	t.Run("a cluster in another state", func(t *testing.T) {
		h, api := newCluster(t)
		deployed(t, h, map[string]any{"DBClusterIdentifier": "app"})
		api.errs["DeleteDBCluster"] = fault("InvalidDBClusterStateFault")
		err := h.DeleteWithToken(t.Context(), region, "app", "snap")
		require.ErrorContains(t, err, "InvalidDBClusterStateFault", "a cluster that isn't deleting keeps AWS's error")
	})

	t.Run("a cluster another product owns", func(t *testing.T) {
		h, api := newCluster(t)
		api.store.Put(clusterType, "app", map[string]any{"DBClusterIdentifier": "app", "Engine": "neptune"})
		var other *native.OtherProductError
		require.ErrorAs(t, h.DeleteWithToken(t.Context(), region, "app", "snap"), &other)
		require.Empty(t, api.snapshots)
	})

	t.Run("a cluster that disappears while deleting", func(t *testing.T) {
		h, api := newCluster(t)
		deployed(t, h, map[string]any{"DBClusterIdentifier": "app"})
		api.errs["DeleteDBCluster"] = fault("InvalidDBClusterStateFault")
		api.goneAfterDelete = true
		api.calls = nil
		err := h.DeleteWithToken(t.Context(), region, "app", "snap")
		require.Contains(t, api.calls, "DeleteDBCluster")
		require.ErrorIs(t, err, native.ErrNotFound, "the cluster is gone, so the delete is done")
	})
}

func Test_Cluster_ReadErrors(t *testing.T) {
	h, api := newCluster(t)
	deployed(t, h, map[string]any{"DBClusterIdentifier": "app"})

	api.errs["ListTagsForResource"] = errors.New("throttled")
	_, err := h.Read(t.Context(), region, "app")
	require.ErrorContains(t, err, "throttled")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "throttled")

	api.errs["DescribeDBClusters"] = errors.New("unavailable")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "unavailable")
	_, err = h.Progress(t.Context(), region, "app", native.OperationCreate)
	require.ErrorContains(t, err, "unavailable")
}

// emptyAPI returns no clusters or instances for a lookup by identifier, which AWS does when a filter matches nothing.
type emptyAPI struct {
	*fakeAPI
}

func (e emptyAPI) DescribeDBClusters(ctx context.Context, in *docdb.DescribeDBClustersInput, _ ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
	return &docdb.DescribeDBClustersOutput{}, nil
}

func (e emptyAPI) DescribeDBInstances(ctx context.Context, in *docdb.DescribeDBInstancesInput, _ ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
	return &docdb.DescribeDBInstancesOutput{}, nil
}

func Test_EmptyDescribe(t *testing.T) {
	api := emptyAPI{newFakeAPI(awsfake.NewCloudControl())}
	_, err := NewCluster(api).Read(t.Context(), region, "app")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = NewInstance(api).Read(t.Context(), region, "app-1")
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_ListPages(t *testing.T) {
	h, api := newCluster(t)
	deployed(t, h, map[string]any{"DBClusterIdentifier": "one"})
	api.store.Put(clusterType, "aurora", map[string]any{"DBClusterIdentifier": "aurora", "Engine": "aurora-postgresql"})
	paged := &pagedAPI{fakeAPI: api}
	list, err := NewCluster(paged).List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "one", list[0].Identifier)
	require.Equal(t, 2, paged.pages, "List reads every page")
}

// pagedAPI returns the clusters a lookup by filter finds on a second page.
type pagedAPI struct {
	*fakeAPI
	pages int
}

func (p *pagedAPI) DescribeDBClusters(ctx context.Context, in *docdb.DescribeDBClustersInput, opts ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
	p.pages++
	if in.Marker == nil {
		return &docdb.DescribeDBClustersOutput{Marker: aws.String("next")}, nil
	}
	return p.fakeAPI.DescribeDBClusters(ctx, &docdb.DescribeDBClustersInput{}, opts...)
}

func Test_Instance_Errors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewInstance(api)

	api.errs["CreateDBInstance"] = fault("DBClusterNotFoundFault")
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"DBInstanceIdentifier": "one"}})
	require.ErrorContains(t, err, "DBClusterNotFoundFault")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"DBInstanceIdentifier": "one", "PromotionTier": "x"}})
	require.ErrorContains(t, err, "failed to read the properties")
	delete(api.errs, "CreateDBInstance")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"DBInstanceIdentifier": "one", "DBClusterIdentifier": "app"}})
	require.NoError(t, err)
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "one", Desired: map[string]any{"PromotionTier": "x"}, Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/PromotionTier"}}})
	require.NoError(t, err, "PromotionTier isn't a property of the type, so it is ignored")
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "gone", Patch: jsondiff.Patch{}})
	require.ErrorIs(t, err, native.ErrNotFound)

	api.errs["DeleteDBInstance"] = fault("InvalidDBInstanceState")
	require.ErrorContains(t, h.Delete(t.Context(), region, "one"), "InvalidDBInstanceState")
	require.ErrorIs(t, h.Delete(t.Context(), region, "gone"), native.ErrNotFound)

	api.errs["ListTagsForResource"] = errors.New("throttled")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "throttled")
	api.errs["DescribeDBInstances"] = errors.New("unavailable")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "unavailable")
	_, err = h.Progress(t.Context(), region, "one", native.OperationCreate)
	require.ErrorContains(t, err, "unavailable")
}

// Test_Cluster_UpdatesEveryUpdatableProperty checks that Update applies every property the schema lets a deploy
// change, so UCP never records a change AWS didn't get. Any other property must be create-only, which UCP rejects.
func Test_Cluster_UpdatesEveryUpdatableProperty(t *testing.T) {
	require.ElementsMatch(t, nativetest.Updatable(t, schema(t, "aws-docdb-dbcluster.json")), append(slices.Clone(modifiable), "Tags"))
}

// Test_Instance_UpdatesEveryUpdatableProperty is Test_Cluster_UpdatesEveryUpdatableProperty for instances.
func Test_Instance_UpdatesEveryUpdatableProperty(t *testing.T) {
	require.ElementsMatch(t, nativetest.Updatable(t, schema(t, "aws-docdb-dbinstance.json")), append(slices.Clone(modifiableInstance), "Tags"))
}

// Test_Instance_RemovedDefaults checks that a removed property with an AWS default goes back to it, and one without
// is left as AWS has it.
func Test_Instance_RemovedDefaults(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewInstance(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{
		"DBInstanceIdentifier": "one", "DBClusterIdentifier": "app", "AutoMinorVersionUpgrade": false,
		"EnablePerformanceInsights": true, "PreferredMaintenanceWindow": "sun:05:00-sun:05:30",
	}})
	require.NoError(t, err)
	api.calls = nil

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "one", Desired: map[string]any{}, Patch: jsondiff.Patch{
		{Type: jsondiff.OperationRemove, Path: "/AutoMinorVersionUpgrade"},
		{Type: jsondiff.OperationRemove, Path: "/EnablePerformanceInsights"},
		{Type: jsondiff.OperationRemove, Path: "/PreferredMaintenanceWindow"},
	}})
	require.NoError(t, err)
	require.Equal(t, []string{"DescribeDBInstances", "ModifyDBInstance"}, api.calls)
	got := api.store.Resource(instanceType, "one")
	require.Equal(t, true, got["AutoMinorVersionUpgrade"])
	require.Equal(t, false, got["EnablePerformanceInsights"])
	require.Equal(t, "sun:05:00-sun:05:30", got["PreferredMaintenanceWindow"], "the window has no default, so AWS keeps it")
}

// Test_Instance_CertificateRotationRestart checks that a change of only the CA certificate sends the template's
// CertificateRotationRestart, which is write-only and so not in the patch when it didn't change. Without it, DocumentDB
// restarts the instance whatever the template says.
func Test_Instance_CertificateRotationRestart(t *testing.T) {
	for _, restart := range []bool{false, true} {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewInstance(api)
		properties := map[string]any{"DBInstanceIdentifier": "one", "DBClusterIdentifier": "app", "CACertificateIdentifier": "rds-ca-2019", "CertificateRotationRestart": restart}
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
		require.NoError(t, err)

		desired := map[string]any{"DBInstanceIdentifier": "one", "DBClusterIdentifier": "app", "CACertificateIdentifier": "rds-ca-rsa2048-g1", "CertificateRotationRestart": restart}
		require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "one", Desired: desired, Patch: jsondiff.Patch{
			{Type: jsondiff.OperationReplace, Path: "/CACertificateIdentifier", Value: "rds-ca-rsa2048-g1"},
		}}))
		require.NotNil(t, api.lastModifyInstance.CertificateRotationRestart, "restart %v", restart)
		require.Equal(t, restart, *api.lastModifyInstance.CertificateRotationRestart)
	}

	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewInstance(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"DBInstanceIdentifier": "one", "DBClusterIdentifier": "app"}})
	require.NoError(t, err)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "one", Desired: map[string]any{"CACertificateIdentifier": "rds-ca-rsa2048-g1"}, Patch: jsondiff.Patch{
		{Type: jsondiff.OperationAdd, Path: "/CACertificateIdentifier", Value: "rds-ca-rsa2048-g1"},
	}}))
	require.Nil(t, api.lastModifyInstance.CertificateRotationRestart, "a template without the flag leaves DocumentDB's default")
}

func Test_SnapshotName(t *testing.T) {
	at := time.Date(2026, 11, 5, 5, 27, 51, 0, time.UTC)
	h := NewCluster(nil)
	h.now = func() time.Time { return at }
	name := func(identifier string) string {
		token, err := h.DeleteToken(t.Context(), "us-west-2", identifier)
		require.NoError(t, err)
		return token
	}

	require.Equal(t, "app-docdb-final-20261105-052751", name("app-docdb"), "a name that fits is unchanged")
	fits := strings.Repeat("a", 41)
	require.Equal(t, fits+"-final-20261105-052751", name(fits), "41 characters fit")

	long := "a" + strings.Repeat("b", 62)
	got := name(long)
	require.Len(t, got, 63)
	require.True(t, strings.HasPrefix(got, "a"+strings.Repeat("b", 31)+"-"))
	require.True(t, strings.HasSuffix(got, "-final-20261105-052751"), "the time stays")
	require.True(t, validSnapshotName(got))

	other := "a" + strings.Repeat("b", 61) + "c"
	require.NotEqual(t, got, name(other), "identifiers that start the same get different names")

	hyphen := strings.Repeat("a", 31) + "-" + strings.Repeat("b", 31)
	require.True(t, validSnapshotName(name(hyphen)), "a hyphen at the cut isn't doubled: %s", name(hyphen))

	h.now = func() time.Time { return at.Add(time.Second) }
	require.NotEqual(t, got, name(long), "each delete gets its own name")

	saved := long + "-final-20261105-052751"
	require.Equal(t, got, snapshotName(saved), "a long name an older version saved is shortened the same way")
	require.Equal(t, got, snapshotName(got))
}

func Test_Fake_SnapshotNameRules(t *testing.T) {
	for _, tt := range []struct {
		name  string
		valid bool
	}{
		{"a", true},
		{"app-final-1", true},
		{strings.Repeat("a", 63), true},
		{strings.Repeat("a", 64), false},
		{"1app", false},
		{"app--final", false},
		{"app-", false},
		{"app_final", false},
	} {
		require.Equal(t, tt.valid, validSnapshotName(tt.name), tt.name)
	}
}
