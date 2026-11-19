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
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/awsresources"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// busyMessage is the 409 a request gets when a native handler reports a busy parent and UCP doesn't wait.
const busyMessage = `AWS can't change the AWS resource "app-native" yet: the parent "parent-1" is busy. UCP changed nothing. Retry later.`

// newWaitHarness returns a native harness whose handlers wait up to an hour for AWS to take a call.
func newWaitHarness(t *testing.T) *nativeHarness {
	h := newNativeHarness(t)
	h.fake.Opts.ReadyTimeout = time.Hour
	h.assigned.Opts.ReadyTimeout = time.Hour
	return h
}

// callsOf returns the calls of method.
func callsOf(calls []nativefake.Call, method string) []nativefake.Call {
	out := []nativefake.Call{}
	for _, c := range calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// requireWaiting requires the record of path to hold an operation in state that waits for AWS to take its call.
func (h *nativeHarness) requireWaiting(path string, state v1.ProvisioningState) *datamodel.AWSResource {
	h.t.Helper()
	rec := h.record(path)
	require.NotNil(h.t, rec)
	require.Equal(h.t, records.StartWaiting, rec.Properties.StartState)
	require.Equal(h.t, nativefake.BusyReason, rec.Properties.StartReason)
	require.Equal(h.t, state, rec.ProvisioningState())
	require.NotEmpty(h.t, rec.Properties.RequestToken)
	return rec
}

// requireDone requires the record of path to show a finished operation with nothing left of a waiting call.
func (h *nativeHarness) requireDone(path string, state v1.ProvisioningState) *datamodel.AWSResource {
	h.t.Helper()
	rec := h.record(path)
	require.NotNil(h.t, rec)
	require.Equal(h.t, state, rec.ProvisioningState())
	require.Empty(h.t, rec.Properties.StartState)
	require.Empty(h.t, rec.Properties.StartReason)
	require.Nil(h.t, rec.Properties.StartPatch)
	require.Nil(h.t, rec.Properties.StartCurrent)
	return rec
}

// Test_Native_Wait_Create checks that a create whose handler reports a busy parent is accepted, and that the worker
// sends the same create, with the same client token and generated name, until AWS takes it.
func Test_Native_Wait_Create(t *testing.T) {
	h := newWaitHarness(t)
	h.fake.SetBusy(3)
	resp := h.put(nativePath, map[string]any{"Size": 1})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.Equal(t, string(v1.ProvisioningStateProvisioning), resp.body["properties"].(map[string]any)["provisioningState"])
	rec := h.requireWaiting(nativePath, v1.ProvisioningStateProvisioning)
	name := rec.Properties.AWSIdentifier
	require.True(t, strings.HasPrefix(name, "radius-"), "the generated name is saved before the create")
	require.Nil(t, h.cc.Resource(nativeType, name), "nothing reached AWS")
	operationID := rec.Properties.RequestToken

	results := h.finish()
	require.Len(t, results, 1)
	require.Nil(t, results[0].Error)
	creates := callsOf(h.fake.Calls(), "Create")
	require.Len(t, creates, 4, "the request's attempt, two more while busy, and the one AWS took")
	for _, c := range creates {
		require.Equal(t, rec.Properties.ClientToken, c.ClientToken)
		require.Equal(t, name, c.Properties["Name"])
	}
	rec = h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
	require.Equal(t, operationID, rec.Properties.LastRequestToken, "one operation ID from the request to the end")
	require.EqualValues(t, 1, rec.Properties.LastApplied["Size"])
	require.NotNil(t, h.cc.Resource(nativeType, name))
}

// Test_Native_Wait_OperationTimeout checks that the async operation of a waiting call also runs for ReadyTimeout.
func Test_Native_Wait_OperationTimeout(t *testing.T) {
	h := newWaitHarness(t)
	h.fake.Opts.CreateTimeout = time.Hour
	h.fake.Opts.ReadyTimeout = 30 * time.Minute
	h.fake.SetBusy(1)
	require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Size": 1}).status)
	msg, err := h.queue.Dequeue(t.Context(), queue.QueueClientConfig{})
	require.NoError(t, err)
	req := &ctrl.Request{}
	require.NoError(t, json.Unmarshal(msg.Data, req))
	require.Equal(t, time.Hour+30*time.Minute+nativeTimeoutMargin, req.Timeout())
}

// Test_Native_Wait_Update checks that an update whose handler reports a busy parent is sent by the worker with the
// patch the request computed and the same update token, and that the record never holds a write-only value.
func Test_Native_Wait_Update(t *testing.T) {
	h := newWaitHarness(t)
	h.deploy(nativePath, nativeProperties(1))
	h.fake.Calls()
	h.fake.SetBusy(2)

	resp := h.put(nativePath, nativeProperties(2))
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	rec := h.requireWaiting(nativePath, v1.ProvisioningStateUpdating)
	require.NotEmpty(t, rec.Properties.StartPatch)
	require.NotContains(t, rec.Properties.StartCurrent, "Password")
	require.NotContains(t, h.rawRecord(nativePath), password, "UCP never stores a write-only value")
	token := rec.Properties.UpdateToken
	require.NotEmpty(t, token)
	require.EqualValues(t, 1, h.cc.Resource(nativeType, "app-native")["Size"])

	results := h.finish()
	require.Nil(t, results[0].Error)
	updates := callsOf(h.fake.Calls(), "Update")
	require.Len(t, updates, 3)
	for _, c := range updates {
		require.Equal(t, token, c.Token)
		require.Equal(t, []map[string]any{{"op": "replace", "path": "/Size", "value": float64(2)}}, c.Patch)
	}
	rec = h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
	require.EqualValues(t, 2, rec.Properties.LastApplied["Size"])
	require.Empty(t, rec.Properties.UpdateToken)
	require.EqualValues(t, 2, h.cc.Resource(nativeType, "app-native")["Size"])
}

// Test_Native_Wait_Delete checks that a delete whose handler reports a busy parent is sent by the worker with the
// same delete token, so it takes one final copy, and that a reset is sent the same way.
func Test_Native_Wait_Delete(t *testing.T) {
	t.Run("delete token", func(t *testing.T) {
		h := newWaitHarness(t)
		registry, err := native.NewRegistry(schemas.Catalog{Native: map[string]string{nativeType: nativeSchemas[nativeType]}}, h.fake.DeleteTokens())
		require.NoError(t, err)
		h.native = registry
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(2)

		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
		rec := h.requireWaiting(nativePath, v1.ProvisioningStateDeleting)
		require.NotEmpty(t, rec.Properties.DeleteToken)

		results := h.finish()
		require.Nil(t, results[0].Error)
		require.Nil(t, h.record(nativePath))
		require.Nil(t, h.cc.Resource(nativeType, "app-native"))
		require.Equal(t, 1, h.fake.Copies(), "every attempt sends the same delete token")
	})

	t.Run("reset", func(t *testing.T) {
		h := newWaitHarness(t)
		h.withMetadata(schemas.Metadata{TypeName: nativeType, OnDelete: schemas.OnDeleteReset})
		h.fake.ResetTo = map[string]any{"Name": "app-native", "Size": 0}
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Calls()
		h.fake.SetBusy(1)

		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		results := h.finish()
		require.Nil(t, results[0].Error)
		require.Equal(t, []string{"Reset", "Reset"}, writes(h.fake.Calls()))
		require.Nil(t, h.record(nativePath))
		require.EqualValues(t, 0, h.cc.Resource(nativeType, "app-native")["Size"])
	})

	t.Run("the resource is gone when AWS takes the call", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		h.cc.Remove(nativeType, "app-native")
		results := h.finish()
		require.Nil(t, results[0].Error)
		require.Nil(t, h.record(nativePath))
	})
}

// Test_Native_Wait_Timeout checks that an operation whose call AWS never takes within ReadyTimeout fails, names the
// reason, and leaves the record as if AWS rejected the call, so the next deploy starts fresh.
func Test_Native_Wait_Timeout(t *testing.T) {
	timedOut := func(op string) string {
		return `UCP waited 1ns for AWS to take the ` + op + ` of the AWS resource "app-native", because the parent "parent-1" is busy. Nothing was sent to AWS. Deploy again.`
	}

	t.Run("create", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.Opts.ReadyTimeout = time.Nanosecond
		h.fake.SetBusy(-1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		first := h.record(nativePath).Properties.ClientToken
		results := h.finish()
		require.Equal(t, timedOut("create"), results[0].Error.Message)
		require.Nil(t, h.record(nativePath), "nothing reached AWS, so the record is removed")
		require.Nil(t, h.cc.Resource(nativeType, "app-native"))

		h.fake.SetBusy(0)
		h.deploy(nativePath, map[string]any{"Name": "app-native", "Size": 1})
		creates := callsOf(h.fake.Calls(), "Create")
		require.NotEqual(t, first, creates[len(creates)-1].ClientToken, "the next deploy is a new create")
	})

	t.Run("update", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Opts.ReadyTimeout = time.Nanosecond
		h.fake.SetBusy(-1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
		token := h.record(nativePath).Properties.UpdateToken
		results := h.finish()
		require.Equal(t, timedOut("update"), results[0].Error.Message)
		rec := h.requireDone(nativePath, v1.ProvisioningStateFailed)
		require.EqualValues(t, 1, rec.Properties.LastApplied["Size"], "the update never reached AWS")
		require.Nil(t, rec.Properties.PendingLastApplied)
		require.Empty(t, rec.Properties.UnresolvedPaths)
		require.Empty(t, rec.Properties.UpdateToken)

		h.fake.SetBusy(0)
		h.fake.Calls()
		h.deploy(nativePath, nativeProperties(2))
		require.NotEqual(t, token, lastUpdate(t, h.fake.Calls()).Token, "the next deploy is a new update")
	})

	t.Run("delete", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Opts.ReadyTimeout = time.Nanosecond
		h.fake.SetBusy(-1)
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		results := h.finish()
		require.Equal(t, timedOut("delete"), results[0].Error.Message)
		h.requireDone(nativePath, v1.ProvisioningStateFailed)
		require.NotNil(t, h.cc.Resource(nativeType, "app-native"))

		h.fake.SetBusy(0)
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		h.finish()
		require.Nil(t, h.record(nativePath))
	})
}

// Test_Native_Wait_NotEligible checks the calls UCP doesn't wait for: they get 409 with the reason, and the record is
// as when AWS rejects the call.
func Test_Native_Wait_NotEligible(t *testing.T) {
	t.Run("no ReadyTimeout", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.SetBusy(-1)
		resp := h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Equal(t, busyMessage, resp.message())
		require.Nil(t, h.record(nativePath), "a rejected first create removes the record")
		require.Empty(t, h.finish())
	})

	t.Run("create with a write-only value", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(-1)
		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Equal(t, busyMessage, resp.message())
		require.Nil(t, h.record(nativePath))
		require.Len(t, callsOf(h.fake.Calls(), "Create"), 1)
		require.Empty(t, h.finish())
	})

	t.Run("update that changes a write-only value", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(-1)
		properties := nativeProperties(1)
		properties["Password"] = "another-password"
		resp := h.put(nativePath, properties)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		rec := h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
		require.Nil(t, rec.Properties.PendingLastApplied)
		require.Empty(t, rec.Properties.UpdateToken, "a rejected first attempt clears the token")
		require.Empty(t, h.finish())
	})

	t.Run("create sent again", func(t *testing.T) {
		h := newWaitHarness(t)
		h.failCreate(h.fake, nativePath, map[string]any{"Name": "app-native", "Size": 1})
		token := h.record(nativePath).Properties.ClientToken
		h.fake.SetBusy(-1)
		resp := h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		h.requireCreateKept(nativePath)
		require.Equal(t, token, h.record(nativePath).Properties.ClientToken)
		require.Empty(t, h.record(nativePath).Properties.StartState)
		require.Empty(t, h.finish())
	})

	t.Run("update sent again", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Errors["Update"] = nativefake.ServerError("InternalFailure", "unavailable")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(2)).status)
		delete(h.fake.Errors, "Update")
		token := h.record(nativePath).Properties.UpdateToken
		require.NotEmpty(t, token)

		h.fake.SetBusy(-1)
		resp := h.put(nativePath, nativeProperties(2))
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		rec := h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
		require.Equal(t, token, rec.Properties.UpdateToken, "AWS may have the earlier attempt")
		require.Empty(t, h.finish())
	})

	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		t.Run("lost create sent again on "+method, func(t *testing.T) {
			h := newWaitHarness(t)
			h.loseCreate(h.assigned, assignedPath, map[string]any{"Size": 1})
			h.assigned.SetBusy(-1)
			var resp response
			if method == http.MethodPut {
				resp = h.put(assignedPath, map[string]any{"Size": 1})
			} else {
				resp = h.delete(assignedPath, testOwner)
			}
			require.Equal(t, http.StatusConflict, resp.status, resp.raw)
			require.Equal(t, `AWS can't change the AWS resource "app-assigned" yet: the parent "parent-1" is busy. UCP changed nothing. Retry later.`, resp.message())
			rec := h.record(assignedPath)
			require.NotEmpty(t, rec.Properties.ClientToken)
			require.Empty(t, rec.Properties.StartState)
			require.Empty(t, h.finish())
		})
	}
}

// Test_Native_Wait_Replaced checks that a new PUT or DELETE replaces an operation that still waits and sent nothing:
// the new request continues, the old operation ends canceled, and the worker never sends its call.
func Test_Native_Wait_Replaced(t *testing.T) {
	requireCanceled := func(t *testing.T, result ctrl.Result) {
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
	}

	t.Run("DELETE replaces a waiting create", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
		require.Nil(t, h.record(nativePath))
		h.fake.Calls()

		results := h.finish()
		require.Len(t, results, 1)
		requireCanceled(t, results[0])
		require.Empty(t, writes(h.fake.Calls()), "the worker sends nothing")
		require.Nil(t, h.cc.Resource(nativeType, "app-native"))
	})

	t.Run("PUT replaces a waiting create", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		first := h.record(nativePath).Properties.ClientToken
		resp := h.put(nativePath, map[string]any{"Name": "app-native", "Size": 2})
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		require.NotEqual(t, first, h.record(nativePath).Properties.ClientToken, "nothing of the first create reached AWS")

		results := h.finish()
		require.Len(t, results, 2)
		requireCanceled(t, results[0])
		require.Nil(t, results[1].Error)
		require.EqualValues(t, 2, h.cc.Resource(nativeType, "app-native")["Size"])
		require.EqualValues(t, 2, h.requireDone(nativePath, v1.ProvisioningStateSucceeded).Properties.LastApplied["Size"])
	})

	t.Run("PUT replaces a waiting update", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
		first := h.record(nativePath).Properties.UpdateToken
		h.fake.Calls()

		resp := h.put(nativePath, nativeProperties(3))
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		results := h.finish()
		require.Len(t, results, 2)
		requireCanceled(t, results[0])
		require.Nil(t, results[1].Error)
		updates := callsOf(h.fake.Calls(), "Update")
		require.Len(t, updates, 1, "only the new update reaches AWS")
		require.NotEqual(t, first, updates[0].Token)
		require.EqualValues(t, 3, h.cc.Resource(nativeType, "app-native")["Size"])
	})

	t.Run("the replaced operation leaves the newer one running", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(3)).status)

		req := &ctrl.Request{}
		msg, err := h.queue.Dequeue(t.Context(), queue.QueueClientConfig{})
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(msg.Data, req))
		result := h.rerun(req, v1.OperationPut)
		requireCanceled(t, result)
		require.True(t, result.KeepResourceState, "the record shows the newer update")
		require.Equal(t, v1.ProvisioningStateUpdating, h.record(nativePath).ProvisioningState())
		resp := h.put(nativePath, nativeProperties(4))
		require.Equal(t, http.StatusConflict, resp.status, "the newer update is still running")

		require.NoError(t, h.queue.FinishMessage(t.Context(), msg))
		results := h.finish()
		require.Nil(t, results[0].Error)
		require.EqualValues(t, 3, h.cc.Resource(nativeType, "app-native")["Size"])
	})

	t.Run("DELETE replaces a waiting update", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		results := h.finish()
		require.Len(t, results, 2)
		requireCanceled(t, results[0])
		require.Nil(t, results[1].Error)
		require.Nil(t, h.record(nativePath))
		require.Nil(t, h.cc.Resource(nativeType, "app-native"))
	})

	t.Run("PUT replaces a waiting delete", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		h.fake.Calls()

		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		results := h.finish()
		require.Len(t, results, 1)
		requireCanceled(t, results[0])
		require.Empty(t, writes(h.fake.Calls()))
		require.NotNil(t, h.cc.Resource(nativeType, "app-native"))
		// The worker picked up the replaced delete after the PUT, and marked the record Updating as it did.
		h.requireDone(nativePath, v1.ProvisioningStateCanceled)
		require.Equal(t, http.StatusOK, h.put(nativePath, nativeProperties(1)).status, "the record isn't busy")
	})

	t.Run("a request while the worker sends the call gets 409", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		var during response
		h.fake.OnStart = func() {
			h.fake.OnStart = nil
			require.Equal(t, records.StartSending, h.record(nativePath).Properties.StartState)
			during = h.delete(nativePath, testOwner)
		}
		results := h.finish()
		require.Nil(t, results[0].Error)
		require.Equal(t, http.StatusConflict, during.status, during.raw)
		require.NotNil(t, h.cc.Resource(nativeType, "app-native"))
		h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
	})

	t.Run("the worker saves that it sends the call first", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		paused, resume := h.pauseRead(nativePath)
		first, _ := h.interleave(paused, resume, func() response { return h.delete(nativePath, testOwner) }, func() response { return response{} }, h.fake)
		require.Equal(t, http.StatusConflict, first.status, first.raw)
		require.NotNil(t, h.cc.Resource(nativeType, "app-native"))
		require.NotNil(t, h.record(nativePath), "AWS has the resource, so the record stays")
	})

	t.Run("the worker saves that it sends an update first", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
		paused, resume := h.pauseRead(nativePath)
		first, _ := h.interleave(paused, resume, func() response { return h.put(nativePath, nativeProperties(3)) }, func() response { return response{} }, h.fake)
		require.Equal(t, http.StatusConflict, first.status, first.raw)
		require.EqualValues(t, 2, h.cc.Resource(nativeType, "app-native")["Size"], "the update AWS took stays")
		h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
	})

	t.Run("a request replaces the operation while the worker reads it", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		h.fake.Calls()
		// The worker's second read of the record is the one before it saves that it sends the call.
		h.db = &hookDB{Client: h.db, id: strings.ToLower(nativePath), at: 2, hook: func() {
			require.Equal(t, http.StatusNoContent, h.delete(nativePath, testOwner).status)
		}}
		results := h.finish()
		requireCanceled(t, results[0])
		require.Empty(t, writes(h.fake.Calls()))
		require.Nil(t, h.cc.Resource(nativeType, "app-native"))
	})
}

// hookDB runs hook before the at-th read of the record id, counting from 1, once.
type hookDB struct {
	database.Client
	id    string
	at    int
	reads atomic.Int32
	hook  func()
}

func (d *hookDB) Get(ctx context.Context, id string, options ...database.GetOptions) (*database.Object, error) {
	if strings.EqualFold(id, d.id) && int(d.reads.Add(1)) == d.at {
		d.hook()
	}
	return d.Client.Get(ctx, id, options...)
}

// Test_Native_Wait_QueueFails checks that a waiting operation the queue never took is replaced by the next request.
func Test_Native_Wait_QueueFails(t *testing.T) {
	h := newWaitHarness(t)
	h.fake.SetBusy(1)
	h.withFailingQueue()
	require.Equal(t, http.StatusInternalServerError, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
	h.requireWaiting(nativePath, v1.ProvisioningStateAccepted)
	h.sm = statusmanager.New(h.db, h.queue, "global")

	h.deploy(nativePath, map[string]any{"Name": "app-native", "Size": 1})
	require.Len(t, callsOf(h.fake.Calls(), "Create"), 2)
	h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
}

// runStopped runs the next queued operation with a context that stop cancels, as when the worker stops, and returns
// the request, which the redelivered message runs again.
func (h *nativeHarness) runStopped(stop func(cancel context.CancelFunc)) *ctrl.Request {
	h.t.Helper()
	msg, err := h.queue.Dequeue(h.t.Context(), queue.QueueClientConfig{})
	require.NoError(h.t, err)
	req := &ctrl.Request{}
	require.NoError(h.t, json.Unmarshal(msg.Data, req))
	ctx, cancel := context.WithCancel(h.t.Context())
	defer cancel()
	stop(cancel)
	controller, err := awsresources.NewAwaitController(ctrl.Options{DatabaseClient: h.db}, h.clients(), v1.OperationPut)
	require.NoError(h.t, err)
	_, err = controller.Run(ctx, req)
	require.ErrorIs(h.t, err, context.Canceled)
	return req
}

// rerun runs req again, as the redelivered message.
func (h *nativeHarness) rerun(req *ctrl.Request, method v1.OperationMethod) ctrl.Result {
	h.t.Helper()
	controller, err := awsresources.NewAwaitController(ctrl.Options{DatabaseClient: h.db}, h.clients(), method)
	require.NoError(h.t, err)
	result, err := controller.Run(h.t.Context(), req)
	require.NoError(h.t, err)
	return result
}

// Test_Native_Wait_WorkerStops checks what a redelivered operation does after the worker stopped while it waited, and
// after it stopped while the call may have been in flight.
func Test_Native_Wait_WorkerStops(t *testing.T) {
	t.Run("while waiting", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(2)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		req := h.runStopped(func(cancel context.CancelFunc) {
			h.fake.OnStart = func() { h.fake.OnStart = nil; cancel() }
		})
		h.requireWaiting(nativePath, v1.ProvisioningStateProvisioning)
		require.Nil(t, h.cc.Resource(nativeType, "app-native"))

		result := h.rerun(req, v1.OperationPut)
		require.Nil(t, result.Error)
		h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
		require.NotNil(t, h.cc.Resource(nativeType, "app-native"))
	})

	t.Run("after AWS took the call", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		h.fake.Waits = 1
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		req := h.runStopped(func(cancel context.CancelFunc) {
			h.fake.OnStart = func() { h.fake.OnStart = nil; cancel() }
		})
		rec := h.record(nativePath)
		require.Empty(t, rec.Properties.StartState, "AWS took the call")
		require.Equal(t, v1.ProvisioningStateProvisioning, rec.ProvisioningState())

		result := h.rerun(req, v1.OperationPut)
		require.Nil(t, result.Error)
		require.Len(t, callsOf(h.fake.Calls(), "Create"), 2, "the redelivered operation only waits")
		h.requireDone(nativePath, v1.ProvisioningStateSucceeded)
	})

	// The worker stopped after it saved that it sends the call, and AWS may or may not have the call. The
	// redelivered operation fails, and the next request finds the resource as after any interrupted call.
	inFlight := []struct {
		name     string
		path     string
		fake     func(h *nativeHarness) *nativefake.Handler
		reached  bool
		recovery native.Recovery
	}{
		{name: "identifier, reached AWS", path: nativePath, fake: func(h *nativeHarness) *nativefake.Handler { return h.fake }, reached: true, recovery: native.RecoverByIdentifier},
		{name: "identifier, never reached AWS", path: nativePath, fake: func(h *nativeHarness) *nativefake.Handler { return h.fake }, recovery: native.RecoverByIdentifier},
		{name: "resend, reached AWS", path: assignedPath, fake: func(h *nativeHarness) *nativefake.Handler { return h.assigned }, reached: true, recovery: native.RecoverByResend},
		{name: "search, reached AWS", path: assignedPath, fake: func(h *nativeHarness) *nativefake.Handler { return h.assigned }, reached: true, recovery: native.RecoverBySearch},
		{name: "search, never reached AWS", path: assignedPath, fake: func(h *nativeHarness) *nativefake.Handler { return h.assigned }, recovery: native.RecoverBySearch},
	}
	for _, tc := range inFlight {
		t.Run("create in flight, "+tc.name, func(t *testing.T) {
			h := newWaitHarness(t)
			fake := tc.fake(h)
			fake.Opts.Recovery = tc.recovery
			properties := map[string]any{"Size": 1}
			if tc.path == nativePath {
				properties["Name"] = "app-native"
			}
			fake.SetBusy(1)
			require.Equal(t, http.StatusCreated, h.put(tc.path, properties).status)
			h.saveRecord(tc.path, func(rec *datamodel.AWSResource) { rec.Properties.StartState = records.StartSending })
			rec := h.record(tc.path)
			if tc.reached {
				_, err := fake.Create(t.Context(), native.CreateRequest{Properties: properties, ClientToken: rec.Properties.ClientToken})
				require.NoError(t, err)
			}
			fake.Calls()

			results := h.finish()
			require.Equal(t, `UCP stopped while it sent the create of the AWS resource "`+rec.Name+`", so it can't tell whether AWS took it. Deploy again to continue.`, results[0].Error.Message)
			require.Empty(t, writes(fake.Calls()), "the worker doesn't send a call AWS may have")
			kept := h.requireDone(tc.path, v1.ProvisioningStateFailed)
			require.Equal(t, rec.Properties.ClientToken, kept.Properties.ClientToken)

			h.deploy(tc.path, properties)
			require.Len(t, h.cc.List(fake.Name), 1, "AWS has one resource")
			require.Empty(t, h.record(tc.path).Properties.ClientToken)
		})
	}

	t.Run("update in flight", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
		token := h.record(nativePath).Properties.UpdateToken
		h.saveRecord(nativePath, func(rec *datamodel.AWSResource) { rec.Properties.StartState = records.StartSending })
		h.fake.Calls()

		results := h.finish()
		require.NotNil(t, results[0].Error)
		rec := h.requireDone(nativePath, v1.ProvisioningStateFailed)
		require.Equal(t, token, rec.Properties.UpdateToken, "the next deploy sends the update again with its token")
		require.EqualValues(t, 2, rec.Properties.LastApplied["Size"], "the update may be in AWS")

		h.deploy(nativePath, nativeProperties(2))
		require.Equal(t, token, lastUpdate(t, h.fake.Calls()).Token)
	})

	t.Run("delete in flight", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		h.saveRecord(nativePath, func(rec *datamodel.AWSResource) { rec.Properties.StartState = records.StartSending })
		results := h.finish()
		require.Nil(t, results[0].Error, "a delete is safe to send again")
		require.Nil(t, h.record(nativePath))
	})
}

// Test_Native_Wait_WorkerFailures checks how the worker settles a call AWS refuses after it waited, as the request
// does.
func Test_Native_Wait_WorkerFailures(t *testing.T) {
	t.Run("AWS rejects the create", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		h.fake.Errors["Create"] = nativefake.ClientError("ValidationException", "bad size")
		results := h.finish()
		require.Equal(t, v1.CodeInvalid, results[0].Error.Code)
		require.Nil(t, h.record(nativePath), "nothing exists")
	})

	t.Run("a create AWS fully applied", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		h.fake.LostResponses["Create"] = &native.Failure{Message: "tagging failed", Outcome: native.OutcomeApplied}
		results := h.finish()
		require.Equal(t, "tagging failed", results[0].Error.Message)
		rec := h.requireDone(nativePath, v1.ProvisioningStateFailed)
		require.Empty(t, rec.Properties.ClientToken, "the create is finished")
		require.EqualValues(t, 1, rec.Properties.LastApplied["Size"])
	})

	t.Run("a create with an unknown outcome", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		h.fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
		results := h.finish()
		require.Equal(t, v1.CodeInternal, results[0].Error.Code)
		rec := h.requireDone(nativePath, v1.ProvisioningStateFailed)
		require.NotEmpty(t, rec.Properties.ClientToken)
		require.EqualValues(t, 1, rec.Properties.PendingLastApplied["Size"], "the next request finds the create with these values")
		h.deploy(nativePath, map[string]any{"Name": "app-native", "Size": 1})
	})

	t.Run("an update AWS applied in part", func(t *testing.T) {
		h := newWaitHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.SetBusy(1)
		properties := nativeProperties(2)
		properties["Color"] = "blue"
		require.Equal(t, http.StatusCreated, h.put(nativePath, properties).status)
		h.fake.FailOn(1, nativefake.ServerError("InternalFailure", "unavailable"))
		results := h.finish()
		require.NotNil(t, results[0].Error)
		rec := h.requireDone(nativePath, v1.ProvisioningStateFailed)
		require.NotEmpty(t, rec.Properties.UpdateToken)
		require.NotEmpty(t, rec.Properties.UnresolvedPaths)
	})

	t.Run("no native handler", func(t *testing.T) {
		h := newWaitHarness(t)
		h.fake.SetBusy(1)
		require.Equal(t, http.StatusCreated, h.put(nativePath, map[string]any{"Name": "app-native", "Size": 1}).status)
		registry, err := native.NewRegistry(schemas.Catalog{})
		require.NoError(t, err)
		h.native = registry
		results := h.finish()
		require.Contains(t, results[0].Error.Message, "UCP has no native handler for AWS::Test::Native")
		require.Nil(t, h.record(nativePath), "nothing reached AWS")
	})
}

func Test_patchTouchesSecret(t *testing.T) {
	secrets := []string{"Password", "Users/*/Secret"}
	cases := map[string]bool{
		"/Password":          true,
		"/Password/Inner":    true,
		"/Users":             true,
		"/Users/0":           true,
		"/Users/0/Secret":    true,
		"/Users/0/Name":      false,
		"/Size":              false,
		"/PasswordRetention": false,
		"":                   true,
	}
	for path, want := range cases {
		patch := jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: path, Value: 1}}
		require.Equal(t, want, patchTouchesSecret(patch, secrets), path)
	}
	require.False(t, patchTouchesSecret(jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "", Value: 1}}, nil))
}
