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
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/medialive"
	"github.com/aws/aws-sdk-go-v2/service/medialive/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// schemaNode is a node of a JSON schema, as much as the field check reads.
type schemaNode struct {
	Ref        string                 `json:"$ref"`
	Type       any                    `json:"type"`
	Items      *schemaNode            `json:"items"`
	Properties map[string]*schemaNode `json:"properties"`
}

// fieldGaps walks the schema node n and the Go type rt together, and returns the schema paths with no field of the
// same name, ignoring case, as awsapi.Decode and Encode match them.
func fieldGaps(definitions map[string]*schemaNode, n *schemaNode, rt reflect.Type, path string, seen map[string]bool) []string {
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if ref, ok := strings.CutPrefix(n.Ref, "#/definitions/"); ok {
		if key := ref + "|" + rt.String(); !seen[key] {
			seen[key] = true
			return fieldGaps(definitions, definitions[ref], rt, path, seen)
		}
		return nil
	}
	switch {
	case n.Items != nil && rt.Kind() == reflect.Slice:
		return fieldGaps(definitions, n.Items, rt.Elem(), path+"/*", seen)
	case len(n.Properties) > 0 && rt.Kind() == reflect.Struct:
		gaps := []string{}
		for name, child := range n.Properties {
			f, ok := rt.FieldByNameFunc(func(s string) bool { return strings.EqualFold(s, name) })
			if !ok {
				gaps = append(gaps, path+"/"+name)
				continue
			}
			gaps = append(gaps, fieldGaps(definitions, child, f.Type, path+"/"+name, seen)...)
		}
		return gaps
	}
	return nil
}

// Test_SchemaMatchesAPI checks that every property of the schema has a field of the same name in CreateChannelInput,
// so a create sends all of it, and in DescribeChannelOutput, so Read returns it, apart from the known differences.
func Test_SchemaMatchesAPI(t *testing.T) {
	var parsed struct {
		Properties  map[string]*schemaNode `json:"properties"`
		Definitions map[string]*schemaNode `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal([]byte(schema(t)), &parsed))
	root := &schemaNode{Properties: parsed.Properties}

	create := fieldGaps(parsed.Definitions, root, reflect.TypeOf(medialive.CreateChannelInput{}), "", map[string]bool{})
	slices.Sort(create)
	require.Equal(t, []string{"/Arn", "/Id", "/Inputs"}, create, "only the read-only properties are not sent")

	read := fieldGaps(parsed.Definitions, root, reflect.TypeOf(medialive.DescribeChannelOutput{}), "", map[string]bool{})
	slices.Sort(read)
	require.Equal(t, []string{"/DryRun", "/Inputs", "/Vpc/PublicAddressAllocationIds"}, read, "AWS returns every other property; the handler adds Inputs, and the others are write-only")
}

func newChannel(t *testing.T) (*Channel, *fakeAPI) {
	t.Helper()
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewChannel(api), api
}

// created creates a channel from properties and reads it until it is IDLE.
func created(t *testing.T, h *Channel, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties, ClientToken: "token-" + t.Name()})
	require.NoError(t, err)
	settle(t, h, id, native.OperationCreate)
	return id
}

func settle(t *testing.T, h *Channel, id string, op native.Operation) {
	t.Helper()
	for range 3 {
		progress, err := h.Progress(t.Context(), region, id, op)
		require.NoError(t, err)
		if progress.Done {
			require.Nil(t, progress.Failure)
			return
		}
	}
	require.Fail(t, "the operation did not finish")
}

func Test_CreateIdempotent(t *testing.T) {
	h, api := newChannel(t)
	properties := channel(t, outputGroups(t, "", ""))
	request := native.CreateRequest{Region: region, Properties: properties, ClientToken: "11111111-2222-3333-4444-555555555555"}
	first, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	again, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, first, again, "the client token returns the first channel")
	require.Len(t, api.store.List(channelType), 1)
	require.Equal(t, []string{"CreateChannel " + region, "CreateChannel " + region}, api.calls)
}

func Test_Read(t *testing.T) {
	h, api := newChannel(t)
	properties := channel(t, outputGroups(t, "", ""))
	properties["Maintenance"] = map[string]any{"MaintenanceDay": "MONDAY", "MaintenanceStartTime": "02:00"}
	id := created(t, h, properties)

	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, id, got["Id"])
	require.Equal(t, arnPrefix+id, got["Arn"])
	require.Equal(t, []any{"7654321"}, got["Inputs"])
	require.Equal(t, map[string]any{"team": "a"}, got["Tags"], "tags AWS adds are left out")
	require.Equal(t, map[string]any{"MaintenanceDay": "MONDAY", "MaintenanceStartTime": "02:00"}, got["Maintenance"])
	for _, name := range []string{"State", "EgressEndpoints", "PipelinesRunningCount", "PipelineDetails"} {
		require.NotContains(t, got, name)
	}

	api.store.Put(channelType, "bare", map[string]any{"Name": "bare"})
	got, err = h.Read(t.Context(), region, "bare")
	require.NoError(t, err)
	require.NotContains(t, got, "Inputs")
	require.NotContains(t, got, "Tags")
}

func Test_Properties(t *testing.T) {
	out := &medialive.DescribeChannelOutput{
		Id:                   aws.String("1"),
		ChannelEngineVersion: &types.ChannelEngineVersionResponse{Version: aws.String("1.0"), ExpirationDate: aws.Time(time.Now())},
		Vpc:                  &types.VpcOutputSettingsDescription{AvailabilityZones: []string{"us-west-2a"}, NetworkInterfaceIds: []string{"eni-1"}},
		LinkedChannelSettings: &types.DescribeLinkedChannelSettings{PrimaryChannelSettings: &types.DescribePrimaryChannelSettings{
			FollowingChannelArns: []string{"arn"}, LinkedChannelType: types.LinkedChannelTypePrimaryChannel,
		}},
		InputAttachments: []types.InputAttachment{{InputAttachmentName: aws.String("no-id")}},
	}
	got, err := properties(out)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Version": "1.0"}, got["ChannelEngineVersion"])
	require.NotContains(t, got, "Vpc", "an object left empty is removed")
	require.Equal(t, map[string]any{"PrimaryChannelSettings": map[string]any{"LinkedChannelType": "PRIMARY_CHANNEL"}}, got["LinkedChannelSettings"])
	require.NotContains(t, got, "Inputs")
}

func Test_DryRun(t *testing.T) {
	h, api := newChannel(t)
	properties := channel(t, outputGroups(t, "", ""))
	properties["DryRun"] = true
	failure := &native.Failure{}
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties, ClientToken: "t"})
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.ErrorAs(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "1", Desired: properties}), &failure)
	require.Empty(t, api.calls)

	properties["DryRun"] = false
	id := created(t, h, properties)
	require.NotContains(t, api.store.Resource(channelType, id), "DryRun", "DryRun is never sent")
}

func patch(t *testing.T, raw string) jsondiff.Patch {
	t.Helper()
	p := jsondiff.Patch{}
	require.NoError(t, json.Unmarshal([]byte(raw), &p))
	return p
}

func Test_UpdateChannelClass(t *testing.T) {
	h, api := newChannel(t)
	properties := channel(t, outputGroups(t, "", ""))
	id := created(t, h, properties)
	current, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)

	destinations := object(t, `[
		{"Id": "archive", "Settings": [{"Url": "s3ssl://a"}, {"Url": "s3ssl://b"}]},
		{"Id": "mp", "MediaPackageSettings": [{"ChannelId": "app-mediapackage"}]},
		{"Id": "hls", "Settings": [{"Url": "https://p-ep.akamaientrypoint.net/cmaf/live/1/out"}, {"Url": "https://b-ep.akamaientrypoint.net/cmaf/live/1/out"}]}
	]`)
	desired := with(properties, map[string]any{"ChannelClass": "STANDARD", "Destinations": destinations})
	api.calls = nil
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: id, Current: current, Desired: desired,
		Patch: patch(t, `[{"op":"replace","path":"/ChannelClass","value":"STANDARD"},{"op":"replace","path":"/Destinations/0/Settings","value":[]},{"op":"replace","path":"/Destinations/2/Settings","value":[]}]`),
	}))
	require.Equal(t, []string{"DescribeChannel " + region, "UpdateChannelClass " + region}, api.calls)
	settle(t, h, id, native.OperationUpdate)
	stored := api.store.Resource(channelType, id)
	require.Equal(t, "STANDARD", stored["ChannelClass"])
	require.Len(t, stored["Destinations"].([]any)[0].(map[string]any)["Settings"], 2)

	failure := &native.Failure{}
	err = h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: id, Current: current, Desired: with(desired, map[string]any{"ChannelClass": "SINGLE_PIPELINE", "LogLevel": "DEBUG"}),
		Patch: patch(t, `[{"op":"replace","path":"/ChannelClass","value":"SINGLE_PIPELINE"},{"op":"replace","path":"/LogLevel","value":"DEBUG"}]`),
	})
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "LogLevel")
	require.Equal(t, "STANDARD", api.store.Resource(channelType, id)["ChannelClass"], "nothing changed")
}

// Test_UpdateKeepsRole checks that an update that doesn't change RoleArn sends it, because UpdateChannel removes the
// role of a call that leaves it out.
func Test_UpdateKeepsRole(t *testing.T) {
	h, api := newChannel(t)
	properties := channel(t, outputGroups(t, "", ""))
	id := created(t, h, properties)
	current, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)

	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: id, Current: current, Desired: with(properties, map[string]any{"LogLevel": "DEBUG"}),
		Patch: patch(t, `[{"op":"replace","path":"/LogLevel","value":"DEBUG"}]`),
	}))
	settle(t, h, id, native.OperationUpdate)
	require.Equal(t, properties["RoleArn"], api.store.Resource(channelType, id)["RoleArn"])
}

func Test_UpdateRemoved(t *testing.T) {
	h, api := newChannel(t)
	properties := channel(t, outputGroups(t, "", ""))
	id := created(t, h, properties)
	current, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)

	desired := with(properties, map[string]any{"LogLevel": nil, "Name": nil, "ChannelClass": nil})
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: id, Current: current, Desired: desired,
		Patch: patch(t, `[{"op":"remove","path":"/LogLevel"},{"op":"remove","path":"/Name"},{"op":"remove","path":"/ChannelClass"}]`),
	}))
	settle(t, h, id, native.OperationUpdate)
	stored := api.store.Resource(channelType, id)
	require.Equal(t, "DISABLED", stored["LogLevel"], "a removed LogLevel goes back to AWS's default")
	require.Equal(t, "app-channel", stored["Name"], "AWS keeps a removed Name")
	require.Equal(t, "SINGLE_PIPELINE", stored["ChannelClass"])
}

func Test_RunningChannel(t *testing.T) {
	h, api := newChannel(t)
	properties := channel(t, outputGroups(t, "", ""))
	id := created(t, h, properties)
	current, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	api.state[id] = types.ChannelStateRunning

	err = h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: id, Current: current, Desired: with(properties, map[string]any{"LogLevel": "DEBUG"}),
		Patch: patch(t, `[{"op":"replace","path":"/LogLevel","value":"DEBUG"}]`),
	})
	require.ErrorContains(t, err, "ConflictException", "AWS's error is returned as it is")
	err = h.Delete(t.Context(), region, id)
	require.ErrorContains(t, err, "ConflictException")

	progress, err := h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done, "a channel someone started after the create is done")
}

func Test_Delete(t *testing.T) {
	h, api := newChannel(t)
	id := created(t, h, channel(t, outputGroups(t, "", "")))

	require.NoError(t, h.Delete(t.Context(), region, id))
	progress, err := h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "DELETING", Waiting: "deleted"}, progress)
	api.state[id] = types.ChannelStateDeleting
	require.NoError(t, h.Delete(t.Context(), region, id), "a channel AWS is already deleting")
	_, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound, "a DELETED channel AWS still lists is gone")
	require.ErrorIs(t, h.Delete(t.Context(), region, id), native.ErrNotFound)
	_, err = h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, native.ErrNotFound)

	api.store.Put(channelType, "gone", map[string]any{})
	api.state["gone"] = types.ChannelStateDeleted
	api.errs["DeleteChannel"] = conflict(types.ChannelStateDeleted)
	require.ErrorIs(t, h.Delete(t.Context(), region, "gone"), native.ErrNotFound)
}

func Test_Progress(t *testing.T) {
	tests := []struct {
		op     native.Operation
		state  string
		done   bool
		failed bool
	}{
		{native.OperationCreate, "CREATING", false, false},
		{native.OperationCreate, "IDLE", true, false},
		{native.OperationCreate, "CREATE_FAILED", true, true},
		{native.OperationUpdate, "UPDATING", false, false},
		{native.OperationUpdate, "IDLE", true, false},
		{native.OperationUpdate, "RUNNING", true, false},
		{native.OperationUpdate, "UPDATE_FAILED", true, true},
	}
	for _, tt := range tests {
		h, api := newChannel(t)
		api.store.Put(channelType, "1", map[string]any{})
		api.state["1"] = types.ChannelState(tt.state)
		progress, err := h.Progress(t.Context(), region, "1", tt.op)
		require.NoError(t, err)
		require.Equal(t, tt.done, progress.Done, "%s %s", tt.op, tt.state)
		require.Equal(t, tt.failed, progress.Failure != nil, "%s %s", tt.op, tt.state)
		if !tt.done {
			require.Equal(t, "IDLE", progress.Waiting)
		}
	}
}

func Test_List(t *testing.T) {
	h, api := newChannel(t)
	api.pageSize = 1
	for _, name := range []string{"a", "b"} {
		api.store.Put(channelType, name, map[string]any{"Name": name})
	}
	api.store.Put(channelType, "deleted", map[string]any{"Name": "deleted"})
	api.state["deleted"] = types.ChannelStateDeleted

	list, err := h.List(t.Context(), region)
	require.NoError(t, err)
	ids := []string{}
	for _, r := range list {
		ids = append(ids, r.Identifier)
		require.Equal(t, r.Identifier, r.Properties["Name"])
	}
	require.Equal(t, []string{"a", "b"}, ids, "a DELETED channel is left out")
}

func Test_Errors(t *testing.T) {
	boom := errors.New("boom")
	for _, method := range []string{"CreateChannel", "DescribeChannel", "ListChannels"} {
		h, api := newChannel(t)
		api.store.Put(channelType, "1", map[string]any{})
		api.errs[method] = boom
		var err error
		switch method {
		case "CreateChannel":
			_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"Name": "x"}, ClientToken: "t"})
		case "DescribeChannel":
			_, err = h.Read(t.Context(), region, "1")
			require.ErrorIs(t, err, boom)
			_, err = h.Progress(t.Context(), region, "1", native.OperationCreate)
			require.ErrorIs(t, err, boom)
			require.ErrorIs(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "1"}), boom)
			_, err = h.List(t.Context(), region)
		case "ListChannels":
			_, err = h.List(t.Context(), region)
		}
		require.ErrorIs(t, err, boom, method)
	}

	h, api := newChannel(t)
	api.errs["DeleteChannel"] = conflict(types.ChannelStateIdle)
	require.ErrorIs(t, h.Delete(t.Context(), region, "missing"), native.ErrNotFound, "the describe after a conflict finds no channel")

	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"LogLevel": 5}, ClientToken: "t"})
	require.Error(t, err)

	h = NewChannel(noChannel{api})
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"Name": "x"}, ClientToken: "t"})
	require.ErrorContains(t, err, "no channel ID")

	h, api = newChannel(t)
	api.store.Put(channelType, "1", map[string]any{})
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "1", Desired: map[string]any{"LogLevel": 5}, Patch: patch(t, `[{"op":"add","path":"/LogLevel","value":5}]`)})
	require.Error(t, err)
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "1", Desired: map[string]any{"ChannelClass": "STANDARD", "Destinations": "x"}, Patch: patch(t, `[{"op":"add","path":"/ChannelClass","value":"STANDARD"}]`)})
	require.Error(t, err)
}

// noChannel is a MediaLive client whose CreateChannel returns no channel.
type noChannel struct {
	*fakeAPI
}

func (noChannel) CreateChannel(ctx context.Context, in *medialive.CreateChannelInput, optFns ...func(*medialive.Options)) (*medialive.CreateChannelOutput, error) {
	return &medialive.CreateChannelOutput{}, nil
}

func Test_Equivalent(t *testing.T) {
	groups := []any{map[string]any{"Name": "a", "Settings": map[string]any{"Mode": "X"}}}
	tests := []struct {
		name     string
		template any
		aws      any
		want     bool
	}{
		{"defaults AWS adds", groups, []any{map[string]any{"Name": "a", "Settings": map[string]any{"Mode": "X", "Rollover": 300.0}}}, true},
		{"a changed value", groups, []any{map[string]any{"Name": "a", "Settings": map[string]any{"Mode": "Y"}}}, false},
		{"a value AWS lacks", groups, []any{map[string]any{"Name": "a"}}, false},
		{"another type", groups, []any{map[string]any{"Name": "a", "Settings": "X"}}, false},
		{"another length", groups, []any{}, false},
		{"not a list", groups, "a", false},
		{"nested lists", []any{[]any{"a"}}, []any{[]any{"b"}}, false},
		{"a value, not a list", "a", "a", false},
		{"an empty object AWS returns", []any{map[string]any{"M3u8Settings": map[string]any{}}}, []any{map[string]any{"M3u8Settings": map[string]any{}}}, true},
		{"an empty list AWS lacks", []any{map[string]any{"Name": "a", "Names": []any{}}}, []any{map[string]any{"Name": "a"}}, true},
		{"an empty selector AWS lacks", []any{map[string]any{"Name": "a", "M3u8Settings": map[string]any{}}}, []any{map[string]any{"Name": "a"}}, false},
		{"another container than an empty selector", []any{map[string]any{"HlsSettings": map[string]any{"Fmp4HlsSettings": map[string]any{}}}}, []any{map[string]any{"HlsSettings": map[string]any{"StandardHlsSettings": map[string]any{"M3u8Settings": map[string]any{}}}}}, false},
		{"a string AWS lacks", []any{map[string]any{"Name": "a", "Mode": "X"}}, []any{map[string]any{"Name": "a"}}, false},
		{"an object AWS lacks", []any{map[string]any{"M3u8Settings": map[string]any{"PcrPid": "1"}}}, []any{map[string]any{}}, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, equivalent("EncoderSettings/OutputGroups", tt.template, tt.aws), tt.name)
	}

	// Output groups that set a secret are never equivalent, so UCP's patch holds the secret, which its fingerprint
	// decides whether to send.
	for _, saltOnly := range []bool{false, true} {
		groups := outputGroups(t, staticKey, akamaiSalt)
		hls := groups.([]any)[2].(map[string]any)["OutputGroupSettings"].(map[string]any)["HlsGroupSettings"].(map[string]any)
		if saltOnly {
			delete(hls, "KeyProviderSettings")
		}
		require.False(t, equivalent("EncoderSettings/OutputGroups", groups, outputGroups(t, "", "")))
	}
	require.True(t, equivalent("EncoderSettings/OutputGroups", outputGroups(t, "", ""), outputGroups(t, "", "")))
}

func Test_EncodeSettings(t *testing.T) {
	got, err := encodeSettings(&types.EncoderSettings{
		OutputGroups: []types.OutputGroup{{
			Name:    aws.String(""),
			Outputs: []types.Output{{OutputSettings: &types.OutputSettings{MediaPackageOutputSettings: &types.MediaPackageOutputSettings{}}, AudioDescriptionNames: []string{}}},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"OutputGroups": []any{map[string]any{
		"Outputs": []any{map[string]any{"OutputSettings": map[string]any{"MediaPackageOutputSettings": map[string]any{}}}},
	}}}, got, "empty objects stay; unset values, empty strings, and empty lists don't")

	got, err = encodeSettings((*types.EncoderSettings)(nil))
	require.NoError(t, err)
	require.Nil(t, got)
	_, err = encodeSettings(func() {})
	require.Error(t, err)
}
