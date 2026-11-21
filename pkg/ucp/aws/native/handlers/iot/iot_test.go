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

package iot

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/iot"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func membership(group any, thing any) map[string]any {
	return map[string]any{"ThingGroupName": group, "ThingName": thing}
}

func newHandler() (*ThingGroupMembership, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewThingGroupMembership(api), api
}

func create(t *testing.T, h *ThingGroupMembership, group string, thing string) string {
	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: membership(group, thing)})
	require.NoError(t, err)
	return identifier
}

func Test_Suite_ThingGroupMembership(t *testing.T) {
	properties := membership("fleet-east", "sensor-1")
	nativetest.Run(t, nativetest.Suite{
		TypeName:    membershipType,
		Schema:      readFile(t, "aws-iot-thinggroupmembership.json"),
		Metadata:    readFile(t, "aws-iot-thinggroupmembership.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewThingGroupMembership(newFakeAPI(store))
		},
		Properties: properties,
		Identifier: "fleet-east|sensor-1",
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: membership("labs", "sensor-1"),
		Invalid: []map[string]any{
			membership("fleet|east", "sensor-1"),
			membership("fleet-east", "sensor|1"),
			membership("fleet-east", 7),
			membership([]any{"fleet-east"}, "sensor-1"),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseID(identifier)
			require.NoError(t, err)
			return membership(id.group, id.thing)
		},
		API: membershipAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := iot.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Identifier(t *testing.T) {
	h, _ := newHandler()
	id, err := h.Identifier(membership("fleet-east", "sensor-1"))
	require.NoError(t, err)
	require.Equal(t, "fleet-east|sensor-1", id)

	for _, properties := range []map[string]any{{"ThingGroupName": "fleet-east"}, {"ThingName": "sensor-1"}, membership("", "sensor-1"), membership(nil, "sensor-1")} {
		id, err := h.Identifier(properties)
		require.NoError(t, err)
		require.Empty(t, id, "a membership without both names has no identifier")
	}

	var failure *native.Failure
	_, err = h.Identifier(membership("fleet-east", 1))
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "ThingName of an AWS::IoT::ThingGroupMembership must be a string", failure.Message)
	_, err = h.Identifier(membership("a|b", "sensor-1"))
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Contains(t, failure.Message, "ThingGroupName")
}

func Test_BadIdentifier(t *testing.T) {
	h, api := newHandler()
	for _, identifier := range []string{"fleet-east", "|sensor-1", "fleet-east|", "a|b|c"} {
		var failure *native.Failure
		_, err := h.Read(t.Context(), region, identifier)
		require.ErrorAs(t, err, &failure, identifier)
		require.True(t, failure.ClientError)
		require.ErrorAs(t, h.Delete(t.Context(), region, identifier), &failure)
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		require.ErrorAs(t, err, &failure)
	}
	require.Zero(t, api.removes)
}

func Test_Create(t *testing.T) {
	h, api := newHandler()
	var failure *native.Failure
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ThingName": "sensor-1"}})
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "an AWS::IoT::ThingGroupMembership needs ThingGroupName and ThingName", failure.Message)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: membership("fleet-east", 3)})
	require.ErrorAs(t, err, &failure)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: membership("online", "sensor-1")})
	require.ErrorContains(t, err, invalidRequest, "AWS's error is returned, such as for a dynamic group")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: membership("fleet-east", "missing")})
	require.ErrorContains(t, err, notFound)
	require.Zero(t, api.store.Count(membershipType), "nothing is created")

	require.Equal(t, "fleet-east|sensor-1", create(t, h, "fleet-east", "sensor-1"))
	properties, err := h.Read(t.Context(), region, "fleet-east|sensor-1")
	require.NoError(t, err)
	require.Equal(t, membership("fleet-east", "sensor-1"), properties)
}

func Test_ReadPages(t *testing.T) {
	h, api := newHandler()
	for _, group := range []string{"fleet-east", "labs", "depot"} {
		create(t, h, group, "sensor-1")
	}
	_, err := h.Read(t.Context(), region, "depot|sensor-1")
	require.NoError(t, err, "the third group is on the second page")

	_, err = h.Read(t.Context(), region, "fleet-west|sensor-1")
	require.ErrorIs(t, err, native.ErrNotFound, "the thing isn't in the group")
	_, err = h.Read(t.Context(), region, "fleet-east|missing")
	require.ErrorIs(t, err, native.ErrNotFound, "a thing that doesn't exist is in no group")

	api.errs["ListThingGroupsForThing"] = fault("ThrottlingException")
	_, err = h.Read(t.Context(), region, "fleet-east|sensor-1")
	require.ErrorContains(t, err, "ThrottlingException")
	require.NotErrorIs(t, err, native.ErrNotFound)

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources)
}

func Test_Delete(t *testing.T) {
	h, api := newHandler()
	require.ErrorIs(t, h.Delete(t.Context(), region, "fleet-east|sensor-1"), native.ErrNotFound)
	require.Zero(t, api.removes, "nothing is removed for a thing that isn't in the group")

	create(t, h, "fleet-east", "sensor-1")
	create(t, h, "labs", "sensor-1")
	require.NoError(t, h.Delete(t.Context(), region, "fleet-east|sensor-1"))
	require.Equal(t, 1, api.removes)
	_, err := h.Read(t.Context(), region, "labs|sensor-1")
	require.NoError(t, err, "the thing stays in its other groups")

	api.errs["RemoveThingFromThingGroup"] = fault(notFound)
	require.ErrorIs(t, h.Delete(t.Context(), region, "labs|sensor-1"), native.ErrNotFound, "a group deleted since the read is not found")
	api.errs["RemoveThingFromThingGroup"] = fault("ThrottlingException")
	err = h.Delete(t.Context(), region, "labs|sensor-1")
	require.ErrorContains(t, err, "ThrottlingException")
	require.NotErrorIs(t, err, native.ErrNotFound)
}

func Test_Update(t *testing.T) {
	h, _ := newHandler()
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region}))
	progress, err := h.Progress(t.Context(), region, "anything", native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

// Test_Progress checks that a create is done once AWS lists the thing in the group, and that a delete waits until
// it doesn't.
func Test_Progress(t *testing.T) {
	h, api := newHandler()
	_, err := h.Progress(t.Context(), region, "fleet-east|sensor-1", native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "a create that never reached AWS is not found, so the next deploy sends it again")

	identifier := create(t, h, "fleet-east", "sensor-1")
	progress, err := h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	progress, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "in the group", Waiting: "removed"}, progress)

	require.NoError(t, h.Delete(t.Context(), region, identifier))
	_, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound, "the delete is done once AWS no longer lists the thing in the group")

	api.store.Remove(thingType, "sensor-2")
	_, err = h.Progress(t.Context(), region, "fleet-east|sensor-2", native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound, "a thing deleted outside Radius is in no group")
}
