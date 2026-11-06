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

// Package emr has the native handler for Amazon EMR clusters, which Cloud Control can create only in some regions.
// AWS assigns a cluster's ID and RunJobFlow takes no idempotency token, so the handler tags each create with the
// operation ID and finds a lost create by that tag.
package emr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/emr"
	"github.com/aws/aws-sdk-go-v2/service/emr/types"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	clusterType = "AWS::EMR::Cluster"

	// clusterIDVariable is how an automatic scaling rule's dimension names the cluster it belongs to. AWS returns the
	// cluster's ID in its place.
	clusterIDVariable = "${emr.clusterId}"
)

// API is the part of the EMR client the handler uses.
type API interface {
	RunJobFlow(ctx context.Context, params *emr.RunJobFlowInput, optFns ...func(*emr.Options)) (*emr.RunJobFlowOutput, error)
	DescribeCluster(ctx context.Context, params *emr.DescribeClusterInput, optFns ...func(*emr.Options)) (*emr.DescribeClusterOutput, error)
	ListClusters(ctx context.Context, params *emr.ListClustersInput, optFns ...func(*emr.Options)) (*emr.ListClustersOutput, error)
	ListInstanceGroups(ctx context.Context, params *emr.ListInstanceGroupsInput, optFns ...func(*emr.Options)) (*emr.ListInstanceGroupsOutput, error)
	ListInstanceFleets(ctx context.Context, params *emr.ListInstanceFleetsInput, optFns ...func(*emr.Options)) (*emr.ListInstanceFleetsOutput, error)
	GetManagedScalingPolicy(ctx context.Context, params *emr.GetManagedScalingPolicyInput, optFns ...func(*emr.Options)) (*emr.GetManagedScalingPolicyOutput, error)
	GetAutoTerminationPolicy(ctx context.Context, params *emr.GetAutoTerminationPolicyInput, optFns ...func(*emr.Options)) (*emr.GetAutoTerminationPolicyOutput, error)
	ModifyCluster(ctx context.Context, params *emr.ModifyClusterInput, optFns ...func(*emr.Options)) (*emr.ModifyClusterOutput, error)
	ModifyInstanceGroups(ctx context.Context, params *emr.ModifyInstanceGroupsInput, optFns ...func(*emr.Options)) (*emr.ModifyInstanceGroupsOutput, error)
	SetVisibleToAllUsers(ctx context.Context, params *emr.SetVisibleToAllUsersInput, optFns ...func(*emr.Options)) (*emr.SetVisibleToAllUsersOutput, error)
	SetTerminationProtection(ctx context.Context, params *emr.SetTerminationProtectionInput, optFns ...func(*emr.Options)) (*emr.SetTerminationProtectionOutput, error)
	SetUnhealthyNodeReplacement(ctx context.Context, params *emr.SetUnhealthyNodeReplacementInput, optFns ...func(*emr.Options)) (*emr.SetUnhealthyNodeReplacementOutput, error)
	PutManagedScalingPolicy(ctx context.Context, params *emr.PutManagedScalingPolicyInput, optFns ...func(*emr.Options)) (*emr.PutManagedScalingPolicyOutput, error)
	RemoveManagedScalingPolicy(ctx context.Context, params *emr.RemoveManagedScalingPolicyInput, optFns ...func(*emr.Options)) (*emr.RemoveManagedScalingPolicyOutput, error)
	PutAutoTerminationPolicy(ctx context.Context, params *emr.PutAutoTerminationPolicyInput, optFns ...func(*emr.Options)) (*emr.PutAutoTerminationPolicyOutput, error)
	RemoveAutoTerminationPolicy(ctx context.Context, params *emr.RemoveAutoTerminationPolicyInput, optFns ...func(*emr.Options)) (*emr.RemoveAutoTerminationPolicyOutput, error)
	RemoveAutoScalingPolicy(ctx context.Context, params *emr.RemoveAutoScalingPolicyInput, optFns ...func(*emr.Options)) (*emr.RemoveAutoScalingPolicyOutput, error)
	AddTags(ctx context.Context, params *emr.AddTagsInput, optFns ...func(*emr.Options)) (*emr.AddTagsOutput, error)
	RemoveTags(ctx context.Context, params *emr.RemoveTagsInput, optFns ...func(*emr.Options)) (*emr.RemoveTagsOutput, error)
	TerminateJobFlows(ctx context.Context, params *emr.TerminateJobFlowsInput, optFns ...func(*emr.Options)) (*emr.TerminateJobFlowsOutput, error)
}

func inRegion(region string) func(*emr.Options) {
	return func(o *emr.Options) { o.Region = region }
}

var (
	_ native.Handler = (*Cluster)(nil)
	_ native.Finder  = (*Cluster)(nil)
)

// activeStates are the states of a cluster that is starting or running.
var activeStates = []types.ClusterState{
	types.ClusterStateStarting, types.ClusterStateBootstrapping, types.ClusterStateRunning, types.ClusterStateWaiting,
}

// endedStates are the states of a cluster that is shutting down or gone. AWS lists such a cluster for about two
// months, but it can't run again, so Read treats it as not found.
var endedStates = []types.ClusterState{
	types.ClusterStateTerminating, types.ClusterStateTerminated, types.ClusterStateTerminatedWithErrors,
}

// Cluster serves AWS::EMR::Cluster. Its identifier, Id, is the cluster ID AWS assigns, such as j-2AXXXXXXGAPLF.
type Cluster struct {
	api API
}

// NewCluster returns the handler for EMR clusters.
func NewCluster(api API) *Cluster {
	return &Cluster{api: api}
}

// TypeName returns AWS::EMR::Cluster.
func (h *Cluster) TypeName() string {
	return clusterType
}

// Options returns how UCP uses the handler. A cluster usually starts in 10 to 20 minutes, and resizes and
// terminations take about as long.
func (h *Cluster) Options() native.Options {
	return native.Options{
		List:          native.ListRegion,
		Recovery:      native.RecoverBySearch,
		Equivalent:    equivalent,
		CreateTimeout: time.Hour,
		UpdateTimeout: time.Hour,
		DeleteTimeout: time.Hour,
	}
}

// describe returns the cluster with the identifier in any state, or ErrNotFound.
func (h *Cluster) describe(ctx context.Context, region string, identifier string) (*types.Cluster, error) {
	out, err := h.api.DescribeCluster(ctx, &emr.DescribeClusterInput{ClusterId: aws.String(identifier)}, inRegion(region))
	if err != nil {
		// EMR answers InvalidRequestException for a cluster ID it doesn't know, which is the only input here.
		return nil, awsapi.NotFound(err, "InvalidRequestException")
	}
	if out.Cluster == nil {
		return nil, native.ErrNotFound
	}
	return out.Cluster, nil
}

// active returns the cluster with the identifier, or ErrNotFound when AWS has none or it has ended.
func (h *Cluster) active(ctx context.Context, region string, identifier string) (*types.Cluster, error) {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	if slices.Contains(endedStates, state(cluster)) {
		return nil, fmt.Errorf("%w: the cluster is %s", native.ErrNotFound, state(cluster))
	}
	return cluster, nil
}

func state(c *types.Cluster) types.ClusterState {
	if c.Status == nil {
		return ""
	}
	return c.Status.State
}

// Read returns the cluster's properties.
func (h *Cluster) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	cluster, err := h.active(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return h.properties(ctx, region, cluster)
}

// properties returns the properties of the cluster. It leaves out the write-only values, including the Kerberos
// passwords, even when AWS returns them.
func (h *Cluster) properties(ctx context.Context, region string, c *types.Cluster) (map[string]any, error) {
	opt := inRegion(region)
	attrs := c.Ec2InstanceAttributes
	if attrs == nil {
		attrs = &types.Ec2InstanceAttributes{}
	}
	p := map[string]any{
		"Id":                      c.Id,
		"Name":                    c.Name,
		"MasterPublicDNS":         c.MasterPublicDnsName,
		"Applications":            c.Applications,
		"AutoScalingRole":         c.AutoScalingRole,
		"Configurations":          configurations(c.Configurations),
		"CustomAmiId":             c.CustomAmiId,
		"EbsRootVolumeIops":       c.EbsRootVolumeIops,
		"EbsRootVolumeSize":       c.EbsRootVolumeSize,
		"EbsRootVolumeThroughput": c.EbsRootVolumeThroughput,
		"JobFlowRole":             attrs.IamInstanceProfile,
		"LogUri":                  c.LogUri,
		"OSReleaseLabel":          c.OSReleaseLabel,
		"PlacementGroupConfigs":   c.PlacementGroups,
		"ReleaseLabel":            c.ReleaseLabel,
		"ScaleDownBehavior":       c.ScaleDownBehavior,
		"SecurityConfiguration":   c.SecurityConfiguration,
		"ServiceRole":             c.ServiceRole,
		"SessionEnabled":          c.SessionEnabled,
		"StepConcurrencyLevel":    c.StepConcurrencyLevel,
		"VisibleToAllUsers":       c.VisibleToAllUsers,
		"Tags":                    tagList(c.Tags),
	}
	if k := c.KerberosAttributes; k != nil {
		p["KerberosAttributes"] = map[string]any{"Realm": k.Realm}
	}
	if m := c.MonitoringConfiguration; m != nil {
		p["MonitoringConfiguration"] = map[string]any{"CloudWatchLogConfiguration": m.CloudWatchLogConfiguration}
	}
	instances, err := h.instances(ctx, region, c, attrs)
	if err != nil {
		return nil, err
	}
	p["Instances"] = instances

	scaling, err := h.api.GetManagedScalingPolicy(ctx, &emr.GetManagedScalingPolicyInput{ClusterId: c.Id}, opt)
	if err != nil {
		return nil, err
	}
	p["ManagedScalingPolicy"] = scaling.ManagedScalingPolicy
	termination, err := h.api.GetAutoTerminationPolicy(ctx, &emr.GetAutoTerminationPolicyInput{ClusterId: c.Id}, opt)
	if err != nil {
		return nil, err
	}
	p["AutoTerminationPolicy"] = termination.AutoTerminationPolicy
	return awsapi.EncodeObject(p)
}

// instances returns the Instances property of the cluster. HadoopVersion is an input for old AMI versions that AWS
// doesn't return, so the schema marks it write-only.
func (h *Cluster) instances(ctx context.Context, region string, c *types.Cluster, attrs *types.Ec2InstanceAttributes) (map[string]any, error) {
	inst := map[string]any{
		"AdditionalMasterSecurityGroups": attrs.AdditionalMasterSecurityGroups,
		"AdditionalSlaveSecurityGroups":  attrs.AdditionalSlaveSecurityGroups,
		"Ec2KeyName":                     attrs.Ec2KeyName,
		"Ec2SubnetId":                    attrs.Ec2SubnetId,
		"EmrManagedMasterSecurityGroup":  attrs.EmrManagedMasterSecurityGroup,
		"EmrManagedSlaveSecurityGroup":   attrs.EmrManagedSlaveSecurityGroup,
		"ServiceAccessSecurityGroup":     attrs.ServiceAccessSecurityGroup,
		"TerminationProtected":           c.TerminationProtected,
		"UnhealthyNodeReplacement":       c.UnhealthyNodeReplacement,
	}
	if c.AutoTerminate != nil {
		inst["KeepJobFlowAliveWhenNoSteps"] = !*c.AutoTerminate
	}
	if attrs.Ec2AvailabilityZone != nil {
		inst["Placement"] = map[string]any{"AvailabilityZone": attrs.Ec2AvailabilityZone}
	}
	clusterID := aws.ToString(c.Id)
	if c.InstanceCollectionType == types.InstanceCollectionTypeInstanceFleet {
		inst["Ec2SubnetIds"] = attrs.RequestedEc2SubnetIds
		fleets, err := h.fleets(ctx, region, clusterID)
		if err != nil {
			return nil, err
		}
		tasks := []any{}
		for _, f := range fleets {
			switch f.InstanceFleetType {
			case types.InstanceFleetTypeMaster:
				inst["MasterInstanceFleet"] = fleet(f)
			case types.InstanceFleetTypeCore:
				inst["CoreInstanceFleet"] = fleet(f)
			default:
				tasks = append(tasks, fleet(f))
			}
		}
		inst["TaskInstanceFleets"] = tasks
		return inst, nil
	}
	groups, err := h.groups(ctx, region, clusterID)
	if err != nil {
		return nil, err
	}
	tasks := []any{}
	for _, g := range groups {
		switch g.InstanceGroupType {
		case types.InstanceGroupTypeMaster:
			inst["MasterInstanceGroup"] = group(g, clusterID)
		case types.InstanceGroupTypeCore:
			inst["CoreInstanceGroup"] = group(g, clusterID)
		default:
			tasks = append(tasks, group(g, clusterID))
		}
	}
	inst["TaskInstanceGroups"] = tasks
	return inst, nil
}

func (h *Cluster) groups(ctx context.Context, region string, clusterID string) ([]types.InstanceGroup, error) {
	out := []types.InstanceGroup{}
	var marker *string
	for {
		page, err := h.api.ListInstanceGroups(ctx, &emr.ListInstanceGroupsInput{ClusterId: aws.String(clusterID), Marker: marker}, inRegion(region))
		if err != nil {
			return nil, err
		}
		out = append(out, page.InstanceGroups...)
		if marker = page.Marker; marker == nil {
			return out, nil
		}
	}
}

func (h *Cluster) fleets(ctx context.Context, region string, clusterID string) ([]types.InstanceFleet, error) {
	out := []types.InstanceFleet{}
	var marker *string
	for {
		page, err := h.api.ListInstanceFleets(ctx, &emr.ListInstanceFleetsInput{ClusterId: aws.String(clusterID), Marker: marker}, inRegion(region))
		if err != nil {
			return nil, err
		}
		out = append(out, page.InstanceFleets...)
		if marker = page.Marker; marker == nil {
			return out, nil
		}
	}
}

// group returns an instance group in the schema's InstanceGroupConfig format.
func group(g types.InstanceGroup, clusterID string) map[string]any {
	return map[string]any{
		"AutoScalingPolicy": autoScalingPolicy(g.AutoScalingPolicy, clusterID),
		"BidPrice":          g.BidPrice,
		"Configurations":    configurations(g.Configurations),
		"CustomAmiId":       g.CustomAmiId,
		"EbsConfiguration":  ebsConfiguration(g.EbsBlockDevices, g.EbsOptimized),
		"InstanceCount":     g.RequestedInstanceCount,
		"InstanceType":      g.InstanceType,
		"Market":            g.Market,
		"Name":              g.Name,
	}
}

// fleet returns an instance fleet in the schema's InstanceFleetConfig format.
func fleet(f types.InstanceFleet) map[string]any {
	configs := []any{}
	for _, s := range f.InstanceTypeSpecifications {
		configs = append(configs, map[string]any{
			"BidPrice":                            s.BidPrice,
			"BidPriceAsPercentageOfOnDemandPrice": s.BidPriceAsPercentageOfOnDemandPrice,
			"Configurations":                      configurations(s.Configurations),
			"CustomAmiId":                         s.CustomAmiId,
			"EbsConfiguration":                    ebsConfiguration(s.EbsBlockDevices, s.EbsOptimized),
			"InstanceType":                        s.InstanceType,
			"Priority":                            s.Priority,
			"WeightedCapacity":                    s.WeightedCapacity,
		})
	}
	return map[string]any{
		"InstanceTypeConfigs":    configs,
		"LaunchSpecifications":   f.LaunchSpecifications,
		"Name":                   f.Name,
		"ResizeSpecifications":   f.ResizeSpecifications,
		"TargetOnDemandCapacity": f.TargetOnDemandCapacity,
		"TargetSpotCapacity":     f.TargetSpotCapacity,
	}
}

// configurations returns configurations in the schema's format, which names Properties ConfigurationProperties.
func configurations(list []types.Configuration) []any {
	out := []any{}
	for _, c := range list {
		out = append(out, map[string]any{
			"Classification":          c.Classification,
			"ConfigurationProperties": c.Properties,
			"Configurations":          configurations(c.Configurations),
		})
	}
	return out
}

// ebsConfiguration returns the EBS configuration of an instance group or instance type, from the volumes AWS
// attached to each instance: volumes with the same specification count as one config with VolumesPerInstance.
func ebsConfiguration(devices []types.EbsBlockDevice, optimized *bool) map[string]any {
	type config struct {
		spec  *types.VolumeSpecification
		count int
	}
	configs := []*config{}
	for _, d := range devices {
		if d.VolumeSpecification == nil {
			continue
		}
		i := slices.IndexFunc(configs, func(c *config) bool { return sameVolume(c.spec, d.VolumeSpecification) })
		if i < 0 {
			configs = append(configs, &config{spec: d.VolumeSpecification})
			i = len(configs) - 1
		}
		configs[i].count++
	}
	list := []any{}
	for _, c := range configs {
		list = append(list, map[string]any{"VolumeSpecification": c.spec, "VolumesPerInstance": c.count})
	}
	return map[string]any{"EbsBlockDeviceConfigs": list, "EbsOptimized": optimized}
}

func sameVolume(a *types.VolumeSpecification, b *types.VolumeSpecification) bool {
	return aws.ToInt32(a.SizeInGB) == aws.ToInt32(b.SizeInGB) && aws.ToString(a.VolumeType) == aws.ToString(b.VolumeType) &&
		aws.ToInt32(a.Iops) == aws.ToInt32(b.Iops) && aws.ToInt32(a.Throughput) == aws.ToInt32(b.Throughput)
}

// autoScalingPolicy returns an instance group's automatic scaling policy in the schema's format. A dimension whose
// value is the cluster's ID reads as ${emr.clusterId}, the form templates set.
func autoScalingPolicy(p *types.AutoScalingPolicyDescription, clusterID string) any {
	if p == nil {
		return nil
	}
	for i := range p.Rules {
		r := &p.Rules[i]
		if r.Trigger == nil || r.Trigger.CloudWatchAlarmDefinition == nil {
			continue
		}
		dims := r.Trigger.CloudWatchAlarmDefinition.Dimensions
		for j := range dims {
			if aws.ToString(dims[j].Value) == clusterID {
				dims[j].Value = aws.String(clusterIDVariable)
			}
		}
	}
	return map[string]any{"Constraints": p.Constraints, "Rules": p.Rules}
}

func tagList(tags []types.Tag) []any {
	m := map[string]string{}
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return awsapi.TagList(m)
}

func sdkTags(tags []awsapi.Tag) []types.Tag {
	out := []types.Tag{}
	for _, t := range tags {
		out = append(out, types.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value)})
	}
	return out
}

// apiNames returns a copy of v, properties in the schema's format, with the names the EMR API uses where they differ:
// a configuration's ConfigurationProperties and a step's StepProperties are both Properties in the API.
func apiNames(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, value := range t {
			if k == "ConfigurationProperties" || k == "StepProperties" {
				k = "Properties"
			}
			out[k] = apiNames(value)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			out[i] = apiNames(value)
		}
		return out
	}
	return v
}

// decode sets out, an EMR SDK struct, from properties in the schema's format.
func decode(properties map[string]any, out any) error {
	return awsapi.Decode(apiNames(properties).(map[string]any), out)
}

// intValue returns v, a number as encoding/json reads it, as an int32, and whether it is one.
func intValue(v any) (int32, bool) {
	n, ok := v.(float64)
	return int32(n), ok
}

// Create starts the cluster with RunJobFlow, tagged with the operation ID so Find can find it.
func (h *Cluster) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	input := &emr.RunJobFlowInput{}
	if err := decode(awsapi.Without(p, "Instances", "Tags", "AdditionalInfo"), input); err != nil {
		return "", err
	}
	instances, err := instancesInput(p["Instances"])
	if err != nil {
		return "", err
	}
	input.Instances = instances
	if info, ok := p["AdditionalInfo"]; ok {
		if input.AdditionalInfo, err = additionalInfo(info); err != nil {
			return "", err
		}
	}
	input.Tags = append(sdkTags(awsapi.Tags(p)), types.Tag{Key: aws.String(awsapi.OperationTag), Value: aws.String(request.ClientToken)})
	out, err := h.api.RunJobFlow(ctx, input, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	return aws.ToString(out.JobFlowId), nil
}

// additionalInfo returns AdditionalInfo as the JSON string RunJobFlow takes. The schema allows a string or an object.
func additionalInfo(v any) (*string, error) {
	if s, ok := v.(string); ok {
		return aws.String(s), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return aws.String(string(b)), nil
}

// instancesInput returns the Instances property as RunJobFlow takes it: the schema names each instance group and
// fleet by its role, and the API lists them with the role in each.
func instancesInput(v any) (*types.JobFlowInstancesConfig, error) {
	m := asMap(v)
	out := &types.JobFlowInstancesConfig{}
	roles := []string{"MasterInstanceGroup", "CoreInstanceGroup", "TaskInstanceGroups", "MasterInstanceFleet", "CoreInstanceFleet", "TaskInstanceFleets"}
	if err := decode(awsapi.Without(m, roles...), out); err != nil {
		return nil, err
	}
	addGroup := func(value any, role types.InstanceRoleType) error {
		g := types.InstanceGroupConfig{}
		if err := decode(asMap(value), &g); err != nil {
			return err
		}
		g.InstanceRole = role
		out.InstanceGroups = append(out.InstanceGroups, g)
		return nil
	}
	addFleet := func(value any, role types.InstanceFleetType) error {
		f := types.InstanceFleetConfig{}
		if err := decode(asMap(value), &f); err != nil {
			return err
		}
		f.InstanceFleetType = role
		out.InstanceFleets = append(out.InstanceFleets, f)
		return nil
	}
	var errs []error
	if g, ok := m["MasterInstanceGroup"]; ok {
		errs = append(errs, addGroup(g, types.InstanceRoleTypeMaster))
	}
	if g, ok := m["CoreInstanceGroup"]; ok {
		errs = append(errs, addGroup(g, types.InstanceRoleTypeCore))
	}
	for _, g := range asList(m["TaskInstanceGroups"]) {
		errs = append(errs, addGroup(g, types.InstanceRoleTypeTask))
	}
	if f, ok := m["MasterInstanceFleet"]; ok {
		errs = append(errs, addFleet(f, types.InstanceFleetTypeMaster))
	}
	if f, ok := m["CoreInstanceFleet"]; ok {
		errs = append(errs, addFleet(f, types.InstanceFleetTypeCore))
	}
	for _, f := range asList(m["TaskInstanceFleets"]) {
		errs = append(errs, addFleet(f, types.InstanceFleetTypeTask))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}

// Find returns the active cluster with the template's name that has the operation tag of request, or "" when there
// is none. ListClusters returns no tags, so it describes each cluster with the name.
func (h *Cluster) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	name := awsapi.String(request.Properties, "Name")
	found := []string{}
	var marker *string
	for {
		page, err := h.api.ListClusters(ctx, &emr.ListClustersInput{ClusterStates: activeStates, Marker: marker}, inRegion(request.Region))
		if err != nil {
			return "", err
		}
		for _, s := range page.Clusters {
			if aws.ToString(s.Name) != name {
				continue
			}
			c, err := h.describe(ctx, request.Region, aws.ToString(s.Id))
			if errors.Is(err, native.ErrNotFound) {
				continue
			}
			if err != nil {
				return "", err
			}
			if slices.ContainsFunc(c.Tags, func(t types.Tag) bool {
				return aws.ToString(t.Key) == awsapi.OperationTag && aws.ToString(t.Value) == request.ClientToken
			}) {
				found = append(found, aws.ToString(c.Id))
			}
		}
		if marker = page.Marker; marker == nil {
			break
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	}
	return "", awsapi.Ambiguous("EMR clusters", found)
}

// unchangeable is the failure of an update that changes properties EMR can't change on a running cluster.
func unchangeable(names []string) error {
	slices.Sort(names)
	return &native.Failure{
		Message:     fmt.Sprintf("EMR can't change these properties of a running cluster: %s. Set them back, or delete the cluster and deploy again.", strings.Join(names, ", ")),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Update makes the changes EMR allows on a running cluster: instance group sizes, termination protection, unhealthy
// node replacement, step concurrency, visibility, the managed scaling and auto-termination policies, and tags. It
// rejects any other change before it calls AWS.
func (h *Cluster) Update(ctx context.Context, request native.UpdateRequest) error {
	if _, err := h.active(ctx, request.Region, request.Identifier); err != nil {
		return err
	}
	opt := inRegion(request.Region)
	id := aws.String(request.Identifier)
	desired := request.Desired
	calls := []func() error{}
	unsupported := []string{}
	for _, name := range awsapi.Changed(request) {
		switch name {
		case "Instances":
			instanceCalls, names, err := h.instanceCalls(ctx, request)
			if err != nil {
				return err
			}
			calls = append(calls, instanceCalls...)
			unsupported = append(unsupported, names...)
		case "StepConcurrencyLevel":
			level, ok := intValue(desired[name])
			if !ok {
				level = 1
			}
			calls = append(calls, func() error {
				_, err := h.api.ModifyCluster(ctx, &emr.ModifyClusterInput{ClusterId: id, StepConcurrencyLevel: aws.Int32(level)}, opt)
				return err
			})
		case "VisibleToAllUsers":
			visible := boolOr(desired[name], true)
			calls = append(calls, func() error {
				_, err := h.api.SetVisibleToAllUsers(ctx, &emr.SetVisibleToAllUsersInput{JobFlowIds: []string{request.Identifier}, VisibleToAllUsers: aws.Bool(visible)}, opt)
				return err
			})
		case "ManagedScalingPolicy":
			call, err := h.scalingCall(ctx, id, desired[name], opt)
			if err != nil {
				return err
			}
			calls = append(calls, call)
		case "AutoTerminationPolicy":
			call, err := h.terminationCall(ctx, id, desired[name], opt)
			if err != nil {
				return err
			}
			calls = append(calls, call)
		case "Tags":
			calls = append(calls, h.tagCalls(ctx, request, opt)...)
		default:
			unsupported = append(unsupported, name)
		}
	}
	if len(unsupported) > 0 {
		return unchangeable(unsupported)
	}
	return awsapi.Calls(calls...)
}

func boolOr(v any, otherwise bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return otherwise
}

func (h *Cluster) scalingCall(ctx context.Context, id *string, value any, opt func(*emr.Options)) (func() error, error) {
	if value == nil {
		return func() error {
			_, err := h.api.RemoveManagedScalingPolicy(ctx, &emr.RemoveManagedScalingPolicyInput{ClusterId: id}, opt)
			return err
		}, nil
	}
	policy := &types.ManagedScalingPolicy{}
	if err := decode(asMap(value), policy); err != nil {
		return nil, err
	}
	return func() error {
		_, err := h.api.PutManagedScalingPolicy(ctx, &emr.PutManagedScalingPolicyInput{ClusterId: id, ManagedScalingPolicy: policy}, opt)
		return err
	}, nil
}

func (h *Cluster) terminationCall(ctx context.Context, id *string, value any, opt func(*emr.Options)) (func() error, error) {
	if value == nil {
		return func() error {
			_, err := h.api.RemoveAutoTerminationPolicy(ctx, &emr.RemoveAutoTerminationPolicyInput{ClusterId: id}, opt)
			return err
		}, nil
	}
	policy := &types.AutoTerminationPolicy{}
	if err := decode(asMap(value), policy); err != nil {
		return nil, err
	}
	return func() error {
		_, err := h.api.PutAutoTerminationPolicy(ctx, &emr.PutAutoTerminationPolicyInput{ClusterId: id, AutoTerminationPolicy: policy}, opt)
		return err
	}, nil
}

// tagCalls returns the calls that add and remove tags to go from the request's current tags to the desired ones.
// The current tags never include the operation tag, so it stays.
func (h *Cluster) tagCalls(ctx context.Context, request native.UpdateRequest, opt func(*emr.Options)) []func() error {
	set, remove := awsapi.TagChanges(awsapi.Tags(request.Current), awsapi.Tags(request.Desired))
	calls := []func() error{}
	if len(set) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.AddTags(ctx, &emr.AddTagsInput{ResourceId: aws.String(request.Identifier), Tags: sdkTags(set)}, opt)
			return err
		})
	}
	if len(remove) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.RemoveTags(ctx, &emr.RemoveTagsInput{ResourceId: aws.String(request.Identifier), TagKeys: remove}, opt)
			return err
		})
	}
	return calls
}

// instanceCalls returns the calls that make the changes to the Instances property, and the parts of it EMR can't
// change. EMR can resize the core and task instance groups, remove a group's automatic scaling policy, and set
// termination protection and unhealthy node replacement. Everything else in Instances is fixed once the cluster
// starts.
func (h *Cluster) instanceCalls(ctx context.Context, request native.UpdateRequest) ([]func() error, []string, error) {
	opt := inRegion(request.Region)
	current := asMap(request.Current["Instances"])
	desired := asMap(request.Desired["Instances"])
	calls := []func() error{}
	unsupported := []string{}
	resizes := []types.InstanceGroupModifyConfig{}
	unscaled := []string{}
	var groups []types.InstanceGroup
	// With a managed scaling policy, EMR sets the instance counts and target capacities itself, so the template's
	// values are where the cluster starts, not values to restore.
	scaled := request.Desired["ManagedScalingPolicy"] != nil
	for _, key := range changedKeys(request.Patch, current, desired) {
		switch key {
		case "TerminationProtected":
			protected := boolOr(desired[key], false)
			calls = append(calls, func() error {
				_, err := h.api.SetTerminationProtection(ctx, &emr.SetTerminationProtectionInput{JobFlowIds: []string{request.Identifier}, TerminationProtected: aws.Bool(protected)}, opt)
				return err
			})
		case "UnhealthyNodeReplacement":
			replace, ok := desired[key].(bool)
			if !ok {
				// The template no longer sets it, so AWS keeps its value.
				continue
			}
			calls = append(calls, func() error {
				_, err := h.api.SetUnhealthyNodeReplacement(ctx, &emr.SetUnhealthyNodeReplacementInput{JobFlowIds: []string{request.Identifier}, UnhealthyNodeReplacement: aws.Bool(replace)}, opt)
				return err
			})
		case "CoreInstanceGroup", "TaskInstanceGroups":
			if removesField(request.Patch, key, []string{"AutoScalingPolicy"}) {
				unsupported = append(unsupported, "Instances/"+key)
				continue
			}
			if groups == nil {
				var err error
				if groups, err = h.groups(ctx, request.Region, request.Identifier); err != nil {
					return nil, nil, err
				}
			}
			changes, ok := groupResizes(key, desired[key], groups, request.Identifier, scaled)
			if !ok {
				unsupported = append(unsupported, "Instances/"+key)
				continue
			}
			resizes = append(resizes, changes.resizes...)
			unscaled = append(unscaled, changes.unscaled...)
		case "MasterInstanceFleet", "CoreInstanceFleet", "TaskInstanceFleets":
			if !scaled || removesField(request.Patch, key, capacities) || !covers(without(desired[key], capacities...), without(current[key], capacities...), false) {
				unsupported = append(unsupported, "Instances/"+key)
			}
		default:
			unsupported = append(unsupported, "Instances/"+key)
		}
	}
	groupCalls := []func() error{}
	for _, id := range unscaled {
		groupCalls = append(groupCalls, func() error {
			_, err := h.api.RemoveAutoScalingPolicy(ctx, &emr.RemoveAutoScalingPolicyInput{ClusterId: aws.String(request.Identifier), InstanceGroupId: aws.String(id)}, opt)
			return err
		})
	}
	if len(resizes) > 0 {
		groupCalls = append(groupCalls, func() error {
			_, err := h.api.ModifyInstanceGroups(ctx, &emr.ModifyInstanceGroupsInput{ClusterId: aws.String(request.Identifier), InstanceGroups: resizes}, opt)
			return err
		})
	}
	return append(groupCalls, calls...), unsupported, nil
}

// removesField reports whether the patch removes a property inside the Instances property key, other than those in
// allowed, which the update handles. EMR has no call that clears any other, and the comparison with AWS only checks
// the properties the template still sets, so a removal would otherwise count as applied.
func removesField(patch jsondiff.Patch, key string, allowed []string) bool {
	prefix := "/Instances/" + key + "/"
	for _, op := range patch {
		if op.Type != jsondiff.OperationRemove || !strings.HasPrefix(op.Path, prefix) {
			continue
		}
		segments := strings.Split(op.Path, "/")
		if !slices.Contains(allowed, segments[len(segments)-1]) {
			return true
		}
	}
	return false
}

// changedKeys returns the properties of Instances the patch changes, sorted.
func changedKeys(patch jsondiff.Patch, current map[string]any, desired map[string]any) []string {
	unescape := strings.NewReplacer("~1", "/", "~0", "~")
	out := []string{}
	add := func(key string) {
		if !slices.Contains(out, key) {
			out = append(out, key)
		}
	}
	for _, op := range patch {
		segments := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
		if segments[0] != "Instances" {
			continue
		}
		if len(segments) > 1 {
			add(unescape.Replace(segments[1]))
			continue
		}
		for _, m := range []map[string]any{current, desired} {
			for key := range m {
				if !covers(desired[key], current[key], false) {
					add(key)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// groupChanges are the changes that take the AWS instance groups to the template's: the groups whose automatic scaling
// policy the template no longer sets, and the instance count changes.
type groupChanges struct {
	unscaled []string
	resizes  []types.InstanceGroupModifyConfig
}

// groupResizes returns the changes that take the AWS instance groups to desired, the template's CoreInstanceGroup or
// TaskInstanceGroups. It returns false when desired changes anything else, such as the instance type, the automatic
// scaling policy, or the number of task groups. It leaves the count of a group EMR scales as it is: every group when
// scaled is true, because the cluster has a managed scaling policy, and a group that keeps an automatic scaling policy.
// A group whose policy the template no longer sets loses it and then gets the template's count.
func groupResizes(key string, desired any, groups []types.InstanceGroup, clusterID string, scaled bool) (groupChanges, bool) {
	role := types.InstanceGroupTypeTask
	want := asList(desired)
	if key == "CoreInstanceGroup" {
		if desired == nil {
			return groupChanges{}, false
		}
		role = types.InstanceGroupTypeCore
		want = []any{desired}
	}
	have := []types.InstanceGroup{}
	for _, g := range groups {
		if g.InstanceGroupType == role {
			have = append(have, g)
		}
	}
	if len(want) != len(have) {
		return groupChanges{}, false
	}
	matches := make([][]bool, len(want))
	for i := range want {
		matches[i] = make([]bool, len(have))
		for j := range have {
			matches[i][j] = covers(withoutCount(want[i]), withoutCount(group(have[j], clusterID)), false)
		}
	}
	pairs, ok := matching(matches)
	if !ok {
		return groupChanges{}, false
	}
	out := groupChanges{}
	for i, j := range pairs {
		policy := asMap(want[i])["AutoScalingPolicy"] != nil
		if have[j].AutoScalingPolicy != nil && !policy {
			out.unscaled = append(out.unscaled, aws.ToString(have[j].Id))
		}
		if scaled || policy {
			continue
		}
		count, ok := intValue(asMap(want[i])["InstanceCount"])
		if !ok {
			return groupChanges{}, false
		}
		if count != aws.ToInt32(have[j].RequestedInstanceCount) {
			out.resizes = append(out.resizes, types.InstanceGroupModifyConfig{InstanceGroupId: have[j].Id, InstanceCount: aws.Int32(count)})
		}
	}
	return out, true
}

// withoutCount returns an instance group, in the schema's format or as group returns it, as JSON without its
// InstanceCount, so two groups can be compared by everything else.
func withoutCount(v any) any {
	return without(v, "InstanceCount")
}

// capacities are the properties of an instance fleet that managed scaling changes.
var capacities = []string{"TargetOnDemandCapacity", "TargetSpotCapacity"}

// without returns v, an object or a list of objects, as JSON without the properties names in each object.
func without(v any, names ...string) any {
	encoded, err := awsapi.Encode(v)
	if err != nil {
		return nil
	}
	if list, ok := encoded.([]any); ok {
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = awsapi.Without(asMap(item), names...)
		}
		return out
	}
	return awsapi.Without(asMap(encoded), names...)
}

// Delete starts terminating the cluster. AWS rejects it when termination protection is on.
func (h *Cluster) Delete(ctx context.Context, region string, identifier string) error {
	if _, err := h.active(ctx, region, identifier); err != nil {
		return err
	}
	_, err := h.api.TerminateJobFlows(ctx, &emr.TerminateJobFlowsInput{JobFlowIds: []string{identifier}}, inRegion(region))
	return err
}

// List returns every active cluster in the region.
func (h *Cluster) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var marker *string
	for {
		page, err := h.api.ListClusters(ctx, &emr.ListClustersInput{ClusterStates: activeStates, Marker: marker}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, s := range page.Clusters {
			id := aws.ToString(s.Id)
			properties, err := h.Read(ctx, region, id)
			if errors.Is(err, native.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: id, Properties: properties})
		}
		if marker = page.Marker; marker == nil {
			return out, nil
		}
	}
}

// Progress reads the cluster's state. A create waits for the cluster to run, an update also for its instance groups
// and fleets to finish resizing, and a delete for the cluster to end.
func (h *Cluster) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	s := state(cluster)
	if op == native.OperationDelete {
		if s == types.ClusterStateTerminated || s == types.ClusterStateTerminatedWithErrors {
			return native.Progress{}, fmt.Errorf("%w: the cluster is %s", native.ErrNotFound, s)
		}
		return native.Progress{State: string(s), Waiting: string(types.ClusterStateTerminated)}, nil
	}
	if slices.Contains(endedStates, s) {
		message := fmt.Sprintf("AWS reports the cluster as %s", s)
		if reason := cluster.Status.StateChangeReason; reason != nil && reason.Message != nil {
			message += ": " + aws.ToString(reason.Message)
		}
		return native.Progress{Done: true, State: string(s), Failure: &native.Failure{Message: message}}, nil
	}
	progress := awsapi.States{Done: []string{string(types.ClusterStateWaiting), string(types.ClusterStateRunning)}}.Progress(string(s))
	if !progress.Done || op != native.OperationUpdate {
		return progress, nil
	}
	busy, err := h.resizing(ctx, region, cluster)
	if err != nil {
		return native.Progress{}, err
	}
	if busy != "" {
		return native.Progress{State: busy, Waiting: string(types.InstanceGroupStateRunning)}, nil
	}
	return progress, nil
}

// resizing returns the state of an instance group or fleet that is still changing, or "" when none is.
func (h *Cluster) resizing(ctx context.Context, region string, c *types.Cluster) (string, error) {
	id := aws.ToString(c.Id)
	if c.InstanceCollectionType == types.InstanceCollectionTypeInstanceFleet {
		fleets, err := h.fleets(ctx, region, id)
		if err != nil {
			return "", err
		}
		for _, f := range fleets {
			if f.Status != nil && f.Status.State != types.InstanceFleetStateRunning {
				return string(f.Status.State), nil
			}
		}
		return "", nil
	}
	groups, err := h.groups(ctx, region, id)
	if err != nil {
		return "", err
	}
	for _, g := range groups {
		if g.Status != nil && g.Status.State != types.InstanceGroupStateRunning {
			return string(g.Status.State), nil
		}
	}
	return "", nil
}
