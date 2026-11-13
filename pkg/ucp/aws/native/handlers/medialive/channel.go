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

// Package medialive has the native handler for MediaLive channels, which Cloud Control can't create in any region.
package medialive

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/medialive"
	"github.com/aws/aws-sdk-go-v2/service/medialive/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const channelType = "AWS::MediaLive::Channel"

// API is the part of the MediaLive client the handler uses.
type API interface {
	CreateChannel(ctx context.Context, params *medialive.CreateChannelInput, optFns ...func(*medialive.Options)) (*medialive.CreateChannelOutput, error)
	DescribeChannel(ctx context.Context, params *medialive.DescribeChannelInput, optFns ...func(*medialive.Options)) (*medialive.DescribeChannelOutput, error)
	UpdateChannel(ctx context.Context, params *medialive.UpdateChannelInput, optFns ...func(*medialive.Options)) (*medialive.UpdateChannelOutput, error)
	UpdateChannelClass(ctx context.Context, params *medialive.UpdateChannelClassInput, optFns ...func(*medialive.Options)) (*medialive.UpdateChannelClassOutput, error)
	DeleteChannel(ctx context.Context, params *medialive.DeleteChannelInput, optFns ...func(*medialive.Options)) (*medialive.DeleteChannelOutput, error)
	ListChannels(ctx context.Context, params *medialive.ListChannelsInput, optFns ...func(*medialive.Options)) (*medialive.ListChannelsOutput, error)
	CreateTags(ctx context.Context, params *medialive.CreateTagsInput, optFns ...func(*medialive.Options)) (*medialive.CreateTagsOutput, error)
	DeleteTags(ctx context.Context, params *medialive.DeleteTagsInput, optFns ...func(*medialive.Options)) (*medialive.DeleteTagsOutput, error)
}

var inRegion = awsapi.InRegion[medialive.Options]

var _ native.Handler = (*Channel)(nil)

// usable are the states of a channel whose create or update finished. A channel is created IDLE, and someone may
// start it before UCP reads it.
var usable = []string{"IDLE", "STARTING", "RUNNING", "RECOVERING", "STOPPING"}

var (
	createStates = awsapi.States{Done: usable, Failed: []string{"CREATE_FAILED"}}
	updateStates = awsapi.States{Done: usable, Failed: []string{"UPDATE_FAILED"}}
)

// Channel serves AWS::MediaLive::Channel. AWS assigns its identifier, Id, and CreateChannel takes the record's client
// token as its idempotency token, so a create sent again returns the first channel. MediaLive updates and deletes
// only a channel that isn't running, and returns its error for one that is.
//
// DescribeChannel returns the channel's secrets, an HLS encryption key (StaticKeyValue) and the Akamai token secret
// (Salt). The schema marks them write-only, so UCP removes them from everything Read and List return.
type Channel struct {
	api API
}

// NewChannel returns the handler for MediaLive channels.
func NewChannel(api API) *Channel {
	return &Channel{api: api}
}

// TypeName returns AWS::MediaLive::Channel.
func (h *Channel) TypeName() string {
	return channelType
}

// Options returns how UCP uses the handler.
func (h *Channel) Options() native.Options {
	return native.Options{
		List:       native.ListRegion,
		Recovery:   native.RecoverByResend,
		Equivalent: equivalent,
	}
}

// equivalent treats a list AWS returns as the template's when every value the template sets in it is there, the same,
// because AWS fills in defaults inside the items of lists such as EncoderSettings/OutputGroups.
//
// AWS never returns the write-only StaticKeyValue and Salt, so output groups that set them are never equivalent. That
// is what makes UCP compare them by fingerprint: an equivalent list takes AWS's value, which would drop a changed key
// from the update. UCP's patch then holds only the secrets, which it leaves out while their fingerprints match.
func equivalent(path string, template any, aws any) bool {
	if _, ok := template.([]any); !ok {
		return false
	}
	if path == "EncoderSettings/OutputGroups" && setsSecret(template) {
		return false
	}
	return within(template, aws)
}

// secretPaths are the paths of the write-only values in an output group.
var secretPaths = [][]string{
	{"OutputGroupSettings", "HlsGroupSettings", "KeyProviderSettings", "StaticKeySettings", "StaticKeyValue"},
	{"OutputGroupSettings", "HlsGroupSettings", "HlsCdnSettings", "HlsAkamaiSettings", "Salt"},
}

// setsSecret reports whether output groups set a write-only value.
func setsSecret(groups any) bool {
	list, _ := groups.([]any)
	for _, group := range list {
		for _, path := range secretPaths {
			value := group
			for _, segment := range path {
				m, _ := value.(map[string]any)
				value = m[segment]
			}
			if value != nil {
				return true
			}
		}
	}
	return false
}

// within reports whether every value template sets is in aws, the same. An empty list that AWS lacks sets nothing,
// because a read leaves out empty lists. An empty object is a value: it selects a kind of settings, such as
// "Fmp4HlsSettings": {} for an HLS output's container, and a read keeps it (see encodeSettings), so AWS must have it.
func within(template any, aws any) bool {
	switch t := template.(type) {
	case map[string]any:
		a, ok := aws.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range t {
			av, ok := a[k]
			if list, isList := v.([]any); !ok && isList && len(list) == 0 {
				continue
			}
			if !ok || !within(v, av) {
				return false
			}
		}
		return true
	case []any:
		a, ok := aws.([]any)
		if !ok || len(a) != len(t) {
			return false
		}
		for i := range t {
			if !within(t[i], a[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(template, aws)
}

// describe returns the channel with the identifier, or ErrNotFound. AWS lists a deleted channel for a while, as
// DELETED, which counts as not found.
func (h *Channel) describe(ctx context.Context, region string, identifier string) (*medialive.DescribeChannelOutput, error) {
	out, err := h.api.DescribeChannel(ctx, &medialive.DescribeChannelInput{ChannelId: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "NotFoundException")
	}
	if out.State == types.ChannelStateDeleted {
		return nil, native.ErrNotFound
	}
	return out, nil
}

// Read returns the channel's properties, including the secrets DescribeChannel returns, which UCP removes.
func (h *Channel) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return properties(out)
}

// readOnly are the values DescribeChannel returns that the schema doesn't have, by path. They describe the running
// channel, or are set by AWS.
var readOnly = [][]string{
	{"State"}, {"EgressEndpoints"}, {"PipelineDetails"}, {"PipelinesRunningCount"},
	{"ChannelEngineVersion", "ExpirationDate"},
	{"Maintenance", "MaintenanceDeadline"}, {"Maintenance", "MaintenanceScheduledDate"},
	{"Vpc", "AvailabilityZones"}, {"Vpc", "NetworkInterfaceIds"},
	{"LinkedChannelSettings", "PrimaryChannelSettings", "FollowingChannelArns"},
}

// properties returns the properties of a channel DescribeChannel returned. The property names match the API's, so
// only values the schema doesn't have are removed, and Inputs, the IDs of the attached inputs, is added.
func properties(out *medialive.DescribeChannelOutput) (map[string]any, error) {
	p, err := awsapi.EncodeObject(out)
	if err != nil {
		return nil, err
	}
	for _, path := range readOnly {
		removePath(p, path)
	}
	for name, value := range map[string]any{"EncoderSettings": out.EncoderSettings, "Destinations": out.Destinations, "InputAttachments": out.InputAttachments} {
		encoded, err := encodeSettings(value)
		if err != nil {
			return nil, err
		}
		if encoded == nil {
			delete(p, name)
		} else {
			p[name] = encoded
		}
	}
	inputs := []any{}
	for _, a := range out.InputAttachments {
		if id := aws.ToString(a.InputId); id != "" {
			inputs = append(inputs, id)
		}
	}
	if len(inputs) > 0 {
		p["Inputs"] = inputs
	}
	tags := map[string]any{}
	for k, v := range out.Tags {
		if !strings.HasPrefix(k, "aws:") {
			tags[k] = v
		}
	}
	delete(p, "Tags")
	if len(tags) > 0 {
		p["Tags"] = tags
	}
	return p, nil
}

// encodeSettings returns v, part of a channel's settings, in the schema's format, as awsapi.Encode does but keeping
// empty objects. An empty object selects a kind of settings, such as "M3u8Settings": {} or
// "MediaPackageOutputSettings": {}, and AWS returns it. Unset values, empty strings, and empty lists are left out,
// because AWS returns them for values the template never set. It returns nil when v is unset.
func encodeSettings(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return pruneUnset(out), nil
}

// pruneUnset removes nil values, empty strings, and empty lists from v, and returns v, or nil when v is one of them.
func pruneUnset(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, value := range t {
			if pruned := pruneUnset(value); pruned == nil {
				delete(t, k)
			} else {
				t[k] = pruned
			}
		}
		return t
	case []any:
		out := []any{}
		for _, value := range t {
			if pruned := pruneUnset(value); pruned != nil {
				out = append(out, pruned)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		if t == "" {
			return nil
		}
	}
	return v
}

// removePath removes the value at path, and the objects it leaves empty.
func removePath(p map[string]any, path []string) {
	if len(path) == 1 {
		delete(p, path[0])
		return
	}
	child, ok := p[path[0]].(map[string]any)
	if !ok {
		return
	}
	removePath(child, path[1:])
	if len(child) == 0 {
		delete(p, path[0])
	}
}

// rejectDryRun returns a failure when the properties set DryRun, because a dry run only checks the settings: Radius
// needs a channel it can read.
func rejectDryRun(properties map[string]any) error {
	if dryRun, _ := properties["DryRun"].(bool); dryRun {
		return &native.Failure{
			Message:     "DryRun only checks a MediaLive channel's settings and changes nothing, so Radius can't deploy it. Remove DryRun to deploy the channel.",
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return nil
}

// Create creates the channel with the record's client token as the idempotency token, so a create sent again returns
// the first channel.
func (h *Channel) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	if err := rejectDryRun(request.Properties); err != nil {
		return "", err
	}
	input := &medialive.CreateChannelInput{}
	if err := awsapi.Decode(awsapi.Without(request.Properties, "DryRun"), input); err != nil {
		return "", err
	}
	input.RequestId = aws.String(request.ClientToken)
	out, err := h.api.CreateChannel(ctx, input, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	if out.Channel == nil || aws.ToString(out.Channel.Id) == "" {
		return "", errors.New("MediaLive created the channel but returned no channel ID")
	}
	return aws.ToString(out.Channel.Id), nil
}

// updatable are the properties UpdateChannel changes. ChannelClass changes through UpdateChannelClass, and Tags
// through their own calls.
var updatable = slices.DeleteFunc(awsapi.FieldNames(&medialive.UpdateChannelInput{}), func(name string) bool {
	return name == "ChannelId" || name == "DryRun" || name == "SpecialRouterSettings"
})

// removedDefaults are the values a property goes back to when the template removes it. AWS keeps the value of any
// other removed property, including ChannelClass: going back to STANDARD would take UpdateChannelClass, which needs
// Destinations for both pipelines, so a template that removes ChannelClass doesn't change the class.
var removedDefaults = map[string]any{"LogLevel": string(types.LogLevelDisabled)}

// Update changes the channel with UpdateChannel, or UpdateChannelClass for a new ChannelClass, then adds and removes
// tags. MediaLive changes the class in a call of its own, which takes only Destinations as well, and the channel is
// busy until it finishes, so a class change with other settings is rejected before any call.
func (h *Channel) Update(ctx context.Context, request native.UpdateRequest) error {
	if err := rejectDryRun(request.Desired); err != nil {
		return err
	}
	channel, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	changed := awsapi.Changed(request)
	values := map[string]any{}
	for _, name := range changed {
		if !slices.Contains(updatable, name) {
			continue
		}
		value, ok := request.Desired[name]
		if !ok {
			if value, ok = removedDefaults[name]; !ok {
				continue
			}
		}
		values[name] = value
	}

	calls := []func() error{}
	class := awsapi.String(request.Desired, "ChannelClass")
	switch {
	case slices.Contains(changed, "ChannelClass") && class != "":
		if others := slices.DeleteFunc(slices.Sorted(maps.Keys(values)), func(name string) bool { return name == "Destinations" }); len(others) > 0 {
			return &native.Failure{
				Message:     "MediaLive changes ChannelClass in a call of its own, which can't also change " + strings.Join(others, ", ") + ". Change ChannelClass in one deploy, and the other properties in the next.",
				ClientError: true,
				Outcome:     native.OutcomeRejected,
			}
		}
		input := &medialive.UpdateChannelClassInput{}
		if err := awsapi.Decode(map[string]any{"Destinations": request.Desired["Destinations"]}, input); err != nil {
			return err
		}
		input.ChannelId, input.ChannelClass = aws.String(request.Identifier), types.ChannelClass(class)
		calls = append(calls, func() error {
			_, err := h.api.UpdateChannelClass(ctx, input, opt)
			return err
		})
	case len(values) > 0:
		// UpdateChannel removes the channel's role when the call leaves out RoleArn, so every update sends the
		// template's role.
		if role, ok := request.Desired["RoleArn"]; ok {
			values["RoleArn"] = role
		}
		input := &medialive.UpdateChannelInput{}
		if err := awsapi.Decode(values, input); err != nil {
			return err
		}
		input.ChannelId = aws.String(request.Identifier)
		calls = append(calls, func() error {
			_, err := h.api.UpdateChannel(ctx, input, opt)
			return err
		})
	}
	if slices.Contains(changed, "Tags") {
		calls = append(calls, h.tagCalls(ctx, channel.Arn, request, opt)...)
	}
	return awsapi.Calls(calls...)
}

// tagMap returns the Tags property, an object, of properties.
func tagMap(properties map[string]any) map[string]string {
	out := map[string]string{}
	tags, _ := properties["Tags"].(map[string]any)
	for k, v := range tags {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// tagCalls returns the calls that add and remove tags to go from the request's current tags to the desired ones.
func (h *Channel) tagCalls(ctx context.Context, arn *string, request native.UpdateRequest, opt func(*medialive.Options)) []func() error {
	current, desired := tagMap(request.Current), tagMap(request.Desired)
	set := map[string]string{}
	for k, v := range desired {
		if have, ok := current[k]; !ok || have != v {
			set[k] = v
		}
	}
	remove := []string{}
	for _, k := range slices.Sorted(maps.Keys(current)) {
		if _, ok := desired[k]; !ok {
			remove = append(remove, k)
		}
	}
	calls := []func() error{}
	if len(set) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.CreateTags(ctx, &medialive.CreateTagsInput{ResourceArn: arn, Tags: set}, opt)
			return err
		})
	}
	if len(remove) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.DeleteTags(ctx, &medialive.DeleteTagsInput{ResourceArn: arn, TagKeys: remove}, opt)
			return err
		})
	}
	return calls
}

// Delete deletes the channel. A channel AWS is already deleting had a delete reach AWS before, so the delete succeeds.
func (h *Channel) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeleteChannel(ctx, &medialive.DeleteChannelInput{ChannelId: aws.String(identifier)}, inRegion(region))
	if awsapi.HasCode(err, "ConflictException") {
		channel, describeErr := h.describe(ctx, region, identifier)
		if describeErr != nil {
			return describeErr
		}
		if channel.State == types.ChannelStateDeleting {
			return nil
		}
	}
	return awsapi.NotFound(err, "NotFoundException")
}

// List returns every channel in the region. ListChannels leaves out the encoder settings, so each channel is read.
func (h *Channel) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	input := &medialive.ListChannelsInput{}
	for {
		page, err := h.api.ListChannels(ctx, input, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, summary := range page.Channels {
			id := aws.ToString(summary.Id)
			p, err := h.Read(ctx, region, id)
			if errors.Is(err, native.ErrNotFound) {
				continue
			} else if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: id, Properties: p})
		}
		if page.NextToken == nil {
			return out, nil
		}
		input.NextToken = page.NextToken
	}
}

// Progress reads the channel's state. A create or update waits for the channel to be IDLE, and a delete for it to be
// gone.
func (h *Channel) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	channel, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	state := string(channel.State)
	switch op {
	case native.OperationDelete:
		return native.Progress{State: state, Waiting: "deleted"}, nil
	case native.OperationUpdate:
		return updateStates.Progress(state), nil
	}
	return createStates.Progress(state), nil
}
