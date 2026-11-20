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

package ses

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// newTopicHandler returns a notification topic handler that doesn't wait, and its fake.
func newTopicHandler() (*IdentityNotificationTopic, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewIdentityNotificationTopic(api)
	h.wait = noWait
	return h, api
}

// headersOn reports whether the identity's notifications of kind include headers, once reads show the last change.
func headersOn(t *testing.T, h *IdentityNotificationTopic, identity string, kind string) bool {
	t.Helper()
	for range staleReads {
		_, _ = h.attributes(t.Context(), region, identity)
	}
	attributes, err := h.attributes(t.Context(), region, identity)
	require.NoError(t, err)
	_, headers := topicAndHeaders(attributes, types.NotificationType(kind))
	return headers
}

func Test_IdentityNotificationTopic_DeleteTurnsHeadersOffFirst(t *testing.T) {
	ctx := t.Context()
	h, api := newTopicHandler()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: notificationTopic("Delivery", topicA, true)})
	require.NoError(t, err)
	api.calls = nil

	require.NoError(t, h.Delete(ctx, region, identifier))
	require.Equal(t, []string{
		"GetIdentityNotificationAttributes",
		"SetIdentityHeadersInNotificationsEnabled",
		"GetIdentityNotificationAttributes", "GetIdentityNotificationAttributes", "GetIdentityNotificationAttributes",
		"SetIdentityNotificationTopic",
	}, api.calls, "headers are turned off, and a read shows it, while the topic is still set")
	require.False(t, headersOn(t, h, testIdentity, "Delivery"))
	_, err = h.Read(ctx, region, identifier)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_IdentityNotificationTopic_CreateTurnsLeftoverHeadersOff(t *testing.T) {
	ctx := t.Context()
	h, api := newTopicHandler()
	// SES keeps the headers setting when a topic is cleared outside Radius.
	api.headers[testIdentity+"|Complaint"] = true

	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: notificationTopic("Complaint", topicA, false)})
	require.NoError(t, err)
	require.False(t, headersOn(t, h, testIdentity, "Complaint"), "the template leaves headers out, so they are off")
}

func Test_IdentityNotificationTopic_DeleteWhileForwardingIsOff(t *testing.T) {
	ctx := t.Context()
	for _, kind := range []string{"Bounce", "Complaint"} {
		t.Run(kind, func(t *testing.T) {
			h, _ := newTopicHandler()
			properties := with(notificationTopic(kind, topicA, false), map[string]any{"Identity": noForwardingIdentity})
			identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties})
			require.NoError(t, err)

			err = h.Delete(ctx, region, identifier)
			requireRejected(t, err, invalidParameterCode)
			require.Contains(t, err.Error(), "turn forwarding on first")
			_, err = h.Read(ctx, region, identifier)
			require.NoError(t, err, "the topic is still set")
		})
	}

	t.Run("a delete that fails for another reason", func(t *testing.T) {
		h, api := newTopicHandler()
		properties := with(notificationTopic("Bounce", topicA, false), map[string]any{"Identity": noForwardingIdentity})
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties})
		require.NoError(t, err)
		api.errs["SetIdentityNotificationTopic"] = errors.New("connection reset")

		err = h.Delete(ctx, region, identifier)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "forwarding", "only a rejected clear is about forwarding")
	})

	t.Run("Delivery", func(t *testing.T) {
		h, _ := newTopicHandler()
		properties := with(notificationTopic("Delivery", topicA, false), map[string]any{"Identity": noForwardingIdentity})
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties})
		require.NoError(t, err)
		require.NoError(t, h.Delete(ctx, region, identifier), "forwarding is only for bounces and complaints")
	})
}

func Test_IdentityNotificationTopic_Identifier(t *testing.T) {
	h := NewIdentityNotificationTopic(nil)
	identifier, err := h.Identifier(notificationTopic("Complaint", topicA, false))
	require.NoError(t, err)
	require.Equal(t, testIdentity+"|Complaint", identifier)

	for _, properties := range []map[string]any{
		notificationTopic("Open", topicA, false),
		notificationTopic("Bounce", "radius-topic", false),
		with(notificationTopic("Bounce", topicA, false), map[string]any{"Identity": identityARN(testIdentity)}),
	} {
		_, err := h.Identifier(properties)
		requireRejected(t, err, "")
	}
}

// requirePartial requires err to be a failure AWS applied some of, and a client error when clientError is true.
func requirePartial(t *testing.T, err error, clientError bool) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomePartial, failure.Outcome, failure.Message)
	require.Equal(t, clientError, failure.ClientError, failure.Message)
}

// Test_IdentityNotificationTopic_FailureAfterAChange checks that a call that fails after an earlier call changed AWS
// is partly applied, so UCP keeps the record, and that a 4xx answer the SDK doesn't model stays a client error.
func Test_IdentityNotificationTopic_FailureAfterAChange(t *testing.T) {
	ctx := t.Context()
	t.Run("reads fail after the create set the topic", func(t *testing.T) {
		h, api := newTopicHandler()
		api.errs["GetIdentityNotificationAttributes"] = unmodeled("Throttling", "Rate exceeded")
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: notificationTopic("Bounce", topicA, true)})
		requireUnknown(t, err, "Rate exceeded")
		require.Equal(t, testIdentity+"|Bounce", identifier, "UCP keeps the record of a create that may have applied")
	})
	t.Run("clearing the topic after a delete turned headers off", func(t *testing.T) {
		h, api := newTopicHandler()
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: notificationTopic("Delivery", topicA, true)})
		require.NoError(t, err)
		api.errs["SetIdentityNotificationTopic"] = unmodeled(invalidParameterCode, "SNS topic is invalid.")
		requirePartial(t, h.Delete(ctx, region, identifier), true)
	})
	t.Run("a lost response after an update set the topic", func(t *testing.T) {
		h, api := newTopicHandler()
		current := notificationTopic("Bounce", topicA, false)
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: current})
		require.NoError(t, err)
		api.errs["SetIdentityHeadersInNotificationsEnabled"] = errors.New("connection reset")
		current[headersProperty] = false
		err = h.Update(ctx, update(t, identifier, current, notificationTopic("Bounce", topicB, true)))
		requirePartial(t, err, false)
	})
}

// update returns the update of the resource with identifier from current to desired, with the patch UCP computes.
func update(t *testing.T, identifier string, current map[string]any, desired map[string]any) native.UpdateRequest {
	t.Helper()
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	return native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: desired, Patch: patch}
}

// requireUnknown requires err to be a failure whose outcome is unknown, which says to deploy again and contains text.
func requireUnknown(t *testing.T, err error, text string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, failure.Message)
	require.False(t, failure.ClientError, failure.Message)
	require.Contains(t, failure.Message, "Deploy again")
	require.Contains(t, failure.Message, text)
}

// Test_IdentityNotificationTopic_UpdateFollowsThePatch checks that an update changes only what its patch changes: a
// headers setting turned on outside Radius stays on when the template never set it, and turns off when the patch
// removes it.
func Test_IdentityNotificationTopic_UpdateFollowsThePatch(t *testing.T) {
	ctx := t.Context()
	h, api := newTopicHandler()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: notificationTopic("Bounce", topicA, true)})
	require.NoError(t, err)
	current, err := h.Read(ctx, region, identifier)
	require.NoError(t, err)

	// A take-over: the template changes the topic and never set headers, so UCP's patch keeps AWS's value.
	desired := notificationTopic("Bounce", topicB, false)
	request := update(t, identifier, current, with(desired, map[string]any{headersProperty: true}))
	request.Desired = desired
	api.calls = nil
	require.NoError(t, h.Update(ctx, request))
	require.NotContains(t, api.calls, "SetIdentityHeadersInNotificationsEnabled")
	require.True(t, headersOn(t, h, testIdentity, "Bounce"), "headers the template never set stay on")

	// The template removes headers it set, so UCP's patch removes them.
	current, err = h.Read(ctx, region, identifier)
	require.NoError(t, err)
	request = update(t, identifier, current, desired)
	require.NoError(t, h.Update(ctx, request))
	require.False(t, headersOn(t, h, testIdentity, "Bounce"))

	// An explicit false the patch sets turns headers off too.
	_, err = h.setHeaders(ctx, region, identifier, true)
	require.NoError(t, err)
	current, err = h.Read(ctx, region, identifier)
	require.NoError(t, err)
	require.NoError(t, h.Update(ctx, update(t, identifier, current, with(desired, map[string]any{headersProperty: false}))))
	require.False(t, headersOn(t, h, testIdentity, "Bounce"))
}
