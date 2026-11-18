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

package neptune

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/neptune"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
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

var endpoint = map[string]any{
	"DBClusterEndpointIdentifier": "reads",
	"DBClusterIdentifier":         "graph",
	"EndpointType":                "READER",
	"StaticMembers":               []any{"graph-1", "graph-2"},
	"Tags":                        tags("owner", "", "team", "a"),
}

func Test_Suite_ClusterEndpoint(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName:    endpointType,
		Schema:      readFile(t, "aws-neptune-dbclusterendpoint.json"),
		Metadata:    readFile(t, "aws-neptune-dbclusterendpoint.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			addClusters(store)
			api := newFakeAPI(store)
			return fs.add(NewClusterEndpoint(api), api)
		},
		IdentifierProperty: "DBClusterEndpointIdentifier",
		Properties:         endpoint,
		Identifier:         "reads",
		Changed:            with(endpoint, map[string]any{"StaticMembers": nil, "ExcludedMembers": []any{"graph-3"}}),
		Removable:          "StaticMembers",
		CreateOnlyChange:   with(endpoint, map[string]any{"DBClusterIdentifier": "other-graph", "StaticMembers": []any{"other-1"}}),
		OtherForm:          map[string]any{"StaticMembers": []any{"graph-2", "graph-1"}},
		SameResource:       []map[string]any{with(endpoint, map[string]any{"DBClusterEndpointIdentifier": "Reads", "DBClusterIdentifier": "Graph"})},
		SameResourceNames:  []string{"READS"},
		Invalid: []map[string]any{
			with(endpoint, map[string]any{"ExcludedMembers": []any{"graph-3"}}),
			with(endpoint, map[string]any{"StaticMembers": []any{"graph-1", 2}}),
			with(endpoint, map[string]any{"ExcludedMembers": "graph-3", "StaticMembers": nil}),
			with(endpoint, map[string]any{"DBClusterIdentifier": 5}),
		},
		OtherProduct: func(store *awsfake.CloudControl, identifier string) {
			store.Put(endpointType, identifier, map[string]any{"DBClusterEndpointIdentifier": identifier, "DBClusterIdentifier": "aurora-app", "EndpointType": "ANY"})
		},
		MultiCall: with(endpoint, map[string]any{"EndpointType": "ANY", "Tags": tags("env", "dev", "team", "b")}),
		Calls:     3,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
		API: clusterEndpointAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := neptune.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Identifier(t *testing.T) {
	h, _ := newHandler()
	id, err := h.Identifier(with(endpoint, map[string]any{"DBClusterEndpointIdentifier": "Reads"}))
	require.NoError(t, err)
	require.Equal(t, "reads", id)
	_, err = h.Identifier(with(endpoint, map[string]any{"ExcludedMembers": []any{"graph-3"}}))
	require.ErrorIs(t, err, bothMembers)
	id, err = h.ResolveIdentifier(t.Context(), region, "READS")
	require.NoError(t, err)
	require.Equal(t, "reads", id)
}

func Test_sameMembers(t *testing.T) {
	tests := []struct {
		path     string
		template any
		aws      any
		want     bool
	}{
		{"StaticMembers", []any{"Graph-1", "graph-2"}, []any{"graph-2", "graph-1"}, true},
		{"ExcludedMembers", []any{"GRAPH-3"}, []any{"graph-3"}, true},
		{"StaticMembers", []any{"graph-1"}, []any{"graph-1", "graph-2"}, false},
		{"StaticMembers", []any{"graph-1", "graph-1"}, []any{"graph-1", "graph-2"}, false},
		{"StaticMembers", "graph-1", []any{"graph-1"}, false},
		{"Tags", []any{"a"}, []any{"A"}, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, sameMembers(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
	}
}

// newHandler returns a handler with a fake API that has the fake's clusters.
func newHandler() (*ClusterEndpoint, *fakeAPI) {
	store := awsfake.NewCloudControl()
	addClusters(store)
	api := newFakeAPI(store)
	return NewClusterEndpoint(api), api
}

// created creates the endpoint and reads it until it is available.
func created(t *testing.T, h *ClusterEndpoint, properties map[string]any) {
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	for range 2 {
		_, err = h.Read(t.Context(), region, "reads")
		require.NoError(t, err)
	}
}

func Test_Create(t *testing.T) {
	h, api := newHandler()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(endpoint, map[string]any{"DBClusterEndpointIdentifier": "Reads"})})
	require.NoError(t, err)
	require.Equal(t, "reads", id, "Neptune stores the identifier in lowercase")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(endpoint, map[string]any{"DBClusterIdentifier": "missing"})})
	require.ErrorContains(t, err, "DBClusterNotFoundFault")

	var failure *native.Failure
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(endpoint, map[string]any{"DBClusterIdentifier": "aurora-app", "StaticMembers": nil})})
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "aurora-postgresql")
	require.Nil(t, api.store.Resource(endpointType, "reads-2"))

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(endpoint, map[string]any{"ExcludedMembers": []any{"graph-3"}})})
	require.ErrorIs(t, err, bothMembers)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(endpoint, map[string]any{"DBClusterEndpointIdentifier": "any", "StaticMembers": nil})})
	require.NoError(t, err)
	require.Nil(t, api.lastCreate.StaticMembers, "a create without members sends no member list")
	require.Nil(t, api.lastCreate.ExcludedMembers)
}

// Test_CreateIgnoresOtherCaseKeys checks that a key in another case, such as dbClusterIdentifier, can't send the
// create to another cluster than the one the product check read.
func Test_CreateIgnoresOtherCaseKeys(t *testing.T) {
	h, api := newHandler()
	properties := with(endpoint, map[string]any{
		"dbClusterIdentifier":         "aurora-app",
		"dbClusterEndpointIdentifier": "other",
		"staticMembers":               []any{"aurora-1"},
	})
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, "reads", id)
	stored := api.store.Resource(endpointType, "reads")
	require.Equal(t, "graph", stored["DBClusterIdentifier"])
	require.Equal(t, []any{"graph-1", "graph-2"}, stored["StaticMembers"])
	require.Nil(t, api.store.Resource(endpointType, "other"))
}

func Test_ReadOtherProduct(t *testing.T) {
	h, api := newHandler()
	api.store.Put(endpointType, "docs-reads", map[string]any{"DBClusterEndpointIdentifier": "docs-reads", "DBClusterIdentifier": "docs", "EndpointType": "ANY"})
	var other *native.OtherProductError
	_, err := h.Read(t.Context(), region, "docs-reads")
	require.ErrorAs(t, err, &other)
	require.Equal(t, "docdb", other.Product)
	require.ErrorAs(t, h.Delete(t.Context(), region, "docs-reads"), &other)
	_, err = h.Progress(t.Context(), region, "docs-reads", native.OperationCreate)
	require.ErrorAs(t, err, &other)
	require.NotNil(t, api.store.Resource(endpointType, "docs-reads"), "nothing changes the other product's endpoint")

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "docs-reads", Desired: map[string]any{"EndpointType": "READER"}})
	require.ErrorAs(t, err, &other)
}

func Test_Update(t *testing.T) {
	h, api := newHandler()
	created(t, h, endpoint)
	current, err := h.Read(t.Context(), region, "reads")
	require.NoError(t, err)

	desired := with(endpoint, map[string]any{"StaticMembers": nil, "ExcludedMembers": []any{"graph-3"}})
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Current: current, Desired: desired, Patch: patch}))
	stored := api.store.Resource(endpointType, "reads")
	require.Equal(t, []any{"graph-3"}, stored["ExcludedMembers"])
	require.NotContains(t, stored, "StaticMembers", "switching lists sends an empty static list")

	require.ErrorIs(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: with(desired, map[string]any{"StaticMembers": []any{"graph-1"}})}), bothMembers)
}

func Test_UpdateRemovesMembers(t *testing.T) {
	h, api := newHandler()
	created(t, h, endpoint)
	current, err := h.Read(t.Context(), region, "reads")
	require.NoError(t, err)
	desired := with(endpoint, map[string]any{"StaticMembers": nil})
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Current: current, Desired: desired, Patch: patch}))
	stored := api.store.Resource(endpointType, "reads")
	require.NotContains(t, stored, "StaticMembers")
	require.NotContains(t, stored, "ExcludedMembers")
	require.Equal(t, "READER", stored["EndpointType"], "a member change doesn't send the type")
}

func Test_Delete(t *testing.T) {
	h, api := newHandler()
	require.ErrorIs(t, h.Delete(t.Context(), region, "reads"), native.ErrNotFound)

	created(t, h, endpoint)
	require.NoError(t, h.Delete(t.Context(), region, "reads"))
	progress, err := h.Progress(t.Context(), region, "reads", native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "deleting", Waiting: "deleted"}, progress)
	_, err = h.Progress(t.Context(), region, "reads", native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	created(t, h, endpoint)
	api.status["reads"] = "deleting"
	require.NoError(t, h.Delete(t.Context(), region, "reads"), "an endpoint AWS is deleting had a delete reach AWS")

	created(t, h, endpoint)
	api.errs["DeleteDBClusterEndpoint"] = fault("DBClusterEndpointNotFoundFault")
	require.ErrorIs(t, h.Delete(t.Context(), region, "reads"), native.ErrNotFound)
}

// Test_DeleteRace covers a delete that AWS rejects because another delete started after the handler's read.
func Test_DeleteRace(t *testing.T) {
	h, api := newHandler()
	created(t, h, endpoint)
	api.errs["DeleteDBClusterEndpoint"] = fault("InvalidDBClusterEndpointStateFault")

	require.ErrorContains(t, h.Delete(t.Context(), region, "reads"), "InvalidDBClusterEndpointStateFault", "an endpoint in another state isn't deleted")

	delete(api.errs, "DeleteDBClusterEndpoint")
	racing := &racingDelete{fakeAPI: api}
	h = NewClusterEndpoint(racing)
	require.NoError(t, h.Delete(t.Context(), region, "reads"), "an endpoint another delete started on is deleted")
}

// racingDelete starts deleting the endpoint when the handler's delete call reaches it, as if another delete got there
// first.
type racingDelete struct {
	*fakeAPI
}

func (r *racingDelete) DeleteDBClusterEndpoint(ctx context.Context, in *neptune.DeleteDBClusterEndpointInput, opts ...func(*neptune.Options)) (*neptune.DeleteDBClusterEndpointOutput, error) {
	r.mu.Lock()
	r.status[aws.ToString(in.DBClusterEndpointIdentifier)] = "deleting"
	r.mu.Unlock()
	return r.fakeAPI.DeleteDBClusterEndpoint(ctx, in, opts...)
}

func Test_List(t *testing.T) {
	h, api := newHandler()
	created(t, h, endpoint)
	api.store.Put(endpointType, "aurora-reads", map[string]any{"DBClusterEndpointIdentifier": "aurora-reads", "DBClusterIdentifier": "aurora-app", "EndpointType": "ANY"})
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, 1, "endpoints of other products and built-in endpoints are left out")
	require.Equal(t, "reads", resources[0].Identifier)
	require.Equal(t, "arn:aws:rds:us-west-2:123456789012:cluster-endpoint:reads", resources[0].Properties["DBClusterEndpointArn"])

	for _, call := range []string{"DescribeDBClusters", "DescribeDBClusterEndpoints", "ListTagsForResource"} {
		api.errs[call] = fault("ThrottlingException")
		_, err = h.List(t.Context(), region)
		require.ErrorContains(t, err, "ThrottlingException", call)
		delete(api.errs, call)
	}
}

func Test_ListPages(t *testing.T) {
	h, api := newHandler()
	putClusters(api.store, 120)
	putEndpoints(api.store, 150)
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, 150)
}

func Test_Progress(t *testing.T) {
	h, api := newHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpoint})
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), region, "reads", native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "creating", Waiting: "available"}, progress)

	api.status["reads"] = "inactive"
	progress, err = h.Progress(t.Context(), region, "reads", native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done, "an inactive endpoint is finished")
	require.Nil(t, progress.Failure)

	api.errs["DescribeDBClusters"] = fault("ThrottlingException")
	_, err = h.Progress(t.Context(), region, "reads", native.OperationCreate)
	require.ErrorContains(t, err, "ThrottlingException")
}

// emptyClusters is a Neptune API whose DescribeDBClusters returns no clusters, and whose delete starts another delete,
// fails, and then fails the describe that follows.
type emptyClusters struct {
	*fakeAPI
	failDescribe bool
}

func (e *emptyClusters) DescribeDBClusters(ctx context.Context, in *neptune.DescribeDBClustersInput, opts ...func(*neptune.Options)) (*neptune.DescribeDBClustersOutput, error) {
	if e.failDescribe {
		return nil, fault("ThrottlingException")
	}
	if in.DBClusterIdentifier != nil && aws.ToString(in.DBClusterIdentifier) == "empty" {
		return &neptune.DescribeDBClustersOutput{}, nil
	}
	return e.fakeAPI.DescribeDBClusters(ctx, in, opts...)
}

func (e *emptyClusters) DeleteDBClusterEndpoint(ctx context.Context, in *neptune.DeleteDBClusterEndpointInput, opts ...func(*neptune.Options)) (*neptune.DeleteDBClusterEndpointOutput, error) {
	e.failDescribe = true
	return nil, fault("InvalidDBClusterEndpointStateFault")
}

func Test_ErrorPaths(t *testing.T) {
	h, api := newHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(endpoint, map[string]any{"EndpointType": 5})})
	require.ErrorContains(t, err, "EndpointType of an AWS::Neptune::DBClusterEndpoint must be a string", "a value of the wrong type isn't sent")

	api.errs["CreateDBClusterEndpoint"] = fault("DBClusterEndpointAlreadyExistsFault")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpoint})
	require.ErrorContains(t, err, "DBClusterEndpointAlreadyExistsFault")
	delete(api.errs, "CreateDBClusterEndpoint")

	api.errs["DescribeDBClusterEndpoints"] = fault("DBClusterEndpointNotFoundFault")
	_, err = h.Read(t.Context(), region, "reads")
	require.ErrorIs(t, err, native.ErrNotFound)
	delete(api.errs, "DescribeDBClusterEndpoints")

	empty := &emptyClusters{fakeAPI: api}
	h = NewClusterEndpoint(empty)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(endpoint, map[string]any{"DBClusterIdentifier": "empty"})})
	require.ErrorIs(t, err, native.ErrNotFound, "a describe that returns no cluster is not found")

	created(t, h, endpoint)
	require.ErrorContains(t, h.Delete(t.Context(), region, "reads"), "ThrottlingException", "the describe after a rejected delete fails")
}

func Test_CreateAlreadyExists(t *testing.T) {
	h, api := newHandler()
	api.errs["CreateDBClusterEndpoint"] = fault("DBClusterEndpointAlreadyExistsFault")
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpoint})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "an earlier attempt may have made the endpoint, so UCP keeps the record")
	require.Contains(t, failure.Message, "DBClusterEndpointAlreadyExistsFault")
	api.errs["CreateDBClusterEndpoint"] = fault("DBClusterEndpointQuotaExceededFault")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpoint})
	require.False(t, errors.As(err, &failure), "any other error is returned as AWS gave it")
	require.ErrorContains(t, err, "DBClusterEndpointQuotaExceededFault")
}

// Test_EmptyMemberLists checks that a template that sets a member list to [] matches AWS, so a redeploy changes
// nothing, and that Read reports both lists.
func Test_EmptyMemberLists(t *testing.T) {
	h, _ := newHandler()
	created(t, h, endpoint)
	properties, err := h.Read(t.Context(), region, "reads")
	require.NoError(t, err)
	require.Equal(t, []any{}, properties["ExcludedMembers"])
	require.Equal(t, []any{"graph-1", "graph-2"}, properties["StaticMembers"])

	current, err := json.Marshal(properties)
	require.NoError(t, err)
	desired, err := json.Marshal(with(endpoint, map[string]any{"ExcludedMembers": []any{}}))
	require.NoError(t, err)
	patch, err := awsoperations.GeneratePatch(current, desired, []byte(readFile(t, "aws-neptune-dbclusterendpoint.json")))
	require.NoError(t, err)
	require.Empty(t, patch, "a redeploy of a template with an empty list sends nothing")
}

// update reads the endpoint reads and sends an update to desired, with the patch UCP computes from the template and the
// last deploy, lastApplied.
func update(t *testing.T, h *ClusterEndpoint, lastApplied map[string]any, desired map[string]any) error {
	current, err := h.Read(t.Context(), region, "reads")
	require.NoError(t, err)
	c, err := json.Marshal(current)
	require.NoError(t, err)
	d, err := json.Marshal(desired)
	require.NoError(t, err)
	removed := []string{}
	for k := range lastApplied {
		if _, ok := desired[k]; !ok {
			removed = append(removed, k)
		}
	}
	patch, err := awsoperations.GeneratePatchWithOptions(c, d, []byte(readFile(t, "aws-neptune-dbclusterendpoint.json")), awsoperations.PatchOptions{LastApplied: lastApplied, Removed: removed})
	require.NoError(t, err)
	return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Current: current, Desired: desired, Patch: patch})
}

// Test_UpdateMemberLists covers which member lists an update sends.
func Test_UpdateMemberLists(t *testing.T) {
	base := with(endpoint, map[string]any{"StaticMembers": nil, "Tags": nil})
	tests := []struct {
		name        string
		aws         map[string]any
		lastApplied map[string]any
		desired     map[string]any
		static      []string
		excluded    []string
		modify      bool
	}{
		{
			name:        "removing a list that is already empty sends nothing and keeps the other list",
			aws:         with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			lastApplied: with(base, map[string]any{"ExcludedMembers": []any{}}),
			desired:     base,
		},
		{
			name:        "removing the static list clears only it",
			aws:         with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			lastApplied: with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			desired:     base,
			static:      []string{},
			modify:      true,
		},
		{
			name:        "switching to an exclusion list clears the static list",
			aws:         with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			lastApplied: with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			desired:     with(base, map[string]any{"ExcludedMembers": []any{"graph-3"}}),
			static:      []string{},
			excluded:    []string{"graph-3"},
			modify:      true,
		},
		{
			name:        "an exclusion list over a static list set outside Radius clears it, as Neptune requires",
			aws:         with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			lastApplied: base,
			desired:     with(base, map[string]any{"ExcludedMembers": []any{"graph-3"}}),
			static:      []string{},
			excluded:    []string{"graph-3"},
			modify:      true,
		},
		{
			name:        "a list in another case is the same list",
			aws:         with(base, map[string]any{"StaticMembers": []any{"graph-1", "graph-2"}}),
			lastApplied: with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			desired:     with(base, map[string]any{"StaticMembers": []any{"Graph-2", "GRAPH-1"}}),
		},
		{
			name:        "a type change sends no member list",
			aws:         with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			lastApplied: with(base, map[string]any{"StaticMembers": []any{"graph-1"}}),
			desired:     with(base, map[string]any{"StaticMembers": []any{"graph-1"}, "EndpointType": "ANY"}),
			modify:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, api := newHandler()
			created(t, h, tt.aws)
			require.NoError(t, update(t, h, tt.lastApplied, tt.desired))
			if !tt.modify {
				require.Nil(t, api.lastModify)
				return
			}
			require.NotNil(t, api.lastModify)
			require.Equal(t, tt.static, api.lastModify.StaticMembers)
			require.Equal(t, tt.excluded, api.lastModify.ExcludedMembers)
		})
	}
}

func Test_MemberListTypes(t *testing.T) {
	h, _ := newHandler()
	for _, properties := range []map[string]any{
		with(endpoint, map[string]any{"StaticMembers": []any{"graph-1", 2}}),
		with(endpoint, map[string]any{"StaticMembers": "graph-1"}),
	} {
		var failure *native.Failure
		_, err := h.Identifier(properties)
		require.ErrorAs(t, err, &failure)
		require.Contains(t, failure.Message, "StaticMembers")
		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
		require.ErrorAs(t, err, &failure, "a create never sends a shorter list")
		require.ErrorAs(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: properties}), &failure)
	}
	id, err := h.Identifier(with(endpoint, map[string]any{"StaticMembers": nil, "ExcludedMembers": nil}))
	require.NoError(t, err, "a list set to null is left out")
	require.Equal(t, "reads", id)
	_, err = h.Identifier(with(endpoint, map[string]any{"StaticMembers": []any{}, "ExcludedMembers": []any{"graph-3"}}))
	require.NoError(t, err, "an empty static list with an exclusion list sets one list")
}

// Test_ValueTypes checks that a value of the wrong type is rejected before any AWS call, instead of being sent as an
// empty value or left out.
func Test_ValueTypes(t *testing.T) {
	h, api := newHandler()
	for name, properties := range map[string]map[string]any{
		"a cluster that isn't a string":        with(endpoint, map[string]any{"DBClusterIdentifier": 5}),
		"an endpoint name that isn't a string": with(endpoint, map[string]any{"DBClusterEndpointIdentifier": 5}),
		"a type that isn't a string":           with(endpoint, map[string]any{"EndpointType": []any{"READER"}}),
		"tags that aren't a list":              with(endpoint, map[string]any{"Tags": map[string]any{"team": "a"}}),
		"a tag that isn't an object":           with(endpoint, map[string]any{"Tags": []any{"team"}}),
		"a tag key that isn't a string":        with(endpoint, map[string]any{"Tags": []any{map[string]any{"Key": 1, "Value": "a"}}}),
		"a tag value that isn't a string":      with(endpoint, map[string]any{"Tags": []any{map[string]any{"Key": "team", "Value": 1}}}),
	} {
		t.Run(name, func(t *testing.T) {
			var failure *native.Failure
			_, err := h.Identifier(properties)
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)
			_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
			require.ErrorAs(t, err, &failure)
			require.Nil(t, api.lastCreate, "nothing is sent")
			require.ErrorAs(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: properties}), &failure)
		})
	}
	_, err := h.Identifier(with(endpoint, map[string]any{"Tags": []any{map[string]any{"Key": "team"}}}))
	require.NoError(t, err, "a tag without a value is a tag with an empty value")
}
