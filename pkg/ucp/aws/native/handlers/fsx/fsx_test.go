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

package fsx

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/fsx/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const testRegion = "us-west-2"

func schema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/aws-fsx-filesystem.json")
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) of(h native.Handler) *fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api[h]
}

// all returns every fake API the suite made.
func (f *fakes) all() []*fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Collect(maps.Values(f.api))
}

// with returns a copy of properties with changes applied, where a nil value removes the property.
func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func tags(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"Key": pairs[i], "Value": pairs[i+1]})
	}
	return out
}

// jsonValue returns v as encoding/json reads it, as UCP passes properties to a handler.
func jsonValue(t *testing.T, v any) map[string]any {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

func lustre(weeklyMaintenance string) map[string]any {
	return map[string]any{
		"FileSystemType":   "LUSTRE",
		"StorageCapacity":  1200,
		"SubnetIds":        []any{"subnet-0a1"},
		"SecurityGroupIds": []any{"sg-0a1"},
		"KmsKeyId":         "1234abcd-12ab-34cd-56ef-1234567890ab",
		"LustreConfiguration": map[string]any{
			"DeploymentType":             "PERSISTENT_2",
			"PerUnitStorageThroughput":   125,
			"WeeklyMaintenanceStartTime": weeklyMaintenance,
		},
		"Tags": tags("owner", "", "team", "a"),
	}
}

// requireOneBackupEach requires every fake to have taken at most one final backup of each file system.
func requireOneBackupEach(t *testing.T, fs *fakes) int {
	total := 0
	for _, api := range fs.all() {
		seen := map[string]bool{}
		for _, b := range api.backups {
			require.False(t, seen[b.fileSystem], "a second final backup of %s", b.fileSystem)
			seen[b.fileSystem] = true
			total++
		}
	}
	return total
}

func Test_Suite_LustreFileSystem(t *testing.T) {
	var fs fakes
	properties := lustre("1:05:00")
	nativetest.Run(t, nativetest.Suite{
		TypeName: fileSystemType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewFileSystem(api), api)
		},
		IdentifierProperty: "Id",
		WriteOnly:          []string{"SecurityGroupIds"},
		Properties:         properties,
		Changed:            with(properties, map[string]any{"LustreConfiguration": lustre("2:05:00")["LustreConfiguration"]}),
		Removable:          "Tags",
		CreateOnlyChange:   with(properties, map[string]any{"SubnetIds": []any{"subnet-0b2"}}),
		OtherForm:          map[string]any{"KmsKeyId": "arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"},
		MultiCall:          with(properties, map[string]any{"StorageCapacity": 2400, "Tags": tags("env", "dev")}),
		Calls:              3,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
	})
	require.Positive(t, requireOneBackupEach(t, &fs), "deleting a persistent Lustre file system takes a final backup")
}

// Test_Suite_OntapFileSystem runs the suite for an ONTAP file system, whose admin password is write-only and can
// change. FSx takes no final backup of it.
func Test_Suite_OntapFileSystem(t *testing.T) {
	var fs fakes
	ontap := func(throughput int, password string) map[string]any {
		return map[string]any{
			"FileSystemType":   "ONTAP",
			"StorageCapacity":  1024,
			"SubnetIds":        []any{"subnet-0a1"},
			"SecurityGroupIds": []any{"sg-0a1"},
			"OntapConfiguration": map[string]any{
				"DeploymentType":     "SINGLE_AZ_1",
				"ThroughputCapacity": throughput,
				"FsxAdminPassword":   password,
			},
			"Tags": tags("team", "a"),
		}
	}
	properties := ontap(128, "Fsx-admin-one")
	nativetest.Run(t, nativetest.Suite{
		TypeName: fileSystemType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewFileSystem(api), api)
		},
		IdentifierProperty: "Id",
		WriteOnly:          []string{"SecurityGroupIds", "OntapConfiguration/FsxAdminPassword"},
		Properties:         properties,
		Changed:            ontap(256, "Fsx-admin-one"),
		ChangedWriteOnly:   ontap(128, "Fsx-admin-two"),
		Removable:          "Tags",
		CreateOnlyChange:   with(properties, map[string]any{"FileSystemType": "WINDOWS"}),
		MultiCall:          with(ontap(256, "Fsx-admin-three"), map[string]any{"Tags": tags("env", "dev")}),
		Calls:              3,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
	})
	require.Zero(t, requireOneBackupEach(t, &fs), "FSx takes no final backup of an ONTAP file system")
}

// Test_Suite_WindowsFileSystem runs the suite for a Windows file system joined to a self-managed Active Directory,
// whose password is write-only and can change. A change to its deployment type, a nested create-only property, is
// rejected. Deleting it takes a final backup.
func Test_Suite_WindowsFileSystem(t *testing.T) {
	var fs fakes
	windows := func(deployment string, throughput int, password string) map[string]any {
		return map[string]any{
			"FileSystemType":   "WINDOWS",
			"StorageCapacity":  32,
			"SubnetIds":        []any{"subnet-0a1"},
			"SecurityGroupIds": []any{"sg-0a1"},
			"WindowsConfiguration": map[string]any{
				"DeploymentType":     deployment,
				"ThroughputCapacity": throughput,
				"SelfManagedActiveDirectoryConfiguration": map[string]any{
					"DomainName": "corp.example.com",
					"UserName":   "fsx-joiner",
					"Password":   password,
					"DnsIps":     []any{"10.0.0.10", "10.0.0.11"},
				},
			},
			"Tags": tags("team", "a"),
		}
	}
	properties := windows("SINGLE_AZ_2", 32, "Ad-join-one")
	nativetest.Run(t, nativetest.Suite{
		TypeName: fileSystemType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewFileSystem(api), api)
		},
		IdentifierProperty: "Id",
		WriteOnly:          []string{"SecurityGroupIds", "WindowsConfiguration/SelfManagedActiveDirectoryConfiguration/Password"},
		Properties:         properties,
		Changed:            windows("SINGLE_AZ_2", 64, "Ad-join-one"),
		ChangedWriteOnly:   windows("SINGLE_AZ_2", 32, "Ad-join-two"),
		Removable:          "Tags",
		CreateOnlyChange:   windows("MULTI_AZ_1", 32, "Ad-join-one"),
	})
	require.Positive(t, requireOneBackupEach(t, &fs), "deleting a Windows file system takes a final backup")
}

// create creates a file system from properties through the handler and returns its ID, once it is available.
func create(t *testing.T, h *FileSystem, properties map[string]any) string {
	id, err := h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: jsonValue(t, properties), ClientToken: uuid.New().String()})
	require.NoError(t, err)
	require.True(t, finish(t, h, id, native.OperationCreate).Done)
	return id
}

// finish reads the progress of an operation until it is done.
func finish(t *testing.T, h *FileSystem, id string, op native.Operation) native.Progress {
	for range 5 {
		progress, err := h.Progress(t.Context(), testRegion, id, op)
		require.NoError(t, err)
		if progress.Done {
			return progress
		}
	}
	t.Fatal("the operation did not finish")
	return native.Progress{}
}

// update makes the handler update the file system to desired, with the patch UCP computes. UCP leaves out the
// write-only values in unsent, whose fingerprints show they didn't change.
func update(t *testing.T, h *FileSystem, id string, base map[string]any, desired map[string]any, unsent ...string) error {
	current, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	desired = jsonValue(t, desired)
	compared := jsonValue(t, desired)
	for _, path := range append(unsent, "SecurityGroupIds", "BackupId") {
		segments := strings.Split(path, "/")
		parent := compared
		for _, s := range segments[:len(segments)-1] {
			parent = asMap(parent[s])
		}
		delete(parent, segments[len(segments)-1])
	}
	a, err := json.Marshal(current)
	require.NoError(t, err)
	b, err := json.Marshal(compared)
	require.NoError(t, err)
	removed := awsoperations.RemovedPaths(jsonValue(t, base), desired)
	patch, err := awsoperations.GeneratePatchWithOptions(a, b, []byte(schema(t)), awsoperations.PatchOptions{Removed: removed, Equivalent: equivalent})
	require.NoError(t, err)
	return h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: id, Current: current, Desired: desired, Patch: patch})
}

// requireDeleted reads the progress of a delete until FSx no longer has the file system.
func requireDeleted(t *testing.T, h *FileSystem, id string) {
	for range 5 {
		progress, err := h.Progress(t.Context(), testRegion, id, native.OperationDelete)
		if err != nil {
			require.ErrorIs(t, err, native.ErrNotFound)
			return
		}
		require.Equal(t, native.Progress{State: "DELETING", Waiting: "deleted"}, progress)
	}
	t.Fatal("the delete did not finish")
}

func Test_DeleteRetryTakesOneBackup(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewFileSystem(api)
	id := create(t, h, map[string]any{
		"FileSystemType": "OPENZFS", "StorageCapacity": 64, "SubnetIds": []any{"subnet-0a1"},
		"OpenZFSConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_1", "ThroughputCapacity": 64},
		"Tags":                 tags("team", "a"),
	})
	token, err := h.DeleteToken(t.Context(), testRegion, id)
	require.NoError(t, err)
	other, err := h.DeleteToken(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.NotEqual(t, token, other, "each delete gets its own token")

	require.NoError(t, h.DeleteWithToken(t.Context(), testRegion, id, token))
	require.NoError(t, h.DeleteWithToken(t.Context(), testRegion, id, token), "a retry after a lost response is the same delete")
	require.Equal(t, 1, api.backupsOf(id))
	require.Equal(t, []types.Tag{{Key: aws.String("team"), Value: aws.String("a")}}, api.backups[0].tags, "the backup has the file system's tags, without the operation tag")

	progress, err := h.Progress(t.Context(), testRegion, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "DELETING", Waiting: "deleted"}, progress)
	_, err = h.Progress(t.Context(), testRegion, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.DeleteWithToken(t.Context(), testRegion, id, token), native.ErrNotFound)
	require.Equal(t, 1, api.backupsOf(id), "Radius never deletes the backup")
}

func Test_FinalBackupByType(t *testing.T) {
	base := map[string]any{"StorageCapacity": 1200, "SubnetIds": []any{"subnet-0a1"}}
	lustre := func(deployment string, extra map[string]any) map[string]any {
		config := map[string]any{"DeploymentType": deployment}
		maps.Copy(config, extra)
		return with(base, map[string]any{"FileSystemType": "LUSTRE", "LustreConfiguration": config})
	}
	tests := []struct {
		name         string
		properties   map[string]any
		associations int
		backup       bool
	}{
		{"Windows", with(base, map[string]any{"FileSystemType": "WINDOWS", "WindowsConfiguration": map[string]any{"ThroughputCapacity": 32}}), 0, true},
		{"OpenZFS", with(base, map[string]any{"FileSystemType": "OPENZFS", "OpenZFSConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_1"}}), 0, true},
		{"persistent Lustre", lustre("PERSISTENT_1", nil), 0, true},
		{"persistent Lustre with no association", lustre("PERSISTENT_2", nil), 0, true},
		{"persistent Lustre linked to S3 by an association", lustre("PERSISTENT_2", nil), 1, false},
		{"persistent Lustre linked to S3 by its import path", lustre("PERSISTENT_1", map[string]any{"ImportPath": "s3://bucket"}), 0, false},
		{"scratch Lustre", lustre("SCRATCH_2", nil), 0, false},
		{"ONTAP", with(base, map[string]any{"FileSystemType": "ONTAP", "OntapConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_1"}}), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			h := NewFileSystem(api)
			id := create(t, h, tt.properties)
			api.associations[id] = tt.associations
			require.NoError(t, h.Delete(t.Context(), testRegion, id))
			want := 0
			if tt.backup {
				want = 1
			}
			require.Equal(t, want, api.backupsOf(id))
			requireDeleted(t, h, id)
		})
	}

	t.Run("OpenZFS options the template set", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		zfs := map[string]any{"DeploymentType": "SINGLE_AZ_1", "Options": []any{"DELETE_CHILD_VOLUMES_AND_SNAPSHOTS"}}
		withOptions := create(t, h, with(base, map[string]any{"FileSystemType": "OPENZFS", "OpenZFSConfiguration": zfs, "Tags": tags("team", "a")}))
		without := create(t, h, with(base, map[string]any{"FileSystemType": "OPENZFS", "OpenZFSConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_1"}}))

		read, err := h.Read(t.Context(), testRegion, withOptions)
		require.NoError(t, err)
		require.Equal(t, tags("team", "a"), read["Tags"], "Read leaves the delete options tag out")

		require.NoError(t, h.Delete(t.Context(), testRegion, withOptions))
		require.NoError(t, h.Delete(t.Context(), testRegion, without))
		require.Equal(t, []types.DeleteFileSystemOpenZFSOption{types.DeleteFileSystemOpenZFSOptionDeleteChildVolumesAndSnapshots}, api.deleteOptions[withOptions])
		require.Nil(t, api.deleteOptions[without], "a template without Options sends none")
		for _, b := range api.backups {
			for _, tag := range b.tags {
				require.NotEqual(t, deleteOptionsTag, aws.ToString(tag.Key), "the final backup doesn't get the tag")
			}
		}
	})

	t.Run("associations can't be read", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		id := create(t, h, lustre("PERSISTENT_2", nil))
		api.errs["DescribeDataRepositoryAssociations"] = fault("AccessDenied", "denied")
		require.ErrorContains(t, h.Delete(t.Context(), testRegion, id), "denied")
		require.NotContains(t, api.calls, "DeleteFileSystem")
	})
}

func Test_FinalBackupFails(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewFileSystem(api)
	id := create(t, h, lustre("1:05:00"))
	api.failBackup = true
	token, err := h.DeleteToken(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.NoError(t, h.DeleteWithToken(t.Context(), testRegion, id, token))

	progress := finish(t, h, id, native.OperationDelete)
	require.Equal(t, "AVAILABLE", progress.State)
	require.Equal(t, "FSx stopped deleting the file system, which is AVAILABLE: The final backup failed.", progress.Failure.Message)
	_, err = h.Read(t.Context(), testRegion, id)
	require.NoError(t, err, "the file system stays")

	require.NoError(t, h.DeleteWithToken(t.Context(), testRegion, id, token), "FSx answers the same token with its first response")
	require.Equal(t, "FSx stopped deleting the file system, which is AVAILABLE: The final backup failed.", finish(t, h, id, native.OperationDelete).Failure.Message,
		"and doesn't delete again")
	require.Zero(t, api.backupsOf(id))

	// UCP clears the record's delete token after a failed delete, so the next DELETE sends a new one.
	retry, err := h.DeleteToken(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.NotEqual(t, token, retry)
	require.NoError(t, h.DeleteWithToken(t.Context(), testRegion, id, retry))
	require.Equal(t, 1, api.backupsOf(id))
	requireDeleted(t, h, id)
}

func Test_Create(t *testing.T) {
	ontap := map[string]any{
		"FileSystemType": "ONTAP", "StorageCapacity": 1024, "SubnetIds": []any{"subnet-0a1"},
		"OntapConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_1", "ThroughputCapacity": 128, "FsxAdminPassword": "Fsx-admin-one"},
	}
	request := native.CreateRequest{Region: testRegion, Properties: jsonValue(t, ontap), ClientToken: "token-a"}

	t.Run("a resent create returns the first file system", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		first, err := h.Create(t.Context(), request)
		require.NoError(t, err)
		again, err := h.Create(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, first, again)

		withoutPassword := request
		withoutPassword.Properties = jsonValue(t, with(ontap, map[string]any{"OntapConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_1", "ThroughputCapacity": 128}}))
		again, err = h.Create(t.Context(), withoutPassword)
		require.NoError(t, err, "FSx rejects the token with other properties, so the handler finds the file system by its tag")
		require.Equal(t, first, again)
		require.Equal(t, 1, api.store.Count(fileSystemType))
	})

	t.Run("a token FSx rejects with no tagged file system", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		api.errs["CreateFileSystem"] = fault("IncompatibleParameterError", "other parameters")
		_, err := h.Create(t.Context(), request)
		require.ErrorContains(t, err, "other parameters")

		delete(api.errs, "CreateFileSystem")
		_, err = h.Create(t.Context(), request)
		require.NoError(t, err)
		other := request
		other.ClientToken = "token-b"
		id, err := h.Create(t.Context(), other)
		require.NoError(t, err)
		api.tokens[id] = "token-a"
		api.errs["CreateFileSystem"] = fault("IncompatibleParameterError", "other parameters")
		_, err = h.Create(t.Context(), request)
		require.ErrorContains(t, err, "found 2 FSx file systems")

		api.errs["DescribeFileSystems"] = fault("ThrottlingException", "slow down")
		_, err = h.Create(t.Context(), request)
		require.ErrorContains(t, err, "slow down")
	})

	t.Run("restore from a backup", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		restore := map[string]any{"FileSystemType": "LUSTRE", "BackupId": "backup-0a1", "SubnetIds": []any{"subnet-0a1"}, "Tags": tags("team", "a")}
		id, err := h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: jsonValue(t, restore), ClientToken: "token-r"})
		require.NoError(t, err)
		require.Contains(t, api.calls, "CreateFileSystemFromBackup")
		again, err := h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: jsonValue(t, restore), ClientToken: "token-r"})
		require.NoError(t, err)
		require.Equal(t, id, again)

		_, err = h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: jsonValue(t, with(restore, map[string]any{"OntapConfiguration": map[string]any{}})), ClientToken: "t"})
		failure := &native.Failure{}
		require.ErrorAs(t, err, &failure)
		require.Equal(t, "An FSx file system restored from BackupId can't also set OntapConfiguration.", failure.Message)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)

		api.errs["CreateFileSystemFromBackup"] = fault("BackupNotFound", "no backup")
		_, err = h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: jsonValue(t, restore), ClientToken: "token-s"})
		require.ErrorContains(t, err, "no backup")
	})

	t.Run("errors", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		_, err := h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: map[string]any{"StorageCapacity": "big"}, ClientToken: "t"})
		require.Error(t, err)
		_, err = h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: map[string]any{"BackupId": "b", "StorageCapacity": "big"}, ClientToken: "t"})
		require.Error(t, err)
		api.errs["CreateFileSystem"] = fault("BadRequest", "invalid subnet")
		_, err = h.Create(t.Context(), request)
		require.ErrorContains(t, err, "invalid subnet")

		_, err = NewFileSystem(noFileSystem{api}).Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: map[string]any{}, ClientToken: "t"})
		require.ErrorContains(t, err, "returned no file system ID")
	})
}

func Test_Read_Passwords(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewFileSystem(api)
	ids := []string{
		create(t, h, map[string]any{
			"FileSystemType": "ONTAP", "StorageCapacity": 1024, "SubnetIds": []any{"subnet-0a1"},
			"OntapConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_1", "ThroughputCapacity": 128, "FsxAdminPassword": "ontap-admin-secret"},
		}),
		create(t, h, map[string]any{
			"FileSystemType": "WINDOWS", "StorageCapacity": 32, "SubnetIds": []any{"subnet-0a1"},
			"WindowsConfiguration": map[string]any{
				"ThroughputCapacity": 32, "Aliases": []any{"files.example.com"},
				"SelfManagedActiveDirectoryConfiguration": map[string]any{
					"DomainName": "example.com", "UserName": "joiner", "Password": "ad-join-secret", "DnsIps": []any{"10.0.0.1"},
					"DomainJoinServiceAccountSecret": "arn:aws:secretsmanager:us-west-2:111122223333:secret:join",
				},
			},
		}),
	}
	fs, ok := api.fileSystem(ids[0])
	require.True(t, ok)
	require.Equal(t, "ontap-admin-secret", aws.ToString(fs.OntapConfiguration.FsxAdminPassword), "the fake returns the password, as if AWS did")

	listed, err := h.List(t.Context(), testRegion)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	for _, id := range ids {
		read, err := h.Read(t.Context(), testRegion, id)
		require.NoError(t, err)
		for _, v := range []any{read, listed} {
			b, err := json.Marshal(v)
			require.NoError(t, err)
			require.NotContains(t, string(b), "ontap-admin-secret")
			require.NotContains(t, string(b), "ad-join-secret")
		}
	}
	windows, err := h.Read(t.Context(), testRegion, ids[1])
	require.NoError(t, err)
	ad := asMap(asMap(windows["WindowsConfiguration"])["SelfManagedActiveDirectoryConfiguration"])
	require.Equal(t, "arn:aws:secretsmanager:us-west-2:111122223333:secret:join", ad["DomainJoinServiceAccountSecret"], "a pointer to a secret is not a secret")
	require.Equal(t, []any{"files.example.com"}, asMap(windows["WindowsConfiguration"])["Aliases"])
}

func Test_Read_Configurations(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewFileSystem(api)
	properties := []map[string]any{
		{
			"FileSystemType": "LUSTRE", "StorageCapacity": 1200, "SubnetIds": []any{"subnet-0a1"},
			"LustreConfiguration": map[string]any{"DeploymentType": "PERSISTENT_1", "PerUnitStorageThroughput": 50, "ImportPath": "s3://bucket/data", "DataCompressionType": "LZ4"},
		},
		{
			"FileSystemType": "OPENZFS", "StorageCapacity": 64, "SubnetIds": []any{"subnet-0a1"},
			"OpenZFSConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_1", "ThroughputCapacity": 64, "RouteTableIds": []any{"rtb-1"}, "Options": []any{"DELETE_CHILD_VOLUMES_AND_SNAPSHOTS"}},
		},
	}
	for _, p := range properties {
		id := create(t, h, p)
		read, err := h.Read(t.Context(), testRegion, id)
		require.NoError(t, err)
		for k, v := range awsapi.Without(jsonValue(t, p), "OpenZFSConfiguration") {
			require.Equal(t, v, read[k], k)
		}
		require.Equal(t, id, read["Id"])
		require.Equal(t, arnPrefix+id, read["ResourceARN"])
	}
	listed, err := h.List(t.Context(), testRegion)
	require.NoError(t, err)
	zfs := listed[1].Properties
	require.Equal(t, "fsvol-0123", zfs["RootVolumeId"])
	require.Equal(t, map[string]any{"DeploymentType": "SINGLE_AZ_1", "ThroughputCapacity": 64.0, "RouteTableIds": []any{"rtb-1"}}, zfs["OpenZFSConfiguration"], "Options is write-only")
	require.Equal(t, "abcdef", listed[0].Properties["LustreMountName"])
}

func Test_Update(t *testing.T) {
	ontap := func(extra map[string]any) map[string]any {
		config := map[string]any{"DeploymentType": "MULTI_AZ_1", "ThroughputCapacity": 128, "FsxAdminPassword": "Fsx-admin-one", "RouteTableIds": []any{"rtb-1", "rtb-2"}}
		maps.Copy(config, extra)
		return map[string]any{"FileSystemType": "ONTAP", "StorageCapacity": 1024, "SubnetIds": []any{"subnet-0a1", "subnet-0b2"}, "OntapConfiguration": config}
	}
	windows := func(extra map[string]any) map[string]any {
		ad := map[string]any{"DomainName": "example.com", "UserName": "joiner", "Password": "ad-one", "DnsIps": []any{"10.0.0.1"}}
		maps.Copy(ad, extra)
		return map[string]any{"FileSystemType": "WINDOWS", "StorageCapacity": 32, "SubnetIds": []any{"subnet-0a1"},
			"WindowsConfiguration": map[string]any{"ThroughputCapacity": 32, "SelfManagedActiveDirectoryConfiguration": ad}}
	}
	tests := []struct {
		name    string
		base    map[string]any
		desired map[string]any
		unsent  []string
		want    *fsx.UpdateFileSystemInput
	}{
		{
			name:    "route tables and a changed admin password",
			base:    ontap(nil),
			desired: ontap(map[string]any{"RouteTableIds": []any{"rtb-2", "rtb-3"}, "FsxAdminPassword": "Fsx-admin-two"}),
			want: &fsx.UpdateFileSystemInput{OntapConfiguration: &types.UpdateFileSystemOntapConfiguration{
				AddRouteTableIds: []string{"rtb-3"}, RemoveRouteTableIds: []string{"rtb-1"}, FsxAdminPassword: aws.String("Fsx-admin-two"),
			}},
		},
		{
			name:    "an unchanged admin password isn't sent",
			base:    ontap(nil),
			desired: ontap(map[string]any{"ThroughputCapacity": 256}),
			unsent:  []string{"OntapConfiguration/FsxAdminPassword"},
			want:    &fsx.UpdateFileSystemInput{OntapConfiguration: &types.UpdateFileSystemOntapConfiguration{ThroughputCapacity: aws.Int32(256)}},
		},
		{
			name:    "a removed property keeps AWS's value",
			base:    ontap(map[string]any{"WeeklyMaintenanceStartTime": "1:05:00"}),
			desired: ontap(nil),
			unsent:  []string{"OntapConfiguration/FsxAdminPassword"},
		},
		{
			name:    "a new Active Directory password sends the user name with it",
			base:    windows(nil),
			desired: windows(map[string]any{"Password": "ad-two"}),
			want: &fsx.UpdateFileSystemInput{WindowsConfiguration: &types.UpdateFileSystemWindowsConfiguration{
				SelfManagedActiveDirectoryConfiguration: &types.SelfManagedActiveDirectoryConfigurationUpdates{UserName: aws.String("joiner"), Password: aws.String("ad-two")},
			}},
		},
		{
			name:    "a new user name sends the password with it",
			base:    windows(nil),
			desired: windows(map[string]any{"UserName": "other", "DnsIps": []any{"10.0.0.2"}}),
			unsent:  []string{"WindowsConfiguration/SelfManagedActiveDirectoryConfiguration/Password"},
			want: &fsx.UpdateFileSystemInput{WindowsConfiguration: &types.UpdateFileSystemWindowsConfiguration{
				SelfManagedActiveDirectoryConfiguration: &types.SelfManagedActiveDirectoryConfigurationUpdates{UserName: aws.String("other"), Password: aws.String("ad-one"), DnsIps: []string{"10.0.0.2"}},
			}},
		},
		{
			name:    "top-level properties",
			base:    lustre("1:05:00"),
			desired: with(lustre("1:05:00"), map[string]any{"StorageCapacity": 2400, "FileSystemTypeVersion": "2.15"}),
			want:    &fsx.UpdateFileSystemInput{StorageCapacity: aws.Int32(2400), FileSystemTypeVersion: aws.String("2.15")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			recorder := &updates{fakeAPI: api}
			h := NewFileSystem(recorder)
			id := create(t, h, tt.base)
			require.NoError(t, update(t, h, id, tt.base, tt.desired, tt.unsent...))
			if tt.want == nil {
				require.Empty(t, recorder.inputs)
				return
			}
			require.Len(t, recorder.inputs, 1)
			tt.want.FileSystemId = aws.String(id)
			require.Equal(t, tt.want, recorder.inputs[0])
			require.True(t, finish(t, h, id, native.OperationUpdate).Done)
		})
	}

	t.Run("a whole configuration the file system had none of", func(t *testing.T) {
		current := map[string]any{}
		desired := map[string]any{"OpenZFSConfiguration": map[string]any{"ThroughputCapacity": 128.0, "DeploymentType": "SINGLE_AZ_1"}}
		patch := jsondiff.Patch{{Type: jsondiff.OperationAdd, Path: "/OpenZFSConfiguration", Value: desired["OpenZFSConfiguration"]}}
		changes := configChanges("OpenZFSConfiguration", native.UpdateRequest{Current: current, Desired: desired, Patch: patch})
		require.Equal(t, map[string]any{"ThroughputCapacity": 128.0}, changes, "DeploymentType can't change, and UCP rejects a change to it")
	})

	t.Run("errors", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		require.ErrorIs(t, h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: "fs-missing"}), native.ErrNotFound)
		id := create(t, h, lustre("1:05:00"))
		err := h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: id, Desired: map[string]any{"StorageCapacity": "big"},
			Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/StorageCapacity"}}})
		require.Error(t, err)
	})
}

func Test_Progress(t *testing.T) {
	t.Run("an update waits for its administrative action, but not for an optimization", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		base := map[string]any{"FileSystemType": "WINDOWS", "StorageCapacity": 32, "SubnetIds": []any{"subnet-0a1"}, "WindowsConfiguration": map[string]any{"ThroughputCapacity": 32}}
		id := create(t, h, base)
		require.NoError(t, update(t, h, id, base, with(base, map[string]any{"StorageCapacity": 64})))
		progress, err := h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "FILE_SYSTEM_UPDATE IN_PROGRESS", Waiting: "COMPLETED"}, progress)
		progress, err = h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
		require.NoError(t, err)
		require.Equal(t, native.Progress{Done: true, State: "AVAILABLE"}, progress, "the storage optimization still runs")
	})

	t.Run("an update whose administrative action fails", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		base := map[string]any{"FileSystemType": "WINDOWS", "StorageCapacity": 32, "SubnetIds": []any{"subnet-0a1"}, "WindowsConfiguration": map[string]any{"ThroughputCapacity": 32}}
		id := create(t, h, base)
		require.NoError(t, update(t, h, id, base, with(base, map[string]any{"StorageCapacity": 64})))
		require.Nil(t, finish(t, h, id, native.OperationUpdate).Failure)

		api.failUpdate = "The throughput capacity is not supported."
		desired := with(base, map[string]any{"WindowsConfiguration": map[string]any{"ThroughputCapacity": 4096}})
		require.NoError(t, update(t, h, id, base, desired))
		progress := finish(t, h, id, native.OperationUpdate)
		require.Equal(t, "FSx failed the FILE_SYSTEM_UPDATE: The throughput capacity is not supported.", progress.Failure.Message)

		require.NoError(t, update(t, h, id, base, with(base, map[string]any{"StorageCapacity": 128})))
		require.Nil(t, finish(t, h, id, native.OperationUpdate).Failure, "a later update that succeeds is done, though an earlier one failed")
	})

	t.Run("a Lustre update waits while the file system is updating", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		id := create(t, h, lustre("1:05:00"))
		require.NoError(t, update(t, h, id, lustre("1:05:00"), lustre("2:05:00")))
		progress, err := h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "UPDATING", Waiting: "AVAILABLE"}, progress)
	})

	t.Run("failures", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewFileSystem(api)
		id := create(t, h, lustre("1:05:00"))
		api.lifecycle[id] = types.FileSystemLifecycleFailed
		api.failures[id] = "The subnet has no free IP addresses."
		progress, err := h.Progress(t.Context(), testRegion, id, native.OperationCreate)
		require.NoError(t, err)
		require.Equal(t, `AWS reports the resource as "FAILED": The subnet has no free IP addresses.`, progress.Failure.Message)

		api.lifecycle[id] = types.FileSystemLifecycleMisconfigured
		delete(api.failures, id)
		progress, err = h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
		require.NoError(t, err)
		require.Equal(t, `AWS reports the file system as "MISCONFIGURED"`, progress.Failure.Message)

		_, err = h.Progress(t.Context(), testRegion, "fs-missing", native.OperationCreate)
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_ReadErrors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewFileSystem(api)
	create(t, h, lustre("1:05:00"))
	api.errs["DescribeFileSystems"] = fault("ThrottlingException", "slow down")
	_, err := h.Read(t.Context(), testRegion, "fs-1")
	require.ErrorContains(t, err, "slow down")
	_, err = h.List(t.Context(), testRegion)
	require.ErrorContains(t, err, "slow down")
	_, err = NewFileSystem(noFileSystem{api}).Read(t.Context(), testRegion, "fs-1")
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_Equivalent(t *testing.T) {
	arn := "arn:aws:kms:us-west-2:111122223333:key/1234abcd"
	require.True(t, equivalent("KmsKeyId", "1234abcd", arn))
	require.True(t, equivalent("KmsKeyId", arn, arn))
	require.True(t, equivalent("KmsKeyId", "alias/fsx", arn))
	require.True(t, equivalent("KmsKeyId", "arn:aws:kms:us-west-2:111122223333:alias/fsx", arn))
	require.False(t, equivalent("KmsKeyId", "5678efgh", arn))
	require.False(t, equivalent("StorageType", "ssd", "SSD"))
	require.False(t, equivalent("KmsKeyId", 1, arn))
}

func Test_Schema(t *testing.T) {
	parsed := struct {
		WriteOnly  []string `json:"writeOnlyProperties"`
		CreateOnly []string `json:"createOnlyProperties"`
	}{}
	require.NoError(t, json.Unmarshal([]byte(schema(t)), &parsed))
	require.Contains(t, parsed.WriteOnly, "/properties/OntapConfiguration/FsxAdminPassword")
	require.Contains(t, parsed.WriteOnly, "/properties/WindowsConfiguration/SelfManagedActiveDirectoryConfiguration/Password")
	require.NotContains(t, parsed.WriteOnly, "/properties/WindowsConfiguration/SelfManagedActiveDirectoryConfiguration/DomainJoinServiceAccountSecret", "it only points to a secret")
	for config, fields := range updatable {
		for _, field := range fields {
			require.NotContains(t, parsed.CreateOnly, "/properties/"+config+"/"+field, "UpdateFileSystem can change it")
		}
	}
}

func Test_Options(t *testing.T) {
	h := NewFileSystem(newFakeAPI(awsfake.NewCloudControl()))
	require.Equal(t, fileSystemType, h.TypeName())
	require.Equal(t, native.RecoverByResend, h.Options().Recovery)
	var options fsx.Options
	inRegion("eu-west-2")(&options)
	require.Equal(t, "eu-west-2", options.Region)
}

// Test_UpdatesEveryUpdatableProperty checks that Update applies every property the schema lets a deploy change,
// including those inside each configuration, so UCP never records a change AWS didn't get. Any other property must be
// create-only, which UCP rejects.
func Test_UpdatesEveryUpdatableProperty(t *testing.T) {
	configs := slices.Sorted(maps.Keys(updatable))
	require.ElementsMatch(t, nativetest.Updatable(t, schema(t)), append([]string{"FileSystemTypeVersion", "NetworkType", "StorageCapacity", "StorageType", "Tags"}, configs...))

	var doc struct {
		Properties map[string]struct {
			Ref string `json:"$ref"`
		} `json:"properties"`
		Definitions map[string]struct {
			Properties map[string]any `json:"properties"`
		} `json:"definitions"`
		CreateOnly []string `json:"createOnlyProperties"`
	}
	require.NoError(t, json.Unmarshal([]byte(schema(t)), &doc))
	for _, config := range configs {
		definition := doc.Definitions[strings.TrimPrefix(doc.Properties[config].Ref, "#/definitions/")]
		want := []string{}
		for name := range definition.Properties {
			if !slices.Contains(doc.CreateOnly, "/properties/"+config+"/"+name) {
				want = append(want, name)
			}
		}
		require.ElementsMatch(t, want, updatable[config], config)
	}
}
