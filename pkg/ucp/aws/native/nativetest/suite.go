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

// Package nativetest has the shared test suite every native AWS handler runs. The suite deploys through UCP's AWS
// resource controllers and finishes operations with UCP's worker, against an in-memory data store, queue, and AWS,
// so it checks the handler together with the code that uses it.
package nativetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/queue"
	queueinmemory "github.com/radius-project/radius/pkg/components/queue/inmemory"
	"github.com/radius-project/radius/pkg/retry"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/awsresources"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/awsproxy"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	owner       = "deploy;/planes/radius/local/resourceGroups/rg;;"
	region      = "us-west-2"
	otherRegion = "us-east-1"
	scope       = "/planes/aws/aws/accounts/1234567/regions/" + region + "/providers/"
	alias       = "suite-alias"
	otherAlias  = "suite-other-alias"
	testHost    = "localhost:5000"
)

// Suite describes the type a native handler serves, for Run.
type Suite struct {
	// TypeName is the type, such as AWS::DocDB::DBCluster.
	TypeName string

	// Schema is the type's schema in CloudFormation format, as it is in pkg/aws/schemas. HandWritten is true when the
	// type has no CloudFormation type.
	Schema      string
	HandWritten bool

	// NewHandler returns the handler under test. It keeps its resources in store, the in-memory AWS the suite's Cloud
	// Control also uses, so the suite can serve the type with either handler.
	NewHandler func(store *awsfake.CloudControl) native.Handler

	// IdentifierProperty is the property whose value is the resource's identifier, and WriteOnly lists the top-level
	// write-only properties. The suite's Cloud Control uses them like AWS does: it assigns an identifier when
	// IdentifierProperty isn't set, and never returns write-only values.
	IdentifierProperty string
	WriteOnly          []string

	// Properties are a template that creates a resource whose identifier is Identifier, or "" when AWS assigns it.
	Properties map[string]any
	Identifier string

	// Changed is Properties with a property the handler can update changed.
	Changed map[string]any

	// ChangedWriteOnly is Properties with a write-only value changed, or nil when the type has none.
	ChangedWriteOnly map[string]any

	// Removable is a top-level property of Properties that can be removed after create, or "" when there is none.
	Removable string

	// CreateOnlyChange is Properties with a create-only property changed, or nil when the type has none.
	CreateOnlyChange map[string]any

	// Metadata is the type's metadata file, as it is in pkg/aws/schemas, or "" when the type has none.
	Metadata string

	// ResetTo is what the resource reads as after a delete resets it, for a type whose onDelete is reset, or nil when
	// it then reads as absent.
	ResetTo map[string]any

	// OtherForm holds values of Properties as AWS returns them, in another form than the template sets them, such as
	// EngineVersion 5.0.0 for 5.0, or nil when there are none. They must not count as changes.
	OtherForm map[string]any

	// OtherProduct puts a resource of another product in store under identifier, replacing any resource there, when
	// the service API also serves other products, such as Aurora clusters for DocumentDB. It is nil otherwise.
	OtherProduct func(store *awsfake.CloudControl, identifier string)

	// MultiCall is Properties changed so the handler's update makes Calls AWS calls. FailCall makes the next update
	// of handler, the handler NewHandler returned, fail with err at its call-th AWS call, counting from 0, after the
	// calls before it took effect, or after every call when call is Calls. They are unset when every update makes one
	// call.
	MultiCall map[string]any
	Calls     int
	FailCall  func(handler native.Handler, call int, err error)
}

// Run runs every case of the suite for s.
func Run(t *testing.T, s Suite) {
	t.Run("create, read, update, and delete", func(t *testing.T) { s.createReadUpdateDelete(t) })
	t.Run("redeploy with no change", func(t *testing.T) { s.redeployNoChange(t) })
	t.Run("deleted outside Radius", func(t *testing.T) { s.deletedOutsideRadius(t) })
	t.Run("removed property", func(t *testing.T) { s.removedProperty(t) })
	t.Run("create-only change", func(t *testing.T) { s.createOnlyChange(t) })
	t.Run("unchanged and changed write-only values", func(t *testing.T) { s.writeOnlyValues(t) })
	t.Run("no write-only value in any response", func(t *testing.T) { s.noWriteOnlyInResponses(t) })
	t.Run("list", func(t *testing.T) { s.list(t) })
	t.Run("existing", func(t *testing.T) { s.existing(t) })
	t.Run("worker restart while waiting", func(t *testing.T) { s.workerRestart(t) })
	t.Run("delete as the metadata says", func(t *testing.T) { s.deleteByMetadata(t) })
	t.Run("identifier another alias holds in the identity scope", func(t *testing.T) { s.identifierHeld(t) })
	t.Run("value AWS returns in another form", func(t *testing.T) { s.otherForm(t) })
	t.Run("identifier of another product", func(t *testing.T) { s.otherProduct(t) })
	t.Run("update fails between calls", func(t *testing.T) { s.multiCall(t) })
	nameProperty := s.NewHandler(awsfake.NewCloudControl()).Options().NameProperty
	for _, reached := range []bool{true, false} {
		suffix := map[bool]string{true: "after the create reached AWS", false: "before the create reached AWS"}[reached]
		t.Run("deploy again "+suffix, func(t *testing.T) { s.lostCreate(t, reached) })
		t.Run("delete "+suffix, func(t *testing.T) { s.lostCreateDelete(t, reached) })
	}
	t.Run("deploy a changed write-only value after a lost create", func(t *testing.T) { s.lostCreateChangedWriteOnly(t) })
	if s.NewHandler(awsfake.NewCloudControl()).Options().Recovery == native.RecoverByResend {
		t.Run("deploy after the create sent again fails partway", func(t *testing.T) { s.resendFailsPartway(t, http.MethodPut, errPartway) })
		t.Run("delete after the create sent again fails partway", func(t *testing.T) { s.resendFailsPartway(t, http.MethodDelete, errPartway) })
		t.Run("deploy after the create sent again is partly applied", func(t *testing.T) { s.resendFailsPartway(t, http.MethodPut, failurePartway) })
		t.Run("delete after the create sent again is partly applied", func(t *testing.T) { s.resendFailsPartway(t, http.MethodDelete, failurePartway) })
	}
	if nameProperty != "" {
		t.Run("generated name is the same on a retry", func(t *testing.T) { s.generatedNameRetry(t, nameProperty) })
	}
	if s.HandWritten {
		t.Run("never asks CloudFormation", func(t *testing.T) { s.neverAsksCloudFormation(t) })
	} else {
		t.Run("create natively, then use Cloud Control", func(t *testing.T) { s.switchHandlers(t, true, "") })
		t.Run("create with Cloud Control, then use the native handler", func(t *testing.T) { s.switchHandlers(t, false, "") })
		if nameProperty != "" {
			t.Run("create natively without the name, then use Cloud Control", func(t *testing.T) { s.switchHandlers(t, true, nameProperty) })
			t.Run("create with Cloud Control without the name, then use the native handler", func(t *testing.T) { s.switchHandlers(t, false, nameProperty) })
		}
		t.Run("DescribeType fails", func(t *testing.T) { s.describeTypeFails(t) })
	}
}

func (s Suite) createReadUpdateDelete(t *testing.T) {
	h := s.newHarness(t)
	h.deploy(s.Properties)
	requireSubset(t, s.readable(s.Properties), h.stored())

	resp := h.get()
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(s.Properties), resp.properties())

	h.deploy(s.Changed)
	requireSubset(t, s.readable(s.Changed), h.stored())

	before := h.stored()
	h.remove()
	h.requireRemoved(before)
	if h.record() == nil {
		require.Equal(t, http.StatusNotFound, h.get().status)
	}
	require.Empty(t, h.cc.Calls(), "the native handler serves every call")
}

func (s Suite) redeployNoChange(t *testing.T) {
	h := s.newHarness(t)
	h.deploy(s.Properties)
	h.handler.reset()

	resp := h.put(s.Properties)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"Read"}, h.handler.reset(), "an unchanged redeploy only reads AWS")
}

func (s Suite) deletedOutsideRadius(t *testing.T) {
	h := s.newHarness(t)
	h.deploy(s.Properties)
	h.cc.Remove(s.TypeName, h.identifier())

	h.deploy(s.Properties)
	requireSubset(t, s.readable(s.Properties), h.stored())
	require.Equal(t, 1, h.cc.Count(s.TypeName))
}

func (s Suite) removedProperty(t *testing.T) {
	if s.Removable == "" {
		t.Skip("the type has no property that can be removed")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)

	without := maps.Clone(s.Properties)
	delete(without, s.Removable)
	h.deploy(without)
	require.NotContains(t, h.stored(), s.Removable)
}

func (s Suite) createOnlyChange(t *testing.T) {
	if s.CreateOnlyChange == nil {
		t.Skip("the type has no create-only property")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	before := h.stored()

	resp := h.put(s.CreateOnlyChange)
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.Equal(t, before, h.stored(), "a rejected update changes nothing")
	require.Equal(t, v1.ProvisioningStateSucceeded, h.record().ProvisioningState())
}

// writeOnlyValues redeploys an unchanged write-only value, which UCP doesn't send, and a changed one, which it does.
func (s Suite) writeOnlyValues(t *testing.T) {
	if s.ChangedWriteOnly == nil {
		t.Skip("the type has no write-only property")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	h.handler.reset()

	resp := h.put(s.Properties)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"Read"}, h.handler.reset(), "an unchanged write-only value isn't sent")

	h.deploy(s.ChangedWriteOnly)
	require.Contains(t, h.handler.reset(), "Update")
	resource := h.cc.Resource(s.TypeName, h.identifier())
	for _, p := range s.WriteOnly {
		require.EqualValues(t, normalize(t, s.ChangedWriteOnly[p]), normalize(t, resource[p]), "AWS has the changed %s", p)
	}

	resp = h.put(s.ChangedWriteOnly)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"Read"}, h.handler.reset())
}

// noWriteOnlyInResponses checks every response for the type, and the record, for the write-only values the templates
// set, including any the handler's read returns.
func (s Suite) noWriteOnlyInResponses(t *testing.T) {
	h := s.newHarness(t)
	secrets := s.writeOnlyStrings(t)
	if len(secrets) == 0 {
		t.Skip("the templates set no write-only value")
	}
	responses := []response{}
	check := func(resp response) {
		t.Helper()
		require.Less(t, resp.status, http.StatusBadRequest, resp.raw)
		responses = append(responses, resp)
	}

	check(h.put(s.Properties))
	operationID := h.record().Properties.RequestToken
	check(h.operation("operationStatuses", operationID))
	check(h.operation("operationResults", operationID))
	h.finish()
	h.requireDeployed()
	check(h.operation("operationStatuses", operationID))
	check(h.get())
	check(h.existingResponse())
	if h.handler.Options().List == native.ListRegion {
		check(h.do(awsproxy.NewListAWSResources, http.MethodGet, h.collection(), "", nil))
	}
	check(h.put(s.Properties))
	for _, changed := range []map[string]any{s.Changed, s.ChangedWriteOnly} {
		if changed != nil {
			check(h.put(changed))
			h.finish()
		}
	}
	raw, err := json.Marshal(h.record())
	require.NoError(t, err)
	responses = append(responses, response{raw: string(raw)})
	responses = append(responses, h.remove())

	for _, resp := range responses {
		for _, secret := range secrets {
			require.NotContains(t, resp.raw, secret)
		}
	}
}

// writeOnlyStrings returns the write-only values the suite's templates set, as they appear in JSON.
func (s Suite) writeOnlyStrings(t *testing.T) []string {
	out := []string{}
	for _, properties := range []map[string]any{s.Properties, s.Changed, s.ChangedWriteOnly} {
		for _, p := range s.WriteOnly {
			if v, ok := properties[p]; ok {
				b, err := json.Marshal(v)
				require.NoError(t, err)
				out = append(out, strings.Trim(string(b), `"`))
			}
		}
	}
	return out
}

func (s Suite) list(t *testing.T) {
	h := s.newHarness(t)
	h.deploy(s.Properties)

	resp := h.do(awsproxy.NewListAWSResources, http.MethodGet, h.collection(), "", nil)
	switch h.handler.Options().List {
	case native.ListRegion:
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		items, _ := resp.body["value"].([]any)
		require.Len(t, items, 1)
		id, _ := items[0].(map[string]any)["id"].(string)
		require.Equal(t, h.identifier(), requireSegment(t, h.collection(), id))
	default:
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	}
}

func (s Suite) existing(t *testing.T) {
	h := s.newHarness(t)
	h.deploy(s.Properties)

	resp := h.existingResponse()
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(s.Properties), resp.properties())

	// Recipes read an `existing` output resource again by the last segment of its ID, so the ID must give back the
	// identifier.
	id, _ := resp.body["id"].(string)
	require.Equal(t, h.identifier(), requireSegment(t, h.collection(), id))
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)
	resp = h.getWithPost(map[string]any{"name": parsed.Name()})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(s.Properties), resp.properties())
}

// requireSegment requires id to be one resource ID segment under collection, and returns the AWS identifier that
// segment holds.
func requireSegment(t *testing.T, collection string, id string) string {
	t.Helper()
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)
	require.Equal(t, collection+"/"+parsed.Name(), id, "the identifier is one segment of the resource ID")
	identifier, err := url.PathUnescape(parsed.Name())
	require.NoError(t, err)
	return identifier
}

// lostCreate loses a create, after it reached AWS or before, and deploys again. The next deploy finds the resource
// as the handler declares (native.Options.Recovery), and AWS ends with exactly one.
func (s Suite) lostCreate(t *testing.T, reached bool) {
	h := s.newHarness(t)
	h.handler.lose("Create", reached)
	resp := h.put(s.Properties)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	require.True(t, records.IsFirstCreate(h.record()))

	h.deployEventually(s.Properties)
	require.Equal(t, 1, h.cc.Count(s.TypeName))
	requireSubset(t, s.readable(s.Properties), h.stored())
	if s.Identifier != "" {
		require.Equal(t, s.Identifier, h.record().Properties.AWSIdentifier)
	}
}

// lostCreateChangedWriteOnly loses a create that reached AWS, and deploys again with a changed write-only value. UCP
// finds the first resource, which may have the first value, so it sends the new one, and AWS ends with it.
func (s Suite) lostCreateChangedWriteOnly(t *testing.T) {
	if s.ChangedWriteOnly == nil {
		t.Skip("the type has no write-only property")
	}
	h := s.newHarness(t)
	h.handler.lose("Create", true)
	resp := h.put(s.Properties)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)

	h.deployEventually(s.ChangedWriteOnly)
	require.Equal(t, 1, h.cc.Count(s.TypeName))
	resource := h.cc.Resource(s.TypeName, h.identifier())
	for _, p := range s.WriteOnly {
		require.EqualValues(t, normalize(t, s.ChangedWriteOnly[p]), normalize(t, resource[p]), "AWS has the changed %s", p)
	}
	resp = h.put(s.ChangedWriteOnly)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
}

// resendFailsPartway loses a create that reached AWS, and then every create sent again returns the resource's
// identifier with an error, as when a later call of a multi-call create fails, such as setting an endpoint's policy.
// A PUT or a DELETE sends the create again and fails. UCP keeps the identifier, so a following DELETE deletes the
// resource instead of sending the failing create again.
func (s Suite) resendFailsPartway(t *testing.T, method string, failure error) {
	h := s.newHarness(t)
	h.handler.lose("Create", true)
	resp := h.put(s.Properties)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	require.Equal(t, 1, h.cc.Count(s.TypeName))

	h.handler.failCreatesPartway(failure)
	if method == http.MethodPut {
		resp = h.put(s.Properties)
	} else {
		resp = h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
	}
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.NotEmpty(t, h.record().Properties.AWSIdentifier, "UCP keeps the identifier the failed create returned")

	h.handler.reset()
	h.removeEventually()
	require.NotContains(t, h.handler.reset(), "Create", "the delete reads AWS by the identifier instead of creating again")
	require.Nil(t, h.record())
	require.Zero(t, h.cc.Count(s.TypeName))
}

// lostCreateDelete loses a create, after it reached AWS or before, and deletes the alias. The delete finds the
// resource first, and AWS ends with none.
//
// For a type whose onDelete is reject, the delete removes only the record when the create never reached AWS, and
// otherwise gets 400 and keeps the resource and the record.
func (s Suite) lostCreateDelete(t *testing.T, reached bool) {
	h := s.newHarness(t)
	od := h.onDelete()
	if od == schemas.OnDeleteNothing || od == schemas.OnDeleteLeave {
		t.Skipf("a delete of the type calls nothing in AWS (onDelete %s), so it never looks for a lost create", od)
	}
	h.handler.lose("Create", reached)
	resp := h.put(s.Properties)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)

	if od != schemas.OnDeleteReject {
		h.removeEventually()
		h.requireRemoved(nil)
		return
	}
	for range maxAttempts {
		resp = h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
		h.finish()
		if resp.status != http.StatusConflict {
			break
		}
	}
	if !reached {
		require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
		require.Nil(t, h.record())
		return
	}
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.NotNil(t, h.record())
	require.Equal(t, 1, h.cc.Count(s.TypeName))
	called := h.handler.reset()
	require.NotContains(t, called, "Delete")
	require.NotContains(t, called, "Reset")
}

// generatedNameRetry loses a create of a template that leaves out the name, and deploys again. Both attempts send
// the same generated name, which fits the type's name rules, and AWS ends with one resource.
func (s Suite) generatedNameRetry(t *testing.T, nameProperty string) {
	h := s.newHarness(t)
	unnamed := without(s.Properties, nameProperty)
	h.handler.lose("Create", true)
	resp := h.put(unnamed)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	first := h.record().Properties.AWSIdentifier
	h.requireGeneratedName(first)

	h.deployEventually(unnamed)
	require.Equal(t, first, h.record().Properties.AWSIdentifier)
	require.Equal(t, 1, h.cc.Count(s.TypeName))
	require.Equal(t, first, h.cc.Resource(s.TypeName, first)[nameProperty])
}

// workerRestart stops the worker while it waits for a create, and runs it again. Waiting only reads AWS, so the
// second run finishes the create without calling Create again.
func (s Suite) workerRestart(t *testing.T) {
	h := s.newHarness(t)
	h.handler.holdProgress(true)
	resp := h.put(s.Properties)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.Contains(t, h.handler.reset(), "Create")

	ctx, cancel := context.WithCancel(t.Context())
	h.handler.onProgress = cancel
	msg, req := h.dequeue()
	_, err := h.run(ctx, req)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, records.IsRunning(h.record()), "the create is still in progress after the worker stops")

	h.handler.onProgress = nil
	h.handler.holdProgress(false)
	result, err := h.run(t.Context(), req)
	require.NoError(t, err)
	require.Nil(t, result.Error)
	require.NoError(t, h.queue.FinishMessage(t.Context(), msg))
	require.Equal(t, v1.ProvisioningStateSucceeded, h.record().ProvisioningState())
	require.NotContains(t, h.handler.reset(), "Create", "the restarted worker does not create again")
}

// switchHandlers creates a resource with one handler, and then reads, updates, and deletes it with the other, as when
// a region gains or loses Cloud Control support for the type. When nameProperty is set, the templates leave it out:
// the native handler generates a name that fits the type's rules, and Cloud Control names the resource itself.
func (s Suite) switchHandlers(t *testing.T, nativeFirst bool, nameProperty string) {
	h := s.newHarness(t)
	properties, changed := s.Properties, s.Changed
	if nameProperty != "" {
		properties, changed = without(properties, nameProperty), without(changed, nameProperty)
	}
	h.useNative(nativeFirst)
	h.deploy(properties)
	switch {
	case nameProperty != "" && nativeFirst:
		h.requireGeneratedName(h.identifier())
	case nameProperty == "" && s.Identifier != "":
		require.Equal(t, s.Identifier, h.record().Properties.AWSIdentifier)
	}

	h.useNative(!nativeFirst)
	h.handler.reset()
	h.cc.Calls()

	resp := h.get()
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(properties), resp.properties())

	h.deploy(changed)
	requireSubset(t, s.readable(changed), h.stored())

	before := h.stored()
	h.remove()
	h.requireRemoved(before)
	if nativeFirst {
		require.Empty(t, h.handler.reset(), "Cloud Control serves every call after the switch")
	} else {
		require.Empty(t, h.cc.Calls(), "the native handler serves every call after the switch")
	}
}

// deleteByMetadata deletes a deployed resource, and checks that the delete does what the type's onDelete metadata says.
func (s Suite) deleteByMetadata(t *testing.T) {
	h := s.newHarness(t)
	h.deploy(s.Properties)
	before, rec := h.stored(), h.record()
	h.handler.reset()

	resp := h.remove()
	called := h.handler.reset()
	h.requireRemoved(before)
	switch h.onDelete() {
	case schemas.OnDeleteDelete:
		require.Contains(t, called, "Delete")
		require.NotContains(t, called, "Reset")
	case schemas.OnDeleteReset:
		require.Contains(t, called, "Reset")
		require.NotContains(t, called, "Delete")
	case schemas.OnDeleteNothing:
		require.Empty(t, called, "the delete calls nothing in AWS")
	case schemas.OnDeleteLeave:
		require.Empty(t, called, "the delete calls nothing in AWS")
		require.Equal(t, rec, h.record(), "the record is kept as it was")
		resp := h.put(s.Properties)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, []string{"Read"}, h.handler.reset(), "a later deploy finds the resource and changes nothing")
	case schemas.OnDeleteReject:
		require.Equal(t, []string{"Read"}, called, "the delete only reads AWS")
		require.Equal(t, rec, h.record(), "the record is kept as it was")
		require.Contains(t, resp.raw, "can't delete")
	}
}

// identifierHeld deploys the template under another alias: in another region for a type whose identity scope is the
// account, and in the same region otherwise. The first alias holds the identifier, so the deploy gets 409 naming it,
// and nothing changes in AWS.
func (s Suite) identifierHeld(t *testing.T) {
	if s.Identifier == "" {
		t.Skip("AWS assigns the identifier, so a template can't name one another alias holds")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	before := h.stored()
	h.handler.reset()

	otherScope := region
	if h.types.Lookup(s.TypeName).Metadata.IdentityScope == schemas.IdentityScopeAccount {
		otherScope = otherRegion
	}
	other := strings.Replace(h.path(), "/regions/"+region+"/", "/regions/"+otherScope+"/", 1)
	other = strings.TrimSuffix(other, alias) + otherAlias
	resp := h.putAt(other, s.Properties)
	require.Equal(t, http.StatusConflict, resp.status, resp.raw)
	require.Contains(t, resp.raw, alias)
	require.Subset(t, []string{"Read"}, h.handler.reset(), "UCP only reads AWS")
	require.Equal(t, before, h.stored())
	require.Nil(t, h.recordAt(other))
}

// otherForm makes AWS return values of the template in another form, and deploys the template again. The values
// don't count as changes, so UCP only reads AWS.
func (s Suite) otherForm(t *testing.T) {
	if s.OtherForm == nil {
		t.Skip("AWS returns every value in the form the template sets")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	ops := []map[string]any{}
	for property, value := range s.OtherForm {
		ops = append(ops, map[string]any{"op": "replace", "path": "/" + property, "value": value})
	}
	require.NoError(t, h.cc.Patch(s.TypeName, h.identifier(), ops))
	h.handler.reset()

	resp := h.put(s.Properties)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"Read"}, h.handler.reset(), "a value in another form is not a change")
}

// otherProduct puts a resource of another product under the identifier: first before any deploy, then in place of
// the deployed resource. A PUT, GET, or `existing` fails with 409 naming the identifier, and nothing changes in AWS or
// in the record. A DELETE treats the identifier as not found: it removes only the record and leaves the other
// product's resource as it is.
func (s Suite) otherProduct(t *testing.T) {
	if s.OtherProduct == nil {
		t.Skip("the service API serves no other product")
	}
	if s.Identifier != "" {
		h := s.newHarness(t)
		s.OtherProduct(h.cc, s.Identifier)
		before := h.cc.Resource(s.TypeName, s.Identifier)
		resp := h.put(s.Properties)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.raw, s.Identifier)
		require.Nil(t, h.record())
		require.Equal(t, before, h.cc.Resource(s.TypeName, s.Identifier))
		require.NotContains(t, h.handler.reset(), "Create")
	}

	h := s.newHarness(t)
	h.deploy(s.Properties)
	identifier := h.identifier()
	s.OtherProduct(h.cc, identifier)
	before, rec := h.cc.Resource(s.TypeName, identifier), h.record()
	h.handler.reset()

	responses := []response{h.put(s.Properties), h.get(), h.existingResponse()}
	for _, resp := range responses {
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.raw, identifier)
	}
	if h.handler.Options().List == native.ListRegion {
		resp := h.do(awsproxy.NewListAWSResources, http.MethodGet, h.collection(), "", nil)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.NotContains(t, resp.raw, identifier, "a list leaves out other products")
	}
	require.NotContains(t, h.handler.reset(), "Update")
	require.Equal(t, before, h.cc.Resource(s.TypeName, identifier), "UCP changed nothing in AWS")
	after := h.record()
	require.Equal(t, rec.Properties.AWSIdentifier, after.Properties.AWSIdentifier)
	require.Equal(t, rec.Properties.LastApplied, after.Properties.LastApplied)
	require.Equal(t, v1.ProvisioningStateSucceeded, after.ProvisioningState())

	resp := h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
	h.finish()
	require.Equal(t, http.StatusNoContent, resp.status, resp.raw)
	if h.onDelete() == schemas.OnDeleteLeave {
		require.NotNil(t, h.record())
	} else {
		require.Nil(t, h.record(), "the resource the alias managed is gone, so the record is removed")
	}
	require.Equal(t, before, h.cc.Resource(s.TypeName, identifier), "UCP never touches the other product's resource")
}

// multiCall fails an update that makes several AWS calls before each call and after the last one, with an error
// AWS returns and with a lost response, and deploys again. The record keeps the pending values as possibly applied,
// except when AWS rejected the first call, and makes them last-applied when every call took effect. The next deploy
// ends fully applied.
func (s Suite) multiCall(t *testing.T) {
	if s.MultiCall == nil {
		t.Skip("every update makes one AWS call")
	}
	failures := []struct {
		name string
		err  error
	}{
		{"AWS rejects it", nativefake.ClientError("ValidationException", "the value is invalid")},
		{"its response is lost", errLost},
	}
	for call := 0; call <= s.Calls; call++ {
		for _, failure := range failures {
			name := fmt.Sprintf("call %d of %d fails, %s", call+1, s.Calls, failure.name)
			if call == s.Calls {
				name = "a call after the last change fails, " + failure.name
			}
			t.Run(name, func(t *testing.T) {
				h := s.newHarness(t)
				h.deploy(s.Properties)
				before := h.record().Properties.LastApplied
				s.FailCall(h.handler.Handler, call, failure.err)

				resp := h.put(s.MultiCall)
				require.GreaterOrEqual(t, resp.status, http.StatusBadRequest, resp.raw)
				applied := h.record().Properties.LastApplied
				switch {
				case call == 0 && failure.err != errLost:
					require.Equal(t, before, applied, "AWS rejected the first call, so the pending values are dropped")
				case call == s.Calls:
					require.Equal(t, normalize(t, s.readable(s.MultiCall)), normalize(t, applied), "every call took effect, so the pending values are last-applied")
				default:
					requireSubset(t, s.readable(s.MultiCall), applied)
					for property := range before {
						require.Contains(t, applied, property, "the values of the last deploy stay, and the pending ones are added as possibly applied")
					}
				}

				h.deploy(s.MultiCall)
				requireSubset(t, s.readable(s.MultiCall), h.stored())
				require.Equal(t, normalize(t, s.readable(s.MultiCall)), normalize(t, h.record().Properties.LastApplied))
				for property := range s.Properties {
					if _, ok := s.MultiCall[property]; !ok {
						require.NotContains(t, h.stored(), property, "the redeploy clears a property the template removed")
					}
				}
				h.handler.reset()
				resp = h.put(s.MultiCall)
				require.Equal(t, http.StatusOK, resp.status, resp.raw)
				require.Equal(t, []string{"Read"}, h.handler.reset(), "the operation is fully applied")
			})
		}
	}
}

func (s Suite) describeTypeFails(t *testing.T) {
	h := s.newHarness(t)
	h.cf.Schemas = map[string]string{}

	resp := h.put(s.Properties)
	require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
	require.Contains(t, resp.raw, "no schema")
	require.Empty(t, h.handler.reset(), "UCP never guesses the handler")
	require.Empty(t, h.cc.Calls())
	require.Nil(t, h.record())
}

func (s Suite) neverAsksCloudFormation(t *testing.T) {
	h := s.newHarness(t)
	h.cf.Schemas = map[string]string{}
	h.deploy(s.Properties)
	require.Equal(t, http.StatusOK, h.get().status)
	h.remove()
	require.Empty(t, h.cc.Calls())
}

// readable returns properties without the write-only ones, which UCP never returns.
func (s Suite) readable(properties map[string]any) map[string]any {
	out := maps.Clone(properties)
	for _, p := range s.WriteOnly {
		delete(out, p)
	}
	return out
}

func without(properties map[string]any, property string) map[string]any {
	out := maps.Clone(properties)
	delete(out, property)
	return out
}

func requireSubset(t *testing.T, want map[string]any, got map[string]any) {
	t.Helper()
	for k, v := range want {
		require.Contains(t, got, k)
		require.EqualValues(t, normalize(t, v), normalize(t, got[k]), k)
	}
}

func normalize(t *testing.T, v any) any {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var out any
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

// harness runs UCP's AWS resource controllers and worker for one suite case.
type harness struct {
	t       *testing.T
	s       Suite
	cc      *awsfake.CloudControl
	cf      *awsfake.CloudFormation
	handler *recorder
	types   *native.Registry
	clients ucp_aws.Clients
	db      database.Client
	queue   *queueinmemory.Client
	sm      statusmanager.StatusManager
	key     *ucp_aws.FingerprintKey

	// lastID is the AWS identifier the last deploy saved, which AWS or UCP may have chosen.
	lastID string
}

func (s Suite) newHarness(t *testing.T) *harness {
	ucp_aws.MinPollInterval, ucp_aws.MaxPollInterval = time.Millisecond, time.Millisecond
	cc := awsfake.NewCloudControl()
	cc.Identifiers[s.TypeName] = s.IdentifierProperty
	cc.WriteOnly[s.TypeName] = s.WriteOnly
	handler := &recorder{Handler: s.NewHandler(cc)}

	registry, err := native.NewRegistry(s.catalog(t), handler.wrap())
	require.NoError(t, err)

	cf := &awsfake.CloudFormation{Schemas: map[string]string{s.TypeName: s.Schema}, ProvisioningTypes: map[string]cftypes.ProvisioningType{}}
	db := inmemory.NewClient()
	q := queueinmemory.New(queueinmemory.NewInMemQueue(time.Minute))
	key, err := ucp_aws.NewFingerprintKey(bytes.Repeat([]byte{7}, ucp_aws.MinFingerprintKeyLength))
	require.NoError(t, err)
	h := &harness{
		t: t, s: s, cc: cc, cf: cf, handler: handler, types: registry,
		clients: ucp_aws.Clients{CloudControl: cc, CloudFormation: cf, Native: registry},
		db:      db, queue: q, sm: statusmanager.New(db, q, "global"), key: key,
	}
	h.useNative(true)
	return h
}

// catalog returns the schema catalog of the type, read as UCP reads pkg/aws/schemas, so the metadata is checked the
// same way.
func (s Suite) catalog(t *testing.T) schemas.Catalog {
	dir, other := schemas.NativeDir, schemas.HandWrittenDir
	if s.HandWritten {
		dir, other = other, dir
	}
	file := strings.ToLower(strings.ReplaceAll(s.TypeName, "::", "-"))
	files := fstest.MapFS{
		dir + "/" + file + ".json": {Data: []byte(s.Schema)},
		other + "/README.md":       {},
	}
	if s.Metadata != "" {
		files[dir+"/"+file+".metadata.json"] = &fstest.MapFile{Data: []byte(s.Metadata)}
	}
	catalog, err := schemas.LoadFS(files)
	require.NoError(t, err)
	return catalog
}

// onDelete returns what a delete of the type does, from its metadata.
func (h *harness) onDelete() schemas.OnDelete {
	return h.types.Lookup(h.s.TypeName).Metadata.OnDelete
}

// requireRemoved requires the result of a delete to be what the type's onDelete metadata says. before is the
// resource before the delete, or nil when there was none.
func (h *harness) requireRemoved(before map[string]any) {
	h.t.Helper()
	switch h.onDelete() {
	case schemas.OnDeleteDelete:
		require.Nil(h.t, h.record())
		require.Nil(h.t, h.stored())
		require.Zero(h.t, h.cc.Count(h.s.TypeName))
	case schemas.OnDeleteReset:
		require.Nil(h.t, h.record())
		if stored := h.stored(); stored != nil {
			require.NotNil(h.t, h.s.ResetTo, "the reset leaves the resource")
			requireSubset(h.t, h.s.ResetTo, stored)
		}
	case schemas.OnDeleteNothing:
		require.Nil(h.t, h.record())
		require.Equal(h.t, before, h.stored(), "the delete changes nothing in AWS")
	default:
		require.NotNil(h.t, h.record(), "the delete keeps the record")
		require.Equal(h.t, before, h.stored(), "the delete changes nothing in AWS")
	}
}

// useNative makes CloudFormation report the type as one Cloud Control can't create, so the native handler serves it,
// or as one it fully manages.
func (h *harness) useNative(on bool) {
	if on {
		h.cf.ProvisioningTypes[h.s.TypeName] = cftypes.ProvisioningTypeNonProvisionable
	} else {
		h.cf.ProvisioningTypes[h.s.TypeName] = cftypes.ProvisioningTypeFullyMutable
	}
}

func (h *harness) collection() string {
	namespace, rest, _ := strings.Cut(strings.TrimPrefix(h.s.TypeName, "AWS::"), "::")
	return scope + "AWS." + namespace + "/" + rest
}

func (h *harness) path() string {
	return h.collection() + "/" + alias
}

// identifier returns the resource's AWS identifier: the one the last deploy saved, or Identifier before any deploy.
func (h *harness) identifier() string {
	if h.lastID != "" {
		return h.lastID
	}
	return h.s.Identifier
}

// requireGeneratedName requires name to be a name UCP generated that fits the type's name rules.
func (h *harness) requireGeneratedName(name string) {
	h.t.Helper()
	rules := h.types.Lookup(h.s.TypeName).NameRules
	require.Regexp(h.t, "^radius-[0-9a-f]{8,12}$", name)
	if rules.MaxLength > 0 {
		require.LessOrEqual(h.t, len(name), rules.MaxLength)
	}
	require.GreaterOrEqual(h.t, len(name), rules.MinLength)
	if rules.Pattern != nil {
		require.Regexp(h.t, rules.Pattern, name)
	}
}

func (h *harness) stored() map[string]any {
	props, ok := h.cc.Read(h.s.TypeName, h.identifier())
	if !ok {
		return nil
	}
	return props
}

func (h *harness) record() *datamodel.AWSResource {
	return h.recordAt(h.path())
}

func (h *harness) recordAt(path string) *datamodel.AWSResource {
	id, err := resources.ParseResource(path)
	require.NoError(h.t, err)
	rec, _, err := records.NewStore(h.db).Get(h.t.Context(), id)
	require.NoError(h.t, err)
	return rec
}

type response struct {
	status int
	body   map[string]any
	raw    string
}

func (r response) properties() map[string]any {
	props, _ := r.body["properties"].(map[string]any)
	return props
}

type controllerFactory func(armrpc_controller.Options, ucp_aws.Clients) (armrpc_controller.Controller, error)

func (h *harness) do(factory controllerFactory, method string, path string, owner string, body any) response {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(h.t, err)
		reader = bytes.NewBuffer(b)
	}
	req, err := http.NewRequest(method, path, reader)
	require.NoError(h.t, err)
	req.Host, req.URL.Host, req.URL.Scheme = testHost, testHost, "http"
	if owner != "" {
		req.Header.Set(sdkclients.AWSResourceOwnerHeader, owner)
	}
	controller, err := factory(armrpc_controller.Options{DatabaseClient: h.db, StatusManager: h.sm}, h.clients)
	require.NoError(h.t, err)
	ctx := rpctest.NewARMRequestContext(req)
	resp, err := controller.Run(ctx, nil, req)
	if err != nil {
		return response{status: http.StatusInternalServerError, raw: err.Error()}
	}
	w := httptest.NewRecorder()
	require.NoError(h.t, resp.Apply(ctx, w, req))
	res := w.Result()
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(h.t, err)
	out := response{status: res.StatusCode, raw: string(raw), body: map[string]any{}}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (h *harness) put(properties map[string]any) response {
	return h.putAt(h.path(), properties)
}

func (h *harness) putAt(path string, properties map[string]any) response {
	put := func(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
		return awsproxy.NewCreateOrUpdateAWSResource(opts, clients, func() (*ucp_aws.FingerprintKey, error) { return h.key, nil })
	}
	return h.do(put, http.MethodPut, path, owner, map[string]any{"properties": properties})
}

func (h *harness) get() response {
	return h.do(awsproxy.NewGetAWSResource, http.MethodGet, h.path(), "", nil)
}

// existingResponse reads the resource as an `existing` block does, by the template's properties and the identifier.
func (h *harness) existingResponse() response {
	properties := maps.Clone(h.s.Properties)
	if h.s.Identifier == "" {
		properties[h.s.IdentifierProperty] = h.identifier()
	}
	return h.getWithPost(map[string]any{"properties": properties})
}

// getWithPost sends POST /:get, as an `existing` block does, with body.
func (h *harness) getWithPost(body map[string]any) response {
	factory := func(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
		return awsproxy.NewGetAWSResourceWithPost(opts, clients, retry.NewNoOpRetryer())
	}
	return h.do(factory, http.MethodPost, h.collection()+"/:get", "", body)
}

// operation reads the status or result of an operation, where kind is operationStatuses or operationResults.
func (h *harness) operation(kind string, operationID string) response {
	namespace, _, _ := strings.Cut(strings.TrimPrefix(h.s.TypeName, "AWS::"), "::")
	factory := func(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
		if kind == "operationResults" {
			return awsproxy.NewGetAWSOperationResults(opts)
		}
		return awsproxy.NewGetAWSOperationStatuses(opts)
	}
	return h.do(factory, http.MethodGet, scope+"AWS."+namespace+"/locations/global/"+kind+"/"+operationID, "", nil)
}

// deploy sends a PUT and finishes the operation it starts.
func (h *harness) deploy(properties map[string]any) {
	h.t.Helper()
	resp := h.put(properties)
	require.Contains(h.t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
	h.finish()
	h.requireDeployed()
}

func (h *harness) requireDeployed() {
	h.t.Helper()
	rec := h.record()
	require.Equal(h.t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
	h.lastID = rec.Properties.AWSIdentifier
}

// maxAttempts is how many times deployEventually and removeEventually send a request that gets 409 because a create
// is still running in AWS.
const maxAttempts = 5

// deployEventually deploys after an interrupted create. While AWS still runs the create, a PUT gets 409, so it
// finishes the operation UCP waits for and deploys again.
func (h *harness) deployEventually(properties map[string]any) {
	h.t.Helper()
	for range maxAttempts {
		resp := h.put(properties)
		h.finish()
		if resp.status != http.StatusConflict {
			require.Contains(h.t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
			h.requireDeployed()
			return
		}
	}
	require.Fail(h.t, "the deploy kept getting 409")
}

// remove sends a DELETE and finishes the operation it starts. The DELETE gets 400 for a type whose onDelete metadata is
// reject.
func (h *harness) remove() response {
	h.t.Helper()
	resp := h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
	if h.onDelete() == schemas.OnDeleteReject {
		require.Equal(h.t, http.StatusBadRequest, resp.status, resp.raw)
	} else {
		require.Contains(h.t, []int{http.StatusAccepted, http.StatusNoContent}, resp.status, resp.raw)
	}
	h.finish()
	return resp
}

// removeEventually deletes after an interrupted create, as deployEventually deploys.
func (h *harness) removeEventually() {
	h.t.Helper()
	for range maxAttempts {
		resp := h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
		h.finish()
		if resp.status != http.StatusConflict {
			require.Contains(h.t, []int{http.StatusAccepted, http.StatusNoContent}, resp.status, resp.raw)
			return
		}
	}
	require.Fail(h.t, "the delete kept getting 409")
}

func (h *harness) dequeue() (*queue.Message, *ctrl.Request) {
	h.t.Helper()
	msg, err := h.queue.Dequeue(h.t.Context(), queue.QueueClientConfig{})
	require.NoError(h.t, err)
	req := &ctrl.Request{}
	require.NoError(h.t, json.Unmarshal(msg.Data, req))
	return msg, req
}

// run runs one queued operation the way UCP's worker does.
func (h *harness) run(ctx context.Context, req *ctrl.Request) (ctrl.Result, error) {
	armCtx, err := req.ARMRequestContext()
	require.NoError(h.t, err)
	controller, err := awsresources.NewAwaitController(ctrl.Options{DatabaseClient: h.db}, h.clients, armCtx.OperationType.Method)
	require.NoError(h.t, err)
	return controller.Run(ctx, req)
}

// finish runs every queued operation and requires each to succeed.
func (h *harness) finish() {
	h.t.Helper()
	for {
		msg, err := h.queue.Dequeue(h.t.Context(), queue.QueueClientConfig{})
		if errors.Is(err, queue.ErrMessageNotFound) {
			return
		}
		require.NoError(h.t, err)
		req := &ctrl.Request{}
		require.NoError(h.t, json.Unmarshal(msg.Data, req))
		result, err := h.run(h.t.Context(), req)
		require.NoError(h.t, err)
		require.Nil(h.t, result.Error, "operation failed")
		require.NoError(h.t, h.queue.FinishMessage(h.t.Context(), msg))
	}
}

// recorder wraps the handler under test. It records the methods called, can hold Progress at "still running", and
// can lose the response of a call.
type recorder struct {
	native.Handler

	mu      sync.Mutex
	methods []string
	hold    bool

	// lost maps a method to whether its next call reaches AWS before its response is lost.
	lost map[string]bool

	// partway, when set, makes every Create call the handler and then return its identifier with this error.
	partway error

	// onProgress, when set, runs on each Progress call.
	onProgress func()
}

func (r *recorder) note(method string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.methods = append(r.methods, method)
}

// reset returns the methods called, except Progress, and clears them.
func (r *recorder) reset() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.methods
	r.methods = nil
	return out
}

func (r *recorder) holdProgress(on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hold = on
}

// lose makes the next call to method fail with a server error, as when UCP stops or the response is lost. When
// reached is true, the call reaches the handler first.
func (r *recorder) lose(method string, reached bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lost == nil {
		r.lost = map[string]bool{}
	}
	r.lost[method] = reached
}

// takeLoss returns, and forgets, the loss set for method.
func (r *recorder) takeLoss(method string) (reached bool, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	reached, ok = r.lost[method]
	delete(r.lost, method)
	return reached, ok
}

var errLost = nativefake.ServerError("InternalFailure", "the response was lost")

// encoder and finder pass the optional methods of the handler under test through a recorder.
type encoder struct {
	r *recorder
}

func (e encoder) Identifier(properties map[string]any) (string, error) {
	return e.r.Handler.(native.IdentifierEncoder).Identifier(properties)
}

type finder struct {
	r *recorder
}

func (f finder) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	f.r.note("Find")
	return f.r.Handler.(native.Finder).Find(ctx, request)
}

// wrap returns the handler the registry gets for r: r, with the optional methods the handler under test has.
func (r *recorder) wrap() native.Handler {
	_, encodes := r.Handler.(native.IdentifierEncoder)
	_, finds := r.Handler.(native.Finder)
	switch {
	case encodes && finds:
		return struct {
			*recorder
			encoder
			finder
		}{r, encoder{r}, finder{r}}
	case encodes:
		return struct {
			*recorder
			encoder
		}{r, encoder{r}}
	case finds:
		return struct {
			*recorder
			finder
		}{r, finder{r}}
	}
	return r
}

func (r *recorder) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	r.note("Read")
	return r.Handler.Read(ctx, region, identifier)
}

// failCreatesPartway makes every later Create return the resource's identifier with err.
func (r *recorder) failCreatesPartway(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.partway = err
}

// errPartway is the plain AWS client error a create that failed partway returns, and failurePartway is the same
// failure as a handler reports it, as partly applied.
var (
	errPartway     = nativefake.ClientError("InvalidParameterValue", "the create made the resource, but a later call failed")
	failurePartway = &native.Failure{Message: "the create made the resource, but a later call failed", ClientError: true, Outcome: native.OutcomePartial}
)

func (r *recorder) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	r.note("Create")
	r.mu.Lock()
	partway := r.partway
	r.mu.Unlock()
	if partway != nil {
		identifier, err := r.Handler.Create(ctx, request)
		return identifier, errors.Join(partway, err)
	}
	reached, lost := r.takeLoss("Create")
	switch {
	case lost && !reached:
		return "", errLost
	case lost:
		_, err := r.Handler.Create(ctx, request)
		return "", errors.Join(errLost, err)
	}
	return r.Handler.Create(ctx, request)
}

func (r *recorder) Update(ctx context.Context, request native.UpdateRequest) error {
	r.note("Update")
	return r.Handler.Update(ctx, request)
}

func (r *recorder) Delete(ctx context.Context, region string, identifier string) error {
	r.note("Delete")
	return r.Handler.Delete(ctx, region, identifier)
}

// Reset passes a reset to the handler under test. The recorder always has Reset, so the registry accepts it for a type
// whose onDelete is reset, and a handler under test without Reset fails here.
func (r *recorder) Reset(ctx context.Context, region string, identifier string) error {
	r.note("Reset")
	resetter, ok := r.Handler.(native.Resetter)
	if !ok {
		return fmt.Errorf("the native handler for %s has no Reset method", r.TypeName())
	}
	return resetter.Reset(ctx, region, identifier)
}

func (r *recorder) List(ctx context.Context, region string) ([]native.Resource, error) {
	r.note("List")
	return r.Handler.List(ctx, region)
}

func (r *recorder) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	r.mu.Lock()
	hold, onProgress := r.hold, r.onProgress
	r.mu.Unlock()
	if onProgress != nil {
		onProgress()
	}
	if hold {
		return native.Progress{State: "in-progress", Waiting: "available"}, nil
	}
	return r.Handler.Progress(ctx, region, identifier, op)
}
