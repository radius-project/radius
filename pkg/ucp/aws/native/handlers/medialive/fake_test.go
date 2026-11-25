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
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

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
// gone. It follows the rules MediaLive documents, recorded in api_test.go: an update, class change, or delete of a
// channel that isn't IDLE fails with ConflictException, and settings that don't fit together fail with
// UnprocessableEntityException.
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

// invalid is the error MediaLive returns for channel settings that don't fit together, its 422 response,
// ChannelConfigurationValidationError.
func invalid(message string) error {
	return &smithy.GenericAPIError{Code: "UnprocessableEntityException", Message: message, Fault: smithy.FaultClient}
}

// pipelines maps a channel class to its number of pipelines, which is the number of addresses each destination's
// Settings has: one for each pipeline.
var pipelines = map[string]int{string(types.ChannelClassStandard): 2, string(types.ChannelClassSinglePipeline): 1}

// groupName is the pattern of an output group's name: at most 32 letters, numbers, and underscores.
var groupName = regexp.MustCompile(`^\w{0,32}$`)

// list returns v as a list of objects, leaving out values that aren't objects.
func list(v any) []map[string]any {
	items, _ := v.([]any)
	out := []map[string]any{}
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// names returns the Name of each object in the list v.
func names(v any) map[string]bool {
	out := map[string]bool{}
	for _, item := range list(v) {
		if name, ok := item["Name"].(string); ok {
			out[name] = true
		}
	}
	return out
}

// destinationRefs returns the DestinationRefId of each kind of settings in settings, such as HlsGroupSettings.
func destinationRefs(settings any) []string {
	kinds, _ := settings.(map[string]any)
	refs := []string{}
	for _, kind := range kinds {
		k, _ := kind.(map[string]any)
		destination, _ := k["Destination"].(map[string]any)
		if ref, ok := destination["DestinationRefId"].(string); ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

// validate returns UnprocessableEntityException, as MediaLive does, when a channel's settings don't fit together:
// each destination has one address in Settings for each pipeline of the channel's class, output groups and outputs
// name destinations and descriptions the channel has, and output group names follow their pattern.
func validate(props map[string]any) error {
	ids := map[string]bool{}
	class, _ := props["ChannelClass"].(string)
	n, classSet := pipelines[class]
	for _, d := range list(props["Destinations"]) {
		if id, ok := d["Id"].(string); ok {
			ids[id] = true
		}
		if settings, _ := d["Settings"].([]any); classSet && len(settings) > 0 && len(settings) != n {
			return invalid("a " + class + " channel has " + strconv.Itoa(n) + " addresses in each destination")
		}
	}
	encoder, _ := props["EncoderSettings"].(map[string]any)
	videos, audios := names(encoder["VideoDescriptions"]), names(encoder["AudioDescriptions"])
	for _, group := range list(encoder["OutputGroups"]) {
		if name, ok := group["Name"].(string); ok && !groupName.MatchString(name) {
			return invalid("output group name " + name + " isn't at most 32 letters, numbers, and underscores")
		}
		refs := destinationRefs(group["OutputGroupSettings"])
		for _, output := range list(group["Outputs"]) {
			refs = append(refs, destinationRefs(output["OutputSettings"])...)
			if video, ok := output["VideoDescriptionName"].(string); ok && !videos[video] {
				return invalid("no video description " + video)
			}
			audio, _ := output["AudioDescriptionNames"].([]any)
			for _, a := range audio {
				if name, _ := a.(string); !audios[name] {
					return invalid("no audio description " + name)
				}
			}
		}
		for _, ref := range refs {
			if !ids[ref] {
				return invalid("no destination " + ref)
			}
		}
	}
	return nil
}

// tagPattern is the pattern of a tag's key and value: letters, numbers, spaces, and _ . : / = + - @.
var tagPattern = regexp.MustCompile(`^[a-zA-Z0-9 _.:/=+\-@]*$`)

// maxTags is the most tags a MediaLive resource has.
const maxTags = 50

// validTags returns BadRequestException, as MediaLive does, when tags break MediaLive's tag restrictions.
func validTags(tags map[string]any) error {
	if len(tags) > maxTags {
		return fault("BadRequestException")
	}
	for k, v := range tags {
		value, _ := v.(string)
		if strings.HasPrefix(k, "aws:") || utf8.RuneCountInString(k) > 128 || utf8.RuneCountInString(value) > 256 ||
			!tagPattern.MatchString(k) || !tagPattern.MatchString(value) {
			return fault("BadRequestException")
		}
	}
	return nil
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
		f := v.FieldByName(name)
		if !f.IsValid() {
			continue
		}
		value, ok := settingsValue(f)
		if !ok {
			continue
		}
		b, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var encoded any
		if err := json.Unmarshal(b, &encoded); err != nil {
			return nil, err
		}
		props[name] = encoded
	}
	return props, nil
}

// settingsValue returns v, part of a request's settings, as encodeSettings does, but keeping the empty strings the
// request sets, such as a DestinationRefId of "", which names a destination as any other value does. It leaves out
// unset values: nil pointers and lists, empty lists, and empty enum values, which the SDK doesn't send. It returns
// false for a value it leaves out.
func settingsValue(v reflect.Value) (any, bool) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil, false
		}
		if v.Kind() == reflect.Pointer && v.Elem().Kind() == reflect.String {
			return v.Elem().String(), true
		}
		return settingsValue(v.Elem())
	case reflect.Struct:
		out := map[string]any{}
		for i := range v.NumField() {
			if field := v.Type().Field(i); field.IsExported() {
				if value, ok := settingsValue(v.Field(i)); ok {
					out[field.Name] = value
				}
			}
		}
		return out, true
	case reflect.Slice:
		out := []any{}
		for i := range v.Len() {
			if v.Index(i).Kind() == reflect.String {
				out = append(out, v.Index(i).String())
			} else if value, ok := settingsValue(v.Index(i)); ok {
				out = append(out, value)
			}
		}
		return out, len(out) > 0
	case reflect.String:
		return v.String(), v.String() != ""
	}
	return v.Interface(), true
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
	props, err := encode(in)
	if err != nil {
		return nil, err
	}
	if err := validate(props); err != nil {
		return nil, err
	}
	if id, ok := f.requests[aws.ToString(in.RequestId)]; ok {
		return &medialive.CreateChannelOutput{Channel: &types.Channel{Id: aws.String(id), State: types.ChannelStateCreating}}, nil
	}
	f.next++
	id := strconv.Itoa(f.next)
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

// change applies a change to the stored channel, as a call that changes AWS, when apply returns no error. A change
// with a state, the channel's state while AWS applies it, needs the channel to be IDLE.
func (f *fakeAPI) change(id string, state types.ChannelState, apply func(props map[string]any) error) error {
	props := f.store.Resource(channelType, id)
	if props == nil {
		return fault("NotFoundException")
	}
	if current, ok := f.state[id]; ok && state != "" {
		return conflict(current)
	}
	if err := apply(props); err != nil {
		return err
	}
	if err := f.failer.Before(); err != nil {
		return err
	}
	f.store.Put(channelType, id, props)
	if state != "" {
		f.state[id] = state
	}
	return f.failer.After()
}

// UpdateChannel changes the settings the call sets, and removes the channel's role when the call leaves out RoleArn,
// as MediaLive documents.
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
	return &medialive.UpdateChannelOutput{}, f.change(id, types.ChannelStateUpdating, func(props map[string]any) error {
		if in.RoleArn == nil {
			delete(props, "RoleArn")
		}
		for k, v := range values {
			props[k] = v
		}
		return validate(props)
	})
}

// UpdateChannelClass changes the channel's class, and Destinations when the call sets them. A change to
// SINGLE_PIPELINE removes the second address of each destination, as MediaLive documents.
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
	return &medialive.UpdateChannelClassOutput{}, f.change(aws.ToString(in.ChannelId), types.ChannelStateUpdating, func(props map[string]any) error {
		for k, v := range values {
			props[k] = v
		}
		if in.ChannelClass == types.ChannelClassSinglePipeline {
			for _, d := range list(props["Destinations"]) {
				if settings, _ := d["Settings"].([]any); len(settings) > 1 {
					d["Settings"] = settings[:1]
				}
			}
		}
		return validate(props)
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
	return &medialive.CreateTagsOutput{}, f.change(strings.TrimPrefix(aws.ToString(in.ResourceArn), arnPrefix), "", func(props map[string]any) error {
		tags, _ := props["Tags"].(map[string]any)
		if tags == nil {
			tags = map[string]any{}
		}
		for k, v := range in.Tags {
			tags[k] = v
		}
		props["Tags"] = tags
		return validTags(tags)
	})
}

func (f *fakeAPI) DeleteTags(ctx context.Context, in *medialive.DeleteTagsInput, optFns ...func(*medialive.Options)) (*medialive.DeleteTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteTags", optFns); err != nil {
		return nil, err
	}
	return &medialive.DeleteTagsOutput{}, f.change(strings.TrimPrefix(aws.ToString(in.ResourceArn), arnPrefix), "", func(props map[string]any) error {
		tags, _ := props["Tags"].(map[string]any)
		for k := range tags {
			if slices.Contains(in.TagKeys, k) {
				delete(tags, k)
			}
		}
		if len(tags) == 0 {
			delete(props, "Tags")
		}
		return nil
	})
}
