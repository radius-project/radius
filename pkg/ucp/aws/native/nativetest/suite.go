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
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/google/uuid"
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

	// IdentifierProperty is the property whose value is the resource's identifier, and WriteOnly lists the paths of the
	// write-only properties, relative to the properties object with "/" between segments and "*" for every array item,
	// such as MasterUserPassword or Config/*/Password. The suite's Cloud Control uses them like AWS does: it assigns an
	// identifier when IdentifierProperty isn't set, and never returns write-only values.
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

	// RejectedCalls is how many of the first calls of MultiCall only remove members, for a handler whose resource is
	// some members of a parent (native.ScopedReader). Such a handler reports a failure in them as rejected, even when
	// the response is lost, so the record keeps every member it may still have to remove.
	RejectedCalls int

	// CloudControlCreate, when set, returns the identifier and properties the suite's Cloud Control gives a new
	// resource, for a type whose identifier includes a value AWS assigns, such as a Client VPN association's
	// AssociationId. When it is nil, the suite's Cloud Control names a resource with the handler's identifier
	// encoding (native.IdentifierEncoder) when it has one, and with IdentifierProperty otherwise.
	CloudControlCreate func(properties map[string]any) (string, map[string]any)

	// IdentifierProperties, when set, returns the properties that give identifier, for `existing` of a type whose
	// identifier joins several properties. When it is nil, `existing` sets IdentifierProperty to the identifier.
	IdentifierProperties func(identifier string) map[string]any

	// NoCloudControl is true when the type's CloudFormation schema has no handlers in any region, so Cloud Control
	// can't create it anywhere, and the cases that switch between handlers don't apply. A handler whose identifier
	// UCP can't rebuild from what Cloud Control returns sets it.
	NoCloudControl bool

	// Outside adds a member to the parent that properties name, as if someone added it outside Radius, for a handler
	// whose resource is some members of a parent (native.ScopedReader). It returns whether the member is still there.
	// It is nil for other handlers.
	Outside func(store *awsfake.CloudControl, properties map[string]any) func() bool

	// Duplicates is true when AWS lets more than one resource have the values the handler's Find searches by, for a
	// handler whose Recovery is native.RecoverBySearch.
	Duplicates bool

	// RecordedForm, when set, is a create-only property whose template value AWS reports in a form only a record of
	// the last deploy shows to match, such as a KMS alias AWS reports as the key's ARN.
	RecordedForm *RecordedForm

	// RemovedItemField, when set, is a field inside an array item that a deploy sets and the next one leaves out, for
	// a type whose Equivalent compares array items only on the values the template sets.
	RemovedItemField *RemovedItemField

	// Invalid are templates the handler rejects as a client failure before any AWS call, such as a value out of the
	// range AWS takes. Each deploy gets 400 and leaves no record.
	Invalid []map[string]any

	// SameResource are other templates for the resource Properties gives, such as one that sets a value AWS defaults
	// to, like the current account. Two aliases must not both own the resource, whichever deploys first.
	SameResource []map[string]any

	// SameResourceNames are other forms of the identifier of the resource Properties gives, which a PUT can send as
	// name to take the resource over. Another alias that sends one with Properties gets 409, as with the identifier.
	SameResourceNames []string

	// SharedMember, for a type whose resource is some members of a parent (native.Options.Members), gives templates
	// for Properties's parent. Two aliases must never both hold a member.
	SharedMember *SharedMember

	// ChangedOutside, when set, is a change to the resource in AWS outside Radius that a deploy of its template must
	// undo, for a type whose Equivalent compares values only on what the template sets.
	ChangedOutside *ChangedOutside

	// API records each AWS call the handler makes, by the name of its AWS SDK client method, such as
	// CreateDBCluster: the call's page in AWS's API reference and the rules AWS documents for it, which the fake must
	// enforce. It is nil for a handler that makes no AWS SDK calls, such as nativefake's.
	API map[string]Call
}

// ChangedOutside is a change to the resource in AWS outside Radius, such as another HLS container than the template's.
type ChangedOutside struct {
	// Properties is the template.
	Properties map[string]any

	// Change returns the resource as AWS has it after the change, given the resource as AWS had it.
	Change func(stored map[string]any) map[string]any

	// Restored reports whether AWS has the template's values again, given the resource as AWS has it.
	Restored func(stored map[string]any) bool
}

// SharedMember gives templates for the parent of a Suite's Properties.
type SharedMember struct {
	// Overlapping lists a member Properties lists, and Disjoint lists none of them.
	Overlapping map[string]any
	Disjoint    map[string]any
}

// RecordedForm is a create-only property AWS reports in another form than the template sets, such as a KMS alias it
// reports as the key's ARN, or a list it adds values to.
type RecordedForm struct {
	// Property is the property, Template and Other are two values the template can set, such as two aliases, and
	// Stored is what AWS reports for Template, and may report for Other, such as the key ARN.
	Property string
	Template any
	Other    any
	Stored   any
}

// RemovedItemField is a field inside an array item, such as an EMR task group's AutoScalingPolicy.
type RemovedItemField struct {
	// Before sets the field, and After is Before without it.
	Before map[string]any
	After  map[string]any

	// Gone reports whether AWS no longer has the field, given the resource as AWS has it.
	Gone func(stored map[string]any) bool
}

// Run runs every case of the suite for s.
func Run(t *testing.T, s Suite) {
	t.Run("create, read, update, and delete", func(t *testing.T) { s.createReadUpdateDelete(t) })
	t.Run("redeploy with no change", func(t *testing.T) { s.redeployNoChange(t) })
	t.Run("deleted outside Radius", func(t *testing.T) { s.deletedOutsideRadius(t) })
	t.Run("removed property", func(t *testing.T) { s.removedProperty(t) })
	t.Run("removed field of an array item", func(t *testing.T) { s.removedItemField(t) })
	t.Run("template the handler rejects", func(t *testing.T) { s.invalid(t) })
	t.Run("another template for a resource another alias holds", func(t *testing.T) { s.sameResource(t, false) })
	t.Run("a template for a resource another alias holds in another form", func(t *testing.T) { s.sameResource(t, true) })
	t.Run("a take-over by a name for a resource another alias holds", s.sameResourceNames)
	t.Run("a member another alias holds", func(t *testing.T) { s.sharedMember(t, false) })
	t.Run("a member another alias holds, deployed first", func(t *testing.T) { s.sharedMember(t, true) })
	t.Run("members another alias doesn't hold", s.disjointMembers)
	t.Run("a delete after a deploy that adds a member another alias holds", s.deleteAfterHeldMember)
	t.Run("a delete of a member both aliases list", s.deleteSharedMember)
	t.Run("deletes of a member both aliases list, started together", s.deleteSharedMemberTogether)
	t.Run("a take-over by name of a parent without it in the properties", s.takeOverMemberByName)
	t.Run("a lost create sent again with a name for another parent", s.resumedCreateByName)
	t.Run("redeploy after a change outside Radius", func(t *testing.T) { s.changedOutside(t, false) })
	t.Run("take over a resource changed outside Radius", func(t *testing.T) { s.changedOutside(t, true) })
	t.Run("create-only change", func(t *testing.T) { s.createOnlyChange(t) })
	t.Run("unchanged and changed write-only values", func(t *testing.T) { s.writeOnlyValues(t) })
	t.Run("no write-only value in any response", func(t *testing.T) { s.noWriteOnlyInResponses(t) })
	t.Run("list", func(t *testing.T) { s.list(t) })
	t.Run("read values have the schema's types", func(t *testing.T) { s.readTypes(t) })
	t.Run("existing", func(t *testing.T) { s.existing(t) })
	t.Run("worker restart while waiting", func(t *testing.T) { s.workerRestart(t) })
	t.Run("delete as the metadata says", func(t *testing.T) { s.deleteByMetadata(t) })
	t.Run("identifier another alias holds in the identity scope", func(t *testing.T) { s.identifierHeld(t) })
	t.Run("value AWS returns in another form", func(t *testing.T) { s.otherForm(t) })
	t.Run("create-only value only a record matches", func(t *testing.T) { s.recordedForm(t) })
	t.Run("identifier of another product", func(t *testing.T) { s.otherProduct(t) })
	t.Run("update fails between calls", func(t *testing.T) { s.multiCall(t) })
	t.Run("member added outside Radius", func(t *testing.T) { s.outsideMember(t) })
	t.Run("update interrupted before its first call", func(t *testing.T) { s.interruptedScopedUpdate(t) })
	t.Run("delete again after the response is lost", func(t *testing.T) { s.deleteTokenRetry(t) })
	t.Run("the fake follows AWS's documented API rules", func(t *testing.T) {
		if s.API == nil {
			t.Skip("the handler has no record of its AWS calls yet")
		}
		requireAPIRules(t, s.API, s.NewHandler(awsfake.NewCloudControl()))
	})
	nameProperty := s.NewHandler(awsfake.NewCloudControl()).Options().NameProperty
	for _, reached := range []bool{true, false} {
		suffix := map[bool]string{true: "after the create reached AWS", false: "before the create reached AWS"}[reached]
		t.Run("deploy again "+suffix, func(t *testing.T) { s.lostCreate(t, reached) })
		t.Run("delete "+suffix, func(t *testing.T) { s.lostCreateDelete(t, reached) })
	}
	t.Run("deploy a changed write-only value after a lost create", func(t *testing.T) { s.lostCreateChangedWriteOnly(t) })
	t.Run("delete fails after it finds a lost create", func(t *testing.T) { s.lostCreateFailedDelete(t) })
	t.Run("delete after the create sent again is lost", func(t *testing.T) { s.lostResend(t) })
	t.Run("create-only change after a lost create", func(t *testing.T) { s.lostResendCreateOnly(t) })
	t.Run("lost create that matches more than one resource", func(t *testing.T) { s.lostCreateDuplicates(t) })
	if s.NewHandler(awsfake.NewCloudControl()).Options().Recovery == native.RecoverByResend {
		t.Run("deploy after the create sent again fails partway", func(t *testing.T) { s.resendFailsPartway(t, http.MethodPut, errPartway) })
		t.Run("delete after the create sent again fails partway", func(t *testing.T) { s.resendFailsPartway(t, http.MethodDelete, errPartway) })
		t.Run("deploy after the create sent again is partly applied", func(t *testing.T) { s.resendFailsPartway(t, http.MethodPut, failurePartway) })
		t.Run("delete after the create sent again is partly applied", func(t *testing.T) { s.resendFailsPartway(t, http.MethodDelete, failurePartway) })
	}
	if nameProperty != "" {
		t.Run("generated name is the same on a retry after the create reached AWS", func(t *testing.T) { s.generatedNameRetry(t, nameProperty, true) })
		t.Run("generated name is the same on a retry before the create reached AWS", func(t *testing.T) { s.generatedNameRetry(t, nameProperty, false) })
	}
	if s.HandWritten {
		t.Run("AWS publishes a type with the same name", func(t *testing.T) { s.sameNameFromAWS(t) })
	} else {
		if !s.NoCloudControl {
			s.runSwitchCases(t, nameProperty)
		}
		t.Run("DescribeType fails", func(t *testing.T) { s.describeTypeFails(t) })
	}
}

// Updatable returns the top-level properties of the schema a deploy can change after create: those that are neither
// read-only nor create-only. A handler's Update must apply each one, or UCP records a change AWS never got.
func Updatable(t *testing.T, schema string) []string {
	var doc struct {
		Properties map[string]any `json:"properties"`
		ReadOnly   []string       `json:"readOnlyProperties"`
		CreateOnly []string       `json:"createOnlyProperties"`
	}
	require.NoError(t, json.Unmarshal([]byte(schema), &doc))
	fixed := map[string]bool{}
	for _, path := range slices.Concat(doc.ReadOnly, doc.CreateOnly) {
		if name, ok := strings.CutPrefix(path, "/properties/"); ok && !strings.Contains(name, "/") {
			fixed[name] = true
		}
	}
	out := []string{}
	for name := range doc.Properties {
		if !fixed[name] {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// runSwitchCases runs the cases that create a resource with one handler, then use the other.
func (s Suite) runSwitchCases(t *testing.T, nameProperty string) {
	t.Run("create natively, then use Cloud Control", func(t *testing.T) { s.switchHandlers(t, true, "") })
	t.Run("create with Cloud Control, then use the native handler", func(t *testing.T) { s.switchHandlers(t, false, "") })
	if nameProperty != "" {
		t.Run("create natively without the name, then use Cloud Control", func(t *testing.T) { s.switchHandlers(t, true, nameProperty) })
		t.Run("create with Cloud Control without the name, then use the native handler", func(t *testing.T) { s.switchHandlers(t, false, nameProperty) })
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

// removedItemField deploys a template with a field inside an array item, then one without it. UCP must remove the
// field in AWS, or reject the deploy with 400 and change nothing, never record the removal while AWS keeps it.
func (s Suite) removedItemField(t *testing.T) {
	f := s.RemovedItemField
	if f == nil {
		t.Skip("the type has no array item field to remove")
	}
	h := s.newHarness(t)
	h.deploy(f.Before)
	require.False(t, f.Gone(h.stored()), "AWS has the field after the first deploy")
	before, applied := h.stored(), h.record().Properties.LastApplied

	resp := h.put(f.After)
	if resp.status == http.StatusBadRequest {
		require.Equal(t, before, h.stored(), "a rejected deploy changes nothing in AWS")
		require.Equal(t, applied, h.record().Properties.LastApplied, "a rejected deploy changes nothing in the record")
		return
	}
	require.Contains(t, []int{http.StatusOK, http.StatusCreated, http.StatusAccepted}, resp.status, resp.raw)
	h.finish()
	h.requireDeployed()
	require.True(t, f.Gone(h.stored()), "the deploy succeeded, so AWS no longer has the field")

	h.handler.reset()
	resp = h.put(f.After)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"Read"}, h.handler.reset(), "the removal is fully applied")
}

// changedOutside changes the resource in AWS outside Radius, and deploys its template: after a deploy, so the record's
// last-applied values are stale, or as a take-over by name, so the record has none. The deploy must undo the change.
func (s Suite) changedOutside(t *testing.T, takeOver bool) {
	c := s.ChangedOutside
	if c == nil {
		t.Skip("the type compares every value it reads")
	}
	h := s.newHarness(t)
	if takeOver {
		identifier, err := h.handler.Handler.Create(t.Context(), native.CreateRequest{Region: region, Properties: c.Properties, ClientToken: uuid.NewString()})
		require.NoError(t, err)
		h.cc.Put(s.TypeName, identifier, c.Change(h.cc.Resource(s.TypeName, identifier)))
		resp := h.do(h.putFactory(), http.MethodPut, h.path(), owner, map[string]any{"name": identifier, "properties": c.Properties})
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
		h.finish()
		h.requireDeployed()
	} else {
		h.deploy(c.Properties)
		h.cc.Put(s.TypeName, h.identifier(), c.Change(h.stored()))
		require.False(t, c.Restored(h.stored()), "the change outside Radius is in AWS")
		h.deploy(c.Properties)
	}
	require.True(t, c.Restored(h.stored()), "the deploy succeeded, so AWS has the template's values")
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
		segments := strings.Split(p, "/")
		require.EqualValues(t, valuesAt(normalizeValue(s.ChangedWriteOnly), segments), valuesAt(normalizeValue(resource), segments), "AWS has the changed %s", p)
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
			for _, v := range valuesAt(normalizeValue(properties), strings.Split(p, "/")) {
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

// readTypes deploys Properties and reads the resource through the handler, and through List when the type can be
// listed. Every value must have the JSON type the schema gives its property, such as a string for a port the schema
// declares as a string, so a read matches what Cloud Control returns for the type.
func (s Suite) readTypes(t *testing.T) {
	h := s.newHarness(t)
	h.deploy(s.Properties)

	reads := map[string]map[string]any{}
	read, err := h.handler.Handler.Read(t.Context(), region, h.identifier())
	require.NoError(t, err)
	reads["Read"] = read
	if h.handler.Options().List == native.ListRegion {
		listed, err := h.handler.Handler.List(t.Context(), region)
		require.NoError(t, err)
		for _, r := range listed {
			reads["List "+r.Identifier] = r.Properties
		}
	}
	for source, properties := range reads {
		requireSchemaTypes(t, s.Schema, properties, source)
	}
}

// requireSchemaTypes requires every value of properties, which source returned, to have the JSON type the schema
// gives its property.
func requireSchemaTypes(t testing.TB, schemaJSON string, properties map[string]any, source string) {
	t.Helper()
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(schemaJSON), &schema))
	var value any
	b, err := json.Marshal(properties)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &value))
	mismatches := []string{}
	schemaTypes(schema, map[string]any{"type": "object", "properties": schema["properties"]}, "", value, &mismatches)
	require.Empty(t, mismatches, "%s returns values whose JSON type doesn't match the schema", source)
}

// schemaTypes adds to mismatches each value under path whose JSON type isn't one the schema definition def allows. It
// follows $ref, properties, patternProperties, and items, and checks nothing a definition doesn't type.
func schemaTypes(schema map[string]any, def map[string]any, path string, value any, mismatches *[]string) {
	if ref, ok := def["$ref"].(string); ok {
		resolved, _ := schema["definitions"].(map[string]any)[strings.TrimPrefix(ref, "#/definitions/")].(map[string]any)
		if resolved == nil {
			*mismatches = append(*mismatches, path+": unknown $ref "+ref)
			return
		}
		def = resolved
	}
	var allowed []string
	switch typ := def["type"].(type) {
	case string:
		allowed = []string{typ}
	case []any:
		for _, v := range typ {
			allowed = append(allowed, fmt.Sprint(v))
		}
	}
	if len(allowed) > 0 && !slices.Contains(allowed, jsonType(value)) && !(jsonType(value) == "integer" && slices.Contains(allowed, "number")) {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: %s, but the schema says %s", path, jsonType(value), strings.Join(allowed, " or ")))
		return
	}
	switch v := value.(type) {
	case map[string]any:
		properties, _ := def["properties"].(map[string]any)
		patterns, _ := def["patternProperties"].(map[string]any)
		for key, item := range v {
			if child, ok := properties[key].(map[string]any); ok {
				schemaTypes(schema, child, path+"/"+key, item, mismatches)
				continue
			}
			for pattern, child := range patterns {
				if childDef, ok := child.(map[string]any); ok && regexp.MustCompile(pattern).MatchString(key) {
					schemaTypes(schema, childDef, path+"/"+key, item, mismatches)
					break
				}
			}
		}
	case []any:
		if items, ok := def["items"].(map[string]any); ok {
			for i, item := range v {
				schemaTypes(schema, items, fmt.Sprintf("%s/%d", path, i), item, mismatches)
			}
		}
	}
}

// jsonType returns the JSON schema type of a value read as JSON.
func jsonType(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if t == float64(int64(t)) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
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
	h.headers = map[string]string{sdkclients.AWSResourceIDHeader: id}
	resp = h.getWithPost(map[string]any{"name": parsed.Name()})
	h.headers = nil
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(s.Properties), resp.properties())
	require.Equal(t, id, resp.body["id"])
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
	if h.record().Properties.AWSIdentifier != "" {
		t.Skip("UCP saves the identifier before the create, so it reads AWS by it instead of sending the create again")
	}

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

// lostCreateDuplicates loses a create that reached AWS, and adds a second resource with the same values. Find then
// matches both, so UCP can't tell which one it made: a deploy and a delete fail, naming both, and keep the record,
// and AWS keeps both resources.
func (s Suite) lostCreateDuplicates(t *testing.T) {
	if !s.Duplicates {
		t.Skip("AWS allows one resource with the values Find searches by")
	}
	h := s.newHarness(t)
	require.Equal(t, native.RecoverBySearch, h.handler.Options().Recovery, "Duplicates is for a handler that recovers by search")
	h.handler.lose("Create", true)
	resp := h.put(s.Properties)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	first := h.cc.List(s.TypeName)
	require.Len(t, first, 1)
	second, err := h.handler.Handler.Create(t.Context(), native.CreateRequest{Region: region, Properties: s.Properties, ClientToken: uuid.NewString()})
	require.NoError(t, err)

	requireAmbiguous := func(resp response) {
		require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
		require.Contains(t, resp.raw, first[0])
		require.Contains(t, resp.raw, second)
		require.True(t, records.IsFirstCreate(h.record()), "the record keeps the create")
		require.Equal(t, 2, h.cc.Count(s.TypeName), "AWS keeps both")
		called := h.handler.reset()
		require.NotContains(t, called, "Delete")
		require.NotContains(t, called, "Create")
	}
	h.handler.reset()
	requireAmbiguous(h.put(s.Properties))
	resp = h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
	h.finish()
	requireAmbiguous(resp)
}

// lostCreateFailedDelete loses a create that reached AWS, and deletes the alias. The delete finds the resource, and
// its call to AWS fails, so the record keeps what the create may have applied. The next delete removes it all, including
// the members of a handler whose resource is some members of a parent (native.ScopedDeleter).
func (s Suite) lostCreateFailedDelete(t *testing.T) {
	h := s.newHarness(t)
	if od := h.onDelete(); od != schemas.OnDeleteDelete {
		t.Skipf("a delete of the type doesn't delete it in AWS (onDelete %s)", od)
	}
	h.handler.lose("Create", true)
	resp := h.put(s.Properties)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)

	h.handler.lose("Delete", false)
	for range maxAttempts {
		resp = h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
		h.finish()
		if resp.status != http.StatusConflict {
			break
		}
	}
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	require.NotNil(t, h.record())

	h.removeEventually()
	h.requireRemoved(nil)
}

// lostResend loses a create that reached AWS, deploys a changed template, and loses the create UCP sends again for
// it too, for a handler whose resource is some members of a parent and that finds a lost create by sending it again.
// Both creates may have added members, so the delete removes the members of both templates.
func (s Suite) lostResend(t *testing.T) {
	if s.Outside == nil {
		t.Skip("the resource is not some members of a parent")
	}
	h := s.resendHarness(t)
	h.handler.lose("Create", true)
	resp := h.put(s.Changed)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)

	h.removeEventually()
	h.requireRemoved(nil)
}

// lostResendCreateOnly loses a create that reached AWS, and deploys a template that changes a create-only property, for
// a handler whose resource is some members of a parent and that finds a lost create by sending it again. Sending the
// create again would add members to another parent, so the deploy gets 400 and changes nothing, and the delete removes
// the lost create's members.
func (s Suite) lostResendCreateOnly(t *testing.T) {
	if s.Outside == nil || s.CreateOnlyChange == nil {
		t.Skip("the resource is not some members of a parent, or the type has no create-only property")
	}
	h := s.resendHarness(t)
	before := h.record().Properties.PendingLastApplied
	h.handler.reset()

	resp := h.put(s.CreateOnlyChange)
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.NotContains(t, h.handler.reset(), "Create", "the create is not sent again")
	require.Equal(t, before, h.record().Properties.PendingLastApplied)

	h.removeEventually()
	h.requireRemoved(nil)
}

// resendHarness returns a harness whose first create reached AWS and was lost, for a handler that finds a lost create
// by sending it again and deletes it in AWS. It skips a handler that saves the identifier before the create, which UCP
// reads AWS by instead.
func (s Suite) resendHarness(t *testing.T) *harness {
	h := s.newHarness(t)
	if h.handler.Options().Recovery != native.RecoverByResend {
		t.Skip("the handler finds a lost create by search")
	}
	if od := h.onDelete(); od != schemas.OnDeleteDelete {
		t.Skipf("a delete of the type doesn't delete it in AWS (onDelete %s)", od)
	}
	h.handler.lose("Create", true)
	resp := h.put(s.Properties)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	if h.record().Properties.AWSIdentifier != "" {
		t.Skip("UCP saves the identifier before the create, so it reads AWS by it instead of sending the create again")
	}
	return h
}

// generatedNameRetry loses a create of a template that leaves out the name, after it reached AWS or before, and
// deploys again. Every attempt sends the same generated name, which fits the type's name rules, the identifier is the
// one the properties with that name give, and AWS ends with one resource.
func (s Suite) generatedNameRetry(t *testing.T, nameProperty string, reached bool) {
	h := s.newHarness(t)
	unnamed := without(s.Properties, nameProperty)
	h.handler.lose("Create", reached)
	resp := h.put(unnamed)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	name := h.requireGeneratedName(nameProperty)
	identifier := h.identifierFor(withValue(unnamed, nameProperty, name))
	require.Equal(t, identifier, h.record().Properties.AWSIdentifier, "the identifier is saved before UCP calls AWS")

	h.deployEventually(unnamed)
	require.Equal(t, name, h.requireGeneratedName(nameProperty), "every attempt sends the same name")
	require.Equal(t, identifier, h.record().Properties.AWSIdentifier)
	require.Equal(t, 1, h.cc.Count(s.TypeName))
	require.Equal(t, name, h.cc.Resource(s.TypeName, identifier)[nameProperty])
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
		name := h.requireGeneratedName(nameProperty)
		require.Equal(t, h.identifierFor(withValue(properties, nameProperty, name)), h.identifier())
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

// invalid deploys each of Invalid, which gets 400 before any AWS call and leaves no record.
func (s Suite) invalid(t *testing.T) {
	if len(s.Invalid) == 0 {
		t.Skip("the type has no template the handler rejects before calling AWS")
	}
	for _, properties := range s.Invalid {
		h := s.newHarness(t)
		h.handler.reset()
		resp := h.put(properties)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Empty(t, h.handler.reset(), "nothing reaches AWS")
		require.Nil(t, h.record())
		require.Zero(t, h.cc.Count(s.TypeName))
	}
}

// sameResource deploys Properties under one alias and each of SameResource under another, or the other way around
// when reversed. Both templates give the same resource, so the second deploy gets 409 and changes nothing, and
// deleting the first alias deletes the resource.
func (s Suite) sameResource(t *testing.T, reversed bool) {
	if len(s.SameResource) == 0 {
		t.Skip("the type has one template form for a resource")
	}
	for i, same := range s.SameResource {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			first, second := s.Properties, same
			if reversed {
				first, second = second, first
			}
			s.deployBoth(t, first, second)
		})
	}
}

// deployBoth deploys first under one alias and second, a template for the same resource, under another.
func (s Suite) deployBoth(t *testing.T, first map[string]any, second map[string]any) {
	h := s.newHarness(t)
	h.deploy(first)
	other := strings.TrimSuffix(h.path(), alias) + otherAlias
	resp := h.putAt(other, second)
	require.Equal(t, http.StatusConflict, resp.status, resp.raw)
	require.Contains(t, resp.raw, alias)
	require.Nil(t, h.recordAt(other))
	require.Equal(t, 1, h.cc.Count(s.TypeName))

	h.remove()
	h.requireRemoved(nil)
}

// sameResourceNames deploys Properties under one alias, and Properties with each of SameResourceNames under another.
func (s Suite) sameResourceNames(t *testing.T) {
	if len(s.SameResourceNames) == 0 {
		t.Skip("the type has one form of identifier for a resource")
	}
	for i, name := range s.SameResourceNames {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			h := s.newHarness(t)
			h.deploy(s.Properties)
			other := strings.TrimSuffix(h.path(), alias) + otherAlias
			resp := h.do(h.putFactory(), http.MethodPut, other, owner, map[string]any{"name": name, "properties": s.Properties})
			require.Equal(t, http.StatusConflict, resp.status, resp.raw)
			require.Nil(t, h.recordAt(other))
			before := h.stored()
			require.NotNil(t, before, "the first alias's resource stays")

			h.remove()
			h.requireRemoved(before)
		})
	}
}

// recordedForm deploys a create-only value, such as a KMS alias, that AWS reports in another form. Deploying the same
// value again changes nothing, because the record shows the template didn't change it. Changing it gets 400, even
// though AWS may report the same form for the new value. Taking the resource over by name gets 400 too, because there
// is no record to show the value unchanged.
func (s Suite) recordedForm(t *testing.T) {
	ref := s.RecordedForm
	if ref == nil {
		t.Skip("the type has no create-only value AWS reports in a form only a record matches")
	}
	resolve := func(h *harness) {
		require.NoError(t, h.cc.Patch(s.TypeName, h.identifier(), []map[string]any{{"op": "replace", "path": "/" + ref.Property, "value": ref.Stored}}))
	}
	template := withValue(s.Properties, ref.Property, ref.Template)

	t.Run("unchanged", func(t *testing.T) {
		h := s.newHarness(t)
		h.deploy(template)
		resolve(h)
		h.handler.reset()
		resp := h.put(template)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, []string{"Read"}, h.handler.reset(), "the same value is no change")
	})

	t.Run("changed", func(t *testing.T) {
		h := s.newHarness(t)
		h.deploy(template)
		resolve(h)
		before, applied := h.stored(), h.record().Properties.LastApplied
		resp := h.put(withValue(s.Properties, ref.Property, ref.Other))
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Contains(t, resp.raw, ref.Property)
		require.Equal(t, before, h.stored(), "nothing changes in AWS")
		require.Equal(t, applied, h.record().Properties.LastApplied, "the record keeps the value AWS has")
	})

	t.Run("taken over by name", func(t *testing.T) {
		h := s.newHarness(t)
		h.deployAt(otherPath(h), template)
		identifier := h.identifier()
		resolve(h)
		id, err := resources.ParseResource(otherPath(h))
		require.NoError(t, err)
		store := records.NewStore(h.db)
		_, etag, err := store.Get(t.Context(), id)
		require.NoError(t, err)
		require.NoError(t, store.DeleteIfUnchanged(t.Context(), id, etag), "the resource has no record, as before a take-over")
		before := h.stored()

		resp := h.putNamed(identifier, template)
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Contains(t, resp.raw, ref.Property)
		require.Equal(t, before, h.stored(), "nothing changes in AWS")
	})
}

// sharedMember deploys Properties under one alias and SharedMember.Overlapping under another, or the other way around
// when reversed. The second deploy gets 409 naming the first alias, changes nothing, and leaves no record.
func (s Suite) sharedMember(t *testing.T, reversed bool) {
	if s.SharedMember == nil {
		t.Skip("the type's resource isn't members of a parent")
	}
	first, second := s.Properties, s.SharedMember.Overlapping
	if reversed {
		first, second = second, first
	}
	h := s.newHarness(t)
	h.deploy(first)
	h.handler.reset()
	resp := h.putAt(otherPath(h), second)
	require.Equal(t, http.StatusConflict, resp.status, resp.raw)
	require.Contains(t, resp.raw, alias)
	require.Empty(t, h.handler.reset(), "nothing reaches AWS")
	require.Nil(t, h.recordAt(otherPath(h)))
	h.requireHeld(first)

	h.remove()
	h.requireRemoved(nil)
}

// disjointMembers deploys Properties under one alias and SharedMember.Disjoint, other members of the same parent,
// under another. Both deploy, and deleting either leaves the other's members.
func (s Suite) disjointMembers(t *testing.T) {
	if s.SharedMember == nil {
		t.Skip("the type's resource isn't members of a parent")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	h.deployAt(otherPath(h), s.SharedMember.Disjoint)
	h.removeAt(otherPath(h))
	h.requireHeld(s.Properties)

	h.deployAt(otherPath(h), s.SharedMember.Disjoint)
	h.remove()
	require.Nil(t, h.record())
	h.requireHeldAt(otherPath(h), s.SharedMember.Disjoint)
	h.removeAt(otherPath(h))
	require.Zero(t, h.cc.Count(s.TypeName))
}

// deleteAfterHeldMember deploys Properties under one alias and SharedMember.Disjoint under another, which then gets
// 409 for SharedMember.Overlapping. Deleting the other alias removes only Disjoint's members.
func (s Suite) deleteAfterHeldMember(t *testing.T) {
	if s.SharedMember == nil {
		t.Skip("the type's resource isn't members of a parent")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	h.deployAt(otherPath(h), s.SharedMember.Disjoint)
	applied := h.recordAt(otherPath(h)).Properties.LastApplied
	resp := h.putAt(otherPath(h), s.SharedMember.Overlapping)
	require.Equal(t, http.StatusConflict, resp.status, resp.raw)
	require.Equal(t, applied, h.recordAt(otherPath(h)).Properties.LastApplied, "the record keeps only its own members")
	require.Nil(t, h.recordAt(otherPath(h)).Properties.PendingLastApplied)

	h.removeAt(otherPath(h))
	h.requireHeld(s.Properties)
	h.remove()
	h.requireRemoved(nil)
}

// deleteSharedMember gives two aliases a member each lists, as two deploys at the same moment can, and deletes them
// one at a time. The first delete leaves the member, which the other alias still lists, and the second removes it.
func (s Suite) deleteSharedMember(t *testing.T) {
	if s.SharedMember == nil {
		t.Skip("the type's resource isn't members of a parent")
	}
	h := s.newHarness(t)
	s.shareMember(h)

	h.removeAt(otherPath(h))
	h.requireHeld(s.Properties)
	h.remove()
	h.requireRemoved(nil)
}

// shareMember gives the alias at otherPath SharedMember.Disjoint, and its record SharedMember.Overlapping, as two
// deploys at the same moment can, so both aliases list a member.
func (s Suite) shareMember(h *harness) {
	h.t.Helper()
	h.deploy(s.Properties)
	h.deployAt(otherPath(h), s.SharedMember.Disjoint)
	id, err := resources.ParseResource(otherPath(h))
	require.NoError(h.t, err)
	store := records.NewStore(h.db)
	rec, etag, err := store.Get(h.t.Context(), id)
	require.NoError(h.t, err)
	rec.Properties.LastApplied = s.SharedMember.Overlapping
	_, err = store.Save(h.t.Context(), rec, etag)
	require.NoError(h.t, err)
}

// deleteSharedMemberTogether deletes two aliases that both list a member, the second before the first finishes. The
// member must not outlive both.
func (s Suite) deleteSharedMemberTogether(t *testing.T) {
	if s.SharedMember == nil {
		t.Skip("the type's resource isn't members of a parent")
	}
	h := s.newHarness(t)
	s.shareMember(h)
	for _, path := range []string{h.path(), otherPath(h)} {
		resp := h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, path, owner, nil)
		require.Contains(t, []int{http.StatusAccepted, http.StatusNoContent}, resp.status, resp.raw)
	}
	h.finish()
	require.Nil(t, h.record())
	require.Nil(t, h.recordAt(otherPath(h)))
	require.Zero(t, h.cc.Count(s.TypeName), "the last delete removes the member both listed")
}

// takeOverMemberByName deploys Properties, then has another alias take over the same parent by a name, with
// SharedMember.Overlapping without the parent property. The handler takes the parent from the name, so the deploy
// gets 409 like one that sets it.
func (s Suite) takeOverMemberByName(t *testing.T) {
	if s.SharedMember == nil {
		t.Skip("the type's resource isn't members of a parent")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	parent, _, ok := strings.Cut(h.identifier(), "|")
	require.True(t, ok, "the identifier is the parent and a name, joined with |")
	properties := maps.Clone(s.SharedMember.Overlapping)
	delete(properties, h.handler.Options().Members.Parent)
	resp := h.do(h.putFactory(), http.MethodPut, otherPath(h), owner, map[string]any{"name": parent + "|other", "properties": properties})
	require.Equal(t, http.StatusConflict, resp.status, resp.raw)
	require.Nil(t, h.recordAt(otherPath(h)))
	h.requireHeld(s.Properties)
}

// resumedCreateByName deploys CreateOnlyChange, the same members of another parent, under one alias. The create of
// Properties under another alias is lost, and a PUT sends it again with a name for CreateOnlyChange's parent. UCP finds
// the lost create in Properties's parent, which the other alias doesn't hold, so the alias keeps every member it
// added, and its delete removes them.
func (s Suite) resumedCreateByName(t *testing.T) {
	if s.SharedMember == nil || s.CreateOnlyChange == nil {
		t.Skip("the type's resource isn't members of a parent")
	}
	h := s.newHarness(t)
	h.deployAt(otherPath(h), s.CreateOnlyChange)
	parent, _, ok := strings.Cut(h.identifier(), "|")
	require.True(t, ok, "the identifier is the parent and a name, joined with |")
	held := h.cc.Count(s.TypeName)

	h.handler.lose("Create", true)
	resp := h.put(s.Properties)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	resp = h.do(h.putFactory(), http.MethodPut, h.path(), owner, map[string]any{"name": parent + "|other", "properties": s.Properties})
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
	h.finish()
	h.requireHeld(s.Properties)

	h.remove()
	require.Nil(t, h.record())
	require.Equal(t, held, h.cc.Count(s.TypeName), "the delete removes every member the alias added, and only those")
	h.requireHeldAt(otherPath(h), s.CreateOnlyChange)
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

// interruptedScopedUpdate stops UCP after it reserves the update to MultiCall and before the first call, for a
// handler whose resource is some members of a parent (native.ScopedReader), and deploys MultiCall again. The record
// still owns the members only the last deploy had, so the redeploy removes them.
func (s Suite) interruptedScopedUpdate(t *testing.T) {
	if s.RejectedCalls == 0 {
		t.Skip("the handler's resource is not some members of a parent, or its update removes none")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	id, err := resources.ParseResource(h.path())
	require.NoError(t, err)
	store := records.NewStore(h.db)
	rec, etag, err := store.Get(t.Context(), id)
	require.NoError(t, err)
	rec.Properties.PendingOperation = records.OperationUpdate
	rec.Properties.PendingLastApplied = s.readable(s.MultiCall)
	rec.SetProvisioningState(records.StateOf(records.OperationUpdate))
	_, err = store.Save(t.Context(), rec, etag)
	require.NoError(t, err)

	defer func() { records.Now = time.Now }()
	records.Now = func() time.Time { return time.Now().Add(records.StaleAfter + time.Minute) }
	h.deploy(s.MultiCall)
	records.Now = time.Now
	requireSubset(t, s.readable(s.MultiCall), h.stored())
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
				case call < s.RejectedCalls:
					require.Equal(t, before, applied, "the call only removed members, so the record keeps every member it may still have to remove")
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

// outsideMember adds a member to the parent outside Radius, before the first deploy, and checks that a take-over, an
// update, and a delete all leave it in place.
func (s Suite) outsideMember(t *testing.T) {
	if s.Outside == nil {
		t.Skip("the resource is not some members of a parent")
	}
	h := s.newHarness(t)
	there := s.Outside(h.cc, s.Properties)
	h.deploy(s.Properties)
	require.True(t, there(), "a take-over keeps a member added outside Radius")

	h.deploy(s.Changed)
	require.True(t, there(), "an update keeps a member added outside Radius")

	resp := h.get()
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(s.Changed), resp.properties())

	h.remove()
	require.Nil(t, h.record())
	require.True(t, there(), "a delete keeps a member added outside Radius")
}

// deleteTokenRetry loses the response of a delete that reached AWS, and deletes again. UCP saved the delete token on
// the record before the first call, and the retry sends the same token, so AWS takes no second copy.
func (s Suite) deleteTokenRetry(t *testing.T) {
	handler := s.NewHandler(awsfake.NewCloudControl())
	if _, ok := handler.(native.DeletePreparer); !ok {
		t.Skip("the handler sends no delete token")
	}
	if _, ok := handler.(native.ScopedDeleter); ok {
		t.Skip("the handler removes only the members it manages, which takes no delete token")
	}
	h := s.newHarness(t)
	h.deploy(s.Properties)
	saved := []string{}
	h.handler.onDelete = func(token string) {
		saved = append(saved, h.record().Properties.DeleteToken)
	}
	h.handler.lose("Delete", true)
	resp := h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
	require.GreaterOrEqual(t, resp.status, http.StatusInternalServerError, resp.raw)
	require.NotNil(t, h.record())
	first := h.record().Properties.DeleteToken
	require.NotEmpty(t, first, "the delete token is saved before the first delete call")

	h.removeEventually()
	h.requireRemoved(nil)
	require.GreaterOrEqual(t, len(h.handler.tokens), 2, "the lost delete and its retry both send a token")
	for _, token := range h.handler.tokens {
		require.Equal(t, first, token, "every attempt sends the same delete token")
	}
	for _, token := range saved {
		require.Equal(t, first, token, "the record has the token when the delete call is made")
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

// sameNameFromAWS checks that nothing changes when AWS publishes a CloudFormation type with the hand-written type's
// name, with another identifier and fully managed by Cloud Control: UCP never asks CloudFormation or Cloud Control
// about it, and serves every request with the native handler and the Radius schema.
func (s Suite) sameNameFromAWS(t *testing.T) {
	h := s.newHarness(t)
	h.cf.Schemas[s.TypeName] = `{"typeName": "` + s.TypeName + `", "properties": {"Arn": {"type": "string"}}, "primaryIdentifier": ["/properties/Arn"], "readOnlyProperties": ["/properties/Arn"]}`
	h.useNative(false)

	h.deploy(s.Properties)
	h.deploy(s.Changed)
	resp := h.get()
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(s.Changed), resp.properties())
	resp = h.existingResponse()
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	resp = h.do(awsproxy.NewListAWSResources, http.MethodGet, h.collection(), "", nil)
	wantList := map[bool]int{true: http.StatusOK, false: http.StatusBadRequest}[h.handler.Options().List == native.ListRegion]
	require.Equal(t, wantList, resp.status, resp.raw)
	before := h.stored()
	h.remove()
	h.requireRemoved(before)

	require.Zero(t, h.cf.DescribeTypeCalls(), "UCP never asks CloudFormation about a hand-written type")
	require.Empty(t, h.cc.Calls(), "UCP never asks Cloud Control about a hand-written type")
}

// readable returns properties without the write-only values, which UCP never returns.
func (s Suite) readable(properties map[string]any) map[string]any {
	out, _ := normalizeValue(properties).(map[string]any)
	for _, p := range s.WriteOnly {
		removeAt(out, strings.Split(p, "/"))
	}
	return out
}

// normalizeValue returns a deep copy of v as JSON reads it.
func normalizeValue(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

// valuesAt returns the values at the path segments of v, where "*" matches every array item.
func valuesAt(v any, segments []string) []any {
	if len(segments) == 0 {
		return []any{v}
	}
	switch t := v.(type) {
	case map[string]any:
		if child, ok := t[segments[0]]; ok {
			return valuesAt(child, segments[1:])
		}
	case []any:
		if segments[0] == "*" {
			out := []any{}
			for _, item := range t {
				out = append(out, valuesAt(item, segments[1:])...)
			}
			return out
		}
	}
	return nil
}

// removeAt removes the value at the path segments of obj, where "*" matches every array item.
func removeAt(obj map[string]any, segments []string) {
	if len(segments) == 1 {
		delete(obj, segments[0])
		return
	}
	switch child := obj[segments[0]].(type) {
	case map[string]any:
		removeAt(child, segments[1:])
	case []any:
		if segments[1] == "*" && len(segments) > 2 {
			for _, item := range child {
				if m, ok := item.(map[string]any); ok {
					removeAt(m, segments[2:])
				}
			}
		}
	}
}

func without(properties map[string]any, property string) map[string]any {
	out := maps.Clone(properties)
	delete(out, property)
	return out
}

func withValue(properties map[string]any, property string, value any) map[string]any {
	out := maps.Clone(properties)
	out[property] = value
	return out
}

// requireSubset requires got to have every value of want. An object in want is compared the same way, so got may have
// more values inside it, such as an AccountId AWS fills in, as UCP's patch keeps a value AWS has that the template
// leaves out. Any other value, including a list, must be equal.
func requireSubset(t testing.TB, want map[string]any, got map[string]any) {
	t.Helper()
	requireSubsetAt(t, "", normalize(t, want).(map[string]any), normalize(t, got).(map[string]any))
}

func requireSubsetAt(t testing.TB, path string, want map[string]any, got map[string]any) {
	t.Helper()
	for k, v := range want {
		require.Contains(t, got, k, path)
		w, wantObject := v.(map[string]any)
		g, gotObject := got[k].(map[string]any)
		if wantObject && gotObject && len(w) > 0 {
			requireSubsetAt(t, path+"/"+k, w, g)
			continue
		}
		require.EqualValues(t, v, got[k], strings.TrimPrefix(path+"/"+k, "/"))
	}
}

func normalize(t testing.TB, v any) any {
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

	// headers are added to every request.
	headers map[string]string
}

func (s Suite) newHarness(t *testing.T) *harness {
	ucp_aws.MinPollInterval, ucp_aws.MaxPollInterval = time.Millisecond, time.Millisecond
	cc := awsfake.NewCloudControl()
	cc.Identifiers[s.TypeName] = s.IdentifierProperty
	cc.WriteOnly[s.TypeName] = s.WriteOnly
	handler := &recorder{Handler: s.NewHandler(cc)}
	switch encoder, ok := handler.Handler.(native.IdentifierEncoder); {
	case s.CloudControlCreate != nil:
		cc.Assign[s.TypeName] = s.CloudControlCreate
	case ok:
		// Both handlers store the same identifier, so Cloud Control computes it the way the handler encodes it.
		cc.IdentifierOf[s.TypeName] = func(properties map[string]any) string {
			identifier, _ := encoder.Identifier(properties)
			return identifier
		}
	}

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

// requireGeneratedName requires every create the handler got to send the same value of the name property, a name UCP
// generated that fits the type's name rules, and returns it.
func (h *harness) requireGeneratedName(nameProperty string) string {
	h.t.Helper()
	creates := h.handler.createRequests()
	require.NotEmpty(h.t, creates, "the handler got a create")
	name, _ := creates[0].Properties[nameProperty].(string)
	for _, create := range creates[1:] {
		require.Equal(h.t, name, create.Properties[nameProperty], "every create sends the same name")
	}
	rules := h.types.Lookup(h.s.TypeName).NameRules
	require.Regexp(h.t, "^radius-[0-9a-f]{8,12}$", name)
	if rules.MaxLength > 0 {
		require.LessOrEqual(h.t, len(name), rules.MaxLength)
	}
	require.GreaterOrEqual(h.t, len(name), rules.MinLength)
	if rules.Pattern != nil {
		require.Regexp(h.t, rules.Pattern, name)
	}
	return name
}

// identifierFor returns the identifier properties give: the handler's own encoding, or the identifier property.
func (h *harness) identifierFor(properties map[string]any) string {
	h.t.Helper()
	if encoder, ok := h.handler.Handler.(native.IdentifierEncoder); ok {
		identifier, err := encoder.Identifier(properties)
		require.NoError(h.t, err)
		return identifier
	}
	identifier, _ := properties[h.s.IdentifierProperty].(string)
	return identifier
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
	for k, v := range h.headers {
		req.Header.Set(k, v)
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
	return h.do(h.putFactory(), http.MethodPut, path, owner, map[string]any{"properties": properties})
}

// putFactory returns the factory of UCP's PUT controller.
func (h *harness) putFactory() controllerFactory {
	return func(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
		return awsproxy.NewCreateOrUpdateAWSResource(opts, clients, func() (*ucp_aws.FingerprintKey, error) { return h.key, nil })
	}
}

func (h *harness) get() response {
	return h.do(awsproxy.NewGetAWSResource, http.MethodGet, h.path(), "", nil)
}

// existingResponse reads the resource as an `existing` block does, by the template's properties and the identifier.
func (h *harness) existingResponse() response {
	properties := maps.Clone(h.s.Properties)
	switch {
	case h.s.IdentifierProperties != nil:
		maps.Copy(properties, h.s.IdentifierProperties(h.identifier()))
	case h.s.Identifier == "":
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

// deployAt deploys properties at the alias path and requires it to succeed, as deploy does at the suite's alias.
func (h *harness) deployAt(path string, properties map[string]any) {
	h.t.Helper()
	resp := h.putAt(path, properties)
	require.Contains(h.t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
	h.finish()
	rec := h.recordAt(path)
	require.Equal(h.t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
	h.lastID = rec.Properties.AWSIdentifier
}

// otherPath returns the path of the suite's other alias.
func otherPath(h *harness) string {
	return h.collection() + "/" + otherAlias
}

// putNamed sends a PUT that takes over the AWS resource with the identifier, by name, with properties.
func (h *harness) putNamed(identifier string, properties map[string]any) response {
	put := func(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
		return awsproxy.NewCreateOrUpdateAWSResource(opts, clients, func() (*ucp_aws.FingerprintKey, error) { return h.key, nil })
	}
	return h.do(put, http.MethodPut, h.path(), owner, map[string]any{"name": identifier, "properties": properties})
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
// removeAt deletes the alias at path, which must succeed.
func (h *harness) removeAt(path string) {
	h.t.Helper()
	resp := h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, path, owner, nil)
	require.Contains(h.t, []int{http.StatusAccepted, http.StatusNoContent}, resp.status, resp.raw)
	h.finish()
	require.Nil(h.t, h.recordAt(path))
}

// requireHeld checks that AWS still has every member of properties under the alias: deploying them again only reads.
func (h *harness) requireHeld(properties map[string]any) {
	h.t.Helper()
	h.requireHeldAt(h.path(), properties)
}

// requireHeldAt checks that AWS still has every member of properties under the alias at path.
func (h *harness) requireHeldAt(path string, properties map[string]any) {
	h.t.Helper()
	h.handler.reset()
	resp := h.putAt(path, properties)
	require.Equal(h.t, http.StatusOK, resp.status, resp.raw)
	require.Equal(h.t, []string{"Read"}, h.handler.reset(), "AWS has every member, so the deploy only reads")
}

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

	// creates are the requests Create got.
	creates []native.CreateRequest

	// onProgress, when set, runs on each Progress call.
	onProgress func()

	// onDelete, when set, runs on each delete call that sends a delete token, before the handler gets it. tokens are
	// the delete tokens the handler got.
	onDelete func(token string)
	tokens   []string
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

// wrap returns the handler the registry gets for r: r, with the optional methods the handler under test has, so UCP
// sees the same interfaces as for the handler itself.
func (r *recorder) wrap() native.Handler {
	_, encodes := r.Handler.(native.IdentifierEncoder)
	_, finds := r.Handler.(native.Finder)
	_, scopes := r.Handler.(native.ScopedReader)
	e, f, sc := encoder{r}, finder{r}, scoped{r}
	switch {
	case encodes && finds && scopes:
		return struct {
			*recorder
			encoder
			finder
			scoped
		}{r, e, f, sc}
	case encodes && finds:
		return struct {
			*recorder
			encoder
			finder
		}{r, e, f}
	case encodes && scopes:
		return struct {
			*recorder
			encoder
			scoped
		}{r, e, sc}
	case finds && scopes:
		return struct {
			*recorder
			finder
			scoped
		}{r, f, sc}
	case encodes:
		return struct {
			*recorder
			encoder
		}{r, e}
	case finds:
		return struct {
			*recorder
			finder
		}{r, f}
	case scopes:
		return struct {
			*recorder
			scoped
		}{r, sc}
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

// createRequests returns the requests Create got.
func (r *recorder) createRequests() []native.CreateRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.creates)
}

func (r *recorder) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	r.note("Create")
	r.mu.Lock()
	partway := r.partway
	r.creates = append(r.creates, request)
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
	return r.lostDelete(func() error { return r.Handler.Delete(ctx, region, identifier) })
}

// lostDelete makes a delete call, losing its response as lose set.
func (r *recorder) lostDelete(call func() error) error {
	reached, lost := r.takeLoss("Delete")
	switch {
	case lost && !reached:
		return errLost
	case lost:
		return errors.Join(errLost, call())
	}
	return call()
}

// scoped passes the methods of a handler under test whose resource is some members of a parent (native.ScopedReader
// and native.ScopedDeleter) through a recorder.
type scoped struct {
	r *recorder
}

func (s scoped) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	s.r.note("Read")
	return s.r.Handler.(native.ScopedReader).ReadScoped(ctx, region, identifier, managed)
}

func (s scoped) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	s.r.note("Delete")
	return s.r.lostDelete(func() error {
		return s.r.Handler.(native.ScopedDeleter).DeleteScoped(ctx, region, identifier, managed)
	})
}

// The recorder always has the methods of native.DeletePreparer, so the registry and UCP see them whatever the
// handler under test has. DeleteToken returns "" for a handler without them, so UCP never calls DeleteWithToken.

// ResolveIdentifier passes the identifier to the handler under test when it resolves identifiers
// (native.IdentifierResolver), and otherwise returns it as UCP uses it for a handler that doesn't.
func (r *recorder) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	if resolver, ok := r.Handler.(native.IdentifierResolver); ok {
		return resolver.ResolveIdentifier(ctx, region, identifier)
	}
	return identifier, nil
}

func (r *recorder) DeleteToken(ctx context.Context, region string, identifier string) (string, error) {
	if preparer, ok := r.Handler.(native.DeletePreparer); ok {
		return preparer.DeleteToken(ctx, region, identifier)
	}
	return "", nil
}

func (r *recorder) DeleteWithToken(ctx context.Context, region string, identifier string, token string) error {
	r.note("Delete")
	r.mu.Lock()
	r.tokens = append(r.tokens, token)
	onDelete := r.onDelete
	r.mu.Unlock()
	if onDelete != nil {
		onDelete(token)
	}
	return r.lostDelete(func() error {
		return r.Handler.(native.DeletePreparer).DeleteWithToken(ctx, region, identifier, token)
	})
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
