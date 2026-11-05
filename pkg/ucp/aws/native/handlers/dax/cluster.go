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

// Package dax has the native handler for Amazon DynamoDB Accelerator (DAX) clusters, which Cloud Control can't create
// in some regions.
package dax

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dax"
	"github.com/aws/aws-sdk-go-v2/service/dax/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	clusterType = "AWS::DAX::Cluster"

	// defaultParameterGroup is the parameter group DAX gives a cluster created without one.
	defaultParameterGroup = "default.dax1.0"
)

// API is the part of the DAX client the handler uses.
type API interface {
	CreateCluster(ctx context.Context, params *dax.CreateClusterInput, optFns ...func(*dax.Options)) (*dax.CreateClusterOutput, error)
	DescribeClusters(ctx context.Context, params *dax.DescribeClustersInput, optFns ...func(*dax.Options)) (*dax.DescribeClustersOutput, error)
	UpdateCluster(ctx context.Context, params *dax.UpdateClusterInput, optFns ...func(*dax.Options)) (*dax.UpdateClusterOutput, error)
	IncreaseReplicationFactor(ctx context.Context, params *dax.IncreaseReplicationFactorInput, optFns ...func(*dax.Options)) (*dax.IncreaseReplicationFactorOutput, error)
	DecreaseReplicationFactor(ctx context.Context, params *dax.DecreaseReplicationFactorInput, optFns ...func(*dax.Options)) (*dax.DecreaseReplicationFactorOutput, error)
	DeleteCluster(ctx context.Context, params *dax.DeleteClusterInput, optFns ...func(*dax.Options)) (*dax.DeleteClusterOutput, error)
	TagResource(ctx context.Context, params *dax.TagResourceInput, optFns ...func(*dax.Options)) (*dax.TagResourceOutput, error)
	UntagResource(ctx context.Context, params *dax.UntagResourceInput, optFns ...func(*dax.Options)) (*dax.UntagResourceOutput, error)
	ListTags(ctx context.Context, params *dax.ListTagsInput, optFns ...func(*dax.Options)) (*dax.ListTagsOutput, error)
}

func inRegion(region string) func(*dax.Options) {
	return func(o *dax.Options) { o.Region = region }
}

var (
	_ native.Handler           = (*Cluster)(nil)
	_ native.IdentifierEncoder = (*Cluster)(nil)
)

// clusterStates are the states a cluster's create and update wait for. DAX reports no failed state.
var clusterStates = awsapi.States{Done: []string{"available"}}

// Cluster serves AWS::DAX::Cluster. Its identifier is ClusterName, which DAX stores in lowercase, and which UCP
// generates when the template leaves it out.
type Cluster struct {
	api API
}

// NewCluster returns the handler for DAX clusters.
func NewCluster(api API) *Cluster {
	return &Cluster{api: api}
}

// TypeName returns AWS::DAX::Cluster.
func (h *Cluster) TypeName() string {
	return clusterType
}

// Options returns how UCP uses the handler.
func (h *Cluster) Options() native.Options {
	return native.Options{
		List:         native.ListRegion,
		Recovery:     native.RecoverByIdentifier,
		NameProperty: "ClusterName",
		Equivalent:   equivalent,
	}
}

// Identifier returns ClusterName in lowercase, as DAX stores it.
func (h *Cluster) Identifier(properties map[string]any) (string, error) {
	return strings.ToLower(awsapi.String(properties, "ClusterName")), nil
}

// caseInsensitive are the properties DAX returns in another case than the template may set them: names it stores in
// lowercase, and the maintenance window.
var caseInsensitive = []string{"ClusterName", "SubnetGroupName", "PreferredMaintenanceWindow"}

// equivalent treats a value of caseInsensitive that differs only in case as the template's value.
func equivalent(path string, template any, aws any) bool {
	t, okT := template.(string)
	a, okA := aws.(string)
	return okT && okA && slices.Contains(caseInsensitive, path) && strings.EqualFold(t, a)
}

// describe returns the cluster with the name, or ErrNotFound.
func (h *Cluster) describe(ctx context.Context, region string, name string) (*types.Cluster, error) {
	out, err := h.api.DescribeClusters(ctx, &dax.DescribeClustersInput{ClusterNames: []string{name}}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "ClusterNotFoundFault")
	}
	if len(out.Clusters) == 0 {
		return nil, native.ErrNotFound
	}
	return &out.Clusters[0], nil
}

// Read returns the cluster's properties.
func (h *Cluster) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return h.properties(ctx, region, cluster)
}

// properties returns the cluster's properties. AvailabilityZones is write-only: DAX reports the zone of each node,
// which changes as nodes are removed or replaced, not the list the template sent.
func (h *Cluster) properties(ctx context.Context, region string, c *types.Cluster) (map[string]any, error) {
	tags, err := h.tags(ctx, region, c.ClusterArn)
	if err != nil {
		return nil, err
	}
	groups := []string{}
	for _, g := range c.SecurityGroups {
		groups = append(groups, aws.ToString(g.SecurityGroupIdentifier))
	}
	p := map[string]any{
		"ClusterName":                   c.ClusterName,
		"Arn":                           c.ClusterArn,
		"Description":                   c.Description,
		"IAMRoleARN":                    c.IamRoleArn,
		"NodeType":                      c.NodeType,
		"SubnetGroupName":               c.SubnetGroup,
		"PreferredMaintenanceWindow":    c.PreferredMaintenanceWindow,
		"SecurityGroupIds":              groups,
		"ClusterEndpointEncryptionType": c.ClusterEndpointEncryptionType,
		"NetworkType":                   c.NetworkType,
		"Tags":                          tags,
	}
	if c.TotalNodes != nil {
		p["ReplicationFactor"] = replicationFactor(c)
	}
	if c.ParameterGroup != nil {
		p["ParameterGroupName"] = c.ParameterGroup.ParameterGroupName
	}
	if n := c.NotificationConfiguration; n != nil && strings.EqualFold(aws.ToString(n.TopicStatus), "active") {
		p["NotificationTopicARN"] = n.TopicArn
	}
	if c.SSEDescription != nil {
		status := c.SSEDescription.Status
		p["SSESpecification"] = map[string]any{"SSEEnabled": status == types.SSEStatusEnabled || status == types.SSEStatusEnabling}
	}
	if e := c.ClusterDiscoveryEndpoint; e != nil && e.Address != nil {
		p["ClusterDiscoveryEndpoint"] = fmt.Sprintf("%s:%d", aws.ToString(e.Address), e.Port)
		p["ClusterDiscoveryEndpointURL"] = e.URL
	}
	return awsapi.EncodeObject(p)
}

// replicationFactor returns the number of nodes the cluster has once the nodes DAX is removing are gone.
func replicationFactor(c *types.Cluster) int32 {
	return aws.ToInt32(c.TotalNodes) - int32(len(c.NodeIdsToRemove))
}

// tags returns the cluster's tags as the Tags property, a map from key to value, without tags AWS adds, or nil when
// there are none.
func (h *Cluster) tags(ctx context.Context, region string, arn *string) (map[string]any, error) {
	tags := map[string]any{}
	var next *string
	for {
		out, err := h.api.ListTags(ctx, &dax.ListTagsInput{ResourceName: arn, NextToken: next}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, t := range out.Tags {
			if key := aws.ToString(t.Key); !strings.HasPrefix(key, "aws:") {
				tags[key] = aws.ToString(t.Value)
			}
		}
		if next = out.NextToken; next == nil {
			break
		}
	}
	if len(tags) == 0 {
		return nil, nil
	}
	return tags, nil
}

// tagList returns the Tags property of properties, a map from key to value, as a list sorted by key.
func tagList(properties map[string]any) []awsapi.Tag {
	m, _ := properties["Tags"].(map[string]any)
	out := []awsapi.Tag{}
	for _, key := range slices.Sorted(maps.Keys(m)) {
		out = append(out, awsapi.Tag{Key: key, Value: fmt.Sprint(m[key])})
	}
	return out
}

func sdkTags(tags []awsapi.Tag) []types.Tag {
	out := []types.Tag{}
	for _, t := range tags {
		out = append(out, types.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value)})
	}
	return out
}

// Create creates the cluster.
func (h *Cluster) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	input := &dax.CreateClusterInput{}
	if err := awsapi.Decode(awsapi.Without(p, "Tags", "SSESpecification"), input); err != nil {
		return "", err
	}
	if sse, ok := p["SSESpecification"].(map[string]any); ok {
		if enabled, ok := sse["SSEEnabled"].(bool); ok {
			input.SSESpecification = &types.SSESpecification{Enabled: aws.Bool(enabled)}
		}
	}
	input.Tags = sdkTags(tagList(p))
	if _, err := h.api.CreateCluster(ctx, input, inRegion(request.Region)); err != nil {
		return "", err
	}
	return h.Identifier(p)
}

// modifiable are the properties UpdateCluster changes.
var modifiable = []string{"Description", "NotificationTopicARN", "ParameterGroupName", "PreferredMaintenanceWindow", "SecurityGroupIds"}

// Update adds and removes tags, makes the patch's changes with UpdateCluster, then changes the number of nodes. Tags
// go first, because DAX takes the other calls only while the cluster is available, and UpdateCluster and a change of
// the number of nodes can make it modifying.
func (h *Cluster) Update(ctx context.Context, request native.UpdateRequest) error {
	cluster, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	changed := awsapi.Changed(request)
	update, err := h.updateCall(ctx, cluster, request, changed, opt)
	if err != nil {
		return err
	}
	replication, err := h.replicationCall(ctx, cluster, request, changed, opt)
	if err != nil {
		return err
	}
	calls := []func() error{}
	if slices.Contains(changed, "Tags") {
		calls = append(calls, h.tagCalls(ctx, cluster.ClusterArn, request, opt)...)
	}
	for _, call := range []func() error{update, replication} {
		if call != nil {
			calls = append(calls, call)
		}
	}
	return awsapi.Calls(calls...)
}

// updateCall returns the UpdateCluster call for the changed properties UpdateCluster changes, or nil when there are
// none. A removed parameter group goes back to DAX's default, and a removed notification topic is made inactive. DAX
// keeps its value of any other removed property.
func (h *Cluster) updateCall(ctx context.Context, cluster *types.Cluster, request native.UpdateRequest, changed []string, opt func(*dax.Options)) (func() error, error) {
	values := map[string]any{}
	for _, name := range changed {
		if !slices.Contains(modifiable, name) {
			continue
		}
		if value, ok := request.Desired[name]; ok {
			values[name] = value
		} else if name == "ParameterGroupName" {
			values[name] = defaultParameterGroup
		}
	}
	input := &dax.UpdateClusterInput{}
	if err := awsapi.Decode(values, input); err != nil {
		return nil, err
	}
	if slices.Contains(changed, "NotificationTopicARN") {
		switch {
		case input.NotificationTopicArn != nil:
			input.NotificationTopicStatus = aws.String("active")
		case cluster.NotificationConfiguration != nil && cluster.NotificationConfiguration.TopicArn != nil:
			input.NotificationTopicArn = cluster.NotificationConfiguration.TopicArn
			input.NotificationTopicStatus = aws.String("inactive")
		}
	}
	if len(values) == 0 && input.NotificationTopicStatus == nil {
		return nil, nil
	}
	input.ClusterName = cluster.ClusterName
	return func() error {
		_, err := h.api.UpdateCluster(ctx, input, opt)
		return err
	}, nil
}

// replicationCall returns the call that changes the number of nodes to the desired ReplicationFactor, or nil when it
// doesn't change. New nodes go in the desired AvailabilityZones. A decrease removes nodes from the zones the desired
// list no longer has, and DAX chooses when the template sets no zones. DAX has no call that moves a node, so a change
// of AvailabilityZones that adding or removing nodes can't reach is rejected rather than recorded as applied.
func (h *Cluster) replicationCall(ctx context.Context, cluster *types.Cluster, request native.UpdateRequest, changed []string, opt func(*dax.Options)) (func() error, error) {
	if !slices.Contains(changed, "ReplicationFactor") && !slices.Contains(changed, "AvailabilityZones") {
		return nil, nil
	}
	var desired struct {
		ReplicationFactor int32
		AvailabilityZones []string
	}
	if err := awsapi.Decode(request.Desired, &desired); err != nil {
		return nil, err
	}
	current := replicationFactor(cluster)
	if slices.Contains(changed, "AvailabilityZones") && len(desired.AvailabilityZones) > 0 && !zonesReachable(cluster.Nodes, desired.AvailabilityZones, int(desired.ReplicationFactor-current)) {
		return nil, &native.Failure{
			Message:     "DAX can't move a node to another Availability Zone, so the cluster's nodes can't end in the template's AvailabilityZones. Set AvailabilityZones to the zones of the nodes kept and added, or delete the cluster and deploy again.",
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	switch {
	case desired.ReplicationFactor > current:
		input := &dax.IncreaseReplicationFactorInput{
			ClusterName:          cluster.ClusterName,
			NewReplicationFactor: desired.ReplicationFactor,
			AvailabilityZones:    desired.AvailabilityZones,
		}
		return func() error {
			_, err := h.api.IncreaseReplicationFactor(ctx, input, opt)
			return err
		}, nil
	case desired.ReplicationFactor < current:
		input := &dax.DecreaseReplicationFactorInput{
			ClusterName:          cluster.ClusterName,
			NewReplicationFactor: desired.ReplicationFactor,
			AvailabilityZones:    removedZones(cluster.Nodes, desired.AvailabilityZones, int(current-desired.ReplicationFactor)),
		}
		return func() error {
			_, err := h.api.DecreaseReplicationFactor(ctx, input, opt)
			return err
		}, nil
	}
	return nil, nil
}

// zonesReachable reports whether adding delta nodes, or removing -delta, can leave the nodes in exactly the desired
// zones, counting each zone as often as the list has it.
func zonesReachable(nodes []types.Node, desired []string, delta int) bool {
	if delta < 0 {
		return removedZones(nodes, desired, -delta) != nil
	}
	left := map[string]int{}
	for _, zone := range desired {
		left[zone]++
	}
	for _, node := range nodes {
		zone := aws.ToString(node.AvailabilityZone)
		if left[zone] == 0 {
			return false
		}
		left[zone]--
	}
	return len(desired)-len(nodes) == delta
}

// removedZones returns the zone of each node to remove, so the nodes left are in the desired zones, or nil when
// desired is empty or doesn't leave exactly count nodes to remove.
func removedZones(nodes []types.Node, desired []string, count int) []string {
	if len(desired) == 0 {
		return nil
	}
	left := map[string]int{}
	for _, zone := range desired {
		left[zone]++
	}
	out := []string{}
	for _, node := range nodes {
		zone := aws.ToString(node.AvailabilityZone)
		if left[zone] > 0 {
			left[zone]--
			continue
		}
		out = append(out, zone)
	}
	if len(out) != count {
		return nil
	}
	return out
}

// tagCalls returns the calls that add and remove tags to go from the request's current tags to the desired ones.
func (h *Cluster) tagCalls(ctx context.Context, arn *string, request native.UpdateRequest, opt func(*dax.Options)) []func() error {
	set, remove := awsapi.TagChanges(tagList(request.Current), tagList(request.Desired))
	calls := []func() error{}
	if len(set) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.TagResource(ctx, &dax.TagResourceInput{ResourceName: arn, Tags: sdkTags(set)}, opt)
			return err
		})
	}
	if len(remove) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.UntagResource(ctx, &dax.UntagResourceInput{ResourceName: arn, TagKeys: remove}, opt)
			return err
		})
	}
	return calls
}

// Delete deletes the cluster. A cluster DAX is already deleting had a delete reach AWS before, so the delete
// succeeds.
func (h *Cluster) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeleteCluster(ctx, &dax.DeleteClusterInput{ClusterName: aws.String(identifier)}, inRegion(region))
	if awsapi.HasCode(err, "InvalidClusterStateFault") {
		cluster, describeErr := h.describe(ctx, region, identifier)
		if describeErr != nil {
			return describeErr
		}
		if strings.EqualFold(aws.ToString(cluster.Status), "deleting") {
			return nil
		}
	}
	return awsapi.NotFound(err, "ClusterNotFoundFault")
}

// List returns every DAX cluster in the region.
func (h *Cluster) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var next *string
	for {
		page, err := h.api.DescribeClusters(ctx, &dax.DescribeClustersInput{NextToken: next}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.Clusters {
			c := &page.Clusters[i]
			properties, err := h.properties(ctx, region, c)
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: aws.ToString(c.ClusterName), Properties: properties})
		}
		if next = page.NextToken; next == nil {
			return out, nil
		}
	}
}

// Progress reads the cluster's status. A create or update waits for available, and a delete for the cluster to be
// gone.
func (h *Cluster) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := aws.ToString(cluster.Status)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	return clusterStates.Progress(status), nil
}
