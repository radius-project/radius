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

package awsproxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// cleanupCall is a call to stateful's Cleanup: the record as UCP had saved it then, and the create token it got.
type cleanupCall struct {
	saved       datamodel.AWSResourceProperties
	createToken string
}

// stateful is nativeType's fake handler as a native.UpdatePreparer and a native.Cleaner, like FSx. It logs the states
// it makes and gets, and each cleanup.
type stateful struct {
	*nativefake.Handler
	h        *nativeHarness
	states   *int
	stateErr *error
	progress *[]string
	cleanups *[]cleanupCall
	cleanErr *error
}

func (s stateful) UpdateState(ctx context.Context, region string, identifier string) (string, error) {
	if *s.stateErr != nil {
		return "", *s.stateErr
	}
	*s.states++
	return fmt.Sprintf("state-%d", *s.states), nil
}

func (s stateful) UpdateProgress(ctx context.Context, region string, identifier string, state string) (native.Progress, error) {
	*s.progress = append(*s.progress, state)
	return s.Progress(ctx, region, identifier, native.OperationUpdate)
}

func (s stateful) Cleanup(ctx context.Context, region string, identifier string, createToken string) error {
	*s.cleanups = append(*s.cleanups, cleanupCall{saved: s.h.record(nativePath).Properties, createToken: createToken})
	return *s.cleanErr
}

// withStateful registers nativeType's handler as a stateful handler.
func (h *nativeHarness) withStateful() *stateful {
	h.t.Helper()
	s := &stateful{Handler: h.fake, h: h, states: new(int), stateErr: new(error), progress: &[]string{}, cleanups: &[]cleanupCall{}, cleanErr: new(error)}
	catalog := schemas.Catalog{Native: map[string]string{nativeType: nativeSchemas[nativeType]}}
	registry, err := native.NewRegistry(catalog, *s)
	require.NoError(h.t, err)
	h.native = registry
	return s
}

// lastUpdate returns the last Update call the fake got.
func lastUpdate(t *testing.T, calls []nativefake.Call) nativefake.Call {
	t.Helper()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == "Update" {
			return calls[i]
		}
	}
	require.Fail(t, "no Update call")
	return nativefake.Call{}
}

// Test_Native_UpdateToken checks the token UCP gives a native update: a new one for each update, saved before the
// call and used as the operation ID, and the same one when UCP sends an update again that may have reached AWS.
func Test_Native_UpdateToken(t *testing.T) {
	t.Run("each update gets a new token, which is its operation ID", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Calls()

		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
		first := lastUpdate(t, h.fake.Calls()).Token
		require.NotEmpty(t, first)
		rec := h.record(nativePath)
		require.Equal(t, first, rec.Properties.UpdateToken, "saved before the call")
		require.Equal(t, first, rec.Properties.RequestToken, "the operation ID")
		for _, result := range h.finish() {
			require.Nil(t, result.Error)
		}
		require.Empty(t, h.record(nativePath).Properties.UpdateToken, "a finished update clears it")

		h.deploy(nativePath, nativeProperties(3))
		require.NotEqual(t, first, lastUpdate(t, h.fake.Calls()).Token)
	})

	t.Run("an update whose response is lost is sent again with its token", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.LostResponses["Update"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(2)).status)
		first := lastUpdate(t, h.fake.Calls()).Token
		require.Equal(t, first, h.record(nativePath).Properties.UpdateToken, "AWS may have the update")

		h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Size": 1})
		h.deploy(nativePath, nativeProperties(2))
		require.Equal(t, first, lastUpdate(t, h.fake.Calls()).Token)
	})

	t.Run("an update UCP reserved but never tracked is sent again with its token", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.LostResponses["Update"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(2)).status)
		first := lastUpdate(t, h.fake.Calls()).Token
		// As if UCP stopped after the update call and before it tracked the update: the reservation stays.
		h.saveRecord(nativePath, func(rec *datamodel.AWSResource) {
			rec.Properties.PendingLastApplied = map[string]any{"Name": "app-native", "Size": 2}
			rec.Properties.PendingOperation = records.OperationUpdate
			rec.SetProvisioningState(v1.ProvisioningStateUpdating)
		})
		records.Now = func() time.Time { return time.Now().Add(records.StaleAfter + time.Minute) }
		t.Cleanup(func() { records.Now = time.Now })
		h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Size": 1})
		h.fake.Calls()

		resp := h.put(nativePath, nativeProperties(2))
		records.Now = time.Now
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		require.Equal(t, first, lastUpdate(t, h.fake.Calls()).Token)
	})

	t.Run("a different update after a lost response gets a new token", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.LostResponses["Update"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(2)).status)
		first := lastUpdate(t, h.fake.Calls()).Token

		h.fake.LostResponses["Update"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(3)).status)
		second := lastUpdate(t, h.fake.Calls()).Token
		require.NotEqual(t, first, second, "another template is another update")

		h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Size": 3})
		h.deploy(nativePath, nativeProperties(2))
		third := lastUpdate(t, h.fake.Calls()).Token
		require.NotEqual(t, first, third, "the first template again is a new update, so AWS can't answer it with the first one's result")
		require.NotEqual(t, second, third)
	})

	t.Run("a changed write-only value is a different update", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.LostResponses["Update"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(2)).status)
		first := lastUpdate(t, h.fake.Calls()).Token

		changed := nativeProperties(2)
		changed["Password"] = "other-password"
		h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Size": 1})
		h.deploy(nativePath, changed)
		require.NotEqual(t, first, lastUpdate(t, h.fake.Calls()).Token)
	})

	t.Run("a Cloud Control update clears a native update's token", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.LostResponses["Update"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(2)).status)
		first := lastUpdate(t, h.fake.Calls()).Token

		h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
		h.deploy(nativePath, nativeProperties(3))
		require.Empty(t, h.record(nativePath).Properties.UpdateToken)

		h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeNonProvisionable
		h.deploy(nativePath, nativeProperties(2))
		require.NotEqual(t, first, lastUpdate(t, h.fake.Calls()).Token, "the native update after Cloud Control's is a new one")
	})

	t.Run("a rejected update gets a new token next time", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Errors["Update"] = nativefake.ClientError("ValidationException", "the value is invalid")
		require.Equal(t, http.StatusBadRequest, h.put(nativePath, nativeProperties(2)).status)
		first := lastUpdate(t, h.fake.Calls()).Token
		require.Empty(t, h.record(nativePath).Properties.UpdateToken)

		delete(h.fake.Errors, "Update")
		h.deploy(nativePath, nativeProperties(2))
		require.NotEqual(t, first, lastUpdate(t, h.fake.Calls()).Token)
	})

	t.Run("a deploy with nothing to change clears a kept token", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.saveRecord(nativePath, func(rec *datamodel.AWSResource) { rec.Properties.UpdateToken = "kept" })
		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status)
		require.Empty(t, h.record(nativePath).Properties.UpdateToken)
	})

	t.Run("another operation clears a kept token", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.saveRecord(nativePath, func(rec *datamodel.AWSResource) { rec.Properties.UpdateToken = "kept" })
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		require.Empty(t, h.record(nativePath).Properties.UpdateToken)
	})
}

// Test_Native_UpdateState checks the state UCP keeps for a handler's update: made once per update, before the first
// call, passed to the update and to every progress read, kept when UCP sends the update again, and cleared when the
// update finishes.
func Test_Native_UpdateState(t *testing.T) {
	h := newNativeHarness(t)
	s := h.withStateful()
	h.deploy(nativePath, nativeProperties(1))
	require.Zero(t, *s.states, "a create makes no update state")

	h.fake.LostResponses["Update"] = nativefake.ServerError("InternalFailure", "the response was lost")
	require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(2)).status)
	require.Equal(t, "state-1", lastUpdate(t, h.fake.Calls()).State)
	require.Equal(t, "state-1", h.record(nativePath).Properties.UpdateState)

	h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Size": 1})
	h.fake.Waits = 1
	require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
	require.Equal(t, "state-1", lastUpdate(t, h.fake.Calls()).State, "the update sent again keeps its state")
	require.Equal(t, 1, *s.states)
	for _, result := range h.finish() {
		require.Nil(t, result.Error)
	}
	require.NotEmpty(t, *s.progress)
	for _, state := range *s.progress {
		require.Equal(t, "state-1", state, "the worker reads progress with the update's state")
	}
	require.Empty(t, h.record(nativePath).Properties.UpdateState)

	h.deploy(nativePath, nativeProperties(3))
	require.Equal(t, "state-2", lastUpdate(t, h.fake.Calls()).State, "a new update gets a new state")

	*s.stateErr = nativefake.ClientError("ThrottlingException", "slow down")
	h.fake.Calls()
	resp := h.put(nativePath, nativeProperties(4))
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.Equal(t, []string{"Read"}, methods(h.fake.Calls()), "no update without its state")
	rec := h.record(nativePath)
	require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState(), "nothing was reserved")
	require.Empty(t, rec.Properties.UpdateToken)
}

// Test_Native_Cleanup checks when UCP asks a handler to clean up a resource (native.Cleaner): after a create, only once
// the record is saved with the identifier and without the client token, and at the next deploys until it succeeds.
func Test_Native_Cleanup(t *testing.T) {
	t.Run("after the create is saved, and once more at the next deploy", func(t *testing.T) {
		h := newNativeHarness(t)
		s := h.withStateful()
		h.deploy(nativePath, nativeProperties(1))
		require.Len(t, *s.cleanups, 1)
		first := (*s.cleanups)[0]
		require.Equal(t, "app-native", first.saved.AWSIdentifier)
		require.Empty(t, first.saved.ClientToken, "the record was saved before the cleanup")
		require.NotEmpty(t, first.createToken)
		require.Equal(t, first.createToken, first.saved.CreateToken, "the create's client token")
		require.Nil(t, h.record(nativePath).Properties.CleanedUpAt)

		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status)
		require.Len(t, *s.cleanups, 2)
		require.Equal(t, first.createToken, (*s.cleanups)[1].createToken)
		rec := h.record(nativePath)
		require.NotNil(t, rec.Properties.CleanedUpAt)
		require.Empty(t, rec.Properties.CreateToken, "no longer needed")

		h.deploy(nativePath, nativeProperties(2))
		require.Len(t, *s.cleanups, 2, "a cleaned-up resource isn't asked again")
	})

	t.Run("a failed cleanup doesn't fail the deploy, and the next deploy tries again", func(t *testing.T) {
		h := newNativeHarness(t)
		s := h.withStateful()
		*s.cleanErr = errors.New("throttled")
		h.deploy(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status)
		require.Len(t, *s.cleanups, 2)
		require.Nil(t, h.record(nativePath).Properties.CleanedUpAt)

		*s.cleanErr = nil
		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status)
		require.Len(t, *s.cleanups, 3)
		require.NotNil(t, h.record(nativePath).Properties.CleanedUpAt)
	})

	t.Run("a restart between the create and saving it never cleans up first", func(t *testing.T) {
		h := newNativeHarness(t)
		s := h.withStateful()
		h.loseCreate(h.fake, nativePath, nativeProperties(1))
		require.NotEmpty(t, h.record(nativePath).Properties.ClientToken)

		h.deploy(nativePath, nativeProperties(1))
		require.Empty(t, *s.cleanups, "the deploy that finds the lost create hasn't saved it yet")
		require.Empty(t, h.record(nativePath).Properties.ClientToken)

		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status)
		require.Len(t, *s.cleanups, 1)
	})

	t.Run("a resource created again is cleaned up again", func(t *testing.T) {
		h := newNativeHarness(t)
		s := h.withStateful()
		h.deploy(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status)
		require.NotNil(t, h.record(nativePath).Properties.CleanedUpAt)

		h.cc.Remove(nativeType, "app-native")
		*s.cleanErr = errors.New("throttled")
		h.deploy(nativePath, nativeProperties(1))
		rec := h.record(nativePath)
		require.Nil(t, rec.Properties.CleanedUpAt, "the new resource has its own marks")
		require.NotEqual(t, (*s.cleanups)[0].createToken, rec.Properties.CreateToken)

		*s.cleanErr = nil
		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status)
		last := (*s.cleanups)[len(*s.cleanups)-1]
		require.Equal(t, rec.Properties.CreateToken, last.createToken)
		require.NotNil(t, h.record(nativePath).Properties.CleanedUpAt)
	})

	t.Run("a take-over is cleaned up without a create token", func(t *testing.T) {
		h := newNativeHarness(t)
		s := h.withStateful()
		h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Size": 1})
		h.deploy(nativePath, nativeProperties(1))
		require.Empty(t, *s.cleanups, "the take-over deploy itself doesn't clean up")

		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status)
		require.Len(t, *s.cleanups, 1)
		require.Empty(t, (*s.cleanups)[0].createToken, "another alias may have lost the create, so its marks stay")
	})
}
