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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/neptune"
	"github.com/aws/aws-sdk-go-v2/service/neptune/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	arnPrefix = "arn:aws:rds:us-west-2:123456789012:"

	// clusterType keeps the fake's clusters in the store, apart from the endpoints.
	clusterType = "AWS::Neptune::DBCluster"

	// pageSize is how many records a describe returns when it doesn't set MaxRecords.
	pageSize = 100
)

// instances are the DB instances of each cluster the fake has. clusters adds the clusters to a store.
var instances = map[string][]string{
	"graph":       {"graph-1", "graph-2", "graph-3"},
	"other-graph": {"other-1"},
	"aurora-app":  {"aurora-1"},
	"docs":        {"docs-1"},
}

// addClusters adds the fake's clusters to store: two Neptune clusters, an Aurora cluster, and a DocumentDB cluster,
// which Neptune's API also returns.
func addClusters(store *awsfake.CloudControl) {
	for id, engine := range map[string]string{"graph": Engine, "other-graph": Engine, "aurora-app": "aurora-postgresql", "docs": "docdb"} {
		store.Put(clusterType, id, map[string]any{"DBClusterIdentifier": id, "Engine": engine})
	}
}

// fakeAPI is an in-memory Neptune API for custom cluster endpoints. It keeps them in an awsfake store, in the type's
// property format, so the suite's Cloud Control sees the same resources. Clusters are in the store too, under
// clusterType, and each has the instances in instances. A new or changed endpoint reports creating or modifying for one
// read before it is available, and a deleted one reports deleting for one read before it is gone. It follows the
// rules in clusterEndpointAPI.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu     sync.Mutex
	status map[string]string

	// lastModify is the input of the last ModifyDBClusterEndpoint call that reached the fake.
	lastModify *neptune.ModifyDBClusterEndpointInput

	// lastCreate is the input of the last CreateDBClusterEndpoint call.
	lastCreate *neptune.CreateDBClusterEndpointInput

	// busy are the clusters and instances that aren't available, by identifier.
	busy map[string]bool

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, status: map[string]string{}, busy: map[string]bool{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// rules collects the first Neptune rule a call breaks, as the client fault AWS returns for it.
type rules struct {
	err error
}

func (r *rules) check(ok bool, code string) {
	if r.err == nil && !ok {
		r.err = fault(code)
	}
}

var (
	endpointTypes = []string{"READER", "WRITER", "ANY"}
	tagPattern    = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-]*$`)
	filterValues  = map[string][]string{
		"db-cluster-endpoint-type":        {"reader", "writer", "custom"},
		"db-cluster-endpoint-custom-type": {"reader", "any"},
		"db-cluster-endpoint-status":      {"available", "creating", "deleting", "inactive", "modifying"},
		"db-cluster-endpoint-id":          nil,
	}
)

// validTags reports whether every tag follows the Tag data type's rules.
func validTags(tags []types.Tag) bool {
	for _, t := range tags {
		key, value := aws.ToString(t.Key), aws.ToString(t.Value)
		if n := utf8.RuneCountInString(key); n < 1 || n > 128 || utf8.RuneCountInString(value) > 256 ||
			!tagPattern.MatchString(key) || !tagPattern.MatchString(value) {
			return false
		}
		for _, prefix := range []string{"aws:", "rds:"} {
			if strings.HasPrefix(key, prefix) || strings.HasPrefix(value, prefix) {
				return false
			}
		}
	}
	return true
}

func validMaxRecords(n *int32) bool {
	return n == nil || (*n >= 20 && *n <= 100)
}

// page returns the items of list from the position marker names, up to maxRecords, and the marker of the rest.
func page[T any](list []T, marker *string, maxRecords *int32) ([]T, *string) {
	size := pageSize
	if maxRecords != nil {
		size = int(*maxRecords)
	}
	start, _ := strconv.Atoi(aws.ToString(marker))
	end := min(start+size, len(list))
	var next *string
	if end < len(list) {
		next = aws.String(strconv.Itoa(end))
	}
	return list[min(start, end):end], next
}

// clusterOf returns the cluster with the identifier, ignoring case, or nil.
func (f *fakeAPI) clusterOf(id string) map[string]any {
	return f.store.Resource(clusterType, strings.ToLower(id))
}

// state returns the endpoint's status, and moves a transitional status on: a create or update finishes, and a delete
// removes the endpoint, which then returns false.
func (f *fakeAPI) state(id string) (string, bool) {
	status, ok := f.status[id]
	if !ok {
		return "available", true
	}
	switch status {
	case "creating", "modifying":
		delete(f.status, id)
	case "deleting":
		delete(f.status, id)
		f.store.Remove(endpointType, id)
	}
	return status, true
}

func (f *fakeAPI) change(apply func() error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

// members checks the static and excluded members of an endpoint of cluster: one list or the other, of the cluster's
// available instances.
func (f *fakeAPI) members(r *rules, cluster string, static []string, excluded []string) {
	r.check(len(static) == 0 || len(excluded) == 0, "InvalidParameterCombination")
	for _, m := range slices.Concat(static, excluded) {
		r.check(slices.Contains(instances[strings.ToLower(cluster)], strings.ToLower(m)), "DBInstanceNotFound")
		r.check(!f.busy[strings.ToLower(m)], "InvalidDBInstanceState")
	}
}

func lowerAll(list []string) []any {
	out := []any{}
	for _, v := range list {
		out = append(out, strings.ToLower(v))
	}
	return out
}

func (f *fakeAPI) CreateDBClusterEndpoint(ctx context.Context, in *neptune.CreateDBClusterEndpointInput, _ ...func(*neptune.Options)) (*neptune.CreateDBClusterEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["CreateDBClusterEndpoint"]; err != nil {
		return nil, err
	}
	f.lastCreate = in
	id := strings.ToLower(aws.ToString(in.DBClusterEndpointIdentifier))
	clusterID := strings.ToLower(aws.ToString(in.DBClusterIdentifier))
	r := &rules{}
	r.check(id != "" && len(id) <= 63, "InvalidParameterValue")
	r.check(slices.Contains(endpointTypes, aws.ToString(in.EndpointType)), "InvalidParameterValue")
	r.check(validTags(in.Tags), "InvalidParameterValue")
	r.check(f.store.Resource(endpointType, id) == nil, "DBClusterEndpointAlreadyExistsFault")
	r.check(f.clusterOf(clusterID) != nil, "DBClusterNotFoundFault")
	r.check(!f.busy[clusterID], "InvalidDBClusterStateFault")
	f.members(r, clusterID, in.StaticMembers, in.ExcludedMembers)
	if r.err != nil {
		return nil, r.err
	}
	return &neptune.CreateDBClusterEndpointOutput{}, f.change(func() error {
		props := map[string]any{
			"DBClusterEndpointIdentifier":         id,
			"DBClusterIdentifier":                 clusterID,
			"EndpointType":                        aws.ToString(in.EndpointType),
			"DBClusterEndpointArn":                arnPrefix + "cluster-endpoint:" + id,
			"DBClusterEndpointResourceIdentifier": "cluster-endpoint-" + strings.ToUpper(id),
			"Endpoint":                            id + ".cluster-custom-abc123.us-west-2.neptune.amazonaws.com",
		}
		if len(in.StaticMembers) > 0 {
			props["StaticMembers"] = lowerAll(in.StaticMembers)
		}
		if len(in.ExcludedMembers) > 0 {
			props["ExcludedMembers"] = lowerAll(in.ExcludedMembers)
		}
		if len(in.Tags) > 0 {
			props["Tags"] = awsapi.TagListOf(in.Tags)
		}
		f.store.Put(endpointType, id, props)
		f.status[id] = "creating"
		return nil
	})
}

// endpointOf returns the endpoint in the store as AWS returns it, with its status, or false when it is gone.
func (f *fakeAPI) endpointOf(id string) (types.DBClusterEndpoint, bool) {
	props := f.store.Resource(endpointType, id)
	if props == nil {
		return types.DBClusterEndpoint{}, false
	}
	status, ok := f.state(id)
	if !ok {
		return types.DBClusterEndpoint{}, false
	}
	str := func(name string) *string {
		if s, ok := props[name].(string); ok {
			return aws.String(s)
		}
		return nil
	}
	return types.DBClusterEndpoint{
		DBClusterEndpointIdentifier:         str("DBClusterEndpointIdentifier"),
		DBClusterIdentifier:                 str("DBClusterIdentifier"),
		CustomEndpointType:                  str("EndpointType"),
		EndpointType:                        aws.String(customEndpoint),
		StaticMembers:                       awsapi.Strings(props, "StaticMembers"),
		ExcludedMembers:                     awsapi.Strings(props, "ExcludedMembers"),
		DBClusterEndpointArn:                aws.String(arnPrefix + "cluster-endpoint:" + id),
		DBClusterEndpointResourceIdentifier: str("DBClusterEndpointResourceIdentifier"),
		Endpoint:                            str("Endpoint"),
		Status:                              aws.String(status),
	}, true
}

// builtIn returns the writer and reader endpoints every cluster has, which have no identifier.
func builtIn(cluster string) []types.DBClusterEndpoint {
	out := []types.DBClusterEndpoint{}
	for _, t := range []string{"WRITER", "READER"} {
		out = append(out, types.DBClusterEndpoint{
			DBClusterIdentifier: aws.String(cluster),
			EndpointType:        aws.String(t),
			Endpoint:            aws.String(cluster + ".cluster-abc123.us-west-2.neptune.amazonaws.com"),
			Status:              aws.String("available"),
		})
	}
	return out
}

// DescribeDBClusterEndpoints returns the custom endpoints of every cluster, of every product, and each cluster's
// built-in endpoints, as Neptune's API does.
func (f *fakeAPI) DescribeDBClusterEndpoints(ctx context.Context, in *neptune.DescribeDBClusterEndpointsInput, _ ...func(*neptune.Options)) (*neptune.DescribeDBClusterEndpointsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DescribeDBClusterEndpoints"]; err != nil {
		return nil, err
	}
	r := &rules{}
	r.check(validMaxRecords(in.MaxRecords), "InvalidParameterValue")
	for _, filter := range in.Filters {
		allowed, ok := filterValues[aws.ToString(filter.Name)]
		r.check(ok, "InvalidParameterValue")
		for _, v := range filter.Values {
			r.check(allowed == nil || slices.Contains(allowed, v), "InvalidParameterValue")
		}
	}
	cluster := strings.ToLower(aws.ToString(in.DBClusterIdentifier))
	r.check(cluster == "" || f.clusterOf(cluster) != nil, "DBClusterNotFoundFault")
	if r.err != nil {
		return nil, r.err
	}
	all := []types.DBClusterEndpoint{}
	for _, c := range f.store.List(clusterType) {
		all = append(all, builtIn(c)...)
	}
	for _, id := range f.store.List(endpointType) {
		if e, ok := f.endpointOf(id); ok {
			all = append(all, e)
		}
	}
	want := strings.ToLower(aws.ToString(in.DBClusterEndpointIdentifier))
	matches := slices.DeleteFunc(all, func(e types.DBClusterEndpoint) bool {
		if want != "" && aws.ToString(e.DBClusterEndpointIdentifier) != want {
			return true
		}
		if cluster != "" && aws.ToString(e.DBClusterIdentifier) != cluster {
			return true
		}
		for _, filter := range in.Filters {
			if aws.ToString(filter.Name) == "db-cluster-endpoint-type" && !slices.Contains(filter.Values, strings.ToLower(aws.ToString(e.EndpointType))) {
				return true
			}
		}
		return false
	})
	list, marker := page(matches, in.Marker, in.MaxRecords)
	return &neptune.DescribeDBClusterEndpointsOutput{DBClusterEndpoints: list, Marker: marker}, nil
}

func (f *fakeAPI) ModifyDBClusterEndpoint(ctx context.Context, in *neptune.ModifyDBClusterEndpointInput, _ ...func(*neptune.Options)) (*neptune.ModifyDBClusterEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ModifyDBClusterEndpoint"]; err != nil {
		return nil, err
	}
	f.lastModify = in
	id := strings.ToLower(aws.ToString(in.DBClusterEndpointIdentifier))
	props := f.store.Resource(endpointType, id)
	if props == nil {
		return nil, fault("DBClusterEndpointNotFoundFault")
	}
	r := &rules{}
	r.check(in.EndpointType == nil || slices.Contains(endpointTypes, aws.ToString(in.EndpointType)), "InvalidParameterValue")
	r.check(f.status[id] == "", "InvalidDBClusterEndpointStateFault")
	r.check(!f.busy[awsapi.String(props, "DBClusterIdentifier")], "InvalidDBClusterStateFault")
	static, excluded := awsapi.Strings(props, "StaticMembers"), awsapi.Strings(props, "ExcludedMembers")
	if in.StaticMembers != nil {
		static = in.StaticMembers
	}
	if in.ExcludedMembers != nil {
		excluded = in.ExcludedMembers
	}
	f.members(r, awsapi.String(props, "DBClusterIdentifier"), static, excluded)
	if r.err != nil {
		return nil, r.err
	}
	return &neptune.ModifyDBClusterEndpointOutput{}, f.change(func() error {
		if in.EndpointType != nil {
			props["EndpointType"] = aws.ToString(in.EndpointType)
		}
		for name, list := range map[string][]string{"StaticMembers": static, "ExcludedMembers": excluded} {
			if len(list) == 0 {
				delete(props, name)
			} else {
				props[name] = lowerAll(list)
			}
		}
		f.store.Put(endpointType, id, props)
		f.status[id] = "modifying"
		return nil
	})
}

func (f *fakeAPI) DeleteDBClusterEndpoint(ctx context.Context, in *neptune.DeleteDBClusterEndpointInput, _ ...func(*neptune.Options)) (*neptune.DeleteDBClusterEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DeleteDBClusterEndpoint"]; err != nil {
		return nil, err
	}
	id := strings.ToLower(aws.ToString(in.DBClusterEndpointIdentifier))
	if f.store.Resource(endpointType, id) == nil {
		return nil, fault("DBClusterEndpointNotFoundFault")
	}
	if f.status[id] != "" {
		return nil, fault("InvalidDBClusterEndpointStateFault")
	}
	if f.busy[awsapi.String(f.store.Resource(endpointType, id), "DBClusterIdentifier")] {
		return nil, fault("InvalidDBClusterStateFault")
	}
	return &neptune.DeleteDBClusterEndpointOutput{}, f.change(func() error {
		f.status[id] = "deleting"
		return nil
	})
}

// DescribeDBClusters returns clusters of every product, as Neptune's API does, unless a filter on engine leaves them
// out.
func (f *fakeAPI) DescribeDBClusters(ctx context.Context, in *neptune.DescribeDBClustersInput, _ ...func(*neptune.Options)) (*neptune.DescribeDBClustersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DescribeDBClusters"]; err != nil {
		return nil, err
	}
	if !validMaxRecords(in.MaxRecords) {
		return nil, fault("InvalidParameterValue")
	}
	ids := f.store.List(clusterType)
	if want := aws.ToString(in.DBClusterIdentifier); want != "" {
		if f.clusterOf(want) == nil {
			return nil, fault("DBClusterNotFoundFault")
		}
		ids = []string{strings.ToLower(want)}
	}
	clusters := []types.DBCluster{}
	for _, id := range ids {
		props := f.clusterOf(id)
		engine := awsapi.String(props, "Engine")
		keep := true
		for _, filter := range in.Filters {
			if aws.ToString(filter.Name) == "engine" && !slices.Contains(filter.Values, engine) {
				keep = false
			}
		}
		if keep {
			clusters = append(clusters, types.DBCluster{DBClusterIdentifier: aws.String(id), Engine: aws.String(engine)})
		}
	}
	list, marker := page(clusters, in.Marker, in.MaxRecords)
	return &neptune.DescribeDBClustersOutput{DBClusters: list, Marker: marker}, nil
}

// endpointOfARN returns the identifier of the endpoint arn names, or "" when the store has none.
func (f *fakeAPI) endpointOfARN(arn *string) string {
	id, ok := strings.CutPrefix(aws.ToString(arn), arnPrefix+"cluster-endpoint:")
	if !ok || f.store.Resource(endpointType, id) == nil {
		return ""
	}
	return id
}

// endpointNotFound is the error the fake returns for a tag call on an endpoint it doesn't have. The tag pages don't
// name one for endpoints.
const endpointNotFound = "DBClusterEndpointNotFoundFault"

func (f *fakeAPI) ListTagsForResource(ctx context.Context, in *neptune.ListTagsForResourceInput, _ ...func(*neptune.Options)) (*neptune.ListTagsForResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListTagsForResource"]; err != nil {
		return nil, err
	}
	id := f.endpointOfARN(in.ResourceName)
	if id == "" {
		return nil, fault(endpointNotFound)
	}
	return &neptune.ListTagsForResourceOutput{TagList: awsapi.SDKTags[types.Tag](awsapi.Tags(f.store.Resource(endpointType, id)))}, nil
}

func (f *fakeAPI) AddTagsToResource(ctx context.Context, in *neptune.AddTagsToResourceInput, _ ...func(*neptune.Options)) (*neptune.AddTagsToResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["AddTagsToResource"]; err != nil {
		return nil, err
	}
	if !validTags(in.Tags) {
		return nil, fault("InvalidParameterValue")
	}
	id := f.endpointOfARN(in.ResourceName)
	if id == "" {
		return nil, fault(endpointNotFound)
	}
	return &neptune.AddTagsToResourceOutput{}, f.change(func() error {
		props := f.store.Resource(endpointType, id)
		tags := awsapi.Tags(props)
		for _, t := range in.Tags {
			tags = slices.DeleteFunc(tags, func(e awsapi.Tag) bool { return e.Key == aws.ToString(t.Key) })
			tags = append(tags, awsapi.Tag{Key: aws.ToString(t.Key), Value: aws.ToString(t.Value)})
		}
		props["Tags"] = awsapi.TagListOf(awsapi.SDKTags[types.Tag](tags))
		f.store.Put(endpointType, id, props)
		return nil
	})
}

func (f *fakeAPI) RemoveTagsFromResource(ctx context.Context, in *neptune.RemoveTagsFromResourceInput, _ ...func(*neptune.Options)) (*neptune.RemoveTagsFromResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["RemoveTagsFromResource"]; err != nil {
		return nil, err
	}
	id := f.endpointOfARN(in.ResourceName)
	if id == "" {
		return nil, fault(endpointNotFound)
	}
	return &neptune.RemoveTagsFromResourceOutput{}, f.change(func() error {
		props := f.store.Resource(endpointType, id)
		tags := slices.DeleteFunc(awsapi.Tags(props), func(t awsapi.Tag) bool { return slices.Contains(in.TagKeys, t.Key) })
		if len(tags) == 0 {
			delete(props, "Tags")
		} else {
			props["Tags"] = awsapi.TagListOf(awsapi.SDKTags[types.Tag](tags))
		}
		f.store.Put(endpointType, id, props)
		return nil
	})
}
