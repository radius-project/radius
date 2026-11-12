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
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dax"
	"github.com/aws/aws-sdk-go-v2/service/dax/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:dax:us-west-2:123456789012:cache/"

// fakeAPI is an in-memory DAX API. It keeps clusters in an awsfake store, in the type's property format, so the
// suite's Cloud Control sees the same clusters. A cluster reports a creating, modifying, or deleting status for one
// read before it finishes.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu     sync.Mutex
	status map[string]string
	calls  []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error

	// pageSize, when set, is how many clusters or tags one page holds.
	pageSize int

	// awsTags are tags AWS adds to every cluster, which ListTags returns.
	awsTags map[string]string

	// lastIncrease and lastDecrease are the inputs of the last calls that change the number of nodes.
	lastIncrease *dax.IncreaseReplicationFactorInput
	lastDecrease *dax.DecreaseReplicationFactorInput
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, status: map[string]string{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

func (f *fakeAPI) call(name string) error {
	f.calls = append(f.calls, name)
	return f.errs[name]
}

// busy returns DAX's error for a call that needs the cluster available while it is creating or modifying, or nil.
func (f *fakeAPI) busy(name *string) error {
	if _, ok := f.status[strings.ToLower(aws.ToString(name))]; ok {
		return fault("InvalidClusterStateFault")
	}
	return nil
}

// state returns the status of the cluster, and moves a transitional status on: a create or update finishes, and a
// delete removes the cluster, which then returns false.
func (f *fakeAPI) state(id string) (string, bool) {
	status, ok := f.status[id]
	if !ok {
		return "available", true
	}
	switch status {
	case "creating", "modifying":
		f.status[id] = "available-next"
		return status, true
	case "deleting":
		f.status[id] = "deleted-next"
		return status, true
	case "available-next":
		delete(f.status, id)
		return "available", true
	case "deleted-next":
		delete(f.status, id)
		f.store.Remove(clusterType, id)
		return "", false
	}
	return status, true
}

func (f *fakeAPI) CreateCluster(ctx context.Context, in *dax.CreateClusterInput, _ ...func(*dax.Options)) (*dax.CreateClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateCluster"); err != nil {
		return nil, err
	}
	id := strings.ToLower(aws.ToString(in.ClusterName))
	if f.store.Resource(clusterType, id) != nil {
		return nil, fault("ClusterAlreadyExistsFault")
	}
	p := map[string]any{
		"ClusterName":                   id,
		"Description":                   in.Description,
		"ReplicationFactor":             in.ReplicationFactor,
		"ParameterGroupName":            in.ParameterGroupName,
		"AvailabilityZones":             in.AvailabilityZones,
		"IAMRoleARN":                    in.IamRoleArn,
		"SubnetGroupName":               in.SubnetGroupName,
		"PreferredMaintenanceWindow":    in.PreferredMaintenanceWindow,
		"ClusterEndpointEncryptionType": in.ClusterEndpointEncryptionType,
		"NotificationTopicARN":          in.NotificationTopicArn,
		"SecurityGroupIds":              in.SecurityGroupIds,
		"NetworkType":                   in.NetworkType,
		"NodeType":                      in.NodeType,
	}
	if in.SSESpecification != nil {
		p["SSESpecification"] = map[string]any{"SSEEnabled": aws.ToBool(in.SSESpecification.Enabled)}
	}
	if len(in.Tags) > 0 {
		tags := map[string]any{}
		for _, t := range in.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		p["Tags"] = tags
	}
	props, err := awsapi.EncodeObject(p)
	if err != nil {
		return nil, err
	}
	f.store.Put(clusterType, id, props)
	f.status[id] = "creating"
	return &dax.CreateClusterOutput{}, nil
}

// number returns the number property name of props.
func number(props map[string]any, name string) int32 {
	n, _ := props[name].(float64)
	return int32(n)
}

func stringProp(props map[string]any, name string) *string {
	if s := awsapi.String(props, name); s != "" {
		return aws.String(s)
	}
	return nil
}

// cluster returns the cluster with the identifier as DescribeClusters does, and false when there is none.
func (f *fakeAPI) cluster(id string) (types.Cluster, bool) {
	props := f.store.Resource(clusterType, id)
	if props == nil {
		return types.Cluster{}, false
	}
	status, ok := f.state(id)
	if !ok {
		return types.Cluster{}, false
	}
	name := awsapi.String(props, "ClusterName")
	if name == "" {
		name = id
	}
	address := id + ".abc123.dax-clusters.us-west-2.amazonaws.com"
	c := types.Cluster{
		ClusterName:                   aws.String(name),
		ClusterArn:                    aws.String(arnPrefix + id),
		Status:                        aws.String(status),
		Description:                   stringProp(props, "Description"),
		NodeType:                      stringProp(props, "NodeType"),
		IamRoleArn:                    stringProp(props, "IAMRoleARN"),
		SubnetGroup:                   stringProp(props, "SubnetGroupName"),
		PreferredMaintenanceWindow:    stringProp(props, "PreferredMaintenanceWindow"),
		ClusterEndpointEncryptionType: types.ClusterEndpointEncryptionType(awsapi.String(props, "ClusterEndpointEncryptionType")),
		NetworkType:                   types.NetworkType(awsapi.String(props, "NetworkType")),
		TotalNodes:                    aws.Int32(number(props, "ReplicationFactor")),
		ClusterDiscoveryEndpoint:      &types.Endpoint{Address: aws.String(address), Port: 8111, URL: aws.String("dax://" + address)},
		SSEDescription:                &types.SSEDescription{Status: types.SSEStatusDisabled},
	}
	if group := stringProp(props, "ParameterGroupName"); group != nil {
		c.ParameterGroup = &types.ParameterGroupStatus{ParameterGroupName: group}
	}
	if topic := stringProp(props, "NotificationTopicARN"); topic != nil {
		c.NotificationConfiguration = &types.NotificationConfiguration{TopicArn: topic, TopicStatus: aws.String("active")}
	}
	if sse, ok := props["SSESpecification"].(map[string]any); ok && sse["SSEEnabled"] == true {
		c.SSEDescription.Status = types.SSEStatusEnabled
	}
	for _, g := range awsapi.Strings(props, "SecurityGroupIds") {
		c.SecurityGroups = append(c.SecurityGroups, types.SecurityGroupMembership{SecurityGroupIdentifier: aws.String(g), Status: aws.String("active")})
	}
	for i, zone := range awsapi.Strings(props, "AvailabilityZones") {
		c.Nodes = append(c.Nodes, types.Node{NodeId: aws.String(fmt.Sprintf("%s-%d", id, i)), AvailabilityZone: aws.String(zone)})
	}
	return c, true
}

func (f *fakeAPI) DescribeClusters(ctx context.Context, in *dax.DescribeClustersInput, _ ...func(*dax.Options)) (*dax.DescribeClustersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeClusters"); err != nil {
		return nil, err
	}
	ids := f.store.List(clusterType)
	if len(in.ClusterNames) > 0 {
		ids = []string{}
		for _, name := range in.ClusterNames {
			ids = append(ids, strings.ToLower(name))
		}
	}
	out := &dax.DescribeClustersOutput{}
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	for i := start; i < len(ids); i++ {
		if f.pageSize > 0 && len(out.Clusters) == f.pageSize {
			out.NextToken = aws.String(strconv.Itoa(i))
			break
		}
		c, ok := f.cluster(ids[i])
		if !ok {
			if len(in.ClusterNames) > 0 {
				return nil, fault("ClusterNotFoundFault")
			}
			continue
		}
		out.Clusters = append(out.Clusters, c)
	}
	return out, nil
}

// change applies a change to the stored cluster, as a call that changes AWS.
func (f *fakeAPI) change(id string, apply func(props map[string]any)) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	props := f.store.Resource(clusterType, id)
	if props == nil {
		return fault("ClusterNotFoundFault")
	}
	apply(props)
	f.store.Put(clusterType, id, props)
	return f.failer.After()
}

func (f *fakeAPI) UpdateCluster(ctx context.Context, in *dax.UpdateClusterInput, _ ...func(*dax.Options)) (*dax.UpdateClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UpdateCluster"); err != nil {
		return nil, err
	}
	if err := f.busy(in.ClusterName); err != nil {
		return nil, err
	}
	id := strings.ToLower(aws.ToString(in.ClusterName))
	err := f.change(id, func(props map[string]any) {
		set := func(name string, value *string) {
			if value != nil {
				props[name] = *value
			}
		}
		set("Description", in.Description)
		set("ParameterGroupName", in.ParameterGroupName)
		set("PreferredMaintenanceWindow", in.PreferredMaintenanceWindow)
		set("NotificationTopicARN", in.NotificationTopicArn)
		if aws.ToString(in.NotificationTopicStatus) == "inactive" {
			delete(props, "NotificationTopicARN")
		}
		if in.SecurityGroupIds != nil {
			props["SecurityGroupIds"] = in.SecurityGroupIds
		}
		f.status[id] = "modifying"
	})
	return &dax.UpdateClusterOutput{}, err
}

func (f *fakeAPI) IncreaseReplicationFactor(ctx context.Context, in *dax.IncreaseReplicationFactorInput, _ ...func(*dax.Options)) (*dax.IncreaseReplicationFactorOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("IncreaseReplicationFactor"); err != nil {
		return nil, err
	}
	if err := f.busy(in.ClusterName); err != nil {
		return nil, err
	}
	f.lastIncrease = in
	id := strings.ToLower(aws.ToString(in.ClusterName))
	err := f.change(id, func(props map[string]any) {
		props["ReplicationFactor"] = in.NewReplicationFactor
		if len(in.AvailabilityZones) > 0 {
			props["AvailabilityZones"] = in.AvailabilityZones
		}
		f.status[id] = "modifying"
	})
	return &dax.IncreaseReplicationFactorOutput{}, err
}

func (f *fakeAPI) DecreaseReplicationFactor(ctx context.Context, in *dax.DecreaseReplicationFactorInput, _ ...func(*dax.Options)) (*dax.DecreaseReplicationFactorOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DecreaseReplicationFactor"); err != nil {
		return nil, err
	}
	if err := f.busy(in.ClusterName); err != nil {
		return nil, err
	}
	f.lastDecrease = in
	id := strings.ToLower(aws.ToString(in.ClusterName))
	err := f.change(id, func(props map[string]any) {
		props["ReplicationFactor"] = in.NewReplicationFactor
		zones := awsapi.Strings(props, "AvailabilityZones")
		for _, zone := range in.AvailabilityZones {
			if i := slices.Index(zones, zone); i >= 0 {
				zones = slices.Delete(zones, i, i+1)
			}
		}
		if len(zones) > int(in.NewReplicationFactor) {
			zones = zones[:in.NewReplicationFactor]
		}
		if len(zones) == 0 {
			delete(props, "AvailabilityZones")
		} else {
			props["AvailabilityZones"] = zones
		}
		f.status[id] = "modifying"
	})
	return &dax.DecreaseReplicationFactorOutput{}, err
}

func (f *fakeAPI) DeleteCluster(ctx context.Context, in *dax.DeleteClusterInput, _ ...func(*dax.Options)) (*dax.DeleteClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteCluster"); err != nil {
		return nil, err
	}
	id := strings.ToLower(aws.ToString(in.ClusterName))
	if f.store.Resource(clusterType, id) == nil {
		return nil, fault("ClusterNotFoundFault")
	}
	if strings.HasPrefix(f.status[id], "delet") {
		return nil, fault("InvalidClusterStateFault")
	}
	f.status[id] = "deleting"
	return &dax.DeleteClusterOutput{}, nil
}

// clusterOf returns the identifier of the cluster arn names.
func clusterOf(arn *string) string {
	return strings.TrimPrefix(aws.ToString(arn), arnPrefix)
}

// storedTags returns the Tags property of props.
func storedTags(props map[string]any) map[string]any {
	tags, _ := props["Tags"].(map[string]any)
	if tags == nil {
		return map[string]any{}
	}
	return tags
}

func (f *fakeAPI) ListTags(ctx context.Context, in *dax.ListTagsInput, _ ...func(*dax.Options)) (*dax.ListTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListTags"); err != nil {
		return nil, err
	}
	props := f.store.Resource(clusterType, clusterOf(in.ResourceName))
	if props == nil {
		return nil, fault("ClusterNotFoundFault")
	}
	all := []types.Tag{}
	for key, value := range f.awsTags {
		all = append(all, types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}
	for _, t := range tagList(props) {
		all = append(all, types.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value)})
	}
	out := &dax.ListTagsOutput{}
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	for i := start; i < len(all); i++ {
		if f.pageSize > 0 && len(out.Tags) == f.pageSize {
			out.NextToken = aws.String(strconv.Itoa(i))
			break
		}
		out.Tags = append(out.Tags, all[i])
	}
	return out, nil
}

func (f *fakeAPI) TagResource(ctx context.Context, in *dax.TagResourceInput, _ ...func(*dax.Options)) (*dax.TagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("TagResource"); err != nil {
		return nil, err
	}
	return &dax.TagResourceOutput{}, f.change(clusterOf(in.ResourceName), func(props map[string]any) {
		tags := storedTags(props)
		for _, t := range in.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		props["Tags"] = tags
	})
}

func (f *fakeAPI) UntagResource(ctx context.Context, in *dax.UntagResourceInput, _ ...func(*dax.Options)) (*dax.UntagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UntagResource"); err != nil {
		return nil, err
	}
	return &dax.UntagResourceOutput{}, f.change(clusterOf(in.ResourceName), func(props map[string]any) {
		tags := storedTags(props)
		for _, key := range in.TagKeys {
			delete(tags, key)
		}
		if len(tags) == 0 {
			delete(props, "Tags")
		} else {
			props["Tags"] = tags
		}
	})
}
