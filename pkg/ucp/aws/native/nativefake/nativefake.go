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

// Package nativefake has a fake native AWS handler for tests. It keeps its resources in an awsfake.CloudControl, so a
// test can serve one type with Cloud Control and with the handler and see the same resources from both.
package nativefake

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

var _ native.Handler = (*Handler)(nil)

// Handler is a fake native handler. Create and Update change the store at once, a delete removes the resource when it
// finishes, and Progress reports each operation as running for Waits calls before it finishes, like an AWS service
// with waiters.
type Handler struct {
	// Name is the type the handler serves.
	Name string

	// Store holds the resources. It applies the store's Identifiers and WriteOnly settings for the type.
	Store *awsfake.CloudControl

	// Opts is what Options returns.
	Opts native.Options

	// Waits is how many Progress calls report an operation as still running before it finishes.
	Waits int

	// CreateOnly names top-level properties AWS can't change after create. An update that changes one is rejected
	// with a client error, and changes nothing.
	CreateOnly []string

	// Errors maps a method name, such as "Create", to an error the handler returns instead of calling AWS.
	Errors map[string]error

	// LostResponses maps a method name to an error the handler returns once after the call takes effect, as when AWS
	// applies a request and the response is lost on the way back.
	LostResponses map[string]error

	// Failures maps an operation to the failure Progress reports once the operation finishes.
	Failures map[native.Operation]*native.Failure

	// OnProgress, when set, runs at the start of each Progress call, outside the handler's lock. Tests use it to
	// change something while UCP waits.
	OnProgress func()

	// IdentifierOf, when set, computes a new resource's identifier from its properties instead of the store's
	// identifier property. Encoding returns the handler with IdentifierOf as its identifier encoding.
	IdentifierOf func(properties map[string]any) (string, error)

	mu      sync.Mutex
	calls   []Call
	running map[string]*operation
}

// Call is a call the handler received.
type Call struct {
	// Method is Read, Create, Update, Delete, List, or Progress.
	Method     string
	Identifier string

	// Properties are what Create got, and Patch is what Update got.
	Properties map[string]any
	Patch      []map[string]any
}

type operation struct {
	op    native.Operation
	waits int
}

// New returns a Handler for typeName that keeps its resources in store.
func New(typeName string, store *awsfake.CloudControl) *Handler {
	return &Handler{
		Name:          typeName,
		Store:         store,
		Errors:        map[string]error{},
		LostResponses: map[string]error{},
		Failures:      map[native.Operation]*native.Failure{},
	}
}

// TypeName returns Name.
func (h *Handler) TypeName() string {
	return h.Name
}

// Options returns Opts.
func (h *Handler) Options() native.Options {
	return h.Opts
}

// Calls returns the calls the handler received, except Progress, and clears them.
func (h *Handler) Calls() []Call {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []Call{}
	for _, call := range h.calls {
		if call.Method != "Progress" {
			out = append(out, call)
		}
	}
	h.calls = nil
	return out
}

func (h *Handler) record(call Call) error {
	h.calls = append(h.calls, call)
	return h.Errors[call.Method]
}

func (h *Handler) lostResponse(method string) error {
	err := h.LostResponses[method]
	delete(h.LostResponses, method)
	return err
}

func (h *Handler) start(identifier string, op native.Operation) {
	if h.running == nil {
		h.running = map[string]*operation{}
	}
	h.running[identifier] = &operation{op: op, waits: h.Waits}
}

// Read returns the resource from the store without its write-only values.
func (h *Handler) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Read", Identifier: identifier}); err != nil {
		return nil, err
	}
	props, ok := h.Store.Read(h.Name, identifier)
	if !ok {
		return nil, native.ErrNotFound
	}
	return props, nil
}

// Create stores the resource and starts its create.
func (h *Handler) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Create", Properties: request.Properties}); err != nil {
		return "", err
	}
	var identifier string
	if h.IdentifierOf != nil {
		var err error
		if identifier, err = h.IdentifierOf(request.Properties); err != nil {
			return "", err
		}
		h.Store.Put(h.Name, identifier, request.Properties)
	} else {
		identifier = h.Store.Create(h.Name, request.Properties)
	}
	h.start(identifier, native.OperationCreate)
	return identifier, h.lostResponse("Create")
}

// Update applies the patch to the stored resource and starts its update.
func (h *Handler) Update(ctx context.Context, request native.UpdateRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	patch := []map[string]any{}
	b, err := json.Marshal(request.Patch)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &patch); err != nil {
		return err
	}
	if err := h.record(Call{Method: "Update", Identifier: request.Identifier, Patch: patch}); err != nil {
		return err
	}
	for _, op := range patch {
		name, _, _ := strings.Cut(strings.TrimPrefix(fmt.Sprint(op["path"]), "/"), "/")
		if slices.Contains(h.CreateOnly, name) {
			return ClientError("InvalidParameterCombination", fmt.Sprintf("%s can't be changed after create", name))
		}
	}
	if err := h.Store.Patch(h.Name, request.Identifier, patch); err != nil {
		return native.ErrNotFound
	}
	h.start(request.Identifier, native.OperationUpdate)
	return h.lostResponse("Update")
}

// Delete starts deleting the resource.
func (h *Handler) Delete(ctx context.Context, region string, identifier string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Delete", Identifier: identifier}); err != nil {
		return err
	}
	if _, ok := h.Store.Read(h.Name, identifier); !ok {
		return native.ErrNotFound
	}
	h.start(identifier, native.OperationDelete)
	return h.lostResponse("Delete")
}

// List returns every resource of the type in the store.
func (h *Handler) List(ctx context.Context, region string) ([]native.Resource, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "List"}); err != nil {
		return nil, err
	}
	out := []native.Resource{}
	for _, identifier := range h.Store.List(h.Name) {
		props, _ := h.Store.Read(h.Name, identifier)
		out = append(out, native.Resource{Identifier: identifier, Properties: props})
	}
	return out, nil
}

// Progress reports the operation as running until it has been read Waits times, and then as finished, with the
// failure in Failures for its operation. A delete that finishes without a failure removes the resource from the
// store. An operation the handler didn't start reads as finished.
func (h *Handler) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if h.OnProgress != nil {
		h.OnProgress()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Progress", Identifier: identifier}); err != nil {
		return native.Progress{}, err
	}
	if _, ok := h.Store.Read(h.Name, identifier); !ok {
		return native.Progress{}, native.ErrNotFound
	}
	running := h.running[identifier]
	if running == nil || running.op != op {
		return native.Progress{Done: true}, nil
	}
	if running.waits > 0 {
		running.waits--
		return native.Progress{State: strings.ToLower(string(op)) + "-in-progress", Waiting: "available"}, nil
	}
	delete(h.running, identifier)
	if failure := h.Failures[op]; failure != nil {
		return native.Progress{Done: true, Failure: failure}, nil
	}
	if op == native.OperationDelete {
		h.Store.Remove(h.Name, identifier)
	}
	return native.Progress{Done: true}, nil
}

// ClientError returns the AWS SDK error AWS returns for a bad request, such as an invalid property value.
func ClientError(code string, message string) error {
	return apiError(http.StatusBadRequest, &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient})
}

// ServerError returns the AWS SDK error AWS returns when it fails, which doesn't show whether it applied the request.
func ServerError(code string, message string) error {
	return apiError(http.StatusInternalServerError, &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultServer})
}

func apiError(status int, err error) error {
	return &smithy.OperationError{
		ServiceID:     "Fake",
		OperationName: "Call",
		Err:           &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}}, Err: err},
	}
}

// Encoding returns h as a handler whose identifier encoding is h.IdentifierOf.
func (h *Handler) Encoding() native.Handler {
	return encoding{h}
}

type encoding struct {
	*Handler
}

func (e encoding) Identifier(properties map[string]any) (string, error) {
	return e.IdentifierOf(properties)
}
