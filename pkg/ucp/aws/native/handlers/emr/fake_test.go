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
	"slices"
	"strconv"
	"sync"

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
// RESIZING for one list. ListClusters returns one cluster per page, and ListInstanceGroups two groups per page.
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

func (f *fakeAPI) RunJobFlow(ctx context.Context, in *emr.RunJobFlowInput, _ ...func(*emr.Options)) (*emr.RunJobFlowOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RunJobFlow"); err != nil {
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
		delete(f.resizing, id)
	}
	out := &emr.ListInstanceFleetsOutput{}
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
		out.InstanceFleets = append(out.InstanceFleets, fl)
	}
	return out, nil
}

func (f *fakeAPI) GetManagedScalingPolicy(ctx context.Context, in *emr.GetManagedScalingPolicyInput, _ ...func(*emr.Options)) (*emr.GetManagedScalingPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("GetManagedScalingPolicy"); err != nil {
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
	out := &emr.GetAutoTerminationPolicyOutput{}
	if p, ok := f.store.Resource(clusterType, aws.ToString(in.ClusterId))["AutoTerminationPolicy"]; ok {
		out.AutoTerminationPolicy = &types.AutoTerminationPolicy{}
		if err := decode(asMap(p), out.AutoTerminationPolicy); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// change applies a change to the stored cluster, as a call that changes AWS.
func (f *fakeAPI) change(name string, id string, apply func(props map[string]any)) error {
	if err := f.call(name); err != nil {
		return err
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
	return &emr.ModifyClusterOutput{StepConcurrencyLevel: in.StepConcurrencyLevel}, f.change("ModifyCluster", aws.ToString(in.ClusterId), func(props map[string]any) {
		set(props, "StepConcurrencyLevel", in.StepConcurrencyLevel)
	})
}

func (f *fakeAPI) ModifyInstanceGroups(ctx context.Context, in *emr.ModifyInstanceGroupsInput, _ ...func(*emr.Options)) (*emr.ModifyInstanceGroupsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := aws.ToString(in.ClusterId)
	return &emr.ModifyInstanceGroupsOutput{}, f.change("ModifyInstanceGroups", id, func(props map[string]any) {
		inst := asMap(props["Instances"])
		for _, m := range in.InstanceGroups {
			count := float64(aws.ToInt32(m.InstanceCount))
			switch groupID := aws.ToString(m.InstanceGroupId); groupID {
			case "ig-core":
				asMap(inst["CoreInstanceGroup"])["InstanceCount"] = count
			default:
				var i int
				if _, err := fmt.Sscanf(groupID, "ig-task-%d", &i); err != nil {
					panic(err)
				}
				asMap(asList(inst["TaskInstanceGroups"])[i])["InstanceCount"] = count
			}
		}
		f.resizing[id] = true
	})
}

func (f *fakeAPI) SetVisibleToAllUsers(ctx context.Context, in *emr.SetVisibleToAllUsersInput, _ ...func(*emr.Options)) (*emr.SetVisibleToAllUsersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.SetVisibleToAllUsersOutput{}, f.change("SetVisibleToAllUsers", in.JobFlowIds[0], func(props map[string]any) {
		props["VisibleToAllUsers"] = aws.ToBool(in.VisibleToAllUsers)
	})
}

func (f *fakeAPI) SetTerminationProtection(ctx context.Context, in *emr.SetTerminationProtectionInput, _ ...func(*emr.Options)) (*emr.SetTerminationProtectionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.SetTerminationProtectionOutput{}, f.change("SetTerminationProtection", in.JobFlowIds[0], func(props map[string]any) {
		asMap(props["Instances"])["TerminationProtected"] = aws.ToBool(in.TerminationProtected)
	})
}

func (f *fakeAPI) SetUnhealthyNodeReplacement(ctx context.Context, in *emr.SetUnhealthyNodeReplacementInput, _ ...func(*emr.Options)) (*emr.SetUnhealthyNodeReplacementOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.SetUnhealthyNodeReplacementOutput{}, f.change("SetUnhealthyNodeReplacement", in.JobFlowIds[0], func(props map[string]any) {
		asMap(props["Instances"])["UnhealthyNodeReplacement"] = aws.ToBool(in.UnhealthyNodeReplacement)
	})
}

func (f *fakeAPI) PutManagedScalingPolicy(ctx context.Context, in *emr.PutManagedScalingPolicyInput, _ ...func(*emr.Options)) (*emr.PutManagedScalingPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.PutManagedScalingPolicyOutput{}, f.change("PutManagedScalingPolicy", aws.ToString(in.ClusterId), func(props map[string]any) {
		set(props, "ManagedScalingPolicy", in.ManagedScalingPolicy)
	})
}

func (f *fakeAPI) RemoveManagedScalingPolicy(ctx context.Context, in *emr.RemoveManagedScalingPolicyInput, _ ...func(*emr.Options)) (*emr.RemoveManagedScalingPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.RemoveManagedScalingPolicyOutput{}, f.change("RemoveManagedScalingPolicy", aws.ToString(in.ClusterId), func(props map[string]any) {
		delete(props, "ManagedScalingPolicy")
	})
}

func (f *fakeAPI) PutAutoTerminationPolicy(ctx context.Context, in *emr.PutAutoTerminationPolicyInput, _ ...func(*emr.Options)) (*emr.PutAutoTerminationPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.PutAutoTerminationPolicyOutput{}, f.change("PutAutoTerminationPolicy", aws.ToString(in.ClusterId), func(props map[string]any) {
		set(props, "AutoTerminationPolicy", in.AutoTerminationPolicy)
	})
}

func (f *fakeAPI) RemoveAutoTerminationPolicy(ctx context.Context, in *emr.RemoveAutoTerminationPolicyInput, _ ...func(*emr.Options)) (*emr.RemoveAutoTerminationPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.RemoveAutoTerminationPolicyOutput{}, f.change("RemoveAutoTerminationPolicy", aws.ToString(in.ClusterId), func(props map[string]any) {
		delete(props, "AutoTerminationPolicy")
	})
}

func (f *fakeAPI) AddTags(ctx context.Context, in *emr.AddTagsInput, _ ...func(*emr.Options)) (*emr.AddTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &emr.AddTagsOutput{}, f.change("AddTags", aws.ToString(in.ResourceId), func(props map[string]any) {
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
	return &emr.RemoveTagsOutput{}, f.change("RemoveTags", aws.ToString(in.ResourceId), func(props map[string]any) {
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
