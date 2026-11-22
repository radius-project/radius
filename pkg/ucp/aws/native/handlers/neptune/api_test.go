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
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/neptune"
	"github.com/aws/aws-sdk-go-v2/service/neptune/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference  = "https://docs.aws.amazon.com/neptune/latest/apiref/"
	commonErrors  = apiReference + "CommonErrors.html"
	tagDoc        = apiReference + "API_Tag.html"
	endpointGuide = "https://docs.aws.amazon.com/neptune/latest/userguide/feature-custom-endpoint-membership.html"

	// codesUnclear is what every call's page leaves unclear: the error code of a value limit or of parameters that
	// can't be combined.
	codesUnclear = "The page doesn't name the error for a value out of its limits or parameters that can't be " +
		"combined; the fake returns InvalidParameterValue and InvalidParameterCombination, from the Common Errors page."

	endpointArn = arnPrefix + "cluster-endpoint:reads"
)

// apiCall is a call against a fake Neptune API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake Neptune API backed by store, which
// has the fake's clusters. It stops at the first call that fails, and returns its error.
func against(calls ...apiCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		addClusters(store)
		f := newFakeAPI(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// createEndpoint creates the endpoint reads of the cluster graph, a READER endpoint over graph-1 and graph-2, with the
// changes edits make to the input.
func createEndpoint(edits ...func(*neptune.CreateDBClusterEndpointInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &neptune.CreateDBClusterEndpointInput{
			DBClusterEndpointIdentifier: aws.String("reads"),
			DBClusterIdentifier:         aws.String("graph"),
			EndpointType:                aws.String("READER"),
			StaticMembers:               []string{"graph-1", "graph-2"},
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateDBClusterEndpoint(ctx, in)
		return err
	}
}

// describeEndpoint returns the endpoint with the identifier.
func describeEndpoint(ctx context.Context, f *fakeAPI, id string) (types.DBClusterEndpoint, error) {
	out, err := f.DescribeDBClusterEndpoints(ctx, &neptune.DescribeDBClusterEndpointsInput{DBClusterEndpointIdentifier: aws.String(id)})
	if err != nil {
		return types.DBClusterEndpoint{}, err
	}
	if len(out.DBClusterEndpoints) != 1 {
		return types.DBClusterEndpoint{}, fmt.Errorf("the describe of %s returned %d endpoints", id, len(out.DBClusterEndpoints))
	}
	return out.DBClusterEndpoints[0], nil
}

// ready reads the endpoint reads until it is available.
func ready(ctx context.Context, f *fakeAPI) error {
	for range 2 {
		if _, err := describeEndpoint(ctx, f, "reads"); err != nil {
			return err
		}
	}
	return nil
}

func modify(in *neptune.ModifyDBClusterEndpointInput) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in.DBClusterEndpointIdentifier = aws.String("reads")
		_, err := f.ModifyDBClusterEndpoint(ctx, in)
		return err
	}
}

func deleteEndpoint(ctx context.Context, f *fakeAPI) error {
	_, err := f.DeleteDBClusterEndpoint(ctx, &neptune.DeleteDBClusterEndpointInput{DBClusterEndpointIdentifier: aws.String("reads")})
	return err
}

// busy returns a call that makes the cluster or instance with the identifier unavailable.
func busy(id string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		f.busy[id] = true
		return nil
	}
}

// expectStatus returns a call that reads the endpoint reads and fails unless its status is want.
func expectStatus(want string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		e, err := describeEndpoint(ctx, f, "reads")
		if err != nil {
			return err
		}
		if got := aws.ToString(e.Status); got != want {
			return fmt.Errorf("the status is %s, not %s", got, want)
		}
		return nil
	}
}

// expectMembers returns a call that reads the endpoint reads and fails unless it has the members.
func expectMembers(static []string, excluded []string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		e, err := describeEndpoint(ctx, f, "reads")
		if err != nil {
			return err
		}
		if !slices.Equal(e.StaticMembers, static) || !slices.Equal(e.ExcludedMembers, excluded) {
			return fmt.Errorf("the members are %v and %v, not %v and %v", e.StaticMembers, e.ExcludedMembers, static, excluded)
		}
		return nil
	}
}

func tag(key string, value string) types.Tag {
	return types.Tag{Key: aws.String(key), Value: aws.String(value)}
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
			Break: against(call([]types.Tag{tag("rds:team", "a")})),
			Code:  "InvalidParameterValue",
		},
	}
}

// existsCheck is the rule that the endpoint a tag call names exists. The tag pages name no error for an endpoint, so
// it checks only that the fake rejects the call.
func existsCheck(call apiCall) nativetest.Rule {
	return nativetest.Rule{
		Rule: "the resource exists",
		Check: against(func(ctx context.Context, f *fakeAPI) error {
			if call(ctx, f) == nil {
				return fmt.Errorf("the fake took a tag call for an endpoint it doesn't have")
			}
			return nil
		}),
	}
}

// tagsUnclear is what the tag pages leave unclear.
const tagsUnclear = " The tag pages name DBClusterNotFoundFault, DBInstanceNotFound, and DBSnapshotNotFound, but no error " +
	"for an endpoint the ARN names that doesn't exist; the fake returns DBClusterEndpointNotFoundFault. The Tag page " +
	"says a value is optional and also 1 to 256 characters; the fake accepts an empty value."

// modifyingUnclear is where the rule that tags can't change while the endpoint is modifying comes from.
const modifyingUnclear = " The pages don't say tags can't change while an endpoint is modifying. AWS answered such a " +
	"call with InvalidDBClusterEndpointStateFault in a run against real AWS, so the handler changes tags before it " +
	"modifies the endpoint. The fake doesn't reject them in other states, which that run didn't cover."

// pagingRules returns the rules of a describe call's MaxRecords and pages. describe makes the call with marker and
// maxRecords, after put stores n records, and returns how many the page has and its marker.
func pagingRules(put func(store *awsfake.CloudControl, n int), describe func(ctx context.Context, f *fakeAPI, marker *string, maxRecords *int32) (int, *string, error)) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "MaxRecords is 20 to 100",
			Doc:  commonErrors,
			Break: against(func(ctx context.Context, f *fakeAPI) error {
				_, _, err := describe(ctx, f, nil, aws.Int32(10))
				return err
			}),
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

// putEndpoints stores n available custom endpoints of the cluster graph.
func putEndpoints(store *awsfake.CloudControl, n int) {
	addClusters(store)
	for i := range n {
		id := "reads-" + strconv.Itoa(i)
		store.Put(endpointType, id, map[string]any{"DBClusterEndpointIdentifier": id, "DBClusterIdentifier": "graph", "EndpointType": "ANY"})
	}
}

// putClusters stores n Neptune clusters.
func putClusters(store *awsfake.CloudControl, n int) {
	for i := range n {
		id := "graph-" + strconv.Itoa(i)
		store.Put(clusterType, id, map[string]any{"DBClusterIdentifier": id, "Engine": Engine})
	}
}

// clusterEndpointAPI records the Neptune calls ClusterEndpoint makes.
var clusterEndpointAPI = map[string]nativetest.Call{
	"CreateDBClusterEndpoint": {
		Doc: apiReference + "API_CreateDBClusterEndpoint.html",
		Rules: append([]nativetest.Rule{
			{
				Rule: "DBClusterEndpointIdentifier and DBClusterIdentifier are stored in lowercase",
				Check: against(createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) {
					in.DBClusterEndpointIdentifier, in.DBClusterIdentifier = aws.String("Reads"), aws.String("GRAPH")
				}), func(ctx context.Context, f *fakeAPI) error {
					e, err := describeEndpoint(ctx, f, "reads")
					if err != nil {
						return err
					}
					if aws.ToString(e.DBClusterEndpointIdentifier) != "reads" || aws.ToString(e.DBClusterIdentifier) != "graph" {
						return fmt.Errorf("the endpoint is %s of %s", aws.ToString(e.DBClusterEndpointIdentifier), aws.ToString(e.DBClusterIdentifier))
					}
					return nil
				}),
			},
			{
				Rule: "the endpoint's name is at most 63 characters",
				Doc:  endpointGuide,
				Break: against(createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) {
					in.DBClusterEndpointIdentifier = aws.String(strings.Repeat("a", 64))
				})),
				Code: "InvalidParameterValue",
			},
			{
				Rule: "no other endpoint in the region has the name",
				Doc:  endpointGuide,
				Break: against(createEndpoint(), createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) {
					in.DBClusterIdentifier = aws.String("other-graph")
					in.StaticMembers = nil
				})),
				Code: "DBClusterEndpointAlreadyExistsFault",
			},
			{
				Rule:  "EndpointType is READER, WRITER, or ANY",
				Doc:   commonErrors,
				Break: against(createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) { in.EndpointType = aws.String("CUSTOM") })),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "the cluster exists",
				Break: against(createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) { in.DBClusterIdentifier = aws.String("missing") })),
				Code:  "DBClusterNotFoundFault",
			},
			{
				Rule:  "the cluster is in a valid state",
				Break: against(busy("graph"), createEndpoint()),
				Code:  "InvalidDBClusterStateFault",
			},
			{
				Rule: "an endpoint has a static list or an exclusion list, not both",
				Doc:  endpointGuide,
				Break: against(createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) {
					in.ExcludedMembers = []string{"graph-3"}
				})),
				Code: "InvalidParameterCombination",
			},
			{
				Rule:  "each member is an instance of the cluster",
				Break: against(createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) { in.StaticMembers = []string{"graph-1", "other-1"} })),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "a member is compared in the case Neptune stores it, lowercase",
				Break: against(createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) { in.StaticMembers = []string{"Graph-1"} })),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "each member is available",
				Break: against(busy("graph-2"), createEndpoint()),
				Code:  "InvalidDBInstanceState",
			},
			{
				Rule:  "a new endpoint is creating, then available",
				Doc:   apiReference + "API_DBClusterEndpoint.html",
				Check: against(createEndpoint(), expectStatus("creating"), expectStatus("available")),
			},
		}, tagRules(func(tags []types.Tag) apiCall {
			return createEndpoint(func(in *neptune.CreateDBClusterEndpointInput) { in.Tags = tags })
		})...),
		Unclear: codesUnclear + " The page lists WRITER as a type, while DescribeDBClusterEndpoints filters custom " +
			"endpoints only by reader and any; the fake takes all three. The guide gives the name's length but no " +
			"pattern, so the fake applies no pattern. It doesn't say which cluster states InvalidDBClusterStateFault " +
			"covers; the fake takes only an available cluster. The page lists DBInstanceNotFound, but AWS answered a " +
			"member in another case than the instance's with InvalidParameterValue (\"does not belong to the cluster\") " +
			"in a run against real AWS, so the fake returns that for any member that isn't an instance of the cluster. " +
			"The SDK doesn't model InvalidParameterValue or InvalidParameterCombination, so their fault is unknown; the " +
			"handler reports any 4xx answer as rejected, which Test_Replay_RejectedCreate shows through the SDK. The page lists DBClusterEndpointQuotaExceededFault without the limit " +
			"it applies, so the fake has none. The call takes no idempotency token: a create sent again gets " +
			"DBClusterEndpointAlreadyExistsFault. The handler reads the endpoint by its name before it creates one, and " +
			"reports that error as an unknown outcome. The SDK sends the call once, so its own retry never gets that error." + tagsUnclear,
	},
	"DescribeDBClusterEndpoints": {
		Doc: apiReference + "API_DescribeDBClusterEndpoints.html",
		Rules: append([]nativetest.Rule{
			{
				Rule: "the call also returns endpoints of Amazon RDS and DocumentDB clusters",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					f.store.Put(endpointType, "aurora-reads", map[string]any{"DBClusterEndpointIdentifier": "aurora-reads", "DBClusterIdentifier": "aurora-app", "EndpointType": "ANY"})
					_, err := describeEndpoint(ctx, f, "aurora-reads")
					return err
				}),
			},
			{
				Rule: "a custom endpoint's EndpointType is CUSTOM, with its type in CustomEndpointType, and a cluster's built-in endpoints are WRITER and READER",
				Check: against(createEndpoint(), func(ctx context.Context, f *fakeAPI) error {
					out, err := f.DescribeDBClusterEndpoints(ctx, &neptune.DescribeDBClusterEndpointsInput{DBClusterIdentifier: aws.String("graph")})
					if err != nil {
						return err
					}
					types := []string{}
					for _, e := range out.DBClusterEndpoints {
						types = append(types, aws.ToString(e.EndpointType)+"/"+aws.ToString(e.CustomEndpointType))
					}
					if want := []string{"WRITER/", "READER/", "CUSTOM/READER"}; !slices.Equal(types, want) {
						return fmt.Errorf("the endpoints are %v, not %v", types, want)
					}
					return nil
				}),
			},
			{
				Rule: "the filter db-cluster-endpoint-type takes reader, writer, or custom",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeDBClusterEndpoints(ctx, &neptune.DescribeDBClusterEndpointsInput{
						Filters: []types.Filter{{Name: aws.String("db-cluster-endpoint-type"), Values: []string{"any"}}},
					})
					return err
				}),
				Code: "InvalidParameterValue",
			},
			{
				Rule: "a filter's name is db-cluster-endpoint-type, db-cluster-endpoint-custom-type, db-cluster-endpoint-id, or db-cluster-endpoint-status",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeDBClusterEndpoints(ctx, &neptune.DescribeDBClusterEndpointsInput{
						Filters: []types.Filter{{Name: aws.String("engine"), Values: []string{"neptune"}}},
					})
					return err
				}),
				Code: "InvalidParameterValue",
			},
			{
				Rule: "the cluster DBClusterIdentifier names exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeDBClusterEndpoints(ctx, &neptune.DescribeDBClusterEndpointsInput{DBClusterIdentifier: aws.String("missing")})
					return err
				}),
				Code: "DBClusterNotFoundFault",
			},
		}, pagingRules(putEndpoints, func(ctx context.Context, f *fakeAPI, marker *string, maxRecords *int32) (int, *string, error) {
			out, err := f.DescribeDBClusterEndpoints(ctx, &neptune.DescribeDBClusterEndpointsInput{
				Filters:    []types.Filter{{Name: aws.String("db-cluster-endpoint-type"), Values: []string{"custom"}}},
				Marker:     marker,
				MaxRecords: maxRecords,
			})
			if err != nil {
				return 0, nil, err
			}
			return len(out.DBClusterEndpoints), out.Marker, nil
		})...),
		Unclear: codesUnclear + " The page doesn't say what AWS returns for a DBClusterEndpointIdentifier no endpoint " +
			"has; the fake returns no endpoints, and the handler treats both that and DBClusterEndpointNotFoundFault " +
			"as not found. It doesn't say whether the identifier is compared ignoring case; the fake folds it to " +
			"lowercase, as it is stored.",
	},
	"ModifyDBClusterEndpoint": {
		Doc: apiReference + "API_ModifyDBClusterEndpoint.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the endpoint exists",
				Break: against(modify(&neptune.ModifyDBClusterEndpointInput{EndpointType: aws.String("ANY")})),
				Code:  "DBClusterEndpointNotFoundFault",
			},
			{
				Rule:  "the endpoint is in a state that allows a change",
				Break: against(createEndpoint(), modify(&neptune.ModifyDBClusterEndpointInput{EndpointType: aws.String("ANY")})),
				Code:  "InvalidDBClusterEndpointStateFault",
			},
			{
				Rule:  "the cluster is in a valid state",
				Break: against(createEndpoint(), ready, busy("graph"), modify(&neptune.ModifyDBClusterEndpointInput{EndpointType: aws.String("ANY")})),
				Code:  "InvalidDBClusterStateFault",
			},
			{
				Rule:  "EndpointType is READER, WRITER, or ANY",
				Doc:   commonErrors,
				Break: against(createEndpoint(), ready, modify(&neptune.ModifyDBClusterEndpointInput{EndpointType: aws.String("CUSTOM")})),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "each member is an instance of the endpoint's cluster",
				Break: against(createEndpoint(), ready, modify(&neptune.ModifyDBClusterEndpointInput{StaticMembers: []string{"graph-9"}})),
				Code:  "InvalidParameterValue",
			},
			{
				Rule:  "each member is available",
				Break: against(createEndpoint(), ready, busy("graph-3"), modify(&neptune.ModifyDBClusterEndpointInput{StaticMembers: []string{"graph-3"}})),
				Code:  "InvalidDBInstanceState",
			},
			{
				Rule: "an endpoint has a static list or an exclusion list, not both",
				Doc:  endpointGuide,
				Break: against(createEndpoint(), ready,
					modify(&neptune.ModifyDBClusterEndpointInput{StaticMembers: []string{"graph-1"}, ExcludedMembers: []string{"graph-3"}})),
				Code: "InvalidParameterCombination",
			},
			{
				Rule: "an endpoint can switch from a static list to an exclusion list",
				Doc:  endpointGuide,
				Check: against(createEndpoint(), ready,
					modify(&neptune.ModifyDBClusterEndpointInput{StaticMembers: []string{}, ExcludedMembers: []string{"graph-3"}}),
					ready, expectMembers(nil, []string{"graph-3"})),
			},
			{
				Rule: "a member list sent empty keeps its members, even with the other list sent empty too",
				Check: against(createEndpoint(), ready,
					modify(&neptune.ModifyDBClusterEndpointInput{StaticMembers: []string{}, ExcludedMembers: []string{}}),
					ready, expectMembers([]string{"graph-1", "graph-2"}, nil)),
			},
			{
				Rule: "a parameter the call leaves out keeps its value",
				Check: against(createEndpoint(), ready, modify(&neptune.ModifyDBClusterEndpointInput{EndpointType: aws.String("ANY")}),
					ready, expectMembers([]string{"graph-1", "graph-2"}, nil)),
			},
			{
				Rule:  "a changed endpoint is modifying, then available",
				Doc:   endpointGuide,
				Check: against(createEndpoint(), ready, modify(&neptune.ModifyDBClusterEndpointInput{EndpointType: aws.String("ANY")}), expectStatus("modifying"), expectStatus("available")),
			},
		},
		Unclear: codesUnclear + " The page doesn't say what an empty member list does. In a run against real AWS, " +
			"Neptune accepted a list sent empty, with or without the other list empty too, and kept its members, while a " +
			"list sent with members replaced the other list; the fake does the same, and the handler rejects a template " +
			"that removes the last list. It doesn't say which endpoint states " +
			"InvalidDBClusterEndpointStateFault or InvalidDBClusterStateFault covers; the fake takes a change only while the " +
			"endpoint and its cluster are available. So a retry of a modify whose response was lost would find the " +
			"endpoint modifying and get InvalidDBClusterEndpointStateFault, and the SDK sends the call once.",
	},
	"DeleteDBClusterEndpoint": {
		Doc: apiReference + "API_DeleteDBClusterEndpoint.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the endpoint exists",
				Break: against(deleteEndpoint),
				Code:  "DBClusterEndpointNotFoundFault",
			},
			{
				Rule:  "the endpoint is in a state that allows a delete",
				Break: against(createEndpoint(), deleteEndpoint),
				Code:  "InvalidDBClusterEndpointStateFault",
			},
			{
				Rule:  "the cluster is in a valid state",
				Break: against(createEndpoint(), ready, busy("graph"), deleteEndpoint),
				Code:  "InvalidDBClusterStateFault",
			},
			{
				Rule: "a deleted endpoint is deleting, then gone",
				Doc:  apiReference + "API_DBClusterEndpoint.html",
				Check: against(createEndpoint(), ready, deleteEndpoint, expectStatus("deleting"), func(ctx context.Context, f *fakeAPI) error {
					if _, err := describeEndpoint(ctx, f, "reads"); err == nil {
						return fmt.Errorf("the endpoint is still there")
					}
					return nil
				}),
			},
		},
		Unclear: "The page doesn't say which endpoint or cluster states InvalidDBClusterEndpointStateFault and " +
			"InvalidDBClusterStateFault cover; the fake deletes only an available endpoint of an available cluster, and " +
			"the handler treats an endpoint already deleting as deleted.",
	},
	"DescribeDBClusters": {
		Doc: apiReference + "API_DescribeDBClusters.html",
		Rules: append([]nativetest.Rule{
			{
				Rule: "the cluster DBClusterIdentifier names exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeDBClusters(ctx, &neptune.DescribeDBClustersInput{DBClusterIdentifier: aws.String("missing")})
					return err
				}),
				Code: "DBClusterNotFoundFault",
			},
			{
				Rule: "DBClusterIdentifier isn't case sensitive",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeDBClusters(ctx, &neptune.DescribeDBClustersInput{DBClusterIdentifier: aws.String("Graph")})
					return err
				}),
			},
			{
				Rule: "the call also returns RDS and DocumentDB clusters, and the filter engine=neptune leaves them out",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					count := func(filters []types.Filter) (int, error) {
						out, err := f.DescribeDBClusters(ctx, &neptune.DescribeDBClustersInput{Filters: filters})
						return len(out.DBClusters), err
					}
					all, err := count(nil)
					if err != nil {
						return err
					}
					neptuneOnly, err := count([]types.Filter{{Name: aws.String("engine"), Values: []string{Engine}}})
					if err != nil {
						return err
					}
					if all != 4 || neptuneOnly != 2 {
						return fmt.Errorf("the call returns %d clusters, and %d with the filter", all, neptuneOnly)
					}
					return nil
				}),
			},
		}, pagingRules(putClusters, func(ctx context.Context, f *fakeAPI, marker *string, maxRecords *int32) (int, *string, error) {
			out, err := f.DescribeDBClusters(ctx, &neptune.DescribeDBClustersInput{Marker: marker, MaxRecords: maxRecords})
			if err != nil {
				return 0, nil, err
			}
			return len(out.DBClusters), out.Marker, nil
		})...),
		Unclear: codesUnclear,
	},
	"ListTagsForResource": {
		Doc:   apiReference + "API_ListTagsForResource.html",
		Rules: []nativetest.Rule{existsCheck(listTags(endpointArn))},
		Unclear: "The page says Filters isn't supported, without saying what AWS does when a call sets it. The " +
			"handler doesn't set it, and the fake ignores it." + tagsUnclear,
	},
	"AddTagsToResource": {
		Doc: apiReference + "API_AddTagsToResource.html",
		Rules: append(tagRules(func(tags []types.Tag) apiCall {
			return func(ctx context.Context, f *fakeAPI) error {
				if err := createEndpoint()(ctx, f); err != nil {
					return err
				}
				return addTags(endpointArn, tags...)(ctx, f)
			}
		}), existsCheck(addTags(endpointArn, tag("team", "a"))), nativetest.Rule{
			Rule: "tags can't change while the endpoint is modifying",
			Break: against(createEndpoint(), ready, modify(&neptune.ModifyDBClusterEndpointInput{EndpointType: aws.String("ANY")}),
				addTags(endpointArn, tag("team", "a"))),
			Code: "InvalidDBClusterEndpointStateFault",
		}),
		Unclear: codesUnclear + tagsUnclear + modifyingUnclear,
	},
	"RemoveTagsFromResource": {
		Doc: apiReference + "API_RemoveTagsFromResource.html",
		Rules: []nativetest.Rule{existsCheck(removeTags(endpointArn)), nativetest.Rule{
			Rule: "tags can't change while the endpoint is modifying",
			Break: against(createEndpoint(), ready, modify(&neptune.ModifyDBClusterEndpointInput{EndpointType: aws.String("ANY")}),
				removeTags(endpointArn)),
			Code: "InvalidDBClusterEndpointStateFault",
		}},
		Unclear: "The page doesn't say what AWS does with a key the resource doesn't have; the fake ignores it." +
			tagsUnclear + modifyingUnclear,
	},
}

func listTags(arn string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.ListTagsForResource(ctx, &neptune.ListTagsForResourceInput{ResourceName: aws.String(arn)})
		return err
	}
}

func addTags(arn string, tags ...types.Tag) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.AddTagsToResource(ctx, &neptune.AddTagsToResourceInput{ResourceName: aws.String(arn), Tags: tags})
		return err
	}
}

func removeTags(arn string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.RemoveTagsFromResource(ctx, &neptune.RemoveTagsFromResourceInput{ResourceName: aws.String(arn), TagKeys: []string{"team"}})
		return err
	}
}
