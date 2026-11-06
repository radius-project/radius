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
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/radius-project/radius/pkg/retry"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/awsresources"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// nativeType has a CloudFormation type and a native handler. Its identifier is Name, which UCP generates when the
	// template leaves it out, and Password is write-only.
	nativeType = "AWS::Test::Native"
	nativePath = providers + "AWS.Test/Native/app-native"

	// assignedType has a native handler, and AWS assigns its identifier, Id. The handler finds a lost create by
	// sending it again with the client token.
	assignedType = "AWS::Test::Assigned"
	assignedPath = providers + "AWS.Test/Assigned/app-assigned"

	// handWrittenType has no CloudFormation type.
	handWrittenType = "AWS::Test::HandWritten"
	handWrittenPath = providers + "AWS.Test/HandWritten/app-handwritten"
)

var nativeSchemas = map[string]string{
	nativeType:      `{"typeName": "AWS::Test::Native", "properties": {"Name": {"type": "string", "maxLength": 16}}, "primaryIdentifier": ["/properties/Name"], "createOnlyProperties": ["/properties/Name"], "writeOnlyProperties": ["/properties/Password"]}`,
	assignedType:    `{"typeName": "AWS::Test::Assigned", "primaryIdentifier": ["/properties/Id"], "readOnlyProperties": ["/properties/Id"]}`,
	handWrittenType: `{"typeName": "AWS::Test::HandWritten", "primaryIdentifier": ["/properties/Name"]}`,
}

func init() {
	// DescribeType returns these schemas. The native schemas replace them.
	harnessSchemas[nativeType] = `{"primaryIdentifier": ["/properties/Name"]}`
	harnessSchemas[assignedType] = `{"primaryIdentifier": ["/properties/Id"]}`
}

// nativeHarness is a harness with native handlers for nativeType, assignedType, and handWrittenType. CloudFormation
// reports nativeType and assignedType as types Cloud Control can't create, so the native handlers serve them.
type nativeHarness struct {
	*harness
	fake        *nativefake.Handler
	assigned    *nativefake.Handler
	handWritten *nativefake.Handler
}

func newNativeHarness(t *testing.T) *nativeHarness {
	h := newHarness(t)
	h.cc.Identifiers[nativeType] = "Name"
	h.cc.WriteOnly[nativeType] = []string{"Password"}
	h.cc.Identifiers[handWrittenType] = "Name"
	nh := &nativeHarness{
		harness:     h,
		fake:        nativefake.New(nativeType, h.cc),
		assigned:    nativefake.New(assignedType, h.cc),
		handWritten: nativefake.New(handWrittenType, h.cc),
	}
	nh.fake.Opts.NameProperty = "Name"
	nh.assigned.Opts.Recovery = native.RecoverByResend
	nh.withMetadata()
	h.provisioningTypes = map[string]cftypes.ProvisioningType{
		nativeType:   cftypes.ProvisioningTypeNonProvisionable,
		assignedType: cftypes.ProvisioningTypeNonProvisionable,
	}
	return nh
}

// withMetadata registers the harness's handlers again, with the per-type metadata given.
func (h *nativeHarness) withMetadata(metadata ...schemas.Metadata) {
	h.t.Helper()
	catalog := schemas.Catalog{
		Native:      map[string]string{nativeType: nativeSchemas[nativeType], assignedType: nativeSchemas[assignedType]},
		HandWritten: map[string]string{handWrittenType: nativeSchemas[handWrittenType]},
		Metadata:    map[string]schemas.Metadata{},
	}
	for _, m := range metadata {
		catalog.Metadata[m.TypeName] = m
	}
	registry, err := native.NewRegistry(catalog, h.fake, h.assigned, h.handWritten)
	require.NoError(h.t, err)
	h.native = registry
}

func nativeProperties(size int) map[string]any {
	return map[string]any{"Name": "app-native", "Size": size, "Password": password}
}

func methods(calls []nativefake.Call) []string {
	out := []string{}
	for _, c := range calls {
		out = append(out, c.Method)
	}
	return out
}

// deploy sends a PUT, finishes the operation, and requires it to succeed.
func (h *nativeHarness) deploy(path string, properties map[string]any) {
	h.t.Helper()
	resp := h.put(path, properties)
	require.Contains(h.t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
	for _, result := range h.finish() {
		require.Nil(h.t, result.Error)
	}
}

func Test_Native_Routing(t *testing.T) {
	t.Run("NON_PROVISIONABLE uses the native handler", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		require.Equal(t, []string{"Read", "Create"}, methods(h.fake.Calls()))
		require.Empty(t, h.cc.Calls())
		rec := h.record(nativePath)
		require.Equal(t, records.HandlerNative, rec.Properties.Handler)
		require.Equal(t, "app-native", rec.Properties.AWSIdentifier)
		require.NotContains(t, rec.Properties.LastApplied, "Password", "the native schema marks Password write-only")
	})

	t.Run("a type Cloud Control can create uses Cloud Control with the native schema", func(t *testing.T) {
		h := newNativeHarness(t)
		h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
		h.deploy(nativePath, nativeProperties(1))
		require.Empty(t, h.fake.Calls())
		require.Equal(t, []string{"GetResource", "CreateResource"}, operations(h.cc.Calls()))
		rec := h.record(nativePath)
		require.Empty(t, rec.Properties.Handler)
		require.NotContains(t, rec.Properties.LastApplied, "Password", "CloudFormation's schema doesn't mark Password, but the native one does")
	})

	t.Run("a type with no native handler uses Cloud Control", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"})
		require.Empty(t, h.fake.Calls())
		require.Equal(t, []string{"CreateResource"}, operations(h.cc.Calls()))
	})
}

// Test_Native_ContinueWithStartingHandler checks that a create a native handler started, and UCP lost, finishes with
// the native handler after the region gains Cloud Control support for the type.
func Test_Native_ContinueWithStartingHandler(t *testing.T) {
	h := newNativeHarness(t)
	h.fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
	resp := h.put(nativePath, nativeProperties(1))
	require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
	rec := h.record(nativePath)
	require.Equal(t, records.HandlerNative, rec.Properties.Handler)
	require.NotEmpty(t, rec.Properties.ClientToken)
	h.fake.Calls()

	h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
	h.deploy(nativePath, nativeProperties(2))
	require.Equal(t, []string{"Read", "Update"}, methods(h.fake.Calls()), "the native handler finds the create and updates it")
	require.Empty(t, h.cc.Calls())
	rec = h.record(nativePath)
	require.Empty(t, rec.Properties.ClientToken)
	require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
	require.EqualValues(t, 2, h.cc.Resource(nativeType, "app-native")["Size"])

	// The create is finished, so the next deploy uses Cloud Control, as the region now allows.
	h.deploy(nativePath, nativeProperties(3))
	require.Empty(t, h.fake.Calls())
	require.Contains(t, operations(h.cc.Calls()), "UpdateResource")
}

func Test_Native_ResumeCreate(t *testing.T) {
	t.Run("the create never reached AWS", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Errors["Create"] = nativefake.ServerError("InternalFailure", "unavailable")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)
		delete(h.fake.Errors, "Create")
		h.fake.Calls()

		h.deploy(nativePath, nativeProperties(1))
		require.Equal(t, []string{"Create"}, methods(h.fake.Calls()))
		require.Equal(t, 1, h.cc.Count(nativeType))
	})

	t.Run("the create is still running", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Waits = 3
		h.fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)
		h.fake.Calls()

		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "still running in AWS")
		require.Empty(t, h.fake.Calls(), "UCP waits for the create AWS has instead of sending it again")
		require.Nil(t, h.record(nativePath).Properties.PendingFingerprints, "which attempt's password AWS has is unknown")

		results := h.finish()
		require.Len(t, results, 1)
		require.Nil(t, results[0].Error)
		rec := h.record(nativePath)
		require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
		require.Empty(t, rec.Properties.ClientToken)
	})

	t.Run("the create failed", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
		h.fake.Failures[native.OperationCreate] = &native.Failure{Message: "the engine version is not supported", ClientError: true}
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)

		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Equal(t, "the engine version is not supported", resp.message())
		require.NotEmpty(t, h.record(nativePath).Properties.ClientToken, "the record stays, so a DELETE can remove the resource")
	})

	t.Run("the created resource was deleted outside Radius", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)
		h.fake.Calls()

		// Progress finds the create finished, and the resource is gone when UCP reads it.
		h.fake.Errors["Read"] = native.ErrNotFound
		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		require.Equal(t, []string{"Read", "Create"}, methods(h.fake.Calls()))
	})
}

func Test_Native_StartErrors(t *testing.T) {
	t.Run("AWS rejects the first create", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Errors["Create"] = nativefake.ClientError("InvalidParameterValue", "Size is too small")
		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Equal(t, "Size is too small", resp.message())
		require.Nil(t, h.record(nativePath), "nothing exists, so the next deploy starts over")
	})

	t.Run("AWS rejects an update", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Errors["Update"] = nativefake.ClientError("InvalidParameterValue", "Size is too large")
		resp := h.put(nativePath, nativeProperties(2))
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		rec := h.record(nativePath)
		require.EqualValues(t, 1, rec.Properties.LastApplied["Size"])
		require.Nil(t, rec.Properties.PendingLastApplied)
		require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
	})

	t.Run("an update may have reached AWS", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Errors["Update"] = nativefake.ServerError("InternalFailure", "unavailable")
		resp := h.put(nativePath, nativeProperties(2))
		require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
		require.EqualValues(t, 2, h.record(nativePath).Properties.LastApplied["Size"], "Radius owns the values the update may have applied")
	})

	t.Run("delete of a resource AWS no longer has", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.cc.Remove(nativeType, "app-native")
		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
		require.Nil(t, h.record(nativePath))
	})

	t.Run("no native handler for an operation one started", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)

		registry, err := native.NewRegistry(schemas.Catalog{})
		require.NoError(t, err)
		h.native = registry
		h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusInternalServerError, resp.status)
		require.Contains(t, resp.raw, "UCP has no native handler for AWS::Test::Native")
		resp = h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusInternalServerError, resp.status)
		require.Contains(t, resp.raw, "UCP has no native handler for AWS::Test::Native")
		require.Empty(t, operations(h.cc.Calls()), "Cloud Control never finishes an operation a native handler started")
	})

	t.Run("DescribeType fails on GET and DELETE", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		delete(harnessSchemas, nativeType)
		t.Cleanup(func() { harnessSchemas[nativeType] = `{"primaryIdentifier": ["/properties/Name"]}` })
		require.Equal(t, http.StatusInternalServerError, h.get(nativePath).status)
		require.Equal(t, http.StatusInternalServerError, h.delete(nativePath, testOwner).status)
		require.NotNil(t, h.record(nativePath))
	})
}

// Test_Native_Worker checks how the worker finishes native operations.
func Test_Native_Worker(t *testing.T) {
	t.Run("the operation runs for the handler's timeout", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Opts.CreateTimeout = time.Hour
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
		msg, err := h.queue.Dequeue(t.Context(), queue.QueueClientConfig{})
		require.NoError(t, err)
		req := &ctrl.Request{}
		require.NoError(t, json.Unmarshal(msg.Data, req))
		require.Equal(t, time.Hour+nativeTimeoutMargin, req.Timeout())
	})

	t.Run("timeout", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Waits = 1_000_000
		h.fake.Opts.CreateTimeout = time.Millisecond
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
		results := h.finish()
		require.Len(t, results, 1)
		require.Equal(t, `UCP waited 1ms for the create of the AWS resource "app-native" to finish, for the state "available". AWS still reports "create-in-progress". Deploy again to continue.`, results[0].Error.Message)
		rec := h.record(nativePath)
		require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())
		require.NotEmpty(t, rec.Properties.ClientToken, "the next deploy reads the resource and continues")

		resp := h.put(nativePath, nativeProperties(1))
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
	})

	t.Run("update timeout keeps the values the update may have applied", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Waits = 1_000_000
		h.fake.Opts.UpdateTimeout = time.Millisecond
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
		results := h.finish()
		require.NotNil(t, results[0].Error)
		require.EqualValues(t, 2, h.record(nativePath).Properties.LastApplied["Size"])
	})

	t.Run("failed create keeps the record", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Failures[native.OperationCreate] = &native.Failure{Message: "invalid engine", ClientError: true}
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
		results := h.finish()
		require.Equal(t, v1.CodeInvalid, results[0].Error.Code)
		require.NotEmpty(t, h.record(nativePath).Properties.ClientToken)
	})

	t.Run("the resource disappears while UCP waits", func(t *testing.T) {
		h := newNativeHarness(t)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
		h.cc.Remove(nativeType, "app-native")
		results := h.finish()
		require.Equal(t, `AWS no longer has the AWS resource "app-native". Deploy again to create it.`, results[0].Error.Message)
	})

	t.Run("delete", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Waits = 2
		resp := h.delete(nativePath, testOwner)
		require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
		require.NotNil(t, h.cc.Resource(nativeType, "app-native"), "the delete is still running")
		results := h.finish()
		require.Nil(t, results[0].Error)
		require.Nil(t, h.record(nativePath))
		require.Nil(t, h.cc.Resource(nativeType, "app-native"))
	})

	t.Run("no native handler", func(t *testing.T) {
		h := newNativeHarness(t)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
		registry, err := native.NewRegistry(schemas.Catalog{})
		require.NoError(t, err)
		h.native = registry
		results := h.finish()
		require.Contains(t, results[0].Error.Message, "UCP has no native handler for AWS::Test::Native")
	})

	t.Run("no identifier", func(t *testing.T) {
		h := newNativeHarness(t)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
		h.saveRecord(nativePath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "" })
		results := h.finish()
		require.Contains(t, results[0].Error.Message, "UCP has no AWS identifier")
	})

	t.Run("record gone for a hand-written type", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(handWrittenPath, map[string]any{"Name": "app-handwritten"})
		h.handWritten.Waits = 1
		require.Equal(t, http.StatusAccepted, h.delete(handWrittenPath, testOwner).status)
		require.NoError(t, h.db.Delete(t.Context(), handWrittenPath))
		h.cc.Calls()
		results := h.finish()
		require.Nil(t, results[0].Error, "a delete whose record is gone is done")
		require.Empty(t, h.cc.Calls(), "UCP never asks Cloud Control about a hand-written type")
	})
}

func Test_Native_List(t *testing.T) {
	h := newNativeHarness(t)
	h.deploy(nativePath, nativeProperties(1))
	list := func() response {
		return h.do(NewListAWSResources, http.MethodGet, providers+"AWS.Test/Native", "", nil)
	}

	resp := list()
	require.Equal(t, http.StatusBadRequest, resp.status)
	require.Equal(t, "The AWS resource type AWS::Test::Native can't be listed.", resp.message())

	h.fake.Opts.List = native.ListParent
	resp = list()
	require.Equal(t, http.StatusBadRequest, resp.status)
	require.Equal(t, "The AWS resource type AWS::Test::Native can be listed only under its parent resource, which Radius does not support.", resp.message())

	h.fake.Opts.List = native.ListRegion
	resp = list()
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []any{map[string]any{
		"id":         providers + "AWS.Test/Native/app-native",
		"name":       "app-native",
		"type":       "AWS.Test/Native",
		"properties": map[string]any{"Name": "app-native", "Size": float64(1)},
	}}, resp.body["value"])

	h.fake.Errors["List"] = nativefake.ClientError("AccessDenied", "not allowed")
	require.Equal(t, http.StatusBadRequest, list().status)
}

func Test_Native_Existing(t *testing.T) {
	h := newNativeHarness(t)
	h.deploy(nativePath, nativeProperties(1))
	getWithPost := func(body map[string]any) response {
		return h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
			return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
		}, http.MethodPost, providers+"AWS.Test/Native/:get", "", body)
	}

	resp := getWithPost(map[string]any{"name": "app-native"})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)

	resp = getWithPost(map[string]any{"properties": map[string]any{"Size": 1}})
	require.Equal(t, http.StatusBadRequest, resp.status)
	require.Equal(t, "Set the primary identifier properties of the AWS resource type AWS::Test::Native to reference it with existing.", resp.message())

	resp = getWithPost(map[string]any{"properties": map[string]any{"Name": "missing"}})
	require.Equal(t, http.StatusNotFound, resp.status)
}

// runWorker runs the queued operations once each with the worker, without the harness's bookkeeping, so tests can
// fail the data store under it.
func (h *nativeHarness) runWorker() []ctrl.Result {
	h.t.Helper()
	results := []ctrl.Result{}
	for {
		msg, err := h.queue.Dequeue(h.t.Context(), queue.QueueClientConfig{})
		if err != nil {
			return results
		}
		req := &ctrl.Request{}
		require.NoError(h.t, json.Unmarshal(msg.Data, req))
		armCtx, err := req.ARMRequestContext()
		require.NoError(h.t, err)
		controller, err := awsresources.NewAwaitController(ctrl.Options{DatabaseClient: h.db}, h.clients(), armCtx.OperationType.Method)
		require.NoError(h.t, err)
		result, err := controller.Run(h.t.Context(), req)
		require.NoError(h.t, err)
		require.NoError(h.t, h.queue.FinishMessage(h.t.Context(), msg))
		results = append(results, result)
	}
}

func Test_Native_WorkerErrors(t *testing.T) {
	down := errors.New("database is down")

	t.Run("Progress fails", func(t *testing.T) {
		h := newNativeHarness(t)
		require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
		h.fake.Errors["Progress"] = nativefake.ServerError("Throttling", "slow down")
		results := h.runWorker()
		require.True(t, results[0].Requeue, "the worker waits again later")
		require.True(t, records.IsRunning(h.record(nativePath)))
	})

	t.Run("a newer operation replaces it while UCP waits", func(t *testing.T) {
		for _, failure := range []*native.Failure{nil, {Message: "failed"}} {
			h := newNativeHarness(t)
			require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
			h.fake.Failures[native.OperationCreate] = failure
			h.fake.OnProgress = func() {
				h.saveRecord(nativePath, func(rec *datamodel.AWSResource) { rec.Properties.RequestToken = "00000000-0000-0000-0000-000000000001" })
			}
			results := h.runWorker()
			require.Equal(t, v1.ProvisioningStateCanceled, results[0].ProvisioningState())
		}
	})

	t.Run("saving the result fails", func(t *testing.T) {
		for _, failure := range []*native.Failure{nil, {Message: "failed"}} {
			h := newNativeHarness(t)
			require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(1)).status)
			h.fake.Failures[native.OperationCreate] = failure
			h.failSavesAfter(1, down)
			results := h.runWorker()
			require.True(t, results[0].Requeue, "the worker saves the result later")
		}
	})

	t.Run("the delete finishes when AWS no longer has the resource", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Waits = 5
		require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
		h.cc.Remove(nativeType, "app-native")
		results := h.runWorker()
		require.Nil(t, results[0].Error)
		require.Nil(t, h.record(nativePath))
	})
}

func Test_Native_ReadErrors(t *testing.T) {
	t.Run("GET", func(t *testing.T) {
		h := newNativeHarness(t)
		h.deploy(nativePath, nativeProperties(1))
		h.fake.Errors["Read"] = nativefake.ClientError("AccessDenied", "not allowed")
		resp := h.get(nativePath)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Equal(t, "not allowed", resp.message())
	})

	resumeHarness := func(t *testing.T) *nativeHarness {
		h := newNativeHarness(t)
		h.fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)
		return h
	}

	t.Run("Progress fails while resuming a create", func(t *testing.T) {
		h := resumeHarness(t)
		h.fake.Errors["Progress"] = nativefake.ClientError("AccessDenied", "not allowed")
		require.Equal(t, http.StatusBadRequest, h.put(nativePath, nativeProperties(1)).status)
	})

	t.Run("reading fails while resuming a create", func(t *testing.T) {
		h := resumeHarness(t)
		h.fake.Errors["Read"] = nativefake.ClientError("AccessDenied", "not allowed")
		require.Equal(t, http.StatusBadRequest, h.put(nativePath, nativeProperties(1)).status)
	})

	t.Run("queuing the wait fails while resuming a create", func(t *testing.T) {
		h := newNativeHarness(t)
		h.fake.Waits = 5
		h.fake.LostResponses["Create"] = nativefake.ServerError("InternalFailure", "the response was lost")
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)
		h.withFailingQueue()
		require.Equal(t, http.StatusInternalServerError, h.put(nativePath, nativeProperties(1)).status)
	})

	t.Run("list when DescribeType fails", func(t *testing.T) {
		h := newNativeHarness(t)
		delete(harnessSchemas, nativeType)
		t.Cleanup(func() { harnessSchemas[nativeType] = `{"primaryIdentifier": ["/properties/Name"]}` })
		resp := h.do(NewListAWSResources, http.MethodGet, providers+"AWS.Test/Native", "", nil)
		require.Equal(t, http.StatusInternalServerError, resp.status)
	})

	t.Run("existing when DescribeType fails", func(t *testing.T) {
		h := newNativeHarness(t)
		resp := h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
			return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
		}, http.MethodPost, providers+"AWS.Test/Unknown/:get", "", map[string]any{"name": "x"})
		require.Equal(t, http.StatusInternalServerError, resp.status)
	})
}

// nilPropertiesHandler lists resources without properties.
type nilPropertiesHandler struct {
	*nativefake.Handler
}

func (h nilPropertiesHandler) List(ctx context.Context, region string) ([]native.Resource, error) {
	return []native.Resource{{Identifier: "app-native"}}, nil
}

func Test_Native_ListWithoutProperties(t *testing.T) {
	h := newNativeHarness(t)
	h.fake.Opts.List = native.ListRegion
	registry, err := native.NewRegistry(schemas.Catalog{Native: map[string]string{nativeType: nativeSchemas[nativeType]}}, nilPropertiesHandler{h.fake})
	require.NoError(t, err)
	h.native = registry
	resp := h.do(NewListAWSResources, http.MethodGet, providers+"AWS.Test/Native", "", nil)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	items := resp.body["value"].([]any)
	require.Equal(t, map[string]any{}, items[0].(map[string]any)["properties"])
}

// Test_Native_RedeliveredWithoutRecord runs a native operation again after its record is gone, as when the worker
// stops after a delete removes the record and before it saves the operation's result. UCP never asks Cloud Control
// about a native operation, even for a type that has a CloudFormation type.
func Test_Native_RedeliveredWithoutRecord(t *testing.T) {
	for _, method := range []string{http.MethodDelete, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			h := newNativeHarness(t)
			h.deploy(nativePath, nativeProperties(1))
			h.cc.Errors["GetResourceRequestStatus"] = nativefake.ClientError("AccessDeniedException", "not allowed")
			if method == http.MethodDelete {
				require.Equal(t, http.StatusAccepted, h.delete(nativePath, testOwner).status)
			} else {
				require.Equal(t, http.StatusCreated, h.put(nativePath, nativeProperties(2)).status)
			}

			msg, err := h.queue.Dequeue(t.Context(), queue.QueueClientConfig{})
			require.NoError(t, err)
			req := &ctrl.Request{}
			require.NoError(t, json.Unmarshal(msg.Data, req))
			require.True(t, strings.HasPrefix(req.OperationType, records.NativeOperationType+"|"), req.OperationType)
			armCtx, err := req.ARMRequestContext()
			require.NoError(t, err)
			controller, err := awsresources.NewAwaitController(ctrl.Options{DatabaseClient: h.db}, h.clients(), armCtx.OperationType.Method)
			require.NoError(t, err)

			require.NoError(t, h.db.Delete(t.Context(), nativePath))
			result, err := controller.Run(t.Context(), req)
			require.NoError(t, err)
			require.False(t, result.Requeue, "UCP must not poll Cloud Control for a native operation")
			if method == http.MethodDelete {
				require.Nil(t, result.Error)
			} else {
				require.Equal(t, `UCP has no record of the AWS resource "app-native". Deploy again.`, result.Error.Message)
			}
		})
	}
}

// encodingHarness serves nativeType with a handler whose identifier encoding is encode.
func encodingHarness(t *testing.T, encode func(properties map[string]any) (string, error)) *nativeHarness {
	h := newNativeHarness(t)
	h.fake.IdentifierOf = encode
	registry, err := native.NewRegistry(schemas.Catalog{Native: map[string]string{nativeType: nativeSchemas[nativeType]}}, h.fake.Encoding())
	require.NoError(t, err)
	h.native = registry
	return h
}

// Test_Native_ExistingWithEncoderOnCloudControl checks that `existing` finds a resource by the handler's identifier
// encoding where Cloud Control serves the type, as create and take-over do.
func Test_Native_ExistingWithEncoderOnCloudControl(t *testing.T) {
	h := encodingHarness(t, func(properties map[string]any) (string, error) {
		name, _ := properties["Name"].(string)
		return "encoded-" + name, nil
	})
	h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
	h.cc.Put(nativeType, "encoded-app", map[string]any{"Name": "app", "Size": 1})

	resp := h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
		return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
	}, http.MethodPost, providers+"AWS.Test/Native/:get", "", map[string]any{"properties": map[string]any{"Name": "app"}})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, providers+"AWS.Test/Native/encoded-app", resp.body["id"])
	calls := h.cc.Calls()
	require.Equal(t, "encoded-app", calls[len(calls)-1].Identifier)
	require.Empty(t, h.fake.Calls(), "Cloud Control serves the read")
}

// Test_Native_IdentifierWithSlash checks that an AWS identifier that isn't one resource ID segment, such as an S3 ARN,
// is used as is with AWS, with either handler, and escaped into one segment in the IDs list and `existing` return. A
// read by that segment finds the resource again.
func Test_Native_IdentifierWithSlash(t *testing.T) {
	for _, identifier := range []string{"arn:aws:s3:::bucket/a/../b", ".", "..", "a%2Fb"} {
		for _, nativeHandler := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s native=%t", identifier, nativeHandler), func(t *testing.T) {
				h := encodingHarness(t, func(map[string]any) (string, error) { return identifier, nil })
				h.fake.Opts.List = native.ListRegion
				if !nativeHandler {
					h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
				}
				// Cloud Control takes the identifier from Arn, as AWS would from the type's primary identifier.
				h.cc.Identifiers[nativeType] = "Arn"
				properties := nativeProperties(1)
				properties["Arn"] = identifier
				h.deploy(nativePath, properties)
				require.Equal(t, identifier, h.record(nativePath).Properties.AWSIdentifier)
				require.NotNil(t, h.cc.Resource(nativeType, identifier), "AWS has the resource under the raw identifier")

				existing := func(body map[string]any) response {
					return h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
						return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
					}, http.MethodPost, providers+"AWS.Test/Native/:get", "", body)
				}
				resp := existing(map[string]any{"properties": map[string]any{"Name": "app-native"}})
				require.Equal(t, http.StatusOK, resp.status, resp.raw)
				id, _ := resp.body["id"].(string)
				parsed, err := resources.ParseResource(id)
				require.NoError(t, err)
				require.Equal(t, providers+"AWS.Test/Native/"+parsed.Name(), id, "the identifier is one segment of the ID")
				require.Equal(t, identifier, resp.body["name"])

				h.headers = map[string]string{sdkclients.AWSResourceIDHeader: id}
				resp = existing(map[string]any{"name": parsed.Name()})
				h.headers = nil
				require.Equal(t, http.StatusOK, resp.status, resp.raw)
				require.Equal(t, id, resp.body["id"])

				if nativeHandler {
					resp = h.do(NewListAWSResources, http.MethodGet, providers+"AWS.Test/Native", "", nil)
					require.Equal(t, http.StatusOK, resp.status, resp.raw)
					require.Equal(t, id, resp.body["value"].([]any)[0].(map[string]any)["id"])
				}
			})
		}
	}

}

// Test_Native_ListEscapesIdentifier checks that a list escapes each identifier into one ID segment, without resolving
// "..", and keeps the raw identifier as the name.
func Test_Native_ListEscapesIdentifier(t *testing.T) {
	h := newNativeHarness(t)
	h.fake.Opts.List = native.ListRegion
	h.cc.Put(nativeType, "x/../app-native", map[string]any{"Name": "x/../app-native"})
	resp := h.do(NewListAWSResources, http.MethodGet, providers+"AWS.Test/Native", "", nil)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	item := resp.body["value"].([]any)[0].(map[string]any)
	require.Equal(t, providers+"AWS.Test/Native/x%2F..%2Fapp-native", item["id"])
	require.Equal(t, "x/../app-native", item["name"])
}

// Test_AddressSegment checks the escaping of an identifier into a resource ID segment, and back.
func Test_AddressSegment(t *testing.T) {
	for _, identifier := range []string{"plain", "a|b", "arn:aws:s3:::b/k", ".", "..", "a%2Fb", "100%", "a/../b"} {
		segment := addressSegment(identifier)
		require.NotContains(t, segment, "/")
		require.NotContains(t, []string{".", ".."}, segment)
		require.Equal(t, identifier, identifierOfSegment(segment), identifier)
	}
	// Only what would split a segment, and "%", is escaped, so other identifiers keep their IDs.
	require.Equal(t, "a|b:c d", addressSegment("a|b:c d"))
	// A segment that isn't valid escaping is used as is.
	require.Equal(t, "100%", identifierOfSegment("100%"))
}

// Test_Native_ExistingNameIsLiteral checks that POST /:get reads a top-level name as the AWS identifier itself, even
// when it looks escaped, and decodes only the last segment of the ID in the AWS resource ID header. Both "a%2Fb" and
// "a/b" exist in AWS, so reading the wrong one shows. It covers a type with a native handler, served natively and by
// Cloud Control, and a type with none, whose IDs are never escaped.
func Test_Native_ExistingNameIsLiteral(t *testing.T) {
	tests := []struct {
		name  string
		typ   string
		path  string
		setup func(h *nativeHarness)
	}{
		{name: "native handler", typ: nativeType, path: "AWS.Test/Native"},
		{name: "native type served by Cloud Control", typ: nativeType, path: "AWS.Test/Native", setup: func(h *nativeHarness) {
			h.provisioningTypes[nativeType] = cftypes.ProvisioningTypeFullyMutable
		}},
		{name: "no native handler", typ: bucketType, path: "AWS.S3/Bucket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newNativeHarness(t)
			if tt.setup != nil {
				tt.setup(h)
			}
			h.cc.Put(tt.typ, "a%2Fb", map[string]any{"Marker": "literal"})
			h.cc.Put(tt.typ, "a/b", map[string]any{"Marker": "decoded"})
			read := func(name string, id string) response {
				h.headers = map[string]string{}
				if id != "" {
					h.headers[sdkclients.AWSResourceIDHeader] = id
				}
				defer func() { h.headers = nil }()
				return h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
					return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
				}, http.MethodPost, providers+tt.path+"/:get", "", map[string]any{"name": name})
			}
			marker := func(resp response) any {
				require.Equal(t, http.StatusOK, resp.status, resp.raw)
				return resp.body["properties"].(map[string]any)["Marker"]
			}

			// A name is literal, as on PUT.
			require.Equal(t, "literal", marker(read("a%2Fb", "")))
			require.Equal(t, "decoded", marker(read("a/b", "")))

			// An ID's last segment is decoded, for every type, because lists escape every identifier, and the ID wins
			// over the name.
			idOf := func(segment string) string { return providers + tt.path + "/" + segment }
			require.Equal(t, "decoded", marker(read("a%2Fb", idOf("a%2Fb"))))
			require.Equal(t, "literal", marker(read("a/b", idOf("a%252Fb"))))
		})
	}
}

// Test_Native_ExistingIDHeaderMismatch checks that the AWS resource ID header must name a resource of the request's
// type, account, and region.
func Test_Native_ExistingIDHeaderMismatch(t *testing.T) {
	for _, id := range []string{
		"not an id",
		providers + "AWS.Test/Other/app",
		"/planes/aws/aws/accounts/999/regions/us-west-2/providers/AWS.Test/Native/app",
		"/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Test/Native/app",
	} {
		t.Run(id, func(t *testing.T) {
			h := newNativeHarness(t)
			h.headers = map[string]string{sdkclients.AWSResourceIDHeader: id}
			resp := h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
				return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
			}, http.MethodPost, providers+"AWS.Test/Native/:get", "", map[string]any{"name": "app"})
			require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
			require.Contains(t, resp.message(), "X-Radius-AWS-Resource-ID header must be the ID of an AWS resource of type AWS.Test/Native")
			require.Empty(t, h.cc.Calls())
		})
	}
}

// Test_ListIDsAreOneSegment lists identifiers that would split a resource ID, through Cloud Control and a native
// handler. Each listed ID is one segment that decodes to the identifier, and a recipe's read by that ID finds the same
// resource. Plain identifiers keep the IDs they had before.
func Test_ListIDsAreOneSegment(t *testing.T) {
	identifiers := []string{"arn:aws:s3:::b/k", ".", "..", "a/../b", "a%2Fb", "plain|x"}
	for _, nativeHandler := range []bool{true, false} {
		t.Run(fmt.Sprintf("native=%t", nativeHandler), func(t *testing.T) {
			h := newNativeHarness(t)
			h.fake.Opts.List = native.ListRegion
			typ, path := nativeType, "AWS.Test/Native"
			if !nativeHandler {
				typ, path = bucketType, "AWS.S3/Bucket"
			}
			for i, identifier := range identifiers {
				h.cc.Put(typ, identifier, map[string]any{"Marker": i})
			}

			resp := h.do(NewListAWSResources, http.MethodGet, providers+path, "", nil)
			require.Equal(t, http.StatusOK, resp.status, resp.raw)
			items := resp.body["value"].([]any)
			require.Len(t, items, len(identifiers))
			for _, item := range items {
				item := item.(map[string]any)
				id := item["id"].(string)
				parsed, err := resources.ParseResource(id)
				require.NoError(t, err)
				require.Equal(t, providers+path+"/"+parsed.Name(), id, "one segment")
				require.Equal(t, item["name"], identifierOfSegment(parsed.Name()))

				h.headers = map[string]string{sdkclients.AWSResourceIDHeader: id}
				read := h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
					return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
				}, http.MethodPost, providers+path+"/:get", "", map[string]any{"name": parsed.Name()})
				h.headers = nil
				require.Equal(t, http.StatusOK, read.status, read.raw)
				require.Equal(t, item["properties"].(map[string]any)["Marker"], read.body["properties"].(map[string]any)["Marker"])
			}
			for _, item := range items {
				if item.(map[string]any)["name"] == "plain|x" {
					require.Equal(t, providers+path+"/plain|x", item.(map[string]any)["id"])
				}
			}
		})
	}
}

// Test_Native_IdentifierErrors checks that a handler's identifier error gets the response its kind calls for on every
// path that computes the identifier: 400 with its message for a client Failure, 500 for a server Failure or another
// error. Nothing is called in AWS, and no record is saved.
func Test_Native_IdentifierErrors(t *testing.T) {
	errs := []struct {
		name   string
		err    error
		status int
	}{
		{"client failure", &native.Failure{Message: "RuleNumber must be between 1 and 32766", ClientError: true}, http.StatusBadRequest},
		{"server failure", &native.Failure{Message: "the identifier could not be computed"}, http.StatusInternalServerError},
		{"other error", errors.New("json: unsupported value"), http.StatusInternalServerError},
	}
	paths := []struct {
		name string
		send func(h *nativeHarness) response
	}{
		{"PUT, take over", func(h *nativeHarness) response { return h.put(nativePath, nativeProperties(1)) }},
		{"PUT, create with name", func(h *nativeHarness) response {
			return h.putBody(nativePath, testOwner, map[string]any{"name": "app-native", "properties": nativeProperties(1)})
		}},
		{"existing", func(h *nativeHarness) response {
			return h.do(func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
				return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
			}, http.MethodPost, providers+"AWS.Test/Native/:get", "", map[string]any{"properties": nativeProperties(1)})
		}},
	}
	for _, e := range errs {
		for _, p := range paths {
			t.Run(e.name+" "+p.name, func(t *testing.T) {
				h := encodingHarness(t, func(map[string]any) (string, error) { return "", e.err })
				resp := p.send(h)
				require.Equal(t, e.status, resp.status, resp.raw)
				if e.status == http.StatusBadRequest {
					require.Equal(t, "RuleNumber must be between 1 and 32766", resp.message())
				}
				for _, call := range h.fake.Calls() {
					require.Equal(t, "Read", call.Method, "nothing changes in AWS")
				}
				require.Nil(t, h.record(nativePath))
			})
		}
	}
}
