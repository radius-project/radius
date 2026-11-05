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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

func metadata(typeName string, scope schemas.IdentityScope, onDelete schemas.OnDelete) schemas.Metadata {
	return schemas.Metadata{TypeName: typeName, IdentityScope: scope, OnDelete: onDelete}
}

func Test_Native_StartCallOutcomes(t *testing.T) {
	failure := func(outcome native.Outcome) error {
		return &native.Failure{Message: "call 2 of 3 failed", ClientError: true, Outcome: outcome}
	}
	tests := []struct {
		name        string
		err         error
		status      int
		wantApplied bool
	}{
		{"AWS rejects the call", nativefake.ClientError("ValidationException", "invalid size"), http.StatusBadRequest, false},
		{"the response is lost", nativefake.ServerError("InternalFailure", "unavailable"), http.StatusInternalServerError, true},
		{"the handler reports a rejected update", failure(native.OutcomeRejected), http.StatusBadRequest, false},
		{"the handler reports a partial update", failure(native.OutcomePartial), http.StatusBadRequest, true},
		{"the handler reports an unknown outcome", failure(native.OutcomeUnknown), http.StatusBadRequest, true},
		{"the handler reports an applied update", failure(native.OutcomeApplied), http.StatusBadRequest, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newNativeHarness(t)
			h.deploy(nativePath, nativeProperties(1))
			h.fake.Errors["Update"] = tt.err

			resp := h.put(nativePath, nativeProperties(2))
			require.Equal(t, tt.status, resp.status, resp.raw)
			rec := h.record(nativePath)
			require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState(), "the release restores the state")
			require.Nil(t, rec.Properties.PendingLastApplied)
			if tt.wantApplied {
				require.EqualValues(t, 2, rec.Properties.LastApplied["Size"], "the values may be in AWS, so Radius owns them")
			} else {
				require.EqualValues(t, 1, rec.Properties.LastApplied["Size"])
			}
		})
	}

	t.Run("an applied update makes the pending fingerprints last-applied", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		previous := h.record(nativePath).Properties.Fingerprints.Digests["Password"]
		h.fake.Errors["Update"] = failure(native.OutcomeApplied)

		changed := nativeProperties(2)
		changed["Password"] = "an0ther-p@ssword"
		require.Equal(t, http.StatusBadRequest, h.put(nativePath, changed).status)
		digest := h.record(nativePath).Properties.Fingerprints.Digests["Password"]
		require.NotEmpty(t, digest)
		require.NotEqual(t, previous, digest)
	})

	t.Run("a partial update drops the fingerprint of a changed write-only value", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Errors["Update"] = failure(native.OutcomePartial)

		changed := nativeProperties(2)
		changed["Password"] = "an0ther-p@ssword"
		require.Equal(t, http.StatusBadRequest, h.put(nativePath, changed).status)
		require.NotContains(t, h.record(nativePath).Properties.Fingerprints.Digests, "Password", "the next deploy sends it again")
	})
}

func Test_Native_CreateOutcomes(t *testing.T) {
	t.Run("a rejected first create removes the record", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Errors["Create"] = &native.Failure{Message: "invalid engine", ClientError: true, Outcome: native.OutcomeRejected}
		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Equal(t, "invalid engine", resp.message())
		require.Nil(t, h.record(nativePath))
	})

	t.Run("a partial create keeps the record and the identifier", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.LostResponses["Create"] = &native.Failure{Message: "tagging failed", Outcome: native.OutcomePartial}
		resp := h.put(assignedPath, map[string]any{"Size": 1, "Description": "first"})
		require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
		h.requireCreateKept(assignedPath)
		require.Equal(t, "id-1", h.record(assignedPath).Properties.AWSIdentifier, "the identifier the create returned is saved")
		h.assigned.Calls()

		// The next deploy finds the resource by the identifier, and the values of the partial create count as applied,
		// so the description the template no longer sets is cleared.
		h.deploy(assignedPath, map[string]any{"Size": 1})
		require.Equal(t, []string{"Read", "Update"}, methods(h.assigned.Calls()))
		require.NotContains(t, h.cc.Resource(assignedType, "id-1"), "Description")
		require.Equal(t, 1, h.cc.Count(assignedType))
	})
}

func Test_Native_WorkerOutcomes(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		tests := []struct {
			outcome native.Outcome
			size    int
		}{
			{native.OutcomeRejected, 1},
			{native.OutcomePartial, 2},
			{native.OutcomeUnknown, 2},
			{native.OutcomeApplied, 2},
		}
		for _, tt := range tests {
			h := newNativeHarness(t)
			h.deploy(nativePath, nativeProperties(1))
			h.fake.Failures[native.OperationUpdate] = &native.Failure{Message: "the update failed", Outcome: tt.outcome}
			require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
			results := h.finish()
			require.Equal(t, v1.CodeInternal, results[0].Error.Code)
			require.Equal(t, "the update failed", results[0].Error.Message)
			rec := h.record(nativePath)
			require.EqualValues(t, tt.size, rec.Properties.LastApplied["Size"], "outcome %d", tt.outcome)
			require.Nil(t, rec.Properties.PendingLastApplied)
			require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())
		}
	})

	t.Run("create", func(t *testing.T) {
		tests := []struct {
			outcome     native.Outcome
			applied     bool
			clientToken bool
		}{
			{native.OutcomeRejected, false, true},
			{native.OutcomeUnknown, true, true},
			{native.OutcomeApplied, true, false},
		}
		for _, tt := range tests {
			h := newNativeHarness(t)
			h.fake.Failures[native.OperationCreate] = &native.Failure{Message: "the create failed", Outcome: tt.outcome}
			require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
			results := h.finish()
			require.Equal(t, "the create failed", results[0].Error.Message)
			rec := h.record(nativePath)
			require.NotNil(t, rec, "the resource may exist, so the record stays")
			require.Equal(t, "app-native", rec.Properties.AWSIdentifier)
			require.Equal(t, tt.applied, rec.Properties.LastApplied != nil, "outcome %d", tt.outcome)
			require.Equal(t, tt.clientToken, rec.Properties.ClientToken != "", "outcome %d", tt.outcome)
		}
	})

	t.Run("an applied delete removes the record", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Failures[native.OperationDelete] = &native.Failure{Message: "the delete failed", Outcome: native.OutcomeApplied}
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		results := h.finish()
		require.Equal(t, "the delete failed", results[0].Error.Message)
		require.Nil(t, h.record(nativePath))
	})

	t.Run("a partial delete keeps the record", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Failures[native.OperationDelete] = &native.Failure{Message: "the delete failed", Outcome: native.OutcomePartial}
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		require.NotNil(t, h.finish()[0].Error)
		rec := h.record(nativePath)
		require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())
		require.Equal(t, "app-native", rec.Properties.AWSIdentifier)
	})

	t.Run("the identifier names another product while UCP waits", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.ProductProperty, h.fake.Product = "Engine", "docdb"
		properties := nativeProperties(1)
		properties["Engine"] = "docdb"
		require.Equal(t, http.StatusCreated, h.put(nativePath, properties).status)
		h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Engine": "neptune"})

		results := h.finish()
		require.Len(t, results, 1, "the worker does not retry")
		require.Equal(t, `The AWS identifier "app-native" of the AWS resource "app-native" now belongs to another product (neptune), not to the AWS type AWS::Test::Native, so UCP stopped waiting for it.`, results[0].Error.Message)
	})
}

// Test_Native_DeleteOtherProduct checks that a DELETE whose identifier now names a resource of another product treats
// it as not found on every path: it removes only the record and never touches the other product's resource.
func Test_Native_DeleteOtherProduct(t *testing.T) {
	neptune := map[string]any{"Name": "app-native", "Engine": "neptune"}
	docdb := func() map[string]any {
		properties := nativeProperties(1)
		properties["Engine"] = "docdb"
		return properties
	}
	newHarness := func(t *testing.T) *nativeHarness {
		h := newNativeHarness(t)
		h.fake.ProductProperty, h.fake.Product = "Engine", "docdb"
		h.assigned.ProductProperty, h.assigned.Product = "Engine", "docdb"
		h.handWritten.ProductProperty, h.handWritten.Product = "Engine", "docdb"
		return h
	}

	t.Run("the delete call", func(t *testing.T) {
		h := newHarness(t)
		h.deploy(nativePath, docdb())
		h.cc.Put(nativeType, "app-native", neptune)

		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
		require.Nil(t, h.record(nativePath))
		require.Equal(t, neptune, h.cc.Resource(nativeType, "app-native"))
	})

	t.Run("while UCP waits for the delete", func(t *testing.T) {
		h := newHarness(t)
		h.deploy(nativePath, docdb())
		h.fake.Waits = 1
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		h.cc.Put(nativeType, "app-native", neptune)

		results := h.finish()
		require.Nil(t, results[0].Error)
		require.Nil(t, h.record(nativePath))
		require.Equal(t, neptune, h.cc.Resource(nativeType, "app-native"))
	})

	t.Run("a lost create found by its identifier", func(t *testing.T) {
		h := newHarness(t)
		h.loseCreate(h.fake, nativePath, docdb())
		h.cc.Put(nativeType, "app-native", neptune)

		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
		require.Nil(t, h.record(nativePath))
		require.Equal(t, neptune, h.cc.Resource(nativeType, "app-native"))
	})

	t.Run("a lost create found by search", func(t *testing.T) {
		h := newHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.loseCreate(h.assigned, assignedPath, map[string]any{"Size": 1, "Engine": "docdb"})
		other := map[string]any{"Id": "id-1", "Engine": "neptune"}
		h.cc.Put(assignedType, "id-1", other)

		resp := h.delete(assignedPath, testOwner)
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
		require.Nil(t, h.record(assignedPath))
		require.Equal(t, other, h.cc.Resource(assignedType, "id-1"))
	})

	t.Run("a type whose onDelete is reject", func(t *testing.T) {
		h := newHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
		h.deploy(handWrittenPath, map[string]any{"Name": "app-handwritten", "Engine": "docdb"})
		other := map[string]any{"Name": "app-handwritten", "Engine": "neptune"}
		h.cc.Put(handWrittenType, "app-handwritten", other)

		resp := h.delete(handWrittenPath, testOwner)
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
		require.Nil(t, h.record(handWrittenPath))
		require.Equal(t, other, h.cc.Resource(handWrittenType, "app-handwritten"))
	})

	t.Run("a PUT still gets 409", func(t *testing.T) {
		h := newHarness(t)
		h.deploy(nativePath, docdb())
		h.cc.Put(nativeType, "app-native", neptune)

		resp := h.put(nativePath, docdb())
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.NotNil(t, h.record(nativePath))
	})
}

func Test_Native_IdentityScope(t *testing.T) {
	otherRegion := strings.Replace(handWrittenPath, "/regions/us-west-2/", "/regions/us-east-1/", 1)
	otherRegion = strings.TrimSuffix(otherRegion, "app-handwritten") + "app-other"
	otherAccount := strings.Replace(otherRegion, "/accounts/1234567/", "/accounts/12345678/", 1)
	properties := map[string]any{"Name": "account-setting"}

	t.Run("account: another alias in another region gets 409", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeAccount, schemas.OnDeleteDelete))
		h.deploy(handWrittenPath, properties)
		h.handWritten.Calls()

		resp := h.put(otherRegion, properties)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Equal(t, `The AWS resource "account-setting" is already managed under the alias "app-handwritten". Reference it with existing instead, or delete it from the other alias first.`, resp.message())
		require.Equal(t, []string{"Read"}, methods(h.handWritten.Calls()))
		require.Nil(t, h.record(otherRegion))
	})

	t.Run("account: another account is not searched", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeAccount, schemas.OnDeleteDelete))
		h.deploy(handWrittenPath, properties)
		h.deploy(otherAccount, properties)
		require.NotNil(t, h.record(otherAccount))
	})

	t.Run("region: another region is not searched", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(handWrittenPath, properties)
		h.deploy(otherRegion, properties)
		require.NotNil(t, h.record(otherRegion))
	})
}

func Test_Native_OnDelete(t *testing.T) {
	t.Run("reject removes a record with nothing in AWS", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
		h.deploy(handWrittenPath, map[string]any{"Name": "app-handwritten"})
		h.saveRecord(handWrittenPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "" })

		require.Equal(t, http.StatusNoContent, h.delete(handWrittenPath, testOwner).status)
		require.Nil(t, h.record(handWrittenPath))
	})

	t.Run("reject removes a record whose resource was deleted outside Radius", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
		h.deploy(handWrittenPath, map[string]any{"Name": "app-handwritten"})
		h.cc.Remove(handWrittenType, "app-handwritten")

		require.Equal(t, http.StatusNoContent, h.delete(handWrittenPath, testOwner).status)
		require.Nil(t, h.record(handWrittenPath))
	})

	t.Run("reject fails when reading AWS fails", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
		h.deploy(handWrittenPath, map[string]any{"Name": "app-handwritten"})
		h.handWritten.Errors["Read"] = nativefake.ClientError("AccessDenied", "not allowed")

		resp := h.delete(handWrittenPath, testOwner)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Equal(t, "not allowed", resp.message())
		require.NotNil(t, h.record(handWrittenPath))
	})

	t.Run("reject after finding a lost create keeps the record", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
		h.loseCreate(h.handWritten, handWrittenPath, map[string]any{"Name": "app-handwritten"})

		resp := h.delete(handWrittenPath, testOwner)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		rec := h.record(handWrittenPath)
		require.Equal(t, "app-handwritten", rec.Properties.AWSIdentifier)
		require.Empty(t, rec.Properties.ClientToken, "the create was found")
		require.Empty(t, rec.Properties.PendingOperation)
		require.NotNil(t, h.cc.Resource(handWrittenType, "app-handwritten"))
	})

	t.Run("reject checks the owner first", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
		h.deploy(handWrittenPath, map[string]any{"Name": "app-handwritten"})

		resp := h.delete(handWrittenPath, "another-owner")
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
	})

	t.Run("reject names the resource and the type", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
		h.deploy(handWrittenPath, map[string]any{"Name": "app-handwritten"})

		resp := h.delete(handWrittenPath, testOwner)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Equal(t, `Radius can't delete the AWS resource "app-handwritten", because AWS doesn't let it delete a resource of type AWS::Test::HandWritten. Nothing was changed.`, resp.message())
	})

	t.Run("nothing applies where Cloud Control serves the type", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(nativeType, schemas.IdentityScopeRegion, schemas.OnDeleteNothing))
		h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
		h.deploy(nativePath, nativeProperties(1))
		h.cc.Calls()

		require.Equal(t, http.StatusNoContent, h.delete(nativePath, testOwner).status)
		require.Nil(t, h.record(nativePath))
		require.Empty(t, h.cc.Calls())
		require.NotNil(t, h.cc.Resource(nativeType, "app-native"))
	})

	t.Run("reset where Cloud Control serves the type deletes it with Cloud Control", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(nativeType, schemas.IdentityScopeRegion, schemas.OnDeleteReset))
		h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Calls()

		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		for _, result := range h.finish() {
			require.Nil(t, result.Error)
		}
		require.Nil(t, h.record(nativePath))
		require.Nil(t, h.cc.Resource(nativeType, "app-native"))
		require.Empty(t, h.fake.Calls())
	})

	t.Run("a failed reset keeps the record", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(nativeType, schemas.IdentityScopeRegion, schemas.OnDeleteReset))
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Errors["Reset"] = nativefake.ClientError("AccessDenied", "not allowed")

		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		rec := h.record(nativePath)
		require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
		require.Equal(t, "app-native", rec.Properties.AWSIdentifier)
	})
}

func Test_Native_Equivalent(t *testing.T) {
	h := newNativeHarness(t)
	paths := []string{}
	h.fake.Opts.Equivalent = func(path string, template any, aws any) bool {
		paths = append(paths, path)
		return path == "Version" && aws == "5.0.0" && template == "5.0"
	}
	properties := nativeProperties(1)
	properties["Version"] = "5.0"
	h.deploy(nativePath, properties)
	h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Size": 1, "Version": "5.0.0"})
	h.fake.Calls()

	resp := h.put(nativePath, properties)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"Read"}, methods(h.fake.Calls()))
	require.Equal(t, []string{"Version"}, paths, "only values that differ are compared")

	properties["Version"] = "6.0"
	require.Equal(t, http.StatusCreated, h.put(nativePath, properties).status)
	require.Equal(t, []string{"Read", "Update"}, methods(h.fake.Calls()))
}

func Test_handleError(t *testing.T) {
	id := resources.MustParse(nativePath)
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{
			name:    "another product",
			err:     &native.OtherProductError{Identifier: "app-native", Product: "aurora-postgresql"},
			status:  http.StatusConflict,
			message: `The AWS identifier "app-native" belongs to another product (aurora-postgresql), not to the AWS type AWS::Test::Native, so UCP changed nothing. Use an identifier of that type.`,
		},
		{name: "a client failure", err: &native.Failure{Message: "invalid engine", ClientError: true}, status: http.StatusBadRequest, message: "invalid engine"},
		{name: "a server failure", err: &native.Failure{Message: "AWS failed"}, status: http.StatusInternalServerError, message: "AWS failed"},
		{name: "an AWS error", err: nativefake.ClientError("ValidationException", "bad value"), status: http.StatusBadRequest, message: "bad value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := handleError(t.Context(), id, tt.err)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			require.NoError(t, resp.Apply(t.Context(), w, httptest.NewRequest(http.MethodGet, "/", nil)))
			got := response{status: w.Code, body: map[string]any{}}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got.body))
			require.Equal(t, tt.status, got.status)
			require.Equal(t, tt.message, got.message())
		})
	}

	t.Run("another error is returned", func(t *testing.T) {
		boom := errors.New("boom")
		resp, err := handleError(t.Context(), id, boom)
		require.Nil(t, resp)
		require.ErrorIs(t, err, boom)
	})
}

func Test_outcomeOf(t *testing.T) {
	require.Equal(t, native.OutcomePartial, outcomeOf(&native.Failure{Outcome: native.OutcomePartial}))
	require.Equal(t, native.OutcomeRejected, outcomeOf(nativefake.ClientError("ValidationException", "bad value")))
	require.Equal(t, native.OutcomeUnknown, outcomeOf(nativefake.ServerError("InternalFailure", "unavailable")))
	require.Equal(t, native.OutcomeUnknown, outcomeOf(errors.New("connection reset")))
}

// Test_Native_AppliedStartCall checks that a start call that fails after AWS applied every change records the
// operation as done: a create's values become last-applied, and a delete or reset removes the record.
func Test_Native_AppliedStartCall(t *testing.T) {
	applied := &native.Failure{Message: "every change was made, then a call failed", Outcome: native.OutcomeApplied}

	t.Run("create", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.LostResponses["Create"] = applied
		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
		rec := h.record(nativePath)
		require.Empty(t, rec.Properties.ClientToken, "the create is finished")
		require.Equal(t, "app-native", rec.Properties.AWSIdentifier)
		require.NotEmpty(t, rec.Properties.Fingerprints.Digests["Password"])
		h.fake.Calls()

		resp = h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, []string{"Read"}, methods(h.fake.Calls()), "the unchanged password isn't sent again")
	})

	t.Run("create whose identifier AWS didn't return", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Errors["Create"] = applied
		require.Equal(t, http.StatusInternalServerError, h.put(assignedPath, map[string]any{"Size": 1}).status)
		h.requireCreateKept(assignedPath)
	})

	for _, method := range []string{"Delete", "Reset"} {
		t.Run(method, func(t *testing.T) {
			h := newNativeHarness(t)
			if method == "Reset" {
				h.withMetadata(metadata(nativeType, schemas.IdentityScopeRegion, schemas.OnDeleteReset))
			}
			h.deploy(nativePath, nativeProperties(1))
			h.fake.Errors[method] = applied
			resp := h.delete(nativePath, testOwner)
			require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
			require.Equal(t, applied.Message, resp.message())
			require.Nil(t, h.record(nativePath))
		})
	}

	t.Run("removing the record fails", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Errors["Delete"] = applied
		h.failSavesAfter(1, errors.New("database is down"))
		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
		require.Equal(t, applied.Message, resp.message())
		require.NotNil(t, h.record(nativePath), "the record stays, and the next DELETE finds nothing to delete")
	})
}

// Test_Native_UpdateOtherProduct checks that an update the handler refuses because the identifier now names another
// product leaves the record's values as they were.
func Test_Native_UpdateOtherProduct(t *testing.T) {
	h := newNativeHarness(t)
	h.deploy(nativePath, nativeProperties(1))
	h.fake.Errors["Update"] = &native.OtherProductError{Identifier: "app-native", Product: "neptune"}

	resp := h.put(nativePath, nativeProperties(2))
	require.Equal(t, http.StatusConflict, resp.status, resp.raw)
	rec := h.record(nativePath)
	require.EqualValues(t, 1, rec.Properties.LastApplied["Size"])
	require.NotEmpty(t, rec.Properties.Fingerprints.Digests["Password"])
}

// Test_Native_RejectAfterInterruptedCreate checks a DELETE of a type whose onDelete is reject after a create was
// interrupted.
func Test_Native_RejectAfterInterruptedCreate(t *testing.T) {
	t.Run("the found create's values count as applied", func(t *testing.T) {
		h := newNativeHarness(t)
		h.withMetadata(metadata(handWrittenType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
		h.loseCreate(h.handWritten, handWrittenPath, map[string]any{"Name": "app-handwritten", "Description": "first"})
		require.Equal(t, http.StatusBadRequest, h.delete(handWrittenPath, testOwner).status)

		h.deploy(handWrittenPath, map[string]any{"Name": "app-handwritten"})
		require.NotContains(t, h.cc.Resource(handWrittenType, "app-handwritten"), "Description", "a later deploy clears what the template no longer sets")
	})

	for _, exists := range []bool{false, true} {
		name := map[bool]string{false: "Cloud Control create that never reached AWS", true: "Cloud Control create that reached AWS"}[exists]
		t.Run(name, func(t *testing.T) {
			h := newNativeHarness(t)
			h.withMetadata(metadata(nativeType, schemas.IdentityScopeRegion, schemas.OnDeleteReject))
			h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
			h.cc.Errors["CreateResource"] = nativefake.ServerError("InternalFailure", "unavailable")
			require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)
			delete(h.cc.Errors, "CreateResource")
			rec := h.record(nativePath)
			require.NotEmpty(t, rec.Properties.ClientToken)
			require.Equal(t, "app-native", rec.Properties.AWSIdentifier)
			if exists {
				h.cc.Put(nativeType, "app-native", map[string]any{"Name": "app-native", "Size": 1})
			}
			h.cc.Calls()

			resp := h.delete(nativePath, testOwner)
			if exists {
				require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
				require.NotNil(t, h.record(nativePath))
				require.NotNil(t, h.cc.Resource(nativeType, "app-native"))
			} else {
				require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
				require.Nil(t, h.record(nativePath))
			}
			require.Zero(t, countOps(h.cc.Calls(), "CreateResource", "DeleteResource"), "the delete never creates or deletes in AWS")
		})
	}
}

func countOps(calls []awsfake.Call, ops ...string) int {
	n := 0
	for _, c := range calls {
		if slices.Contains(ops, c.Operation) {
			n++
		}
	}
	return n
}
