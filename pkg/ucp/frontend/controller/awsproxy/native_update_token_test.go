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
	"net/http"
	"testing"
	"time"

	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

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
