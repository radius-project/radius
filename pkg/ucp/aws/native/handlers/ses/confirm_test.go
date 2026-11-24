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
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// Test_ChangeNoReadShows checks that each change a read never shows fails with an unknown outcome that says to deploy
// again, rather than reporting success, both when reads keep returning the earlier value and when they fail. A create
// keeps its identifier, so UCP keeps the record.
func Test_ChangeNoReadShows(t *testing.T) {
	ctx := t.Context()
	reads := map[string]func(api *fakeAPI){
		"reads return the earlier value": func(api *fakeAPI) { api.staleReads = confirmReads + 1 },
		"reads fail": func(api *fakeAPI) {
			api.errs["GetEmailIdentityPolicies"] = v2Error(http.StatusTooManyRequests, "TooManyRequestsException", "Too many requests")
			api.errs["GetDedicatedIp"] = v2Error(http.StatusTooManyRequests, "TooManyRequestsException", "Too many requests")
			api.errs["GetIdentityNotificationAttributes"] = unmodeled("Throttling", "Too many requests")
		},
	}
	changes := map[string]func(t *testing.T, ctx context.Context, store *awsfake.CloudControl, broken func(*fakeAPI)) (string, error){
		"policy create": func(t *testing.T, ctx context.Context, store *awsfake.CloudControl, broken func(*fakeAPI)) (string, error) {
			api := newFakeAPI(store)
			broken(api)
			h := NewEmailIdentityPolicy(api)
			h.wait = noWait
			return h.Create(ctx, native.CreateRequest{Region: region, Properties: identityPolicy("p1", testPolicy(testIdentity))})
		},
		"policy update": func(t *testing.T, ctx context.Context, store *awsfake.CloudControl, broken func(*fakeAPI)) (string, error) {
			current := identityPolicy("p1", testPolicy(testIdentity))
			require.NoError(t, createPolicy(ctx, store, testIdentity, "p1", current["Policy"].(string)))
			api := newFakeAPI(store)
			broken(api)
			h := NewEmailIdentityPolicy(api)
			h.wait = noWait
			desired := identityPolicy("p1", testPolicy(testIdentity, "ses:SendEmail", "ses:SendRawEmail"))
			return "", h.Update(ctx, update(t, testIdentity+"|p1", current, desired))
		},
		"topic create": func(t *testing.T, ctx context.Context, store *awsfake.CloudControl, broken func(*fakeAPI)) (string, error) {
			api := newFakeAPI(store)
			broken(api)
			h := NewIdentityNotificationTopic(api)
			h.wait = noWait
			return h.Create(ctx, native.CreateRequest{Region: region, Properties: notificationTopic("Bounce", topicA, false)})
		},
		"headers update": func(t *testing.T, ctx context.Context, store *awsfake.CloudControl, broken func(*fakeAPI)) (string, error) {
			current := notificationTopic("Bounce", topicA, false)
			require.NoError(t, setTopic(ctx, store, testIdentity, "Bounce", topicA))
			current[headersProperty] = false
			api := newFakeAPI(store)
			broken(api)
			h := NewIdentityNotificationTopic(api)
			h.wait = noWait
			return "", h.Update(ctx, update(t, testIdentity+"|Bounce", current, notificationTopic("Bounce", topicA, true)))
		},
		"dedicated IP update": func(t *testing.T, ctx context.Context, store *awsfake.CloudControl, broken func(*fakeAPI)) (string, error) {
			api := newFakeAPI(store)
			broken(api)
			h := NewDedicatedIPAssignment(api)
			h.wait = noWait
			current := map[string]any{"Ip": testIP, poolProperty: defaultPool}
			return "", h.Update(ctx, update(t, testIP, current, map[string]any{"Ip": testIP, poolProperty: "pool-a"}))
		},
	}
	for readName, broken := range reads {
		for changeName, change := range changes {
			t.Run(changeName+", "+readName, func(t *testing.T) {
				identifier, err := change(t, ctx, awsfake.NewCloudControl(), broken)
				requireUnknown(t, err, "no read showed it")
				if changeName == "policy create" {
					require.Equal(t, testIdentity+"|p1", identifier)
				}
				if changeName == "topic create" {
					require.Equal(t, testIdentity+"|Bounce", identifier)
				}
			})
		}
	}
}

// Test_EmailIdentityPolicy_CreateAlreadyExists checks that a create SES answers with AlreadyExistsException, such as an
// SDK retry of a create whose first response was lost, keeps the identifier with an unknown outcome, so UCP keeps the
// record and the next deploy reads the policy.
func Test_EmailIdentityPolicy_CreateAlreadyExists(t *testing.T) {
	ctx := t.Context()
	store := awsfake.NewCloudControl()
	require.NoError(t, createPolicy(ctx, store, testIdentity, "p1", testPolicy(testIdentity)))
	h := NewEmailIdentityPolicy(newFakeAPI(store))
	h.wait = noWait

	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: identityPolicy("p1", testPolicy(testIdentity))})
	requireUnknown(t, err, testIdentity+"|p1")
	require.Equal(t, testIdentity+"|p1", identifier)
}

// Test_ThrottledRead checks that a read that fails for a reason other than a missing resource, such as throttling,
// returns an error that is not a *native.Failure, from Read and from each Progress that reads. The worker would take a
// failure as the end of an operation AWS may have applied.
func Test_ThrottledRead(t *testing.T) {
	ctx := t.Context()
	v2Throttled := v2Error(http.StatusTooManyRequests, "TooManyRequestsException", "Too many requests")
	v1Throttled := unmodeled("Throttling", "Rate exceeded")
	policyAPI, topicAPI, ipAPI := newFakeAPI(awsfake.NewCloudControl()), newFakeAPI(awsfake.NewCloudControl()), newFakeAPI(awsfake.NewCloudControl())
	policyAPI.errs["GetEmailIdentityPolicies"] = v2Throttled
	ipAPI.errs["GetDedicatedIp"] = v2Throttled
	topicAPI.errs["GetIdentityNotificationAttributes"] = v1Throttled
	cases := []struct {
		handler    native.Handler
		identifier string
		throttled  error
	}{
		{NewEmailIdentityPolicy(policyAPI), testIdentity + "|p1", v2Throttled},
		{NewIdentityNotificationTopic(topicAPI), testIdentity + "|Bounce", v1Throttled},
		{NewDedicatedIPAssignment(ipAPI), testIP, v2Throttled},
	}
	for _, c := range cases {
		t.Run(c.handler.TypeName(), func(t *testing.T) {
			requireNotFailure := func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, c.throttled)
				var failure *native.Failure
				require.False(t, errors.As(err, &failure), "a throttled read is not a failure: %v", err)
			}
			_, err := c.handler.Read(ctx, region, c.identifier)
			requireNotFailure(t, err)
			if c.handler.TypeName() == dedicatedIPType {
				return // Its Progress doesn't read.
			}
			for _, op := range []native.Operation{native.OperationCreate, native.OperationDelete} {
				_, err := c.handler.Progress(ctx, region, c.identifier, op)
				requireNotFailure(t, err)
			}
		})
	}
}

// Test_CreateKeepsIdentifier checks that each create returns its identifier with any error after it sent its first
// call, such as a lost response, so the caller can find and delete what the create may have made.
func Test_CreateKeepsIdentifier(t *testing.T) {
	ctx := t.Context()
	lost := errors.New("connection reset")
	cases := []struct {
		name       string
		call       string
		handler    func(api *fakeAPI) native.Handler
		properties map[string]any
		identifier string
	}{
		{"policy", "CreateEmailIdentityPolicy", func(api *fakeAPI) native.Handler { return NewEmailIdentityPolicy(api) }, identityPolicy("p1", testPolicy(testIdentity)), testIdentity + "|p1"},
		{"notification topic", "SetIdentityNotificationTopic", func(api *fakeAPI) native.Handler { return NewIdentityNotificationTopic(api) }, notificationTopic("Bounce", topicA, false), testIdentity + "|Bounce"},
		{"dedicated IP", "PutDedicatedIpInPool", func(api *fakeAPI) native.Handler { return NewDedicatedIPAssignment(api) }, map[string]any{"Ip": testIP, poolProperty: "pool-a"}, testIP},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			api.errs[c.call] = lost
			identifier, err := c.handler(api).Create(ctx, native.CreateRequest{Region: region, Properties: c.properties})
			require.ErrorIs(t, err, lost)
			require.Equal(t, c.identifier, identifier)
		})
	}
}

// Test_Confirm_Canceled checks that a confirmation never reads AWS once its context is canceled, before the first
// read or after any read, and reports the cancellation with an unknown outcome.
func Test_Confirm_Canceled(t *testing.T) {
	for _, cancelAfter := range []int{0, 1, 3} {
		ctx, cancel := context.WithCancel(t.Context())
		if cancelAfter == 0 {
			cancel()
		}
		reads := 0
		err := confirm(ctx, sleep, func(context.Context, string, string) (map[string]any, error) {
			reads++
			if reads == cancelAfter {
				cancel()
			}
			return map[string]any{}, nil
		}, region, testIP, "the pool of the dedicated IP address", func(map[string]any) bool { return false })
		cancel()
		require.Equal(t, cancelAfter, reads, "no read after the context is canceled")
		requireUnknown(t, err, context.Canceled.Error())
	}
}
