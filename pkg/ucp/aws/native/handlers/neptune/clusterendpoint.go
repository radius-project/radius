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

// Package neptune has the native handler for Amazon Neptune custom cluster endpoints, which have no CloudFormation
// type. CloudFormation's AWS::RDS::DBClusterEndpoint makes the same API call, but RDS documents it for Aurora clusters
// only. The Neptune API also serves Amazon RDS and DocumentDB, so the handler checks that an endpoint's cluster runs
// the neptune engine before it reads, changes, or deletes the endpoint.
package neptune

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/neptune"
	"github.com/aws/aws-sdk-go-v2/service/neptune/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// Engine is the engine of every Neptune cluster. Others belong to RDS or DocumentDB.
	Engine = "neptune"

	endpointType = "AWS::Neptune::DBClusterEndpoint"

	// customEndpoint is the EndpointType AWS reports for a custom endpoint. The type the template sets is in
	// CustomEndpointType.
	customEndpoint = "CUSTOM"
)

// API is the part of the Neptune client the handler uses.
type API interface {
	CreateDBClusterEndpoint(ctx context.Context, params *neptune.CreateDBClusterEndpointInput, optFns ...func(*neptune.Options)) (*neptune.CreateDBClusterEndpointOutput, error)
	DescribeDBClusterEndpoints(ctx context.Context, params *neptune.DescribeDBClusterEndpointsInput, optFns ...func(*neptune.Options)) (*neptune.DescribeDBClusterEndpointsOutput, error)
	ModifyDBClusterEndpoint(ctx context.Context, params *neptune.ModifyDBClusterEndpointInput, optFns ...func(*neptune.Options)) (*neptune.ModifyDBClusterEndpointOutput, error)
	DeleteDBClusterEndpoint(ctx context.Context, params *neptune.DeleteDBClusterEndpointInput, optFns ...func(*neptune.Options)) (*neptune.DeleteDBClusterEndpointOutput, error)
	DescribeDBClusters(ctx context.Context, params *neptune.DescribeDBClustersInput, optFns ...func(*neptune.Options)) (*neptune.DescribeDBClustersOutput, error)
	ListTagsForResource(ctx context.Context, params *neptune.ListTagsForResourceInput, optFns ...func(*neptune.Options)) (*neptune.ListTagsForResourceOutput, error)
	AddTagsToResource(ctx context.Context, params *neptune.AddTagsToResourceInput, optFns ...func(*neptune.Options)) (*neptune.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(ctx context.Context, params *neptune.RemoveTagsFromResourceInput, optFns ...func(*neptune.Options)) (*neptune.RemoveTagsFromResourceOutput, error)
}

var inRegion = awsapi.InRegion[neptune.Options]

var (
	_ native.Handler            = (*ClusterEndpoint)(nil)
	_ native.IdentifierEncoder  = (*ClusterEndpoint)(nil)
	_ native.IdentifierResolver = (*ClusterEndpoint)(nil)
)

// endpointStates are the states a create and an update wait for. An endpoint is inactive where its cluster can't use
// it, such as a writer endpoint of a read-only secondary cluster in a global database, which is a finished state.
var endpointStates = awsapi.States{Done: []string{"available", "inactive"}}

// ClusterEndpoint serves AWS::Neptune::DBClusterEndpoint. Its identifier is DBClusterEndpointIdentifier, in lowercase
// as Neptune stores it, which UCP generates when the template leaves it out.
type ClusterEndpoint struct {
	api API
}

// NewClusterEndpoint returns the handler for Neptune custom cluster endpoints.
func NewClusterEndpoint(api API) *ClusterEndpoint {
	return &ClusterEndpoint{api: api}
}

// TypeName returns AWS::Neptune::DBClusterEndpoint.
func (h *ClusterEndpoint) TypeName() string {
	return endpointType
}

// Options returns how UCP uses the handler. A change makes the endpoint unusable until Neptune finishes it, so the
// update is disruptive.
func (h *ClusterEndpoint) Options() native.Options {
	return native.Options{
		List:             native.ListRegion,
		Recovery:         native.RecoverByIdentifier,
		NameProperty:     "DBClusterEndpointIdentifier",
		Equivalent:       sameMembers,
		DisruptiveUpdate: true,
	}
}

// sameMembers treats a list of members AWS returns in another case, such as graph-1 for Graph-1, as the same list,
// because they are DB instance identifiers, which Neptune stores in lowercase.
func sameMembers(path string, template any, current any) bool {
	if path != "StaticMembers" && path != "ExcludedMembers" {
		return false
	}
	t, okT := template.([]any)
	c, okC := current.([]any)
	if !okT || !okC || len(t) != len(c) {
		return false
	}
	lower := func(list []any) []string {
		out := []string{}
		for _, v := range list {
			s, _ := v.(string)
			out = append(out, strings.ToLower(s))
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(lower(t), lower(c))
}

// Identifier returns DBClusterEndpointIdentifier in lowercase, as Neptune stores it. UCP calls it before any AWS call,
// so a template that sets both member lists, which Neptune doesn't allow, is rejected here.
func (h *ClusterEndpoint) Identifier(properties map[string]any) (string, error) {
	if setsBoth(properties) {
		return "", bothMembers
	}
	return strings.ToLower(awsapi.String(properties, "DBClusterEndpointIdentifier")), nil
}

// ResolveIdentifier returns the identifier in lowercase, so a name in another case names the same endpoint.
func (h *ClusterEndpoint) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	return strings.ToLower(identifier), nil
}

// cluster returns the cluster with the identifier, as Neptune's API returns it, whatever its engine.
func (h *ClusterEndpoint) cluster(ctx context.Context, region string, identifier string) (*types.DBCluster, error) {
	out, err := h.api.DescribeDBClusters(ctx, &neptune.DescribeDBClustersInput{DBClusterIdentifier: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "DBClusterNotFoundFault")
	}
	if len(out.DBClusters) == 0 {
		return nil, native.ErrNotFound
	}
	return &out.DBClusters[0], nil
}

// describe returns the custom endpoint with the identifier, ErrNotFound, or an OtherProductError when its cluster
// doesn't run neptune.
func (h *ClusterEndpoint) describe(ctx context.Context, region string, identifier string) (*types.DBClusterEndpoint, error) {
	out, err := h.api.DescribeDBClusterEndpoints(ctx, &neptune.DescribeDBClusterEndpointsInput{DBClusterEndpointIdentifier: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "DBClusterEndpointNotFoundFault")
	}
	i := slices.IndexFunc(out.DBClusterEndpoints, func(e types.DBClusterEndpoint) bool {
		return strings.EqualFold(aws.ToString(e.DBClusterEndpointIdentifier), identifier) && strings.EqualFold(aws.ToString(e.EndpointType), customEndpoint)
	})
	if i < 0 {
		return nil, native.ErrNotFound
	}
	endpoint := &out.DBClusterEndpoints[i]
	cluster, err := h.cluster(ctx, region, aws.ToString(endpoint.DBClusterIdentifier))
	if err != nil {
		return nil, err
	}
	if engine := aws.ToString(cluster.Engine); engine != Engine {
		return nil, &native.OtherProductError{Identifier: identifier, Product: engine}
	}
	return endpoint, nil
}

// Read returns the endpoint's properties.
func (h *ClusterEndpoint) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	endpoint, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return h.properties(ctx, region, endpoint)
}

func (h *ClusterEndpoint) properties(ctx context.Context, region string, e *types.DBClusterEndpoint) (map[string]any, error) {
	tags, err := h.api.ListTagsForResource(ctx, &neptune.ListTagsForResourceInput{ResourceName: e.DBClusterEndpointArn}, inRegion(region))
	if err != nil {
		return nil, err
	}
	return awsapi.EncodeObject(map[string]any{
		"DBClusterEndpointIdentifier":         e.DBClusterEndpointIdentifier,
		"DBClusterIdentifier":                 e.DBClusterIdentifier,
		"EndpointType":                        e.CustomEndpointType,
		"StaticMembers":                       e.StaticMembers,
		"ExcludedMembers":                     e.ExcludedMembers,
		"Tags":                                awsapi.TagListOf(tags.TagList),
		"DBClusterEndpointArn":                e.DBClusterEndpointArn,
		"DBClusterEndpointResourceIdentifier": e.DBClusterEndpointResourceIdentifier,
		"Endpoint":                            e.Endpoint,
		"Status":                              e.Status,
	})
}

// bothMembers is the failure for a template that sets StaticMembers and ExcludedMembers, which Neptune doesn't allow.
var bothMembers = &native.Failure{
	Message:     "an AWS::Neptune::DBClusterEndpoint can set StaticMembers or ExcludedMembers, not both",
	ClientError: true,
	Outcome:     native.OutcomeRejected,
}

func setsBoth(properties map[string]any) bool {
	return len(awsapi.Strings(properties, "StaticMembers")) > 0 && len(awsapi.Strings(properties, "ExcludedMembers")) > 0
}

// Create creates the endpoint, after checking that DBClusterIdentifier names a Neptune cluster: the API would also
// make an endpoint of an Aurora or DocumentDB cluster.
func (h *ClusterEndpoint) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	if setsBoth(request.Properties) {
		return "", bothMembers
	}
	clusterID := awsapi.String(request.Properties, "DBClusterIdentifier")
	cluster, err := h.cluster(ctx, request.Region, clusterID)
	if err != nil {
		return "", err
	}
	if engine := aws.ToString(cluster.Engine); engine != Engine {
		return "", &native.Failure{
			Message:     fmt.Sprintf("DBClusterIdentifier %q names a cluster of another product (%s), not a Neptune cluster", clusterID, engine),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	input := &neptune.CreateDBClusterEndpointInput{}
	if err := awsapi.Decode(awsapi.Without(request.Properties, "Tags"), input); err != nil {
		return "", err
	}
	input.Tags = awsapi.SDKTags[types.Tag](awsapi.Tags(request.Properties))
	if _, err := h.api.CreateDBClusterEndpoint(ctx, input, inRegion(request.Region)); err != nil {
		return "", err
	}
	return h.Identifier(request.Properties)
}

// Update changes the endpoint type and members with ModifyDBClusterEndpoint, then adds and removes tags. A change to
// either member list sends both, with an empty list for the one the template doesn't set, so Neptune clears it.
func (h *ClusterEndpoint) Update(ctx context.Context, request native.UpdateRequest) error {
	if setsBoth(request.Desired) {
		return bothMembers
	}
	endpoint, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	changed := awsapi.Changed(request)
	input := &neptune.ModifyDBClusterEndpointInput{DBClusterEndpointIdentifier: aws.String(request.Identifier)}
	modify := false
	if slices.Contains(changed, "EndpointType") {
		input.EndpointType = aws.String(awsapi.String(request.Desired, "EndpointType"))
		modify = true
	}
	if slices.Contains(changed, "StaticMembers") || slices.Contains(changed, "ExcludedMembers") {
		input.StaticMembers = awsapi.Strings(request.Desired, "StaticMembers")
		input.ExcludedMembers = awsapi.Strings(request.Desired, "ExcludedMembers")
		modify = true
	}
	calls := []func() error{}
	if modify {
		calls = append(calls, func() error {
			_, err := h.api.ModifyDBClusterEndpoint(ctx, input, opt)
			return err
		})
	}
	if slices.Contains(changed, "Tags") {
		calls = append(calls, h.tagCalls(ctx, endpoint.DBClusterEndpointArn, request, opt)...)
	}
	return awsapi.Calls(calls...)
}

// tagCalls returns the calls that add and remove tags to go from the request's current tags to the desired ones.
func (h *ClusterEndpoint) tagCalls(ctx context.Context, arn *string, request native.UpdateRequest, opt func(*neptune.Options)) []func() error {
	set, remove := awsapi.TagChanges(awsapi.Tags(request.Current), awsapi.Tags(request.Desired))
	calls := []func() error{}
	if len(set) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.AddTagsToResource(ctx, &neptune.AddTagsToResourceInput{ResourceName: arn, Tags: awsapi.SDKTags[types.Tag](set)}, opt)
			return err
		})
	}
	if len(remove) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.RemoveTagsFromResource(ctx, &neptune.RemoveTagsFromResourceInput{ResourceName: arn, TagKeys: remove}, opt)
			return err
		})
	}
	return calls
}

// Delete deletes the endpoint. An endpoint AWS is already deleting had a delete reach AWS before, so the delete
// succeeds.
func (h *ClusterEndpoint) Delete(ctx context.Context, region string, identifier string) error {
	endpoint, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	if strings.EqualFold(aws.ToString(endpoint.Status), "deleting") {
		return nil
	}
	_, err = h.api.DeleteDBClusterEndpoint(ctx, &neptune.DeleteDBClusterEndpointInput{DBClusterEndpointIdentifier: aws.String(identifier)}, inRegion(region))
	if awsapi.HasCode(err, "InvalidDBClusterEndpointStateFault") {
		endpoint, describeErr := h.describe(ctx, region, identifier)
		if describeErr != nil {
			return describeErr
		}
		if strings.EqualFold(aws.ToString(endpoint.Status), "deleting") {
			return nil
		}
	}
	return awsapi.NotFound(err, "DBClusterEndpointNotFoundFault")
}

// List returns every custom endpoint of a Neptune cluster in the region. Endpoints of other products' clusters are
// left out.
func (h *ClusterEndpoint) List(ctx context.Context, region string) ([]native.Resource, error) {
	clusters := []string{}
	clustersInput := &neptune.DescribeDBClustersInput{Filters: []types.Filter{{Name: aws.String("engine"), Values: []string{Engine}}}}
	for {
		page, err := h.api.DescribeDBClusters(ctx, clustersInput, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, c := range page.DBClusters {
			if aws.ToString(c.Engine) == Engine {
				clusters = append(clusters, strings.ToLower(aws.ToString(c.DBClusterIdentifier)))
			}
		}
		if clustersInput.Marker = page.Marker; clustersInput.Marker == nil {
			break
		}
	}

	out := []native.Resource{}
	input := &neptune.DescribeDBClusterEndpointsInput{Filters: []types.Filter{{Name: aws.String("db-cluster-endpoint-type"), Values: []string{"custom"}}}}
	for {
		page, err := h.api.DescribeDBClusterEndpoints(ctx, input, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.DBClusterEndpoints {
			e := &page.DBClusterEndpoints[i]
			if !strings.EqualFold(aws.ToString(e.EndpointType), customEndpoint) || !slices.Contains(clusters, strings.ToLower(aws.ToString(e.DBClusterIdentifier))) {
				continue
			}
			properties, err := h.properties(ctx, region, e)
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: aws.ToString(e.DBClusterEndpointIdentifier), Properties: properties})
		}
		if input.Marker = page.Marker; input.Marker == nil {
			return out, nil
		}
	}
}

// Progress reads the endpoint's status. A create or update waits for available, and a delete for the endpoint to be
// gone.
func (h *ClusterEndpoint) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	endpoint, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := aws.ToString(endpoint.Status)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	return endpointStates.Progress(status), nil
}
