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

package dms

import (
	"context"
	"errors"
	"maps"
	"os"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	dms "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
	"github.com/aws/aws-sdk-go-v2/service/databasemigrationservice/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func schema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/aws-dms-replicationinstance.json")
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

// instance sets no boolean to true, because the suite looks for the write-only AllowMajorVersionUpgrade, which
// ChangedWriteOnly sets to true, in every response.
var instance = map[string]any{
	"ReplicationInstanceIdentifier":    "app-dms",
	"ReplicationInstanceClass":         "dms.t3.micro",
	"AllocatedStorage":                 50,
	"AutoMinorVersionUpgrade":          false,
	"PubliclyAccessible":               false,
	"EngineVersion":                    "3.5.3",
	"KmsKeyId":                         "key-1234",
	"PreferredMaintenanceWindow":       "sun:06:00-sun:06:30",
	"ReplicationSubnetGroupIdentifier": "app-subnets",
	"VpcSecurityGroupIds":              []any{"sg-1"},
	"Tags":                             tags("owner", "", "team", "a"),
}

func Test_Suite_ReplicationInstance(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName: instanceType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewReplicationInstance(api), api)
		},
		IdentifierProperty: "ReplicationInstanceIdentifier",
		WriteOnly:          []string{"AllowMajorVersionUpgrade", "KmsKeyId"},
		Properties:         instance,
		Identifier:         "app-dms",
		Changed:            with(instance, map[string]any{"ReplicationInstanceClass": "dms.t3.small"}),
		ChangedWriteOnly:   with(instance, map[string]any{"EngineVersion": "3.6.1", "AllowMajorVersionUpgrade": true}),
		Removable:          "Tags",
		CreateOnlyChange:   with(instance, map[string]any{"ReplicationSubnetGroupIdentifier": "other-subnets"}),
		OtherForm: map[string]any{
			"ReplicationInstanceIdentifier":    "APP-DMS",
			"ReplicationSubnetGroupIdentifier": "App-Subnets",
			"PreferredMaintenanceWindow":       "Sun:06:00-Sun:06:30",
		},
		MultiCall: with(instance, map[string]any{"ReplicationInstanceClass": "dms.t3.small", "Tags": tags("env", "dev")}),
		Calls:     3,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
	})
}

// deploy creates the instance from properties with h and waits until it is available.
func deploy(t *testing.T, h *ReplicationInstance, properties map[string]any) string {
	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	wait(t, h, identifier, native.OperationCreate)
	return identifier
}

func wait(t *testing.T, h *ReplicationInstance, identifier string, op native.Operation) {
	for range 5 {
		progress, err := h.Progress(t.Context(), region, identifier, op)
		require.NoError(t, err)
		if progress.Done {
			require.Nil(t, progress.Failure)
			return
		}
	}
	require.Fail(t, "the operation did not finish")
}

// update updates the instance from the template previous to desired with h.
func update(t *testing.T, h *ReplicationInstance, identifier string, previous map[string]any, desired map[string]any) error {
	current, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	patch, err := jsondiff.Compare(previous, desired)
	require.NoError(t, err)
	return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: desired, Patch: patch})
}

func Test_Read(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	identifier := deploy(t, h, with(instance, map[string]any{
		"ReplicationInstanceIdentifier": "App-DMS",
		"ResourceIdentifier":            "App-Rep-1",
		"MultiAZ":                       true,
		"DnsNameServers":                "1.1.1.1,2.2.2.2",
	}))
	require.Equal(t, "app-dms", identifier)

	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"Id":                                    "app-dms",
		"ReplicationInstanceIdentifier":         "app-dms",
		"ReplicationInstanceClass":              "dms.t3.micro",
		"AllocatedStorage":                      float64(50),
		"AutoMinorVersionUpgrade":               false,
		"EngineVersion":                         "3.5.3",
		"MultiAZ":                               true,
		"DnsNameServers":                        "1.1.1.1,2.2.2.2",
		"PreferredMaintenanceWindow":            "sun:06:00-sun:06:30",
		"PubliclyAccessible":                    false,
		"ResourceIdentifier":                    "App-Rep-1",
		"ReplicationSubnetGroupIdentifier":      "app-subnets",
		"VpcSecurityGroupIds":                   []any{"sg-1"},
		"ReplicationInstancePrivateIpAddresses": "10.0.0.1",
		"Tags":                                  tags("owner", "", "team", "a"),
	}, got, "the write-only KmsKeyId is left out")

	_, err = h.Read(t.Context(), region, "missing")
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_ResourceIdentifier(t *testing.T) {
	require.Equal(t, "Example-App-ARN1", resourceIdentifier("arn:aws:dms:eu-west-1:012345678901:rep:Example-App-ARN1"))
}

func Test_Update_EngineVersion(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	identifier := deploy(t, h, instance)

	require.NoError(t, update(t, h, identifier, instance, with(instance, map[string]any{"EngineVersion": "3.6.1"})))
	require.False(t, api.lastModify.AllowMajorVersionUpgrade, "the template doesn't allow a major version upgrade")
	require.True(t, api.lastModify.ApplyImmediately)

	api.lastModify = nil
	require.NoError(t, update(t, h, identifier, with(instance, map[string]any{"EngineVersion": "3.6.1"}), with(instance, map[string]any{"EngineVersion": "3.6.1", "AllowMajorVersionUpgrade": true})))
	require.Nil(t, api.lastModify, "AllowMajorVersionUpgrade alone changes nothing in AWS")

	require.NoError(t, update(t, h, identifier, with(instance, map[string]any{"EngineVersion": "3.6.1", "AllowMajorVersionUpgrade": true}), with(instance, map[string]any{"EngineVersion": "3.7.0", "AllowMajorVersionUpgrade": true})))
	require.True(t, api.lastModify.AllowMajorVersionUpgrade)
	require.Equal(t, "3.7.0", aws.ToString(api.lastModify.EngineVersion))
}

func Test_Update_RemovedProperties(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	created := with(instance, map[string]any{"MultiAZ": true})
	identifier := deploy(t, h, created)

	require.NoError(t, update(t, h, identifier, created, with(instance, map[string]any{"AutoMinorVersionUpgrade": nil, "PreferredMaintenanceWindow": nil})))
	require.True(t, aws.ToBool(api.lastModify.AutoMinorVersionUpgrade), "a removed AutoMinorVersionUpgrade goes back to true")
	require.False(t, aws.ToBool(api.lastModify.MultiAZ), "a removed MultiAZ goes back to false")
	require.Nil(t, api.lastModify.PreferredMaintenanceWindow, "DMS keeps a removed maintenance window")
}

func Test_Update_Errors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	identifier := deploy(t, h, instance)

	require.Error(t, update(t, h, identifier, instance, with(instance, map[string]any{"ReplicationInstanceClass": 5})))
	err := h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "missing"})
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_Progress_Pending(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	identifier := deploy(t, h, instance)

	api.pendingReads = 1
	progress, err := h.Progress(t.Context(), region, identifier, native.OperationUpdate)
	require.NoError(t, err)
	require.False(t, progress.Done, "an available instance with a pending modification is still updating")
	require.Equal(t, "available", progress.Waiting)

	progress, err = h.Progress(t.Context(), region, identifier, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_Progress_Failed(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	identifier := deploy(t, h, instance)

	api.status[identifier] = "incompatible-network"
	api.pendingReads = 1
	progress, err := h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.NotNil(t, progress.Failure)
}

func Test_Create_Errors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(instance, map[string]any{"AllocatedStorage": "big"})})
	require.Error(t, err)

	api.errs["CreateReplicationInstance"] = fault("InsufficientResourceCapacityFault")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: instance})
	require.ErrorContains(t, err, "InsufficientResourceCapacityFault")
}

func Test_ReadErrors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	identifier := deploy(t, h, instance)

	api.errs["ListTagsForResource"] = errors.New("throttled")
	_, err := h.Read(t.Context(), region, identifier)
	require.ErrorContains(t, err, "throttled")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "throttled")

	api.errs["DescribeReplicationInstances"] = errors.New("throttled")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "throttled")
	_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.ErrorContains(t, err, "throttled")
}

// staticAPI is a DMS API whose DescribeReplicationInstances returns instances, one page each, and whose
// ListTagsForResource returns no tags.
type staticAPI struct {
	API
	instances []types.ReplicationInstance
}

func (a staticAPI) DescribeReplicationInstances(ctx context.Context, in *dms.DescribeReplicationInstancesInput, _ ...func(*dms.Options)) (*dms.DescribeReplicationInstancesOutput, error) {
	return &dms.DescribeReplicationInstancesOutput{ReplicationInstances: a.instances}, nil
}

func (a staticAPI) ListTagsForResource(ctx context.Context, in *dms.ListTagsForResourceInput, _ ...func(*dms.Options)) (*dms.ListTagsForResourceOutput, error) {
	return &dms.ListTagsForResourceOutput{}, nil
}

func Test_Deleted(t *testing.T) {
	gone := types.ReplicationInstance{ReplicationInstanceIdentifier: aws.String("app-dms"), ReplicationInstanceStatus: aws.String("deleted")}
	h := NewReplicationInstance(staticAPI{instances: []types.ReplicationInstance{gone}})

	_, err := h.Read(t.Context(), region, "app-dms")
	require.ErrorIs(t, err, native.ErrNotFound, "an instance AWS reports as deleted is gone")
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources)
}

func Test_List_Pages(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	deploy(t, h, instance)
	deploy(t, h, with(instance, map[string]any{"ReplicationInstanceIdentifier": "other"}))
	api.pageSize = 1

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, 2)
	require.Equal(t, "app-dms", resources[0].Identifier)
	require.Equal(t, "other", resources[1].Identifier)
}

func Test_Delete(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	identifier := deploy(t, h, instance)

	api.holdDeleting = true
	require.NoError(t, h.Delete(t.Context(), region, identifier))
	require.NoError(t, h.Delete(t.Context(), region, identifier), "an instance AWS is already deleting counts as deleted")
	api.holdDeleting = false
	progress, err := h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "deleting", Waiting: "deleted"}, progress)
	_, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), region, identifier), native.ErrNotFound)
}

func Test_Delete_InvalidState(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewReplicationInstance(api)
	identifier := deploy(t, h, instance)

	api.errs["DeleteReplicationInstance"] = fault("InvalidResourceStateFault")
	require.ErrorContains(t, h.Delete(t.Context(), region, identifier), "InvalidResourceStateFault", "an instance that isn't being deleted fails, such as one with tasks")

	h = NewReplicationInstance(&describeFailsAfterFirst{fakeAPI: api})
	require.ErrorContains(t, h.Delete(t.Context(), region, identifier), "throttled")
}

// describeFailsAfterFirst is a fake whose DescribeReplicationInstances fails after its first call.
type describeFailsAfterFirst struct {
	*fakeAPI
	described bool
}

func (a *describeFailsAfterFirst) DescribeReplicationInstances(ctx context.Context, in *dms.DescribeReplicationInstancesInput, optFns ...func(*dms.Options)) (*dms.DescribeReplicationInstancesOutput, error) {
	if a.described {
		return nil, errors.New("throttled")
	}
	a.described = true
	return a.fakeAPI.DescribeReplicationInstances(ctx, in, optFns...)
}

func Test_Equivalent(t *testing.T) {
	tests := []struct {
		path     string
		template any
		aws      any
		want     bool
	}{
		{"ReplicationInstanceIdentifier", "App-DMS", "app-dms", true},
		{"ReplicationSubnetGroupIdentifier", "Subnets", "subnets", true},
		{"PreferredMaintenanceWindow", "Sun:06:00-Sun:06:30", "sun:06:00-sun:06:30", true},
		{"ResourceIdentifier", "App-Rep-1", "app-rep-1", true},
		{"EngineVersion", "3.5.3", "3.5.4", false},
		{"ReplicationInstanceClass", "DMS.T3.MICRO", "dms.t3.micro", false},
		{"AllocatedStorage", 50, 50, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, equivalent(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
	}
}

func Test_InRegion(t *testing.T) {
	o := &dms.Options{}
	inRegion("eu-west-2")(o)
	require.Equal(t, "eu-west-2", o.Region)
}
