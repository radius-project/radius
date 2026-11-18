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
// so a template with a value of the wrong type, or whose member lists Neptune wouldn't take, is rejected here.
func (h *ClusterEndpoint) Identifier(properties map[string]any) (string, error) {
	if err := validate(properties); err != nil {
		return "", err
	}
	if _, _, err := memberLists(properties); err != nil {
		return "", err
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
	properties, err := awsapi.EncodeObject(map[string]any{
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
	if err != nil {
		return nil, err
	}
	// An endpoint always has both lists, so a template that sets one to [] matches AWS instead of sending a change,
	// which would make the endpoint unusable until Neptune finishes it.
	for _, name := range []string{"StaticMembers", "ExcludedMembers"} {
		if _, ok := properties[name]; !ok {
			properties[name] = []any{}
		}
	}
	return properties, nil
}

// wrongType is the failure for a property whose value has the wrong type, so it is never sent as an empty value or left
// out.
func wrongType(path string, want string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s of an AWS::Neptune::DBClusterEndpoint must be %s", path, want),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// validate checks that each value of the template other than the member lists, which memberLists checks, has its
// schema's type.
func validate(properties map[string]any) error {
	for _, name := range []string{"DBClusterEndpointIdentifier", "DBClusterIdentifier", "EndpointType"} {
		if value, ok := properties[name]; ok && value != nil {
			if _, ok := value.(string); !ok {
				return wrongType(name, "a string")
			}
		}
	}
	if value, ok := properties["Tags"]; ok && value != nil {
		tags, ok := value.([]any)
		if !ok {
			return wrongType("Tags", "a list of tags")
		}
		for _, item := range tags {
			tag, ok := item.(map[string]any)
			if !ok {
				return wrongType("Tags", "a list of tags")
			}
			if _, ok := tag["Key"].(string); !ok {
				return wrongType("Each tag's Key", "a string")
			}
			if value, ok := tag["Value"]; ok && value != nil {
				if _, ok := value.(string); !ok {
					return wrongType("Each tag's Value", "a string")
				}
			}
		}
	}
	return nil
}

// bothMembers is the failure for a template that sets StaticMembers and ExcludedMembers, which Neptune doesn't allow.
var bothMembers = &native.Failure{
	Message:     "an AWS::Neptune::DBClusterEndpoint can set StaticMembers or ExcludedMembers, not both",
	ClientError: true,
	Outcome:     native.OutcomeRejected,
}

// memberList returns the member list name of properties, or nil when it is absent or empty. A value that isn't a list
// of strings is a client failure, so it is never sent as a shorter list.
func memberList(properties map[string]any, name string) ([]string, error) {
	value, ok := properties[name]
	if !ok || value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	out := []string{}
	for _, v := range list {
		s, isString := v.(string)
		ok = ok && isString
		out = append(out, s)
	}
	if !ok {
		return nil, &native.Failure{
			Message:     fmt.Sprintf("%s of an AWS::Neptune::DBClusterEndpoint must be a list of DB instance identifiers", name),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// memberLists returns the static and excluded members of properties, and fails for a template that sets both.
func memberLists(properties map[string]any) ([]string, []string, error) {
	static, err := memberList(properties, "StaticMembers")
	if err != nil {
		return nil, nil, err
	}
	excluded, err := memberList(properties, "ExcludedMembers")
	if err != nil {
		return nil, nil, err
	}
	if len(static) > 0 && len(excluded) > 0 {
		return nil, nil, bothMembers
	}
	return static, excluded, nil
}

// Create creates the endpoint, after checking that DBClusterIdentifier names a Neptune cluster: the API would also
// make an endpoint of an Aurora or DocumentDB cluster.
func (h *ClusterEndpoint) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	if err := validate(request.Properties); err != nil {
		return "", err
	}
	static, excluded, err := memberLists(request.Properties)
	if err != nil {
		return "", err
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
	// The input reads each property by its exact name, as the check above does, so a key in another case, which a
	// case-insensitive decode would also take, can't send the create to another cluster.
	input := &neptune.CreateDBClusterEndpointInput{
		DBClusterEndpointIdentifier: aws.String(awsapi.String(request.Properties, "DBClusterEndpointIdentifier")),
		DBClusterIdentifier:         aws.String(clusterID),
		EndpointType:                aws.String(awsapi.String(request.Properties, "EndpointType")),
		StaticMembers:               static,
		ExcludedMembers:             excluded,
		Tags:                        awsapi.SDKTags[types.Tag](awsapi.Tags(request.Properties)),
	}
	if _, err := h.api.CreateDBClusterEndpoint(ctx, input, inRegion(request.Region)); err != nil {
		return "", lostCreate(err)
	}
	return h.Identifier(request.Properties)
}

// lostCreate returns err, unless it says the endpoint already exists. UCP reads AWS by the identifier before it creates
// an endpoint, so that endpoint may be the one an earlier attempt of this create made, such as when the SDK retried a
// call whose response was lost. The outcome is then unknown, so UCP keeps the record and the next deploy finds the
// endpoint by its identifier, instead of dropping it and leaving an endpoint no record manages.
func lostCreate(err error) error {
	if !awsapi.HasCode(err, "DBClusterEndpointAlreadyExistsFault") {
		return err
	}
	return &native.Failure{
		Message:     "AWS already has a Neptune custom endpoint with the DBClusterEndpointIdentifier, which an earlier attempt of this deploy may have created. Deploy again to manage it, or choose another name: " + err.Error(),
		ClientError: true,
		Outcome:     native.OutcomeUnknown,
	}
}

// Update changes the endpoint type and members with ModifyDBClusterEndpoint, then adds and removes tags. It sends a
// member list only when the template changes what AWS has, and sends the other list empty only when Neptune must clear
// it to take the new one, so a list the template leaves out keeps the members AWS has.
func (h *ClusterEndpoint) Update(ctx context.Context, request native.UpdateRequest) error {
	if err := validate(request.Desired); err != nil {
		return err
	}
	static, excluded, err := memberLists(request.Desired)
	if err != nil {
		return err
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
	setStatic := slices.Contains(changed, "StaticMembers") && !sameMembers("StaticMembers", anyList(static), anyList(endpoint.StaticMembers))
	setExcluded := slices.Contains(changed, "ExcludedMembers") && !sameMembers("ExcludedMembers", anyList(excluded), anyList(endpoint.ExcludedMembers))
	if setStatic || (setExcluded && len(excluded) > 0) {
		input.StaticMembers = nonNil(static)
		modify = true
	}
	if setExcluded || (setStatic && len(static) > 0) {
		input.ExcludedMembers = nonNil(excluded)
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

// anyList returns list as a JSON list.
func anyList(list []string) []any {
	out := []any{}
	for _, v := range list {
		out = append(out, v)
	}
	return out
}

// nonNil returns list, or an empty list for nil, which the SDK sends as an empty list so Neptune clears it.
func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
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
