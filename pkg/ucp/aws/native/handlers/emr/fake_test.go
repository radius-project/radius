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

package emr

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/emr"
	"github.com/aws/aws-sdk-go-v2/service/emr/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// fakeAPI is an in-memory EMR API. It keeps clusters in an awsfake store in the type's property format, so the
// suite's Cloud Control sees the same resources, and builds EMR's responses from them. A new cluster is STARTING for
// one read, a terminated one is TERMINATING for one read and then TERMINATED, and a resized instance group is
// RESIZING for one list. ListClusters returns one cluster per page, and ListInstanceGroups and ListInstanceFleets two
// groups or fleets per page.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu       sync.Mutex
	next     int
	state    map[string]types.ClusterState
	resizing map[string]bool
	ended    map[string]types.Cluster

	// tokens maps a cluster to the value of its operation tag, which the store leaves out, as Read does.
	tokens map[string]string

	// errs maps a method name to an error the call returns instead of acting.
	errs  map[string]error
	calls []string
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store:    store,
		state:    map[string]types.ClusterState{},
		resizing: map[string]bool{},
		ended:    map[string]types.Cluster{},
		tokens:   map[string]string{},
		errs:     map[string]error{},
	}
}

func fault(code string, message string) error {
	return &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient}
}

func (f *fakeAPI) call(name string) error {
	f.calls = append(f.calls, name)
	return f.errs[name]
}

// cfnNames returns a copy of v, an EMR request encoded as properties, with the schema's names: a configuration's
// Properties is ConfigurationProperties, and a step's StepProperties.
func cfnNames(v any, parent string) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, value := range t {
			name := k
			if k == "Properties" && parent == "Configurations" {
				name = "ConfigurationProperties"
			}
			if k == "Properties" && parent == "HadoopJarStep" {
				name = "StepProperties"
			}
			out[name] = cfnNames(value, k)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			out[i] = cfnNames(value, parent)
		}
		return out
	}
	return v
}

// Limits and defaults EMR's API reference and Management Guide give, which the fake enforces and applies.
const (
	maxID              = 256
	maxJobFlowID       = 10280
	maxName            = 256
	maxAdditionalInfo  = 10280
	maxSteps           = 256
	maxStepConcurrency = 256
	maxTerminate       = 10
	maxTagKey          = 128
	maxTagValue        = 256
	minIdleTimeout     = 60
	maxIdleTimeout     = 604800
	defaultIdleTimeout = 3600
	maxTypeConfigs     = 5
	maxStrategyConfigs = 30
	minVolumeSize      = 1
	maxVolumeSize      = 1024
	minThroughput      = 125
	maxThroughput      = 1000
)

// volumeTypes are the EBS volume types EMR supports.
var volumeTypes = []string{"gp3", "gp2", "io1", "st1", "sc1", "standard"}

// chars returns the length of s in characters, as EMR counts its length limits.
func chars(s string) int {
	return utf8.RuneCountInString(s)
}

// invalid returns ValidationError, the error EMR's Common Errors page gives for input that doesn't meet the required
// format or constraints.
func invalid(format string, args ...any) error {
	return fault("ValidationError", fmt.Sprintf(format, args...))
}

// validID returns ValidationError when a ClusterId or InstanceGroupId is longer than EMR allows.
func validID(name string, id string) error {
	if chars(id) > maxID {
		return invalid("%s must be at most %d characters", name, maxID)
	}
	return nil
}

// validJobFlowIDs returns ValidationError when JobFlowIds is empty or an ID is longer than EMR allows.
func validJobFlowIDs(ids []string) error {
	if len(ids) == 0 {
		return invalid("JobFlowIds is required")
	}
	for _, id := range ids {
		if chars(id) > maxJobFlowID {
			return invalid("a job flow ID must be at most %d characters", maxJobFlowID)
		}
	}
	return nil
}

// validTags returns ValidationError for a tag without a key, or with a key or value longer than EMR allows.
func validTags(tags []types.Tag) error {
	for _, t := range tags {
		key := aws.ToString(t.Key)
		if key == "" || chars(key) > maxTagKey || chars(aws.ToString(t.Value)) > maxTagValue {
			return invalid("a tag needs a key of up to %d characters and a value of up to %d characters", maxTagKey, maxTagValue)
		}
	}
	return nil
}

// validIdleTimeout returns ValidationError for an auto-termination idle timeout outside 60 seconds to seven days.
func validIdleTimeout(p *types.AutoTerminationPolicy) error {
	if p == nil || p.IdleTimeout == nil {
		return nil
	}
	if t := *p.IdleTimeout; t < minIdleTimeout || t > maxIdleTimeout {
		return invalid("IdleTimeout must be %d to %d seconds", minIdleTimeout, maxIdleTimeout)
	}
	return nil
}

// withIdleTimeout returns a copy of p with the idle timeout EMR uses when the policy sets none.
func withIdleTimeout(p *types.AutoTerminationPolicy) *types.AutoTerminationPolicy {
	if p == nil {
		return nil
	}
	out := *p
	if out.IdleTimeout == nil {
		out.IdleTimeout = aws.Int64(defaultIdleTimeout)
	}
	return &out
}

// validScaling returns ValidationError for a managed scaling policy whose compute limits leave out a required value,
// or whose utilization performance index is outside 1 to 100.
func validScaling(p *types.ManagedScalingPolicy) error {
	if p == nil {
		return nil
	}
	if l := p.ComputeLimits; l != nil && (l.MaximumCapacityUnits == nil || l.MinimumCapacityUnits == nil || l.UnitType == "") {
		return invalid("ComputeLimits requires MaximumCapacityUnits, MinimumCapacityUnits, and UnitType")
	}
	if i := p.UtilizationPerformanceIndex; i != nil && (*i < 1 || *i > 100) {
		return invalid("UtilizationPerformanceIndex must be 1 to 100")
	}
	return nil
}

// withScalingDefaults returns a copy of p with the On-Demand and core limits EMR uses when the policy sets none: both
// default to MaximumCapacityUnits.
func withScalingDefaults(p *types.ManagedScalingPolicy) *types.ManagedScalingPolicy {
	if p == nil || p.ComputeLimits == nil {
		return p
	}
	out := *p
	limits := *p.ComputeLimits
	if limits.MaximumOnDemandCapacityUnits == nil {
		limits.MaximumOnDemandCapacityUnits = limits.MaximumCapacityUnits
	}
	if limits.MaximumCoreCapacityUnits == nil {
		limits.MaximumCoreCapacityUnits = limits.MaximumCapacityUnits
	}
	out.ComputeLimits = &limits
	return &out
}

// validEbs returns ValidationError for an EBS volume without a size or type, with a size outside 1 to 1024 GiB, of a
// type EMR doesn't support, or with a throughput outside 125 to 1000 MiB/s or on a volume that isn't gp3.
func validEbs(ebs *types.EbsConfiguration) error {
	if ebs == nil {
		return nil
	}
	for _, c := range ebs.EbsBlockDeviceConfigs {
		v := c.VolumeSpecification
		if v == nil || v.SizeInGB == nil || v.VolumeType == nil {
			return invalid("a volume specification requires SizeInGB and VolumeType")
		}
		if size := *v.SizeInGB; size < minVolumeSize || size > maxVolumeSize {
			return invalid("SizeInGB must be %d to %d", minVolumeSize, maxVolumeSize)
		}
		if !slices.Contains(volumeTypes, *v.VolumeType) {
			return invalid("VolumeType must be one of %v", volumeTypes)
		}
		if t := v.Throughput; t != nil && (*t < minThroughput || *t > maxThroughput || *v.VolumeType != "gp3") {
			return invalid("Throughput must be %d to %d, and only for gp3 volumes", minThroughput, maxThroughput)
		}
	}
	return nil
}

// validGroup returns ValidationError for an instance group without a count, role, or instance type.
func validGroup(g types.InstanceGroupConfig) error {
	if g.InstanceCount == nil || g.InstanceRole == "" || g.InstanceType == nil {
		return invalid("an instance group requires InstanceCount, InstanceRole, and InstanceType")
	}
	if n := chars(*g.InstanceType); n < 1 || n > maxID {
		return invalid("InstanceType must be 1 to %d characters", maxID)
	}
	return validEbs(g.EbsConfiguration)
}

// validFleet returns ValidationError for an instance fleet that breaks the rules of InstanceFleetConfig and
// InstanceTypeConfig: its target capacities, the number of instance type configurations, and their weighted capacity.
func validFleet(fl types.InstanceFleetConfig) error {
	if fl.InstanceFleetType == "" {
		return invalid("an instance fleet requires InstanceFleetType")
	}
	onDemand, spot := fl.TargetOnDemandCapacity, fl.TargetSpotCapacity
	if aws.ToInt32(onDemand) < 0 || aws.ToInt32(spot) < 0 {
		return invalid("target capacities must be at least 0")
	}
	if aws.ToInt32(onDemand) == 0 && aws.ToInt32(spot) == 0 {
		return invalid("at least one of TargetSpotCapacity and TargetOnDemandCapacity must be greater than 0")
	}
	master := fl.InstanceFleetType == types.InstanceFleetTypeMaster
	if master && (onDemand != nil && spot != nil || aws.ToInt32(onDemand)+aws.ToInt32(spot) != 1) {
		return invalid("a master instance fleet sets one of TargetSpotCapacity and TargetOnDemandCapacity, to 1")
	}
	limit := maxTypeConfigs
	if s := fl.LaunchSpecifications; s != nil && (s.OnDemandSpecification != nil && s.OnDemandSpecification.AllocationStrategy != "" ||
		s.SpotSpecification != nil && s.SpotSpecification.AllocationStrategy != "") {
		limit = maxStrategyConfigs
	}
	if len(fl.InstanceTypeConfigs) > limit {
		return invalid("an instance fleet has at most %d instance type configurations", limit)
	}
	for _, c := range fl.InstanceTypeConfigs {
		if n := chars(aws.ToString(c.InstanceType)); n < 1 || n > maxID {
			return invalid("InstanceType must be 1 to %d characters", maxID)
		}
		if w := c.WeightedCapacity; w != nil && (master && *w != 1 || *w < 1) {
			return invalid("WeightedCapacity is 1 for a master instance fleet, and at least 1 for others")
		}
		if err := validEbs(c.EbsConfiguration); err != nil {
			return err
		}
	}
	return nil
}

// validInstances returns ValidationError for instances that don't set instance groups or instance fleets, set both, or
// set only one of the EMR-managed security groups.
func validInstances(in *types.JobFlowInstancesConfig) error {
	if in == nil {
		return invalid("Instances is required")
	}
	uniform := in.MasterInstanceType != nil && in.SlaveInstanceType != nil && in.InstanceCount != nil
	switch {
	case len(in.InstanceGroups) > 0 && len(in.InstanceFleets) > 0:
		return invalid("Instances can't set both InstanceGroups and InstanceFleets")
	case len(in.InstanceGroups) == 0 && len(in.InstanceFleets) == 0 && !uniform:
		return invalid("Instances must set InstanceGroups or InstanceFleets")
	case (in.EmrManagedMasterSecurityGroup == nil) != (in.EmrManagedSlaveSecurityGroup == nil):
		return invalid("EmrManagedMasterSecurityGroup and EmrManagedSlaveSecurityGroup must be set together")
	}
	for _, g := range in.InstanceGroups {
		if err := validGroup(g); err != nil {
			return err
		}
	}
	for _, fl := range in.InstanceFleets {
		if err := validFleet(fl); err != nil {
			return err
		}
	}
	return nil
}

// validRunJobFlow returns ValidationError for a RunJobFlow request EMR rejects.
func validRunJobFlow(in *emr.RunJobFlowInput) error {
	name := aws.ToString(in.Name)
	switch {
	case in.Name == nil:
		return invalid("Name is required")
	case chars(name) > maxName:
		return invalid("Name must be at most %d characters", maxName)
	case strings.ContainsAny(name, "<>$|`"):
		return invalid("Name can't contain <, >, $, |, or `")
	case chars(aws.ToString(in.AdditionalInfo)) > maxAdditionalInfo:
		return invalid("AdditionalInfo must be at most %d characters", maxAdditionalInfo)
	case len(in.Steps) > maxSteps:
		return invalid("a job flow has at most %d steps", maxSteps)
	case aws.ToInt32(in.StepConcurrencyLevel) > maxStepConcurrency:
		return invalid("StepConcurrencyLevel must be at most %d", maxStepConcurrency)
	}
	return firstError(validInstances(in.Instances), validIdleTimeout(in.AutoTerminationPolicy), validScaling(in.ManagedScalingPolicy), validTags(in.Tags))
}

// withRunDefaults returns a copy of in with the values EMR uses for parameters RunJobFlow leaves out.
func withRunDefaults(in *emr.RunJobFlowInput) (*emr.RunJobFlowInput, error) {
	encoded, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	out := &emr.RunJobFlowInput{}
	if err := awsapi.Decode(encoded, out); err != nil {
		return nil, err
	}
	if out.StepConcurrencyLevel == nil {
		out.StepConcurrencyLevel = aws.Int32(1)
	}
	if out.VisibleToAllUsers == nil {
		out.VisibleToAllUsers = aws.Bool(true)
	}
	inst := out.Instances
	if inst.KeepJobFlowAliveWhenNoSteps == nil {
		inst.KeepJobFlowAliveWhenNoSteps = aws.Bool(false)
	}
	if inst.TerminationProtected == nil {
		inst.TerminationProtected = aws.Bool(false)
	}
	if inst.UnhealthyNodeReplacement == nil {
		inst.UnhealthyNodeReplacement = aws.Bool(true)
	}
	for i := range inst.InstanceFleets {
		for j := range inst.InstanceFleets[i].InstanceTypeConfigs {
			c := &inst.InstanceFleets[i].InstanceTypeConfigs[j]
			if c.WeightedCapacity == nil {
				c.WeightedCapacity = aws.Int32(1)
			}
			if c.BidPrice == nil && c.BidPriceAsPercentageOfOnDemandPrice == nil {
				c.BidPriceAsPercentageOfOnDemandPrice = aws.Float64(100)
			}
		}
	}
	// The copy loses an empty policy, which still takes EMR's defaults.
	out.AutoTerminationPolicy = withIdleTimeout(in.AutoTerminationPolicy)
	out.ManagedScalingPolicy = withScalingDefaults(in.ManagedScalingPolicy)
	return out, nil
}

func (f *fakeAPI) RunJobFlow(ctx context.Context, in *emr.RunJobFlowInput, _ ...func(*emr.Options)) (*emr.RunJobFlowOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RunJobFlow"); err != nil {
		return nil, err
	}
	if err := validRunJobFlow(in); err != nil {
		return nil, err
	}
	in, err := withRunDefaults(in)
	if err != nil {
		return nil, err
	}
	encoded, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	props := cfnNames(encoded, "").(map[string]any)
	f.next++
	id := fmt.Sprintf("j-FAKE%06d", f.next)
	tags := map[string]string{}
	for _, t := range in.Tags {
		if aws.ToString(t.Key) == awsapi.OperationTag {
			f.tokens[id] = aws.ToString(t.Value)
		}
		tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	delete(props, "Tags")
	if list := awsapi.TagList(tags); list != nil {
		props["Tags"] = list
	}
	inst := asMap(props["Instances"])
	for _, g := range asList(inst["InstanceGroups"]) {
		m := asMap(g)
		role := m["InstanceRole"]
		delete(m, "InstanceRole")
		switch role {
		case "MASTER":
			inst["MasterInstanceGroup"] = m
		case "CORE":
			inst["CoreInstanceGroup"] = m
		default:
			inst["TaskInstanceGroups"] = append(asList(inst["TaskInstanceGroups"]), m)
		}
	}
	for _, fl := range asList(inst["InstanceFleets"]) {
		m := asMap(fl)
		role := m["InstanceFleetType"]
		delete(m, "InstanceFleetType")
		switch role {
		case "MASTER":
			inst["MasterInstanceFleet"] = m
		case "CORE":
			inst["CoreInstanceFleet"] = m
		default:
			inst["TaskInstanceFleets"] = append(asList(inst["TaskInstanceFleets"]), m)
		}
	}
	delete(inst, "InstanceGroups")
	delete(inst, "InstanceFleets")
	props["Instances"] = inst
	f.store.Put(clusterType, id, props)
	f.state[id] = types.ClusterStateStarting
	return &emr.RunJobFlowOutput{JobFlowId: aws.String(id)}, nil
}

// status returns the state of the cluster, and moves a transitional state on. A terminating cluster ends: the store
// no longer has it, but DescribeCluster still returns it as TERMINATED.
func (f *fakeAPI) status(id string) types.ClusterState {
	s, ok := f.state[id]
	if !ok {
		return types.ClusterStateWaiting
	}
	switch s {
	case types.ClusterStateStarting:
		delete(f.state, id)
	case types.ClusterStateTerminating:
		f.state[id] = types.ClusterStateTerminated
	case types.ClusterStateTerminated:
		c, _ := f.cluster(id)
		c.Status = &types.ClusterStatus{State: types.ClusterStateTerminated}
		f.ended[id] = c
		delete(f.state, id)
		f.store.Remove(clusterType, id)
	}
	return s
}

// cluster returns the stored cluster as DescribeCluster does, without its status.
func (f *fakeAPI) cluster(id string) (types.Cluster, bool) {
	props := f.store.Resource(clusterType, id)
	if props == nil {
		return types.Cluster{}, false
	}
	c := types.Cluster{}
	// The Kerberos attributes keep their passwords, as if AWS returned them.
	if err := decode(awsapi.Without(props, "Instances", "Tags"), &c); err != nil {
		panic(err)
	}
	c.Id = aws.String(id)
	for _, t := range awsapi.Tags(props) {
		c.Tags = append(c.Tags, types.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value)})
	}
	if token, ok := f.tokens[id]; ok {
		c.Tags = append(c.Tags, types.Tag{Key: aws.String(awsapi.OperationTag), Value: aws.String(token)})
	}
	if err := decode(map[string]any{"PlacementGroups": props["PlacementGroupConfigs"]}, &c); err != nil {
		panic(err)
	}
	inst := asMap(props["Instances"])
	attrs := &types.Ec2InstanceAttributes{}
	if err := decode(inst, attrs); err != nil {
		panic(err)
	}
	attrs.IamInstanceProfile = aws.String(awsapi.String(props, "JobFlowRole"))
	if az := awsapi.String(asMap(inst["Placement"]), "AvailabilityZone"); az != "" {
		attrs.Ec2AvailabilityZone = aws.String(az)
	}
	attrs.RequestedEc2SubnetIds = awsapi.Strings(inst, "Ec2SubnetIds")
	c.Ec2InstanceAttributes = attrs
	if keep, ok := inst["KeepJobFlowAliveWhenNoSteps"].(bool); ok {
		c.AutoTerminate = aws.Bool(!keep)
	}
	if v, ok := inst["TerminationProtected"].(bool); ok {
		c.TerminationProtected = aws.Bool(v)
	}
	if v, ok := inst["UnhealthyNodeReplacement"].(bool); ok {
		c.UnhealthyNodeReplacement = aws.Bool(v)
	}
	c.InstanceCollectionType = types.InstanceCollectionTypeInstanceGroup
	if _, ok := inst["MasterInstanceFleet"]; ok {
		c.InstanceCollectionType = types.InstanceCollectionTypeInstanceFleet
	}
	return c, true
}

func (f *fakeAPI) DescribeCluster(ctx context.Context, in *emr.DescribeClusterInput, _ ...func(*emr.Options)) (*emr.DescribeClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeCluster"); err != nil {
		return nil, err
	}
	if err := validID("ClusterId", aws.ToString(in.ClusterId)); err != nil {
		return nil, err
	}
	return f.describe(aws.ToString(in.ClusterId))
}

func (f *fakeAPI) describe(id string) (*emr.DescribeClusterOutput, error) {
	if c, ok := f.ended[id]; ok {
		return &emr.DescribeClusterOutput{Cluster: &c}, nil
	}
	c, ok := f.cluster(id)
	if !ok {
		return nil, fault("InvalidRequestException", fmt.Sprintf("Cluster id '%s' is not valid.", id))
	}
	s := f.status(id)
	c.Status = &types.ClusterStatus{State: s}
	if s == types.ClusterStateTerminating {
		c.Status.StateChangeReason = &types.ClusterStateChangeReason{Code: types.ClusterStateChangeReasonCodeUserRequest, Message: aws.String("Terminated by user request")}
	}
	return &emr.DescribeClusterOutput{Cluster: &c}, nil
}

func (f *fakeAPI) ListClusters(ctx context.Context, in *emr.ListClustersInput, _ ...func(*emr.Options)) (*emr.ListClustersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListClusters"); err != nil {
		return nil, err
	}
	summaries := []types.ClusterSummary{}
	ids := f.store.List(clusterType)
	for id := range f.ended {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		state := types.ClusterStateTerminated
		var name *string
		if c, ok := f.ended[id]; ok {
			name = c.Name
		} else {
			c, _ := f.cluster(id)
			name = c.Name
			if s, ok := f.state[id]; ok {
				state = s
			} else {
				state = types.ClusterStateWaiting
			}
		}
		if len(in.ClusterStates) > 0 && !slices.Contains(in.ClusterStates, state) {
			continue
		}
		summaries = append(summaries, types.ClusterSummary{Id: aws.String(id), Name: name, Status: &types.ClusterStatus{State: state}})
	}
	return page(summaries, in.Marker, 1, func(page []types.ClusterSummary, marker *string) *emr.ListClustersOutput {
		return &emr.ListClustersOutput{Clusters: page, Marker: marker}
	}), nil
}

// page returns the page of items after marker, of up to size items.
func page[T any, O any](items []T, marker *string, size int, out func([]T, *string) O) O {
	start, _ := strconv.Atoi(aws.ToString(marker))
	end := min(start+size, len(items))
	var next *string
	if end < len(items) {
		next = aws.String(strconv.Itoa(end))
	}
	return out(items[start:end], next)
}

// groupList returns the stored instance groups of the cluster as ListInstanceGroups does.
func (f *fakeAPI) groupList(id string) []types.InstanceGroup {
	inst := asMap(f.store.Resource(clusterType, id)["Instances"])
	type entry struct {
		id    string
		role  types.InstanceGroupType
		value any
	}
	entries := []entry{}
	if g, ok := inst["MasterInstanceGroup"]; ok {
		entries = append(entries, entry{"ig-master", types.InstanceGroupTypeMaster, g})
	}
	if g, ok := inst["CoreInstanceGroup"]; ok {
		entries = append(entries, entry{"ig-core", types.InstanceGroupTypeCore, g})
	}
	for i, g := range asList(inst["TaskInstanceGroups"]) {
		entries = append(entries, entry{fmt.Sprintf("ig-task-%d", i), types.InstanceGroupTypeTask, g})
	}
	state := types.InstanceGroupStateRunning
	if f.resizing[id] {
		state = types.InstanceGroupStateResizing
	}
	out := []types.InstanceGroup{}
	for _, e := range entries {
		cfg := types.InstanceGroupConfig{}
		if err := decode(asMap(e.value), &cfg); err != nil {
			panic(err)
		}
		g := types.InstanceGroup{
			Id:                     aws.String(e.id),
			InstanceGroupType:      e.role,
			InstanceType:           cfg.InstanceType,
			RequestedInstanceCount: cfg.InstanceCount,
			RunningInstanceCount:   cfg.InstanceCount,
			Market:                 cfg.Market,
			Name:                   cfg.Name,
			BidPrice:               cfg.BidPrice,
			CustomAmiId:            cfg.CustomAmiId,
			Configurations:         cfg.Configurations,
			Status:                 &types.InstanceGroupStatus{State: state},
		}
		if ebs := cfg.EbsConfiguration; ebs != nil {
			g.EbsOptimized = ebs.EbsOptimized
			g.EbsBlockDevices = devices(ebs)
		}
		if p := cfg.AutoScalingPolicy; p != nil {
			for _, r := range p.Rules {
				for j, d := range r.Trigger.CloudWatchAlarmDefinition.Dimensions {
					if aws.ToString(d.Value) == clusterIDVariable {
						r.Trigger.CloudWatchAlarmDefinition.Dimensions[j].Value = aws.String(id)
					}
				}
			}
			g.AutoScalingPolicy = &types.AutoScalingPolicyDescription{Constraints: p.Constraints, Rules: p.Rules, Status: &types.AutoScalingPolicyStatus{State: types.AutoScalingPolicyStateAttached}}
		}
		out = append(out, g)
	}
	return out
}

// devices returns the volumes AWS attaches to each instance for an EBS configuration.
func devices(ebs *types.EbsConfiguration) []types.EbsBlockDevice {
	out := []types.EbsBlockDevice{}
	for _, c := range ebs.EbsBlockDeviceConfigs {
		for range max(1, int(aws.ToInt32(c.VolumesPerInstance))) {
			out = append(out, types.EbsBlockDevice{Device: aws.String(fmt.Sprintf("/dev/sd%c", 'b'+len(out))), VolumeSpecification: c.VolumeSpecification})
		}
	}
	return out
}

func (f *fakeAPI) ListInstanceGroups(ctx context.Context, in *emr.ListInstanceGroupsInput, _ ...func(*emr.Options)) (*emr.ListInstanceGroupsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListInstanceGroups"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ClusterId)
	if err := validID("ClusterId", id); err != nil {
		return nil, err
	}
	groups := f.groupList(id)
	out := page(groups, in.Marker, 2, func(page []types.InstanceGroup, marker *string) *emr.ListInstanceGroupsOutput {
		return &emr.ListInstanceGroupsOutput{InstanceGroups: page, Marker: marker}
	})
	if out.Marker == nil {
		delete(f.resizing, id)
	}
	return out, nil
}

func (f *fakeAPI) ListInstanceFleets(ctx context.Context, in *emr.ListInstanceFleetsInput, _ ...func(*emr.Options)) (*emr.ListInstanceFleetsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListInstanceFleets"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ClusterId)
	if err := validID("ClusterId", id); err != nil {
		return nil, err
	}
	inst := asMap(f.store.Resource(clusterType, id)["Instances"])
	type entry struct {
		role  types.InstanceFleetType
		value any
	}
	entries := []entry{{types.InstanceFleetTypeMaster, inst["MasterInstanceFleet"]}}
	if fl, ok := inst["CoreInstanceFleet"]; ok {
		entries = append(entries, entry{types.InstanceFleetTypeCore, fl})
	}
	for _, fl := range asList(inst["TaskInstanceFleets"]) {
		entries = append(entries, entry{types.InstanceFleetTypeTask, fl})
	}
	state := types.InstanceFleetStateRunning
	if f.resizing[id] {
		state = types.InstanceFleetStateResizing
	}
	fleets := []types.InstanceFleet{}
	for i, e := range entries {
		cfg := types.InstanceFleetConfig{}
		if err := decode(asMap(e.value), &cfg); err != nil {
			panic(err)
		}
		fl := types.InstanceFleet{
			Id:                     aws.String(fmt.Sprintf("if-%d", i)),
			InstanceFleetType:      e.role,
			Name:                   cfg.Name,
			TargetOnDemandCapacity: cfg.TargetOnDemandCapacity,
			TargetSpotCapacity:     cfg.TargetSpotCapacity,
			LaunchSpecifications:   cfg.LaunchSpecifications,
			ResizeSpecifications:   cfg.ResizeSpecifications,
			Status:                 &types.InstanceFleetStatus{State: state},
		}
		for _, c := range cfg.InstanceTypeConfigs {
			spec := types.InstanceTypeSpecification{
				InstanceType:                        c.InstanceType,
				BidPrice:                            c.BidPrice,
				BidPriceAsPercentageOfOnDemandPrice: c.BidPriceAsPercentageOfOnDemandPrice,
				Configurations:                      c.Configurations,
				CustomAmiId:                         c.CustomAmiId,
				Priority:                            c.Priority,
				WeightedCapacity:                    c.WeightedCapacity,
			}
			if ebs := c.EbsConfiguration; ebs != nil {
				spec.EbsOptimized = ebs.EbsOptimized
				spec.EbsBlockDevices = devices(ebs)
			}
			fl.InstanceTypeSpecifications = append(fl.InstanceTypeSpecifications, spec)
		}
		fleets = append(fleets, fl)
	}
	out := page(fleets, in.Marker, 2, func(page []types.InstanceFleet, marker *string) *emr.ListInstanceFleetsOutput {
		return &emr.ListInstanceFleetsOutput{InstanceFleets: page, Marker: marker}
	})
	if out.Marker == nil {
		delete(f.resizing, id)
	}
	return out, nil
}

func (f *fakeAPI) GetManagedScalingPolicy(ctx context.Context, in *emr.GetManagedScalingPolicyInput, _ ...func(*emr.Options)) (*emr.GetManagedScalingPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("GetManagedScalingPolicy"); err != nil {
		return nil, err
	}
	if err := validID("ClusterId", aws.ToString(in.ClusterId)); err != nil {
		return nil, err
	}
	out := &emr.GetManagedScalingPolicyOutput{}
	if p, ok := f.store.Resource(clusterType, aws.ToString(in.ClusterId))["ManagedScalingPolicy"]; ok {
		out.ManagedScalingPolicy = &types.ManagedScalingPolicy{}
		if err := decode(asMap(p), out.ManagedScalingPolicy); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (f *fakeAPI) GetAutoTerminationPolicy(ctx context.Context, in *emr.GetAutoTerminationPolicyInput, _ ...func(*emr.Options)) (*emr.GetAutoTerminationPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("GetAutoTerminationPolicy"); err != nil {
		return nil, err
	}
	if err := validID("ClusterId", aws.ToString(in.ClusterId)); err != nil {
		return nil, err
	}
	out := &emr.GetAutoTerminationPolicyOutput{}
	if p, ok := f.store.Resource(clusterType, aws.ToString(in.ClusterId))["AutoTerminationPolicy"]; ok {
		out.AutoTerminationPolicy = &types.AutoTerminationPolicy{}
		if err := decode(asMap(p), out.AutoTerminationPolicy); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// current returns the state of the cluster without moving it on.
func (f *fakeAPI) current(id string) types.ClusterState {
	if _, ok := f.ended[id]; ok {
		return types.ClusterStateTerminated
	}
	if s, ok := f.state[id]; ok {
		return s
	}
	return types.ClusterStateWaiting
}

// running are the states of a cluster that runs, which EMR's Management Guide requires for a resize.
var running = []types.ClusterState{types.ClusterStateRunning, types.ClusterStateWaiting}

// change applies a change to the stored cluster, as a call that changes AWS. It rejects the call when check returns
// an error for the request, or when allowed is set and the cluster's state isn't in it.
func (f *fakeAPI) change(name string, id string, check error, allowed []types.ClusterState, apply func(props map[string]any)) error {
	if err := f.call(name); err != nil {
		return err
	}
	if check != nil {
		return check
	}
	if s := f.current(id); allowed != nil && !slices.Contains(allowed, s) {
		return fault("ValidationException", fmt.Sprintf("%s isn't allowed on a cluster that is %s.", name, s))
	}
	if err := f.failer.Before(); err != nil {
		return err
	}
	props := f.store.Resource(clusterType, id)
	if props == nil {
		return fault("InvalidRequestException", fmt.Sprintf("Cluster id '%s' is not valid.", id))
	}
	apply(props)
	f.store.Put(clusterType, id, props)
	return f.failer.After()
}

// set sets a property, or removes it when value is nil.
func set(props map[string]any, name string, value any) {
	if value == nil {
		delete(props, name)
		return
	}
	encoded, err := awsapi.Encode(value)
	if err != nil {
		panic(err)
	}
	props[name] = cfnNames(encoded, name)
}

func (f *fakeAPI) ModifyCluster(ctx context.Context, in *emr.ModifyClusterInput, _ ...func(*emr.Options)) (*emr.ModifyClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var check error
	if l := in.StepConcurrencyLevel; l != nil && (*l < 1 || *l > maxStepConcurrency) {
		check = invalid("StepConcurrencyLevel must be 1 to %d", maxStepConcurrency)
	}
	if err := f.change("ModifyCluster", aws.ToString(in.ClusterId), check, nil, func(props map[string]any) {
		set(props, "StepConcurrencyLevel", in.StepConcurrencyLevel)
	}); err != nil {
		return nil, err
	}
	return &emr.ModifyClusterOutput{StepConcurrencyLevel: in.StepConcurrencyLevel}, nil
}

// instanceGroup returns the group with the ID instanceGroups gives it, in the cluster's Instances.
func instanceGroup(inst map[string]any, groupID string) map[string]any {
	if groupID == "ig-core" {
		return asMap(inst["CoreInstanceGroup"])
	}
	var i int
	if _, err := fmt.Sscanf(groupID, "ig-task-%d", &i); err != nil {
		panic(err)
	}
	return asMap(asList(inst["TaskInstanceGroups"])[i])
}

func (f *fakeAPI) RemoveAutoScalingPolicy(ctx context.Context, in *emr.RemoveAutoScalingPolicyInput, _ ...func(*emr.Options)) (*emr.RemoveAutoScalingPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	check := firstError(validID("ClusterId", aws.ToString(in.ClusterId)), validID("InstanceGroupId", aws.ToString(in.InstanceGroupId)))
	return &emr.RemoveAutoScalingPolicyOutput{}, f.change("RemoveAutoScalingPolicy", aws.ToString(in.ClusterId), check, nil, func(props map[string]any) {
		delete(instanceGroup(asMap(props["Instances"]), aws.ToString(in.InstanceGroupId)), "AutoScalingPolicy")
	})
}

func (f *fakeAPI) ModifyInstanceGroups(ctx context.Context, in *emr.ModifyInstanceGroupsInput, _ ...func(*emr.Options)) (*emr.ModifyInstanceGroupsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := aws.ToString(in.ClusterId)
	checks := []error{validID("ClusterId", id)}
	for _, m := range in.InstanceGroups {
		checks = append(checks, required("InstanceGroupId", m.InstanceGroupId), validID("InstanceGroupId", aws.ToString(m.InstanceGroupId)))
	}
	check := firstError(checks...)
	return &emr.ModifyInstanceGroupsOutput{}, f.change("ModifyInstanceGroups", id, check, running, func(props map[string]any) {
		inst := asMap(props["Instances"])
		for _, m := range in.InstanceGroups {
			instanceGroup(inst, aws.ToString(m.InstanceGroupId))["InstanceCount"] = float64(aws.ToInt32(m.InstanceCount))
		}
		f.resizing[id] = true
	})
}

// firstError returns the first of errs that isn't nil, or nil.
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// jobFlow returns the first of ids, or "" when there is none.
func jobFlow(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// required returns ValidationError, as EMR does, when a required parameter is missing.
func required(name string, v any) error {
	if reflect.ValueOf(v).IsNil() {
		return invalid("%s is required", name)
	}
	return nil
}

func (f *fakeAPI) SetVisibleToAllUsers(ctx context.Context, in *emr.SetVisibleToAllUsersInput, _ ...func(*emr.Options)) (*emr.SetVisibleToAllUsersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	check := firstError(validJobFlowIDs(in.JobFlowIds), required("VisibleToAllUsers", in.VisibleToAllUsers))
	return &emr.SetVisibleToAllUsersOutput{}, f.change("SetVisibleToAllUsers", jobFlow(in.JobFlowIds), check, nil, func(props map[string]any) {
		props["VisibleToAllUsers"] = aws.ToBool(in.VisibleToAllUsers)
	})
}

func (f *fakeAPI) SetTerminationProtection(ctx context.Context, in *emr.SetTerminationProtectionInput, _ ...func(*emr.Options)) (*emr.SetTerminationProtectionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	check := firstError(validJobFlowIDs(in.JobFlowIds), required("TerminationProtected", in.TerminationProtected))
	return &emr.SetTerminationProtectionOutput{}, f.change("SetTerminationProtection", jobFlow(in.JobFlowIds), check, nil, func(props map[string]any) {
		asMap(props["Instances"])["TerminationProtected"] = aws.ToBool(in.TerminationProtected)
	})
}

func (f *fakeAPI) SetUnhealthyNodeReplacement(ctx context.Context, in *emr.SetUnhealthyNodeReplacementInput, _ ...func(*emr.Options)) (*emr.SetUnhealthyNodeReplacementOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	check := firstError(validJobFlowIDs(in.JobFlowIds), required("UnhealthyNodeReplacement", in.UnhealthyNodeReplacement))
	return &emr.SetUnhealthyNodeReplacementOutput{}, f.change("SetUnhealthyNodeReplacement", jobFlow(in.JobFlowIds), check, nil, func(props map[string]any) {
		asMap(props["Instances"])["UnhealthyNodeReplacement"] = aws.ToBool(in.UnhealthyNodeReplacement)
	})
}

func (f *fakeAPI) PutManagedScalingPolicy(ctx context.Context, in *emr.PutManagedScalingPolicyInput, _ ...func(*emr.Options)) (*emr.PutManagedScalingPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	check := firstError(validID("ClusterId", aws.ToString(in.ClusterId)), required("ManagedScalingPolicy", in.ManagedScalingPolicy), validScaling(in.ManagedScalingPolicy))
	return &emr.PutManagedScalingPolicyOutput{}, f.change("PutManagedScalingPolicy", aws.ToString(in.ClusterId), check, nil, func(props map[string]any) {
		set(props, "ManagedScalingPolicy", withScalingDefaults(in.ManagedScalingPolicy))
	})
}

func (f *fakeAPI) RemoveManagedScalingPolicy(ctx context.Context, in *emr.RemoveManagedScalingPolicyInput, _ ...func(*emr.Options)) (*emr.RemoveManagedScalingPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.RemoveManagedScalingPolicyOutput{}, f.change("RemoveManagedScalingPolicy", aws.ToString(in.ClusterId), validID("ClusterId", aws.ToString(in.ClusterId)), nil, func(props map[string]any) {
		delete(props, "ManagedScalingPolicy")
	})
}

func (f *fakeAPI) PutAutoTerminationPolicy(ctx context.Context, in *emr.PutAutoTerminationPolicyInput, _ ...func(*emr.Options)) (*emr.PutAutoTerminationPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	check := firstError(validID("ClusterId", aws.ToString(in.ClusterId)), validIdleTimeout(in.AutoTerminationPolicy))
	return &emr.PutAutoTerminationPolicyOutput{}, f.change("PutAutoTerminationPolicy", aws.ToString(in.ClusterId), check, nil, func(props map[string]any) {
		set(props, "AutoTerminationPolicy", withIdleTimeout(in.AutoTerminationPolicy))
	})
}

func (f *fakeAPI) RemoveAutoTerminationPolicy(ctx context.Context, in *emr.RemoveAutoTerminationPolicyInput, _ ...func(*emr.Options)) (*emr.RemoveAutoTerminationPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.RemoveAutoTerminationPolicyOutput{}, f.change("RemoveAutoTerminationPolicy", aws.ToString(in.ClusterId), validID("ClusterId", aws.ToString(in.ClusterId)), nil, func(props map[string]any) {
		delete(props, "AutoTerminationPolicy")
	})
}

// taggable are the states of a cluster whose tags can change: EMR's Management Guide says a terminated cluster's tags
// can't.
var taggable = []types.ClusterState{
	types.ClusterStateStarting, types.ClusterStateBootstrapping, types.ClusterStateRunning, types.ClusterStateWaiting,
	types.ClusterStateTerminating,
}

func (f *fakeAPI) AddTags(ctx context.Context, in *emr.AddTagsInput, _ ...func(*emr.Options)) (*emr.AddTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	check := validTags(in.Tags)
	if len(in.Tags) == 0 {
		check = invalid("Tags is required")
	}
	return &emr.AddTagsOutput{}, f.change("AddTags", aws.ToString(in.ResourceId), check, taggable, func(props map[string]any) {
		tags := map[string]string{}
		for _, t := range awsapi.Tags(props) {
			tags[t.Key] = t.Value
		}
		for _, t := range in.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		props["Tags"] = awsapi.TagList(tags)
	})
}

func (f *fakeAPI) RemoveTags(ctx context.Context, in *emr.RemoveTagsInput, _ ...func(*emr.Options)) (*emr.RemoveTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var check error
	if len(in.TagKeys) == 0 {
		check = invalid("TagKeys is required")
	}
	return &emr.RemoveTagsOutput{}, f.change("RemoveTags", aws.ToString(in.ResourceId), check, taggable, func(props map[string]any) {
		tags := map[string]string{}
		for _, t := range awsapi.Tags(props) {
			if !slices.Contains(in.TagKeys, t.Key) {
				tags[t.Key] = t.Value
			}
		}
		if list := awsapi.TagList(tags); list != nil {
			props["Tags"] = list
		} else {
			delete(props, "Tags")
		}
	})
}

func (f *fakeAPI) TerminateJobFlows(ctx context.Context, in *emr.TerminateJobFlowsInput, _ ...func(*emr.Options)) (*emr.TerminateJobFlowsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("TerminateJobFlows"); err != nil {
		return nil, err
	}
	if err := validJobFlowIDs(in.JobFlowIds); err != nil {
		return nil, err
	}
	if len(in.JobFlowIds) > maxTerminate {
		return nil, invalid("TerminateJobFlows shuts down at most %d clusters", maxTerminate)
	}
	for _, id := range in.JobFlowIds {
		props := f.store.Resource(clusterType, id)
		if props == nil {
			return nil, fault("ValidationException", fmt.Sprintf("Cluster id '%s' is not valid.", id))
		}
		if protected, _ := asMap(props["Instances"])["TerminationProtected"].(bool); protected {
			return nil, fault("ValidationException", "Could not shut down one or more job flows since they are termination protected.")
		}
		f.state[id] = types.ClusterStateTerminating
	}
	return &emr.TerminateJobFlowsOutput{}, nil
}

// emptyDescribe is an API whose DescribeCluster returns no cluster and no error.
type emptyDescribe struct {
	*fakeAPI
}

func (e emptyDescribe) DescribeCluster(ctx context.Context, in *emr.DescribeClusterInput, _ ...func(*emr.Options)) (*emr.DescribeClusterOutput, error) {
	return &emr.DescribeClusterOutput{}, nil
}

// sparseDescribe is an API whose DescribeCluster returns a cluster with only its ID, as for a cluster AWS reports
// little about.
type sparseDescribe struct {
	*fakeAPI
}

func (s sparseDescribe) DescribeCluster(ctx context.Context, in *emr.DescribeClusterInput, _ ...func(*emr.Options)) (*emr.DescribeClusterOutput, error) {
	return &emr.DescribeClusterOutput{Cluster: &types.Cluster{Id: in.ClusterId}}, nil
}
