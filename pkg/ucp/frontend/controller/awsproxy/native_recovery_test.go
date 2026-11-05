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
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/retry"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
)

// loseCreate sends a PUT whose create reaches AWS but whose response is lost, as when UCP stops after AWS accepted
// it.
func (h *nativeHarness) loseCreate(fake *nativefake.Handler, path string, properties map[string]any) {
	h.t.Helper()
	fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
	require.Equal(h.t, http.StatusInternalServerError, h.put(path, properties).status)
	fake.Calls()
}

// failCreate sends a PUT whose create never reaches AWS.
func (h *nativeHarness) failCreate(fake *nativefake.Handler, path string, properties map[string]any) {
	h.t.Helper()
	fake.Errors["Create"] = nativefake.ServerError("InternalFailure", "unavailable")
	require.Equal(h.t, http.StatusInternalServerError, h.put(path, properties).status)
	delete(fake.Errors, "Create")
	fake.Calls()
}

// requireCreateKept requires the record to still hold a first create that may have reached AWS.
func (h *nativeHarness) requireCreateKept(path string) {
	h.t.Helper()
	rec := h.record(path)
	require.NotNil(h.t, rec)
	require.NotEmpty(h.t, rec.Properties.ClientToken)
	require.Equal(h.t, records.OperationCreate, rec.Properties.PendingOperation)
	require.Equal(h.t, v1.ProvisioningStateAccepted, rec.ProvisioningState())
}

func Test_Native_GeneratedName(t *testing.T) {
	unnamed := map[string]any{"Size": 1, "Password": password}

	t.Run("a template without the name gets one from the client token", func(t *testing.T) {
		h := newNativeHarness(t)
		resp := h.put(nativePath, unnamed)
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		calls := h.fake.Calls()
		require.Equal(t, []string{"Create"}, methods(calls))
		name := calls[0].Properties["Name"].(string)
		require.Regexp(t, "^radius-[0-9a-f]{9}$", name, "the name is cut to the schema's maxLength of 16")
		require.Equal(t, name, resp.body["properties"].(map[string]any)["Name"])
		require.NotContains(t, resp.raw, password)

		rec := h.record(nativePath)
		require.Equal(t, name, rec.Properties.AWSIdentifier, "the name is saved before UCP calls AWS")
		require.Equal(t, h.native.Lookup(nativeType).NameRules.Generate(rec.Properties.ClientToken), name)
		require.NotContains(t, rec.Properties.PendingLastApplied, "Name", "the name isn't part of the template")

		for _, result := range h.finish() {
			require.Nil(t, result.Error)
		}
		resp = h.put(nativePath, unnamed)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, []string{"Read"}, methods(h.fake.Calls()), "a redeploy without the name keeps it")
	})

	t.Run("a retry of one create sends the same name", func(t *testing.T) {
		h := newNativeHarness(t)
		h.failCreate(h.fake, nativePath, unnamed)
		first := h.record(nativePath).Properties.AWSIdentifier

		h.deploy(nativePath, unnamed)
		calls := h.fake.Calls()
		require.Equal(t, first, calls[len(calls)-1].Properties["Name"])
		require.Equal(t, first, h.record(nativePath).Properties.AWSIdentifier)
		require.Equal(t, 1, h.cc.Count(nativeType))
	})

	t.Run("a new create gets a new name", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, unnamed)
		first := h.record(nativePath).Properties.AWSIdentifier
		h.cc.Remove(nativeType, first)

		h.deploy(nativePath, unnamed)
		require.NotEqual(t, first, h.record(nativePath).Properties.AWSIdentifier)
	})

	t.Run("the template's name is used", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		require.Equal(t, "app-native", h.record(nativePath).Properties.AWSIdentifier)
	})

	t.Run("Cloud Control names the resource itself", func(t *testing.T) {
		h := newNativeHarness(t)
		h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
		require.Equal(t, http.StatusCreated, h.put(nativePath, unnamed).status)
		calls := h.cc.Calls()
		create := calls[len(calls)-1]
		require.Equal(t, "CreateResource", create.Operation)
		require.NotContains(t, create.Document, "Name")
	})
}

func Test_Native_IdentifierRequired(t *testing.T) {
	h := newNativeHarness(t)
	h.assigned.Opts.Recovery = native.RecoverByIdentifier
	resp := h.put(assignedPath, map[string]any{"Size": 1})
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.Equal(t, `Set Id of the AWS resource "app-assigned". UCP needs the identifier of an AWS resource of type AWS::Test::Assigned before it creates one, so it can find the resource if the create is interrupted.`, resp.message())
	require.Empty(t, h.assigned.Calls())
	require.Nil(t, h.record(assignedPath))
}

func Test_Native_FindLostCreate_Put(t *testing.T) {
	assigned := map[string]any{"Size": 1}

	t.Run("resend: the create returns the first resource", func(t *testing.T) {
		h := newNativeHarness(t)
		h.loseCreate(h.assigned, assignedPath, assigned)
		token := h.record(assignedPath).Properties.ClientToken

		h.deploy(assignedPath, assigned)
		calls := h.assigned.Calls()
		require.Equal(t, []string{"Create"}, methods(calls))
		require.Equal(t, token, calls[0].ClientToken)
		require.Equal(t, 1, h.cc.Count(assignedType))
		rec := h.record(assignedPath)
		require.Equal(t, "id-1", rec.Properties.AWSIdentifier)
		require.Empty(t, rec.Properties.ClientToken)
	})

	t.Run("search: Find finds the resource", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.loseCreate(h.assigned, assignedPath, assigned)
		token := h.record(assignedPath).Properties.ClientToken

		h.deploy(assignedPath, assigned)
		calls := h.assigned.Calls()
		require.Equal(t, []string{"Find", "Read"}, methods(calls), "the create is found, not sent again")
		require.Equal(t, token, calls[0].ClientToken)
		require.Equal(t, 1, h.cc.Count(assignedType))
		require.Equal(t, "id-1", h.record(assignedPath).Properties.AWSIdentifier)
	})

	t.Run("search: the found create is still running", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.assigned.Waits = 3
		h.loseCreate(h.assigned, assignedPath, assigned)

		resp := h.put(assignedPath, assigned)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Equal(t, "id-1", h.record(assignedPath).Properties.AWSIdentifier, "the found identifier is saved")
		for _, result := range h.finish() {
			require.Nil(t, result.Error)
		}
		require.Equal(t, v1.ProvisioningStateSucceeded, h.record(assignedPath).ProvisioningState())
	})

	t.Run("search: AWS has no resource, so the create is sent again", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.failCreate(h.assigned, assignedPath, assigned)

		h.deploy(assignedPath, assigned)
		require.Equal(t, []string{"Find", "Create"}, methods(h.assigned.Calls()))
		require.Equal(t, 1, h.cc.Count(assignedType))
	})

	t.Run("search: Find fails", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.loseCreate(h.assigned, assignedPath, assigned)
		h.assigned.Errors["Find"] = nativefake.ClientError("AccessDenied", "not allowed")

		resp := h.put(assignedPath, assigned)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Equal(t, "not allowed", resp.message())
		require.Equal(t, []string{"Find"}, methods(h.assigned.Calls()))
		h.requireCreateKept(assignedPath)
	})
}

func Test_Native_FindLostCreate_Delete(t *testing.T) {
	assigned := map[string]any{"Size": 1}

	t.Run("identifier: the create reached AWS", func(t *testing.T) {
		h := newNativeHarness(t)
		h.loseCreate(h.fake, nativePath, nativeProperties(1))

		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
		require.Equal(t, []string{"Delete"}, methods(h.fake.Calls()))
		for _, result := range h.finish() {
			require.Nil(t, result.Error)
		}
		require.Nil(t, h.record(nativePath))
		require.Zero(t, h.cc.Count(nativeType))
	})

	t.Run("identifier: the create never reached AWS", func(t *testing.T) {
		h := newNativeHarness(t)
		h.failCreate(h.fake, nativePath, nativeProperties(1))

		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
		require.Empty(t, h.fake.Calls())
		require.Nil(t, h.record(nativePath))
	})

	t.Run("identifier: the create is still running", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Waits = 3
		h.loseCreate(h.fake, nativePath, nativeProperties(1))

		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Equal(t, `A create of the AWS resource "app-native" is still running in AWS. Delete again when it finishes.`, resp.message())
		require.Empty(t, h.fake.Calls())
		rec := h.record(nativePath)
		require.Nil(t, rec.Properties.PendingFingerprints, "which attempt's password AWS has is unknown")
		require.Equal(t, records.OperationCreate, rec.Properties.PendingOperation)

		for _, result := range h.finish() {
			require.Nil(t, result.Error)
		}
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		h.finish()
		require.Nil(t, h.record(nativePath))
		require.Zero(t, h.cc.Count(nativeType))
	})

	t.Run("identifier: the create failed and left a resource", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Failures[native.OperationCreate] = &native.Failure{Message: "the create failed"}
		h.loseCreate(h.fake, nativePath, nativeProperties(1))

		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		h.finish()
		require.Nil(t, h.record(nativePath))
		require.Zero(t, h.cc.Count(nativeType))
	})

	t.Run("resend: the create returns the first resource", func(t *testing.T) {
		h := newNativeHarness(t)
		h.loseCreate(h.assigned, assignedPath, assigned)
		token := h.record(assignedPath).Properties.ClientToken

		resp := h.delete(assignedPath, testOwner)
		require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
		calls := h.assigned.Calls()
		require.Equal(t, []string{"Create", "Delete"}, methods(calls))
		require.Equal(t, token, calls[0].ClientToken)
		require.Equal(t, "id-1", calls[1].Identifier)
		h.finish()
		require.Nil(t, h.record(assignedPath))
		require.Zero(t, h.cc.Count(assignedType))
	})

	t.Run("resend: the create never reached AWS, so the resend creates it", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Waits = 1
		h.failCreate(h.assigned, assignedPath, assigned)

		resp := h.delete(assignedPath, testOwner)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "Delete again when it finishes")
		h.finish()
		require.Equal(t, http.StatusAccepted, h.delete(assignedPath, testOwner).status)
		h.finish()
		require.Nil(t, h.record(assignedPath))
		require.Zero(t, h.cc.Count(assignedType))
	})

	t.Run("resend: AWS rejects the create", func(t *testing.T) {
		h := newNativeHarness(t)
		h.loseCreate(h.assigned, assignedPath, assigned)
		h.assigned.Errors["Create"] = nativefake.ClientError("IdempotentParameterMismatch", "the request differs")

		resp := h.delete(assignedPath, testOwner)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Equal(t, "the request differs", resp.message())
		h.requireCreateKept(assignedPath)
	})

	t.Run("resend: the handler returns no identifier", func(t *testing.T) {
		h := newNativeHarness(t)
		h.loseCreate(h.assigned, assignedPath, assigned)
		registry, err := native.NewRegistry(schemas.Catalog{Native: map[string]string{assignedType: nativeSchemas[assignedType]}}, noIdentifier{h.assigned})
		require.NoError(t, err)
		h.native = registry

		resp := h.delete(assignedPath, testOwner)
		require.Equal(t, http.StatusInternalServerError, resp.status)
		require.Contains(t, resp.raw, `sent the create of the AWS resource "app-assigned" again, but returned no identifier`)
		h.requireCreateKept(assignedPath)
	})

	t.Run("search: Find finds the resource", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.loseCreate(h.assigned, assignedPath, assigned)

		require.Equal(t, http.StatusAccepted, h.delete(assignedPath, testOwner).status)
		require.Equal(t, []string{"Find", "Delete"}, methods(h.assigned.Calls()))
		h.finish()
		require.Nil(t, h.record(assignedPath))
		require.Zero(t, h.cc.Count(assignedType))
	})

	t.Run("search: AWS has no resource", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.failCreate(h.assigned, assignedPath, assigned)

		require.Equal(t, http.StatusNoContent, h.delete(assignedPath, testOwner).status)
		require.Equal(t, []string{"Find"}, methods(h.assigned.Calls()))
		require.Nil(t, h.record(assignedPath))
	})

	t.Run("search: Find fails", func(t *testing.T) {
		h := newNativeHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.loseCreate(h.assigned, assignedPath, assigned)
		h.assigned.Errors["Find"] = nativefake.ServerError("Throttling", "slow down")

		require.Equal(t, http.StatusInternalServerError, h.delete(assignedPath, testOwner).status)
		h.requireCreateKept(assignedPath)
	})

	t.Run("reading the create fails", func(t *testing.T) {
		h := newNativeHarness(t)
		h.loseCreate(h.fake, nativePath, nativeProperties(1))
		h.fake.Errors["Progress"] = nativefake.ClientError("AccessDenied", "not allowed")

		require.Equal(t, http.StatusBadRequest, h.delete(nativePath, testOwner).status)
		h.requireCreateKept(nativePath)
	})

	t.Run("queuing the wait fails", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Waits = 3
		h.loseCreate(h.fake, nativePath, nativeProperties(1))
		h.withFailingQueue()

		require.Equal(t, http.StatusInternalServerError, h.delete(nativePath, testOwner).status)
		require.NotEmpty(t, h.record(nativePath).Properties.ClientToken)
	})

	t.Run("saving the found identifier fails", func(t *testing.T) {
		h := newNativeHarness(t)
		h.loseCreate(h.fake, nativePath, nativeProperties(1))
		h.failSavesAfter(1, errors.New("database is down"))

		require.Equal(t, http.StatusInternalServerError, h.delete(nativePath, testOwner).status)
		require.Empty(t, h.fake.Calls(), "nothing is deleted until the record has the identifier")
	})
}

// findsOther is a handler whose Find returns the resource id-1, as a search by values AWS keeps unique can.
type findsOther struct {
	*nativefake.Handler
}

func (h findsOther) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	return "id-1", nil
}

func Test_Native_FindLostCreate_HeldByAnotherAlias(t *testing.T) {
	const otherPath = providers + "AWS.Test/Assigned/app-other"
	setup := func(t *testing.T) *nativeHarness {
		h := newNativeHarness(t)
		h.assigned.Opts.Recovery = native.RecoverBySearch
		h.deploy(otherPath, map[string]any{"Size": 1})
		require.Equal(t, "id-1", h.record(otherPath).Properties.AWSIdentifier)
		registry, err := native.NewRegistry(schemas.Catalog{Native: map[string]string{assignedType: nativeSchemas[assignedType]}}, findsOther{h.assigned})
		require.NoError(t, err)
		h.native = registry
		h.failCreate(h.assigned, assignedPath, map[string]any{"Size": 1})
		return h
	}

	t.Run("PUT", func(t *testing.T) {
		h := setup(t)
		resp := h.put(assignedPath, map[string]any{"Size": 1})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), `already managed under the alias "app-other"`)
		require.Empty(t, h.assigned.Calls())
		h.requireCreateKept(assignedPath)
	})

	t.Run("DELETE", func(t *testing.T) {
		h := setup(t)
		resp := h.delete(assignedPath, testOwner)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), `already managed under the alias "app-other"`)
		require.Empty(t, h.assigned.Calls(), "UCP never deletes a resource another alias manages")
		h.requireCreateKept(assignedPath)
		require.Equal(t, 1, h.cc.Count(assignedType))
	})
}

// noIdentifier is a handler whose Create returns no identifier.
type noIdentifier struct {
	*nativefake.Handler
}

func (h noIdentifier) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	_, err := h.Handler.Create(ctx, request)
	return "", err
}

// Test_Native_WriteOnlyFromAWS checks that a write-only value AWS returns, as some service APIs do, is in no response,
// with either handler.
func Test_Native_WriteOnlyFromAWS(t *testing.T) {
	existing := func(h *nativeHarness) response {
		return h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
			return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
		}, http.MethodPost, providers+"AWS.Test/Native/:get", "", map[string]any{"name": "app-native"})
	}

	t.Run("native handler", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.ReturnsWriteOnly = true
		h.fake.Opts.List = native.ListRegion
		h.deploy(nativePath, nativeProperties(1))
		require.Equal(t, password, h.cc.Resource(nativeType, "app-native")["Password"], "AWS has the password")

		responses := []response{
			h.get(nativePath),
			existing(h),
			h.do(NewListAWSResources, http.MethodGet, providers+"AWS.Test/Native", "", nil),
			h.put(nativePath, nativeProperties(1)),
			h.put(nativePath, nativeProperties(2)),
		}
		for _, resp := range responses {
			require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
			require.NotContains(t, resp.raw, password)
		}
		require.Equal(t, []string{"Size"}, patchPaths(h.fake.Calls()), "the update compares the password with its fingerprint, not with what AWS returned")
	})

	t.Run("Cloud Control applies the native schema's write-only marks", func(t *testing.T) {
		h := newNativeHarness(t)
		h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
		// CloudFormation's schema doesn't mark Password write-only, so Cloud Control returns it.
		delete(h.cc.WriteOnly, nativeType)
		h.deploy(nativePath, nativeProperties(1))

		for _, resp := range []response{h.get(nativePath), existing(h), h.put(nativePath, nativeProperties(2))} {
			require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
			require.NotContains(t, resp.raw, password)
		}
	})
}

// patchPaths returns the paths every Update call patched.
func patchPaths(calls []nativefake.Call) []string {
	paths := []string{}
	for _, call := range calls {
		for _, op := range call.Patch {
			b, _ := json.Marshal(op["path"])
			var path string
			_ = json.Unmarshal(b, &path)
			paths = append(paths, path[1:])
		}
	}
	return paths
}

func Test_Native_SchemaErrors(t *testing.T) {
	t.Run("existing with a schema UCP can't read", func(t *testing.T) {
		const brokenType = "AWS::Test::Broken"
		h := newNativeHarness(t)
		harnessSchemas[brokenType] = "not json"
		t.Cleanup(func() { delete(harnessSchemas, brokenType) })
		h.cc.Put(brokenType, "app-broken", map[string]any{"Name": "app-broken"})

		resp := h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
			return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
		}, http.MethodPost, providers+"AWS.Test/Broken/:get", "", map[string]any{"name": "app-broken"})
		require.Equal(t, http.StatusInternalServerError, resp.status, "UCP can't tell which values are write-only")
	})

	t.Run("an identifier is required, but the schema has no primary identifier", func(t *testing.T) {
		h := newNativeHarness(t)
		fake := nativefake.New(handWrittenType, h.cc)
		fake.IdentifierOf = func(map[string]any) (string, error) { return "", nil }
		registry, err := native.NewRegistry(schemas.Catalog{HandWritten: map[string]string{handWrittenType: `{"typeName": "AWS::Test::HandWritten"}`}}, fake.Encoding())
		require.NoError(t, err)
		h.native = registry

		resp := h.put(handWrittenPath, map[string]any{"Size": 1})
		require.Equal(t, http.StatusInternalServerError, resp.status)
		require.Contains(t, resp.raw, "primaryIdentifier not found in schema")
		require.Empty(t, fake.Calls())
	})
}

func Test_removeWriteOnly(t *testing.T) {
	require.NoError(t, removeWriteOnly(nil, []string{"Password"}))

	out := &cloudcontrol.GetResourceOutput{ResourceDescription: &cctypes.ResourceDescription{Properties: aws.String(`{"Name": "a", "Auth": {"Password": "p", "User": "u"}}`)}}
	require.NoError(t, removeWriteOnly(out, []string{"Auth/Password"}))
	require.JSONEq(t, `{"Name": "a", "Auth": {"User": "u"}}`, *out.ResourceDescription.Properties)

	out.ResourceDescription.Properties = aws.String("not json")
	require.Error(t, removeWriteOnly(out, []string{"Password"}))
}
