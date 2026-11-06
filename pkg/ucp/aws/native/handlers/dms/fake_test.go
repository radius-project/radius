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

package dms

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	dms "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
	"github.com/aws/aws-sdk-go-v2/service/databasemigrationservice/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:dms:us-west-2:123456789012:rep:"

// fakeAPI is an in-memory DMS API. It keeps replication instances in an awsfake store, in the type's property
// format, so the suite's Cloud Control sees the same instances. An instance reports a creating, modifying, or
// deleting status for one read before it finishes.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu     sync.Mutex
	status map[string]string
	calls  []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error

	// pageSize, when set, is how many instances one page holds.
	pageSize int

	// holdDeleting keeps a deleting instance deleting, instead of removing it after one read.
	holdDeleting bool

	// pendingReads is how many reads report a modification as pending after the instance is available again.
	pendingReads int

	// lastModify is the input of the last ModifyReplicationInstance call.
	lastModify *dms.ModifyReplicationInstanceInput
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

// state returns the status of the instance, and moves a transitional status on: a create or update finishes, and a
// delete removes the instance, which then returns false.
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
		if !f.holdDeleting {
			f.status[id] = "deleted-next"
		}
		return status, true
	case "available-next":
		delete(f.status, id)
		return "available", true
	case "deleted-next":
		delete(f.status, id)
		f.store.Remove(instanceType, id)
		return "", false
	}
	return status, true
}

func (f *fakeAPI) CreateReplicationInstance(ctx context.Context, in *dms.CreateReplicationInstanceInput, _ ...func(*dms.Options)) (*dms.CreateReplicationInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateReplicationInstance"); err != nil {
		return nil, err
	}
	id := strings.ToLower(aws.ToString(in.ReplicationInstanceIdentifier))
	if f.store.Resource(instanceType, id) != nil {
		return nil, fault("ResourceAlreadyExistsFault")
	}
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	props["ReplicationInstanceIdentifier"] = id
	f.store.Put(instanceType, id, props)
	f.status[id] = "creating"
	return &dms.CreateReplicationInstanceOutput{}, nil
}

// arn returns the ARN of the instance with the identifier and props.
func arn(id string, props map[string]any) string {
	if resource := awsapi.String(props, "ResourceIdentifier"); resource != "" {
		return arnPrefix + resource
	}
	return arnPrefix + "GENERATED" + strings.ToUpper(strings.ReplaceAll(id, "-", ""))
}

// instanceOf returns the identifier of the instance with arn, or "".
func (f *fakeAPI) instanceOf(arnValue *string) string {
	for _, id := range f.store.List(instanceType) {
		if arn(id, f.store.Resource(instanceType, id)) == aws.ToString(arnValue) {
			return id
		}
	}
	return ""
}

// instance returns the instance with the identifier as DescribeReplicationInstances does, and false when there is
// none.
func (f *fakeAPI) instance(id string) (types.ReplicationInstance, bool, error) {
	props := f.store.Resource(instanceType, id)
	if props == nil {
		return types.ReplicationInstance{}, false, nil
	}
	status, ok := f.state(id)
	if !ok {
		return types.ReplicationInstance{}, false, nil
	}
	i := types.ReplicationInstance{}
	if err := awsapi.Decode(awsapi.Without(props, "Tags", "VpcSecurityGroupIds"), &i); err != nil {
		return i, false, err
	}
	if i.ReplicationInstanceIdentifier == nil {
		i.ReplicationInstanceIdentifier = aws.String(id)
	}
	i.ReplicationInstanceArn = aws.String(arn(id, props))
	i.ReplicationInstanceStatus = aws.String(status)
	i.ReplicationInstancePrivateIpAddresses = []string{"10.0.0.1"}
	if group := awsapi.String(props, "ReplicationSubnetGroupIdentifier"); group != "" {
		i.ReplicationSubnetGroup = &types.ReplicationSubnetGroup{ReplicationSubnetGroupIdentifier: aws.String(group)}
	}
	for _, g := range awsapi.Strings(props, "VpcSecurityGroupIds") {
		i.VpcSecurityGroups = append(i.VpcSecurityGroups, types.VpcSecurityGroupMembership{VpcSecurityGroupId: aws.String(g), Status: aws.String("active")})
	}
	if status == "available" && f.pendingReads > 0 {
		f.pendingReads--
		i.PendingModifiedValues = &types.ReplicationPendingModifiedValues{ReplicationInstanceClass: aws.String("dms.t3.large")}
	}
	return i, true, nil
}

func (f *fakeAPI) DescribeReplicationInstances(ctx context.Context, in *dms.DescribeReplicationInstancesInput, _ ...func(*dms.Options)) (*dms.DescribeReplicationInstancesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeReplicationInstances"); err != nil {
		return nil, err
	}
	ids := f.store.List(instanceType)
	filtered := false
	for _, filter := range in.Filters {
		if aws.ToString(filter.Name) == "replication-instance-id" {
			ids, filtered = filter.Values, true
		}
	}
	out := &dms.DescribeReplicationInstancesOutput{}
	start, _ := strconv.Atoi(aws.ToString(in.Marker))
	for n := start; n < len(ids); n++ {
		if f.pageSize > 0 && len(out.ReplicationInstances) == f.pageSize {
			out.Marker = aws.String(strconv.Itoa(n))
			break
		}
		i, ok, err := f.instance(ids[n])
		if err != nil {
			return nil, err
		}
		if ok {
			out.ReplicationInstances = append(out.ReplicationInstances, i)
		}
	}
	if filtered && len(out.ReplicationInstances) == 0 {
		return nil, fault("ResourceNotFoundFault")
	}
	return out, nil
}

// change applies a change to the stored instance arn names, as a call that changes AWS.
func (f *fakeAPI) change(arnValue *string, apply func(id string, props map[string]any)) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	id := f.instanceOf(arnValue)
	if id == "" {
		return fault("ResourceNotFoundFault")
	}
	props := f.store.Resource(instanceType, id)
	apply(id, props)
	f.store.Put(instanceType, id, props)
	return f.failer.After()
}

func (f *fakeAPI) ModifyReplicationInstance(ctx context.Context, in *dms.ModifyReplicationInstanceInput, _ ...func(*dms.Options)) (*dms.ModifyReplicationInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ModifyReplicationInstance"); err != nil {
		return nil, err
	}
	f.lastModify = in
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"ReplicationInstanceArn", "ApplyImmediately", "AllowMajorVersionUpgrade"} {
		delete(values, name)
	}
	if in.EngineVersion != nil {
		values["AllowMajorVersionUpgrade"] = in.AllowMajorVersionUpgrade
	}
	err = f.change(in.ReplicationInstanceArn, func(id string, props map[string]any) {
		for k, v := range values {
			props[k] = v
		}
		f.status[id] = "modifying"
	})
	return &dms.ModifyReplicationInstanceOutput{}, err
}

func (f *fakeAPI) DeleteReplicationInstance(ctx context.Context, in *dms.DeleteReplicationInstanceInput, _ ...func(*dms.Options)) (*dms.DeleteReplicationInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteReplicationInstance"); err != nil {
		return nil, err
	}
	id := f.instanceOf(in.ReplicationInstanceArn)
	if id == "" {
		return nil, fault("ResourceNotFoundFault")
	}
	if strings.HasPrefix(f.status[id], "delet") {
		return nil, fault("InvalidResourceStateFault")
	}
	f.status[id] = "deleting"
	return &dms.DeleteReplicationInstanceOutput{}, nil
}

func (f *fakeAPI) ListTagsForResource(ctx context.Context, in *dms.ListTagsForResourceInput, _ ...func(*dms.Options)) (*dms.ListTagsForResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListTagsForResource"); err != nil {
		return nil, err
	}
	id := f.instanceOf(in.ResourceArn)
	if id == "" {
		return nil, fault("ResourceNotFoundFault")
	}
	out := &dms.ListTagsForResourceOutput{}
	for _, t := range awsapi.Tags(f.store.Resource(instanceType, id)) {
		out.TagList = append(out.TagList, types.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value), ResourceArn: in.ResourceArn})
	}
	return out, nil
}

func (f *fakeAPI) AddTagsToResource(ctx context.Context, in *dms.AddTagsToResourceInput, _ ...func(*dms.Options)) (*dms.AddTagsToResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("AddTagsToResource"); err != nil {
		return nil, err
	}
	return &dms.AddTagsToResourceOutput{}, f.change(in.ResourceArn, func(_ string, props map[string]any) {
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

func (f *fakeAPI) RemoveTagsFromResource(ctx context.Context, in *dms.RemoveTagsFromResourceInput, _ ...func(*dms.Options)) (*dms.RemoveTagsFromResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RemoveTagsFromResource"); err != nil {
		return nil, err
	}
	return &dms.RemoveTagsFromResourceOutput{}, f.change(in.ResourceArn, func(_ string, props map[string]any) {
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
