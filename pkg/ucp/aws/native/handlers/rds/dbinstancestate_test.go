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
	"maps"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func dbStateSchema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func dbInstanceState(state string) map[string]any {
	return map[string]any{"DBInstanceIdentifier": testDBInstance, "State": state}
}

func Test_Suite_DBInstanceState(t *testing.T) {
	stopped := dbInstanceState("stopped")
	nativetest.Run(t, nativetest.Suite{
		TypeName:    dbInstanceStateType,
		Schema:      dbStateSchema(t, "aws-rds-dbinstancestate.json"),
		Metadata:    dbStateSchema(t, "aws-rds-dbinstancestate.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			putDBInstance(store, testDBInstance, "available", nil)
			return NewDBInstanceState(newFakeDBInstances(store))
		},
		IdentifierProperty: "DBInstanceIdentifier",
		Properties:         stopped,
		Identifier:         testDBInstance,
		Changed:            dbInstanceState("running"),
		// RDS stores the identifier in lowercase, so a template in another case names the same DB instance.
		SameResource:      []map[string]any{{"DBInstanceIdentifier": "App-DB", "State": "stopped"}},
		SameResourceNames: []string{"App-DB"},
		Invalid: []map[string]any{
			dbInstanceState("available"),
			{"DBInstanceIdentifier": testDBInstance, "State": 1},
			{"DBInstanceIdentifier": 5, "State": "running"},
		},
		OtherProduct: func(store *awsfake.CloudControl, identifier string) {
			putDBInstance(store, identifier, "available", map[string]any{"Engine": "neptune", "DBClusterIdentifier": "graph"})
		},
		// The DB instance is started from the console between deploys.
		ChangedOutside: &nativetest.ChangedOutside{
			Properties: stopped,
			Change: func(stored map[string]any) map[string]any {
				changed := maps.Clone(stored)
				setStatus(changed, "available")
				return changed
			},
			Restored: func(stored map[string]any) bool { return stored["DBInstanceStatus"] == "stopped" },
		},
		AlwaysPresent: true,
		CreateFails:   true,
		API:           dbInstanceStateAPI,
	})
}

// newDBInstanceState returns a handler with a fake API that has testDBInstance in status, with the extra fields set.
func newDBInstanceState(status string, extra map[string]any) (*DBInstanceState, *fakeDBInstances) {
	store := awsfake.NewCloudControl()
	putDBInstance(store, testDBInstance, status, extra)
	api := newFakeDBInstances(store)
	return NewDBInstanceState(api), api
}

// requireDBRejected requires err to be a rejected client failure whose message contains want.
func requireDBRejected(t *testing.T, err error, want string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, want)
}

func Test_DBInstanceState_Identifier(t *testing.T) {
	h := NewDBInstanceState(nil)
	id, err := h.Identifier(map[string]any{"DBInstanceIdentifier": "App-DB", "State": "running"})
	require.NoError(t, err)
	require.Equal(t, testDBInstance, id, "RDS stores the identifier in lowercase")
	id, err = h.Identifier(map[string]any{"DBInstanceIdentifier": testDBInstance})
	require.NoError(t, err, "`existing` leaves State out")
	require.Equal(t, testDBInstance, id)
	id, err = h.ResolveIdentifier(t.Context(), dbRegion, "APP-DB")
	require.NoError(t, err)
	require.Equal(t, testDBInstance, id)

	_, err = h.Identifier(map[string]any{"DBInstanceIdentifier": 5})
	requireDBRejected(t, err, "DBInstanceIdentifier of an AWS::RDS::DBInstanceState must be a string")
	_, err = h.Identifier(dbInstanceState("available"))
	requireDBRejected(t, err, `State of an AWS::RDS::DBInstanceState must be "running" or "stopped"`)
}

func Test_DBInstanceState_Read(t *testing.T) {
	for status, state := range map[string]string{
		"available":               "running",
		"backing-up":              "running",
		"modifying":               "running",
		"stopped":                 "stopped",
		"starting":                "starting",
		"stopping":                "stopping",
		"incompatible-parameters": "incompatible-parameters",
	} {
		h, _ := newDBInstanceState(status, nil)
		got, err := h.Read(t.Context(), dbRegion, testDBInstance)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"DBInstanceIdentifier": testDBInstance, "State": state, "DBInstanceStatus": status}, got)
	}

	h, _ := newDBInstanceState("deleting", nil)
	_, err := h.Read(t.Context(), dbRegion, testDBInstance)
	require.ErrorIs(t, err, native.ErrNotFound, "a DB instance being deleted can't run again")
	_, err = h.Read(t.Context(), dbRegion, "other-db")
	require.ErrorIs(t, err, native.ErrNotFound)

	h, _ = newDBInstanceState("available", map[string]any{"Engine": "docdb", "DBClusterIdentifier": "docs"})
	_, err = h.Read(t.Context(), dbRegion, testDBInstance)
	var other *native.OtherProductError
	require.ErrorAs(t, err, &other)
	require.Equal(t, native.OtherProductError{Identifier: testDBInstance, Product: "docdb"}, *other)
}

// Test_DBInstanceState_Cluster checks that a DB instance of a cluster is rejected with 400 before any change, saying
// to start or stop an Aurora cluster, and that RDS can't stop a Multi-AZ DB cluster.
func Test_DBInstanceState_Cluster(t *testing.T) {
	aurora := `The DB instance "app-db" belongs to the Aurora DB cluster "app-cluster", and RDS starts and stops it only with the whole cluster. Start or stop the cluster instead.`
	multiAZ := `The DB instance "app-db" belongs to the Multi-AZ DB cluster "app-cluster", which RDS can't stop or start.`
	for engine, want := range map[string]string{"aurora-postgresql": aurora, "aurora-mysql": aurora, "mysql": multiAZ, "postgres": multiAZ} {
		h, api := newDBInstanceState("available", map[string]any{"Engine": engine, "DBClusterIdentifier": "app-cluster"})
		_, err := h.Read(t.Context(), dbRegion, testDBInstance)
		requireDBRejected(t, err, want)
		_, err = h.Create(t.Context(), native.CreateRequest{Region: dbRegion, Properties: dbInstanceState("stopped")})
		requireDBRejected(t, err, want)
		requireDBRejected(t, h.Update(t.Context(), dbUpdateRequest(t, "", "stopped")), want)
		require.NotContains(t, api.calls, "StopDBInstance", engine)
	}
}

func Test_DBInstanceState_Create(t *testing.T) {
	// The read before the create found no DB instance, and RDS reports it now.
	h, api := newDBInstanceState("stopped", nil)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: dbRegion, Properties: dbInstanceState("running")})
	requireDBRejected(t, err, `RDS reported no DB instance "app-db" when the deploy read it, and reports it now. Deploy again.`)
	require.Equal(t, []string{"DescribeDBInstances"}, api.calls, "the create starts nothing")

	h, api = newDBInstanceState("deleting", nil)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: dbRegion, Properties: dbInstanceState("running")})
	requireDBRejected(t, err, `AWS has no RDS DB instance "app-db" that can run`)
	require.Equal(t, []string{"DescribeDBInstances"}, api.calls)

	for _, properties := range []map[string]any{{"DBInstanceIdentifier": 5, "State": "running"}, {"DBInstanceIdentifier": testDBInstance}} {
		h, api = newDBInstanceState("stopped", nil)
		_, err = h.Create(t.Context(), native.CreateRequest{Region: dbRegion, Properties: properties})
		requireDBRejected(t, err, "of an AWS::RDS::DBInstanceState must be")
		require.Empty(t, api.calls, "nothing reaches AWS")
	}
}

// dbUpdateRequest returns the update of testDBInstance to desired, with before as the status UpdateState read.
func dbUpdateRequest(t *testing.T, before string, desired string) native.UpdateRequest {
	patch, err := jsondiff.Compare(map[string]any{}, dbInstanceState(desired))
	require.NoError(t, err)
	return native.UpdateRequest{Region: dbRegion, Identifier: testDBInstance, Desired: dbInstanceState(desired), Patch: patch, State: before}
}

func Test_DBInstanceState_Update(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		desired string
		call    string
	}{
		{"stop an available DB instance", "available", "stopped", "StopDBInstance"},
		{"start a stopped DB instance", "stopped", "running", "StartDBInstance"},
		{"recover a DB instance whose KMS key RDS can't reach", recoverableStatus, "running", "StartDBInstance"},
		{"wait for a starting DB instance", "starting", "running", ""},
		{"wait for a DB instance being created", "creating", "running", ""},
		{"wait for a stopping DB instance", "stopping", "stopped", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, api := newDBInstanceState(tt.status, nil)
			before, err := h.UpdateState(t.Context(), dbRegion, testDBInstance)
			require.NoError(t, err)
			require.Equal(t, tt.status, before)
			if tt.status == "creating" {
				// The fake moves only a starting or stopping DB instance on, so the create finishes here.
				putDBInstance(api.store, testDBInstance, statusStarting, nil)
			}
			require.NoError(t, h.Update(t.Context(), dbUpdateRequest(t, before, tt.desired)))
			want := []string{"DescribeDBInstances", "DescribeDBInstances"}
			if tt.call != "" {
				want = append(want, tt.call)
			}
			require.Equal(t, want, api.calls)
			waitDBInstance(t, h, before, tt.desired)
		})
	}
}

// waitDBInstance reads the update's progress until it finishes, and requires the DB instance to be in want.
func waitDBInstance(t *testing.T, h *DBInstanceState, before string, want string) {
	t.Helper()
	for range 3 {
		progress, err := h.UpdateProgress(t.Context(), dbRegion, testDBInstance, before)
		require.NoError(t, err)
		if progress.Done {
			require.Nil(t, progress.Failure)
			require.Equal(t, want, stateOf(progress.State))
			return
		}
		require.Equal(t, want, progress.Waiting)
	}
	require.Fail(t, "the update didn't finish")
}

func Test_DBInstanceState_UpdateConflicts(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		desired string
		want    string
	}{
		{"stop a starting DB instance", "starting", "stopped", `The DB instance "app-db" is starting, so RDS can't make it stopped now. Deploy again once it is running.`},
		{"start a stopping DB instance", "stopping", "running", `The DB instance "app-db" is stopping, so RDS can't make it running now. Deploy again once it is stopped.`},
		{"stop a failed DB instance", "failed", "stopped", `The DB instance "app-db" is failed, so RDS can't make it stopped now.`},
		// The read before the update saw the DB instance starting, and UpdateState sees it backing up.
		{"a DB instance that changed while the deploy started", "backing-up", "running", `The DB instance "app-db" changed to backing-up while the deploy started. Deploy again.`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, api := newDBInstanceState(tt.status, nil)
			err := h.Update(t.Context(), dbUpdateRequest(t, tt.status, tt.desired))
			requireDBRejected(t, err, tt.want)
			if tt.status == "failed" {
				require.NotContains(t, err.Error(), "Deploy again")
			}
			require.Empty(t, api.calls, "nothing reaches AWS")
		})
	}

	t.Run("an update without state from before", func(t *testing.T) {
		h, api := newDBInstanceState("available", nil)
		require.NoError(t, h.Update(t.Context(), dbUpdateRequest(t, "", "stopped")))
		require.Contains(t, api.calls, "StopDBInstance")
	})
	t.Run("a deleted DB instance", func(t *testing.T) {
		h, api := newDBInstanceState("available", nil)
		api.store.Remove(dbInstanceStateType, testDBInstance)
		requireDBRejected(t, h.Update(t.Context(), dbUpdateRequest(t, "available", "stopped")), "it doesn't exist, or it is being deleted")
	})
	t.Run("an invalid state", func(t *testing.T) {
		h, api := newDBInstanceState("available", nil)
		requireDBRejected(t, h.Update(t.Context(), dbUpdateRequest(t, "available", "available")), "must be")
		require.Empty(t, api.calls)
	})
}

// Test_DBInstanceState_CantStop checks that RDS's error for a DB instance it can't stop is a 400 that names the DB
// instance and its status, and that an error RDS doesn't blame on the request stays as it is.
func Test_DBInstanceState_CantStop(t *testing.T) {
	h, _ := newDBInstanceState("available", map[string]any{"ReadReplica": true})
	err := h.Update(t.Context(), dbUpdateRequest(t, "available", "stopped"))
	requireDBRejected(t, err, `RDS can't stop the DB instance "app-db", which is available: `)
	require.ErrorContains(t, err, "InvalidDBInstanceState")

	h, _ = newDBInstanceState("available", map[string]any{"Engine": "sqlserver-se", "MultiAZ": true})
	err = h.Update(t.Context(), dbUpdateRequest(t, "available", "stopped"))
	requireDBRejected(t, err, `RDS can't stop the DB instance "app-db", which is available: `)

	h, _ = newDBInstanceState("backing-up", nil)
	err = h.Update(t.Context(), dbUpdateRequest(t, "backing-up", "stopped"))
	requireDBRejected(t, err, `RDS can't stop the DB instance "app-db", which is backing-up: `)

	h, api := newDBInstanceState("stopped", nil)
	api.errs["StartDBInstance"] = nativefake.ServerError("InternalFailure", "RDS failed")
	err = h.Update(t.Context(), dbUpdateRequest(t, "stopped", "running"))
	var failure *native.Failure
	require.False(t, errors.As(err, &failure), "the outcome of a server error is unknown")
	require.ErrorContains(t, err, "RDS failed")
}

func Test_DBInstanceState_Progress(t *testing.T) {
	h, api := newDBInstanceState("starting", nil)
	progress, err := h.Progress(t.Context(), dbRegion, testDBInstance, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "starting", Waiting: "running"}, progress)
	progress, err = h.Progress(t.Context(), dbRegion, testDBInstance, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{Done: true, State: "available"}, progress)

	putDBInstance(api.store, testDBInstance, "incompatible-parameters", nil)
	progress, err = h.Progress(t.Context(), dbRegion, testDBInstance, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.Equal(t, `the DB instance "app-db" is incompatible-parameters`, progress.Failure.Message)

	api.store.Remove(dbInstanceStateType, testDBInstance)
	_, err = h.Progress(t.Context(), dbRegion, testDBInstance, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_DBInstanceState_UpdateProgress(t *testing.T) {
	t.Run("a read that doesn't show the change yet", func(t *testing.T) {
		h, _ := newDBInstanceState("stopped", nil)
		progress, err := h.UpdateProgress(t.Context(), dbRegion, testDBInstance, "stopped")
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "stopped", Waiting: "running"}, progress)
	})

	t.Run("a start that ends in another status", func(t *testing.T) {
		h, api := newDBInstanceState("stopped", nil)
		api.failStart[testDBInstance] = "incompatible-parameters"
		require.NoError(t, h.Update(t.Context(), dbUpdateRequest(t, "stopped", "running")))
		progress, err := h.UpdateProgress(t.Context(), dbRegion, testDBInstance, "stopped")
		require.NoError(t, err)
		require.Equal(t, "running", progress.Waiting, "the DB instance is starting")
		progress, err = h.UpdateProgress(t.Context(), dbRegion, testDBInstance, "stopped")
		require.NoError(t, err)
		require.True(t, progress.Done)
		require.Equal(t, `the DB instance "app-db" is incompatible-parameters, not running`, progress.Failure.Message)
	})

	// UpdateState saw the DB instance backing up, the stop was sent once it was available, and the read doesn't show
	// the stop yet.
	t.Run("a read that shows another status in which the DB instance runs", func(t *testing.T) {
		h, _ := newDBInstanceState("available", nil)
		progress, err := h.UpdateProgress(t.Context(), dbRegion, testDBInstance, "backing-up")
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "available", Waiting: "stopped"}, progress)
	})

	t.Run("a started DB instance that backs up", func(t *testing.T) {
		h, _ := newDBInstanceState("backing-up", nil)
		progress, err := h.UpdateProgress(t.Context(), dbRegion, testDBInstance, "stopped")
		require.NoError(t, err)
		require.Equal(t, native.Progress{Done: true, State: "backing-up"}, progress, "a DB instance that backs up runs")
	})

	t.Run("state UCP can't read", func(t *testing.T) {
		h, _ := newDBInstanceState("stopping", nil)
		progress, err := h.UpdateProgress(t.Context(), dbRegion, testDBInstance, "")
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "stopping", Waiting: "stopped"}, progress, "it waits as Progress does")
	})

	t.Run("a deleted DB instance", func(t *testing.T) {
		h, api := newDBInstanceState("available", nil)
		api.store.Remove(dbInstanceStateType, testDBInstance)
		_, err := h.UpdateProgress(t.Context(), dbRegion, testDBInstance, "available")
		require.ErrorIs(t, err, native.ErrNotFound)
		_, err = h.UpdateState(t.Context(), dbRegion, testDBInstance)
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_DBInstanceState_DeleteAndList(t *testing.T) {
	h, api := newDBInstanceState("available", nil)
	require.NoError(t, h.Delete(t.Context(), dbRegion, testDBInstance))
	resources, err := h.List(t.Context(), dbRegion)
	require.NoError(t, err)
	require.Empty(t, resources)
	require.Empty(t, api.calls, "neither calls AWS")
	require.Equal(t, native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}, h.Options())
	require.Equal(t, dbInstanceStateType, h.TypeName())
}

// describeDBErr is an RDS API whose DescribeDBInstances fails.
type describeDBErr struct {
	DBInstanceAPI
}

func (describeDBErr) DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return nil, nativefake.ServerError("InternalFailure", "RDS failed")
}

func Test_DBInstanceState_DescribeFails(t *testing.T) {
	h := NewDBInstanceState(describeDBErr{})
	_, err := h.UpdateState(t.Context(), dbRegion, testDBInstance)
	require.ErrorContains(t, err, "RDS failed")
	_, err = h.Progress(t.Context(), dbRegion, testDBInstance, native.OperationCreate)
	require.ErrorContains(t, err, "RDS failed")
	require.ErrorContains(t, h.Update(t.Context(), dbUpdateRequest(t, "", "stopped")), "RDS failed")
}
