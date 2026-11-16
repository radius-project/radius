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

package medialive

import (
	"context"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/medialive"
	"github.com/aws/aws-sdk-go-v2/service/medialive/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:medialive:us-west-2:123456789012:channel:"

// fakeAPI is an in-memory MediaLive API. It keeps channels in an awsfake store, in the type's property format,
// including the secrets, which DescribeChannel returns like AWS does, so the suite's Cloud Control sees the same
// channels. A channel is CREATING, UPDATING, or DELETING for one read, then IDLE, or DELETED for one read before it is
// gone. Like AWS, an update or delete of a channel that isn't IDLE fails with ConflictException.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu       sync.Mutex
	state    map[string]types.ChannelState
	requests map[string]string
	next     int
	calls    []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error

	// pageSize, when set, is how many channels a ListChannels page holds.
	pageSize int
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, state: map[string]types.ChannelState{}, requests: map[string]string{}, next: 1234500, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// conflict is the error MediaLive returns for a change to a channel that isn't IDLE.
func conflict(state types.ChannelState) error {
	return &smithy.GenericAPIError{Code: "ConflictException", Message: "the channel is " + string(state), Fault: smithy.FaultClient}
}

// call records a call and the region its options set.
func (f *fakeAPI) call(name string, optFns []func(*medialive.Options)) error {
	o := medialive.Options{}
	for _, fn := range optFns {
		fn(&o)
	}
	f.calls = append(f.calls, name+" "+o.Region)
	return f.errs[name]
}

// encode returns the properties of a request, keeping the empty objects in its settings, as AWS keeps them.
func encode(in any) (map[string]any, error) {
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	v := reflect.ValueOf(in).Elem()
	for _, name := range []string{"EncoderSettings", "Destinations", "InputAttachments"} {
		if f := v.FieldByName(name); f.IsValid() {
			encoded, err := encodeSettings(f.Interface())
			if err != nil {
				return nil, err
			}
			if encoded != nil {
				props[name] = encoded
			}
		}
	}
	return props, nil
}

// read returns the channel's state, and moves a transitional state on. A DELETED channel is gone after its read.
func (f *fakeAPI) read(id string) (types.ChannelState, bool) {
	if f.store.Resource(channelType, id) == nil {
		return "", false
	}
	state, ok := f.state[id]
	if !ok {
		return types.ChannelStateIdle, true
	}
	switch state {
	case types.ChannelStateCreating, types.ChannelStateUpdating:
		delete(f.state, id)
	case types.ChannelStateDeleting:
		f.state[id] = types.ChannelStateDeleted
	case types.ChannelStateDeleted:
		delete(f.state, id)
		f.store.Remove(channelType, id)
	}
	return state, true
}

func (f *fakeAPI) CreateChannel(ctx context.Context, in *medialive.CreateChannelInput, optFns ...func(*medialive.Options)) (*medialive.CreateChannelOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateChannel", optFns); err != nil {
		return nil, err
	}
	if id, ok := f.requests[aws.ToString(in.RequestId)]; ok {
		return &medialive.CreateChannelOutput{Channel: &types.Channel{Id: aws.String(id), State: types.ChannelStateCreating}}, nil
	}
	f.next++
	id := strconv.Itoa(f.next)
	props, err := encode(in)
	if err != nil {
		return nil, err
	}
	delete(props, "RequestId")
	props["Id"], props["Arn"] = id, arnPrefix+id
	f.store.Put(channelType, id, props)
	f.state[id] = types.ChannelStateCreating
	if in.RequestId != nil {
		f.requests[aws.ToString(in.RequestId)] = id
	}
	return &medialive.CreateChannelOutput{Channel: &types.Channel{Id: aws.String(id), State: types.ChannelStateCreating}}, nil
}

func (f *fakeAPI) DescribeChannel(ctx context.Context, in *medialive.DescribeChannelInput, optFns ...func(*medialive.Options)) (*medialive.DescribeChannelOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeChannel", optFns); err != nil {
		return nil, err
	}
	return f.describe(aws.ToString(in.ChannelId))
}

func (f *fakeAPI) describe(id string) (*medialive.DescribeChannelOutput, error) {
	props := f.store.Resource(channelType, id)
	state, ok := f.read(id)
	if !ok {
		return nil, fault("NotFoundException")
	}
	out := &medialive.DescribeChannelOutput{}
	if err := awsapi.Decode(props, out); err != nil {
		return nil, err
	}
	out.Id, out.Arn, out.State = aws.String(id), aws.String(arnPrefix+id), state
	out.PipelinesRunningCount = aws.Int32(0)
	out.EgressEndpoints = []types.ChannelEgressEndpoint{{SourceIp: aws.String("192.0.2.1")}}
	if out.Maintenance != nil {
		out.Maintenance.MaintenanceDeadline = aws.String("2026-12-01T00:00:00Z")
	}
	if out.Tags == nil {
		out.Tags = map[string]string{}
	}
	out.Tags["aws:cloudformation:stack-name"] = "outside"
	return out, nil
}

// change applies a change to the stored channel, as a call that changes AWS. The channel must be IDLE.
func (f *fakeAPI) change(id string, state types.ChannelState, apply func(props map[string]any)) error {
	props := f.store.Resource(channelType, id)
	if props == nil {
		return fault("NotFoundException")
	}
	if current, ok := f.state[id]; ok && state != "" {
		return conflict(current)
	}
	if err := f.failer.Before(); err != nil {
		return err
	}
	apply(props)
	f.store.Put(channelType, id, props)
	if state != "" {
		f.state[id] = state
	}
	return f.failer.After()
}

func (f *fakeAPI) UpdateChannel(ctx context.Context, in *medialive.UpdateChannelInput, optFns ...func(*medialive.Options)) (*medialive.UpdateChannelOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UpdateChannel", optFns); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ChannelId)
	values, err := encode(in)
	if err != nil {
		return nil, err
	}
	delete(values, "ChannelId")
	return &medialive.UpdateChannelOutput{}, f.change(id, types.ChannelStateUpdating, func(props map[string]any) {
		for k, v := range values {
			props[k] = v
		}
	})
}

func (f *fakeAPI) UpdateChannelClass(ctx context.Context, in *medialive.UpdateChannelClassInput, optFns ...func(*medialive.Options)) (*medialive.UpdateChannelClassOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UpdateChannelClass", optFns); err != nil {
		return nil, err
	}
	values, err := encode(in)
	if err != nil {
		return nil, err
	}
	delete(values, "ChannelId")
	return &medialive.UpdateChannelClassOutput{}, f.change(aws.ToString(in.ChannelId), types.ChannelStateUpdating, func(props map[string]any) {
		for k, v := range values {
			props[k] = v
		}
	})
}

func (f *fakeAPI) DeleteChannel(ctx context.Context, in *medialive.DeleteChannelInput, optFns ...func(*medialive.Options)) (*medialive.DeleteChannelOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteChannel", optFns); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ChannelId)
	if f.store.Resource(channelType, id) == nil {
		return nil, fault("NotFoundException")
	}
	if state, ok := f.state[id]; ok {
		return nil, conflict(state)
	}
	f.state[id] = types.ChannelStateDeleting
	return &medialive.DeleteChannelOutput{}, nil
}

func (f *fakeAPI) ListChannels(ctx context.Context, in *medialive.ListChannelsInput, optFns ...func(*medialive.Options)) (*medialive.ListChannelsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListChannels", optFns); err != nil {
		return nil, err
	}
	ids := f.store.List(channelType)
	start := 0
	if in.NextToken != nil {
		start, _ = strconv.Atoi(aws.ToString(in.NextToken))
	}
	end := len(ids)
	out := &medialive.ListChannelsOutput{}
	if f.pageSize > 0 && end-start > f.pageSize {
		end = start + f.pageSize
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	for _, id := range ids[start:end] {
		state, ok := f.state[id]
		if !ok {
			state = types.ChannelStateIdle
		}
		out.Channels = append(out.Channels, types.ChannelSummary{Id: aws.String(id), State: state})
	}
	return out, nil
}

func (f *fakeAPI) CreateTags(ctx context.Context, in *medialive.CreateTagsInput, optFns ...func(*medialive.Options)) (*medialive.CreateTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateTags", optFns); err != nil {
		return nil, err
	}
	return &medialive.CreateTagsOutput{}, f.change(strings.TrimPrefix(aws.ToString(in.ResourceArn), arnPrefix), "", func(props map[string]any) {
		tags, _ := props["Tags"].(map[string]any)
		if tags == nil {
			tags = map[string]any{}
		}
		for k, v := range in.Tags {
			tags[k] = v
		}
		props["Tags"] = tags
	})
}

func (f *fakeAPI) DeleteTags(ctx context.Context, in *medialive.DeleteTagsInput, optFns ...func(*medialive.Options)) (*medialive.DeleteTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteTags", optFns); err != nil {
		return nil, err
	}
	return &medialive.DeleteTagsOutput{}, f.change(strings.TrimPrefix(aws.ToString(in.ResourceArn), arnPrefix), "", func(props map[string]any) {
		tags, _ := props["Tags"].(map[string]any)
		for k := range tags {
			if slices.Contains(in.TagKeys, k) {
				delete(tags, k)
			}
		}
		if len(tags) == 0 {
			delete(props, "Tags")
		}
	})
}
