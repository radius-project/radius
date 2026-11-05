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
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
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
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/awsresources"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/awsproxy"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	owner    = "deploy;/planes/radius/local/resourceGroups/rg;;"
	scope    = "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/"
	alias    = "suite-alias"
	testHost = "localhost:5000"
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

	// IdentifierProperty is the property whose value is the resource's identifier, and WriteOnly lists the write-only
	// properties. The suite's Cloud Control uses them like AWS does.
	IdentifierProperty string
	WriteOnly          []string

	// Properties are a template that creates a resource whose identifier is Identifier.
	Properties map[string]any
	Identifier string

	// Changed is Properties with a property the handler can update changed.
	Changed map[string]any

	// Removable is a top-level property of Properties that can be removed after create, or "" when there is none.
	Removable string

	// CreateOnlyChange is Properties with a create-only property changed, or nil when the type has none.
	CreateOnlyChange map[string]any
}

// Run runs every case of the suite for s.
func Run(t *testing.T, s Suite) {
	t.Run("create, read, update, and delete", func(t *testing.T) { s.createReadUpdateDelete(t) })
	t.Run("redeploy with no change", func(t *testing.T) { s.redeployNoChange(t) })
	t.Run("deleted outside Radius", func(t *testing.T) { s.deletedOutsideRadius(t) })
	t.Run("removed property", func(t *testing.T) { s.removedProperty(t) })
	t.Run("create-only change", func(t *testing.T) { s.createOnlyChange(t) })
	t.Run("list", func(t *testing.T) { s.list(t) })
	t.Run("existing", func(t *testing.T) { s.existing(t) })
	t.Run("worker restart while waiting", func(t *testing.T) { s.workerRestart(t) })
	if s.HandWritten {
		t.Run("never asks CloudFormation", func(t *testing.T) { s.neverAsksCloudFormation(t) })
	} else {
		t.Run("create natively, then use Cloud Control", func(t *testing.T) { s.switchHandlers(t, true) })
		t.Run("create with Cloud Control, then use the native handler", func(t *testing.T) { s.switchHandlers(t, false) })
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

	h.remove()
	require.Nil(t, h.stored())
	require.Nil(t, h.record())
	require.Equal(t, http.StatusNotFound, h.get().status)
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
	h.cc.Remove(s.TypeName, s.Identifier)

	h.deploy(s.Properties)
	requireSubset(t, s.readable(s.Properties), h.stored())
	require.Equal(t, s.Identifier, h.record().Properties.AWSIdentifier)
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
		require.Equal(t, s.Identifier, requireSegment(t, h.collection(), id))
	default:
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	}
}

func (s Suite) existing(t *testing.T) {
	h := s.newHarness(t)
	h.deploy(s.Properties)

	getWithPost := func(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
		return awsproxy.NewGetAWSResourceWithPost(opts, clients, retry.NewNoOpRetryer())
	}
	resp := h.do(getWithPost, http.MethodPost, h.collection()+"/:get", "", map[string]any{"properties": s.Properties})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(s.Properties), resp.properties())

	// Recipes read an `existing` output resource again by the last segment of its ID, so the ID must give back the
	// identifier.
	id, _ := resp.body["id"].(string)
	require.Equal(t, s.Identifier, requireSegment(t, h.collection(), id))
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)
	resp = h.do(getWithPost, http.MethodPost, h.collection()+"/:get", "", map[string]any{"name": parsed.Name()})
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
// a region gains or loses Cloud Control support for the type.
func (s Suite) switchHandlers(t *testing.T, nativeFirst bool) {
	h := s.newHarness(t)
	h.useNative(nativeFirst)
	h.deploy(s.Properties)
	require.Equal(t, s.Identifier, h.record().Properties.AWSIdentifier)

	h.useNative(!nativeFirst)
	h.handler.reset()
	h.cc.Calls()

	resp := h.get()
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	requireSubset(t, s.readable(s.Properties), resp.properties())

	h.deploy(s.Changed)
	requireSubset(t, s.readable(s.Changed), h.stored())

	h.remove()
	require.Nil(t, h.stored())
	if nativeFirst {
		require.Empty(t, h.handler.reset(), "Cloud Control serves every call after the switch")
	} else {
		require.Empty(t, h.cc.Calls(), "the native handler serves every call after the switch")
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

// readable returns properties without the write-only ones, which AWS never returns.
func (s Suite) readable(properties map[string]any) map[string]any {
	out := maps.Clone(properties)
	for _, p := range s.WriteOnly {
		delete(out, p)
	}
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
	clients ucp_aws.Clients
	db      database.Client
	queue   *queueinmemory.Client
	sm      statusmanager.StatusManager
	key     *ucp_aws.FingerprintKey
}

func (s Suite) newHarness(t *testing.T) *harness {
	ucp_aws.MinPollInterval, ucp_aws.MaxPollInterval = time.Millisecond, time.Millisecond
	cc := awsfake.NewCloudControl()
	cc.Identifiers[s.TypeName] = s.IdentifierProperty
	cc.WriteOnly[s.TypeName] = s.WriteOnly
	handler := &recorder{Handler: s.NewHandler(cc)}

	catalog := schemas.Catalog{Native: map[string]string{}, HandWritten: map[string]string{}}
	if s.HandWritten {
		catalog.HandWritten[s.TypeName] = s.Schema
	} else {
		catalog.Native[s.TypeName] = s.Schema
	}
	registry, err := native.NewRegistry(catalog, handler.wrap())
	require.NoError(t, err)

	cf := &awsfake.CloudFormation{Schemas: map[string]string{s.TypeName: s.Schema}, ProvisioningTypes: map[string]cftypes.ProvisioningType{}}
	db := inmemory.NewClient()
	q := queueinmemory.New(queueinmemory.NewInMemQueue(time.Minute))
	key, err := ucp_aws.NewFingerprintKey(bytes.Repeat([]byte{7}, ucp_aws.MinFingerprintKeyLength))
	require.NoError(t, err)
	h := &harness{
		t: t, s: s, cc: cc, cf: cf, handler: handler,
		clients: ucp_aws.Clients{CloudControl: cc, CloudFormation: cf, Native: registry},
		db:      db, queue: q, sm: statusmanager.New(db, q, "global"), key: key,
	}
	h.useNative(true)
	return h
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

func (h *harness) stored() map[string]any {
	props, ok := h.cc.Read(h.s.TypeName, h.s.Identifier)
	if !ok {
		return nil
	}
	return props
}

func (h *harness) record() *datamodel.AWSResource {
	id, err := resources.ParseResource(h.path())
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
	put := func(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
		return awsproxy.NewCreateOrUpdateAWSResource(opts, clients, func() (*ucp_aws.FingerprintKey, error) { return h.key, nil })
	}
	return h.do(put, http.MethodPut, h.path(), owner, map[string]any{"properties": properties})
}

func (h *harness) get() response {
	return h.do(awsproxy.NewGetAWSResource, http.MethodGet, h.path(), "", nil)
}

// deploy sends a PUT and finishes the operation it starts.
func (h *harness) deploy(properties map[string]any) {
	h.t.Helper()
	resp := h.put(properties)
	require.Contains(h.t, []int{http.StatusOK, http.StatusCreated}, resp.status, resp.raw)
	h.finish()
	require.Equal(h.t, v1.ProvisioningStateSucceeded, h.record().ProvisioningState())
}

// remove sends a DELETE and finishes the operation it starts.
func (h *harness) remove() {
	h.t.Helper()
	resp := h.do(awsproxy.NewDeleteAWSResource, http.MethodDelete, h.path(), owner, nil)
	require.Contains(h.t, []int{http.StatusAccepted, http.StatusNoContent}, resp.status, resp.raw)
	h.finish()
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

// recorder wraps the handler under test. It records the methods called, and can hold Progress at "still running".
type recorder struct {
	native.Handler

	mu      sync.Mutex
	methods []string
	hold    bool

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

// encodingRecorder is a recorder for a handler with its own identifier encoding.
type encodingRecorder struct {
	*recorder
}

func (r encodingRecorder) Identifier(properties map[string]any) (string, error) {
	return r.Handler.(native.IdentifierEncoder).Identifier(properties)
}

// wrap returns the handler the registry gets for r: r itself, or an encodingRecorder when the handler under test has
// its own identifier encoding.
func (r *recorder) wrap() native.Handler {
	if _, ok := r.Handler.(native.IdentifierEncoder); ok {
		return encodingRecorder{r}
	}
	return r
}

func (r *recorder) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	r.note("Read")
	return r.Handler.Read(ctx, region, identifier)
}

func (r *recorder) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	r.note("Create")
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
