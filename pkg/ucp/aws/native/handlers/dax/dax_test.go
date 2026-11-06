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

package dax

import (
	"context"
	"errors"
	"maps"
	"os"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dax"
	"github.com/aws/aws-sdk-go-v2/service/dax/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func schema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/aws-dax-cluster.json")
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

var cluster = map[string]any{
	"ClusterName":                "app-dax",
	"NodeType":                   "dax.t3.small",
	"ReplicationFactor":          1,
	"IAMRoleARN":                 "arn:aws:iam::123456789012:role/dax",
	"AvailabilityZones":          []any{"us-west-2a"},
	"Description":                "cache",
	"PreferredMaintenanceWindow": "sun:05:00-sun:06:00",
	"SecurityGroupIds":           []any{"sg-1", "sg-2"},
	"SSESpecification":           map[string]any{"SSEEnabled": true},
	"Tags":                       map[string]any{"team": "a", "owner": ""},
}

func Test_Suite_Cluster(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName: clusterType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewCluster(api), api)
		},
		IdentifierProperty: "ClusterName",
		WriteOnly:          []string{"AvailabilityZones"},
		Properties:         cluster,
		Identifier:         "app-dax",
		Changed:            with(cluster, map[string]any{"Tags": map[string]any{"owner": "", "team": "b"}}),
		ChangedWriteOnly:   with(cluster, map[string]any{"ReplicationFactor": 3, "AvailabilityZones": []any{"us-west-2a", "us-west-2b", "us-west-2c"}}),
		Removable:          "Tags",
		CreateOnlyChange:   with(cluster, map[string]any{"NodeType": "dax.r5.large"}),
		OtherForm:          map[string]any{"ClusterName": "APP-DAX", "PreferredMaintenanceWindow": "SUN:05:00-SUN:06:00"},
		MultiCall: with(cluster, map[string]any{
			"ReplicationFactor": 3,
			"AvailabilityZones": []any{"us-west-2a", "us-west-2b", "us-west-2c"},
			"Tags":              map[string]any{"env": "dev", "owner": "", "team": "b"},
		}),
		Calls: 2,
		FailCall: nativetest.FailCall(2, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
	})
}

// deploy creates the cluster from properties with h and waits until it is available.
func deploy(t *testing.T, h *Cluster, properties map[string]any) string {
	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	wait(t, h, identifier, native.OperationCreate)
	return identifier
}

func wait(t *testing.T, h *Cluster, identifier string, op native.Operation) {
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

// update updates the cluster from current to desired with h.
func update(t *testing.T, h *Cluster, identifier string, desired map[string]any) error {
	current, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: desired, Patch: patch})
}

func Test_Read(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	api.awsTags = map[string]string{"aws:cloudformation:stack-name": "s"}
	h := NewCluster(api)
	identifier := deploy(t, h, with(cluster, map[string]any{
		"ClusterName":          "App-DAX",
		"NotificationTopicARN": "arn:aws:sns:us-west-2:123456789012:topic",
		"ParameterGroupName":   "params",
		"SubnetGroupName":      "subnets",
	}))
	require.Equal(t, "app-dax", identifier)

	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"ClusterName":                 "app-dax",
		"Arn":                         arnPrefix + "app-dax",
		"Description":                 "cache",
		"IAMRoleARN":                  "arn:aws:iam::123456789012:role/dax",
		"NodeType":                    "dax.t3.small",
		"ReplicationFactor":           float64(1),
		"SubnetGroupName":             "subnets",
		"ParameterGroupName":          "params",
		"PreferredMaintenanceWindow":  "sun:05:00-sun:06:00",
		"SecurityGroupIds":            []any{"sg-1", "sg-2"},
		"NotificationTopicARN":        "arn:aws:sns:us-west-2:123456789012:topic",
		"SSESpecification":            map[string]any{"SSEEnabled": true},
		"ClusterDiscoveryEndpoint":    "app-dax.abc123.dax-clusters.us-west-2.amazonaws.com:8111",
		"ClusterDiscoveryEndpointURL": "dax://app-dax.abc123.dax-clusters.us-west-2.amazonaws.com",
		"Tags":                        map[string]any{"owner": "", "team": "a"},
	}, got, "AWS's tags and the write-only AvailabilityZones are left out")

	_, err = h.Read(t.Context(), region, "missing")
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_ReplicationFactor(t *testing.T) {
	c := &types.Cluster{TotalNodes: aws.Int32(3), NodeIdsToRemove: []string{"a-1", "a-2"}}
	require.Equal(t, int32(1), replicationFactor(c), "nodes DAX is removing don't count")
}

func Test_Update_NotificationTopicAndParameterGroup(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	topic := "arn:aws:sns:us-west-2:123456789012:topic"
	identifier := deploy(t, h, with(cluster, map[string]any{"NotificationTopicARN": topic, "ParameterGroupName": "params"}))

	require.NoError(t, update(t, h, identifier, with(cluster, map[string]any{"NotificationTopicARN": nil, "ParameterGroupName": nil})))
	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.NotContains(t, got, "NotificationTopicARN", "a removed topic is made inactive")
	require.Equal(t, defaultParameterGroup, got["ParameterGroupName"], "a removed parameter group goes back to the default")

	require.NoError(t, update(t, h, identifier, with(cluster, map[string]any{"NotificationTopicARN": topic})))
	got, err = h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, topic, got["NotificationTopicARN"])
}

func Test_Update_RemovedWithoutDefault(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	identifier := deploy(t, h, cluster)
	api.calls = nil

	require.NoError(t, update(t, h, identifier, with(cluster, map[string]any{"Description": nil})))
	require.NotContains(t, api.calls, "UpdateCluster", "DAX keeps a removed description")

}

// Test_Update_UnreachableZones checks that a change of AvailabilityZones that adding or removing nodes can't reach is
// rejected, since DAX has no call that moves a node, and one it can reach is applied.
func Test_Update_UnreachableZones(t *testing.T) {
	two := with(cluster, map[string]any{"ReplicationFactor": 2, "AvailabilityZones": []any{"us-west-2a", "us-west-2b"}})
	tests := []struct {
		name    string
		desired map[string]any
		ok      bool
	}{
		{"same count, other zone", with(two, map[string]any{"AvailabilityZones": []any{"us-west-2a", "us-west-2c"}}), false},
		{"same zones, other order", with(two, map[string]any{"AvailabilityZones": []any{"us-west-2b", "us-west-2a"}}), true},
		{"increase that moves a node", with(two, map[string]any{"ReplicationFactor": 3, "AvailabilityZones": []any{"us-west-2a", "us-west-2c", "us-west-2c"}}), false},
		{"increase that keeps the nodes", with(two, map[string]any{"ReplicationFactor": 3, "AvailabilityZones": []any{"us-west-2a", "us-west-2b", "us-west-2c"}}), true},
		{"increase with a list that doesn't count the nodes", with(two, map[string]any{"ReplicationFactor": 3, "AvailabilityZones": []any{"us-west-2a", "us-west-2b"}}), false},
		{"decrease to a zone no node is in", with(two, map[string]any{"ReplicationFactor": 1, "AvailabilityZones": []any{"us-west-2d"}}), false},
		{"zones removed", with(two, map[string]any{"AvailabilityZones": nil}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			h := NewCluster(api)
			identifier := deploy(t, h, two)
			api.calls = nil
			err := update(t, h, identifier, tt.desired)
			if tt.ok {
				require.NoError(t, err)
				return
			}
			var failure *native.Failure
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)
			require.Equal(t, native.OutcomeRejected, failure.Outcome)
			for _, call := range []string{"UpdateCluster", "IncreaseReplicationFactor", "DecreaseReplicationFactor", "TagResource", "UntagResource"} {
				require.NotContains(t, api.calls, call, "nothing is changed")
			}
		})
	}
}

func Test_Update_DecreaseReplicationFactor(t *testing.T) {
	tests := []struct {
		name    string
		desired []any
		removed []string
	}{
		{"the template names the zones left", []any{"us-west-2b"}, []string{"us-west-2a", "us-west-2c"}},
		{"the template names no zones", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			h := NewCluster(api)
			three := with(cluster, map[string]any{"ReplicationFactor": 3, "AvailabilityZones": []any{"us-west-2a", "us-west-2b", "us-west-2c"}})
			identifier := deploy(t, h, three)

			desired := with(three, map[string]any{"ReplicationFactor": 1, "AvailabilityZones": nil})
			if tt.desired != nil {
				desired["AvailabilityZones"] = tt.desired
			}
			require.NoError(t, update(t, h, identifier, desired))
			require.Equal(t, int32(1), api.lastDecrease.NewReplicationFactor)
			require.Equal(t, tt.removed, api.lastDecrease.AvailabilityZones)
			wait(t, h, identifier, native.OperationUpdate)

			got, err := h.Read(t.Context(), region, identifier)
			require.NoError(t, err)
			require.EqualValues(t, 1, got["ReplicationFactor"])
		})
	}
}

func Test_Update_Errors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	identifier := deploy(t, h, cluster)

	err := update(t, h, identifier, with(cluster, map[string]any{"Description": 5}))
	require.Error(t, err, "a value of the wrong type fails before any call")
	err = update(t, h, identifier, with(cluster, map[string]any{"ReplicationFactor": "three"}))
	require.Error(t, err)

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "missing"})
	require.ErrorIs(t, err, native.ErrNotFound)

	api.errs["IncreaseReplicationFactor"] = fault("InvalidClusterStateFault")
	err = update(t, h, identifier, with(cluster, map[string]any{"Tags": map[string]any{"team": "b"}, "ReplicationFactor": 3, "AvailabilityZones": nil}))
	var failure *native.Failure
	require.ErrorAs(t, err, &failure, "a failure after the tags changed is partly applied")
	require.Equal(t, native.OutcomePartial, failure.Outcome)
}

func Test_Create_Errors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(cluster, map[string]any{"ReplicationFactor": "one"})})
	require.Error(t, err)

	api.errs["CreateCluster"] = fault("InsufficientClusterCapacityFault")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: cluster})
	require.ErrorContains(t, err, "InsufficientClusterCapacityFault")

	delete(api.errs, "CreateCluster")
	identifier := deploy(t, h, with(cluster, map[string]any{"SSESpecification": map[string]any{}, "Tags": nil}))
	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"SSEEnabled": false}, got["SSESpecification"], "an empty SSESpecification sends nothing")
	require.NotContains(t, got, "Tags")
}

func Test_ReadErrors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	identifier := deploy(t, h, cluster)

	api.errs["ListTags"] = fault("InvalidClusterStateFault")
	_, err := h.Read(t.Context(), region, identifier)
	require.ErrorContains(t, err, "InvalidClusterStateFault")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "InvalidClusterStateFault")

	api.errs["DescribeClusters"] = errors.New("throttled")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "throttled")
	_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.ErrorContains(t, err, "throttled")
}

func Test_DescribeReturnsNone(t *testing.T) {
	h := NewCluster(emptyAPI{})
	_, err := h.Read(t.Context(), region, "app-dax")
	require.ErrorIs(t, err, native.ErrNotFound)
}

// emptyAPI is a DAX API whose DescribeClusters returns no cluster and no error.
type emptyAPI struct {
	API
}

func (emptyAPI) DescribeClusters(ctx context.Context, in *dax.DescribeClustersInput, _ ...func(*dax.Options)) (*dax.DescribeClustersOutput, error) {
	return &dax.DescribeClustersOutput{}, nil
}

func Test_List_Pages(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	deploy(t, h, cluster)
	deploy(t, h, with(cluster, map[string]any{"ClusterName": "other", "Tags": map[string]any{"a": "1", "b": "2"}}))
	api.pageSize = 1

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, 2)
	require.Equal(t, "app-dax", resources[0].Identifier)
	require.Equal(t, "other", resources[1].Identifier)
	require.Equal(t, map[string]any{"a": "1", "b": "2"}, resources[1].Properties["Tags"], "tags are read across pages")
}

func Test_Delete(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	identifier := deploy(t, h, cluster)

	require.NoError(t, h.Delete(t.Context(), region, identifier))
	require.NoError(t, h.Delete(t.Context(), region, identifier), "a cluster AWS is already deleting counts as deleted")
	wait := func() error {
		for range 5 {
			if _, err := h.Progress(t.Context(), region, identifier, native.OperationDelete); err != nil {
				return err
			}
		}
		return nil
	}
	require.ErrorIs(t, wait(), native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), region, identifier), native.ErrNotFound)
}

func Test_Delete_InvalidState(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	identifier := deploy(t, h, cluster)

	api.errs["DeleteCluster"] = fault("InvalidClusterStateFault")
	require.ErrorContains(t, h.Delete(t.Context(), region, identifier), "InvalidClusterStateFault", "a cluster that isn't being deleted fails")

	api.errs["DescribeClusters"] = errors.New("throttled")
	require.ErrorContains(t, h.Delete(t.Context(), region, identifier), "throttled")
}

func Test_Equivalent(t *testing.T) {
	tests := []struct {
		path     string
		template any
		aws      any
		want     bool
	}{
		{"ClusterName", "App-DAX", "app-dax", true},
		{"SubnetGroupName", "Subnets", "subnets", true},
		{"PreferredMaintenanceWindow", "Sun:05:00-Sun:06:00", "sun:05:00-sun:06:00", true},
		{"Description", "Cache", "cache", false},
		{"ClusterName", "app-dax", "other", false},
		{"ReplicationFactor", 1, 1, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, equivalent(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
	}
}

func Test_RemovedZones(t *testing.T) {
	nodes := []types.Node{{AvailabilityZone: aws.String("a")}, {AvailabilityZone: aws.String("a")}, {AvailabilityZone: aws.String("b")}}
	require.Equal(t, []string{"a"}, removedZones(nodes, []string{"a", "b"}, 1))
	require.Equal(t, []string{"a", "b"}, removedZones(nodes, []string{"a"}, 2))
	require.Nil(t, removedZones(nodes, []string{"a"}, 1), "the zones remove more nodes than the decrease")
	require.Nil(t, removedZones(nodes, nil, 1))
}

func Test_InRegion(t *testing.T) {
	o := &dax.Options{}
	inRegion("eu-west-2")(o)
	require.Equal(t, "eu-west-2", o.Region)
}

// Test_Update_SettingsAndNodes checks that a deploy that changes UpdateCluster's settings and the number of nodes is
// rejected before any call, because DAX takes a node change only while the cluster is available, and UpdateCluster can
// leave it modifying. Either change alone is made.
func Test_Update_SettingsAndNodes(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	identifier := deploy(t, h, cluster)
	api.calls = nil

	both := with(cluster, map[string]any{"SecurityGroupIds": []any{"sg-3"}, "ReplicationFactor": 3, "AvailabilityZones": nil})
	err := update(t, h, identifier, both)
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "ReplicationFactor")
	require.Contains(t, failure.Message, "SecurityGroupIds")
	for _, call := range []string{"UpdateCluster", "IncreaseReplicationFactor", "TagResource"} {
		require.NotContains(t, api.calls, call, "nothing is changed")
	}

	settings := with(cluster, map[string]any{"SecurityGroupIds": []any{"sg-3"}, "AvailabilityZones": nil})
	require.NoError(t, update(t, h, identifier, settings))
	wait(t, h, identifier, native.OperationUpdate)
	require.NoError(t, update(t, h, identifier, with(settings, map[string]any{"ReplicationFactor": 3})))
	wait(t, h, identifier, native.OperationUpdate)
	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.EqualValues(t, 3, got["ReplicationFactor"])
	require.Equal(t, []any{"sg-3"}, got["SecurityGroupIds"])
}
