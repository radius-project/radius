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
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

var _ native.Handler = (*Handler)(nil)
var _ native.Finder = (*Handler)(nil)
var _ native.Resetter = (*Handler)(nil)
var _ native.DeletePreparer = tokens{}
var _ native.ScopedReader = scoped{}
var _ native.ScopedDeleter = scoped{}

// Handler is a fake native handler. Create and Update change the store at once, a delete removes the resource when it
// finishes, and Progress reports each operation as running for Waits calls before it finishes, like an AWS service
// with waiters.
//
// Create acts on the client token as Opts.Recovery says: with RecoverByResend, a create sent again with a client token
// returns the first resource, like an API with an idempotency token; with RecoverBySearch, Create tags the resource
// with the client token, and Find finds it by that tag.
//
// Update makes one AWS call for each operation of the patch, so it can fail partway (see FailOn). When Product is set,
// the store also holds resources of other products, like a service API that serves several products.
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
	// applies a request and the response is lost on the way back. A lost Create returns no identifier, unless the
	// error is a *native.Failure.
	LostResponses map[string]error

	// Failures maps an operation to the failure Progress reports once the operation finishes.
	Failures map[native.Operation]*native.Failure

	// OnProgress, when set, runs at the start of each Progress call, outside the handler's lock. Tests use it to
	// change something while UCP waits.
	OnProgress func()

	// ReturnsWriteOnly makes Read and List return write-only values, like a service API that returns secrets, such
	// as MediaLive's DescribeChannel.
	ReturnsWriteOnly bool

	// IdentifierOf, when set, computes a new resource's identifier from its properties instead of the store's
	// identifier property. Encoding returns the handler with IdentifierOf as its identifier encoding.
	IdentifierOf func(properties map[string]any) (string, error)

	// ResetTo is what Reset sets the resource to, like a setting back at AWS's default. nil makes Reset remove the
	// resource, like a setting that then reads as absent.
	ResetTo map[string]any

	// ProductProperty and Product, when set, make a resource whose ProductProperty isn't Product belong to another
	// product: Read, Update, Delete, Reset, Progress, and Find return a native.OtherProductError for it, and List
	// leaves it out.
	ProductProperty string
	Product         string

	mu      sync.Mutex
	calls   []Call
	running map[string]*operation

	// failOn is the call of the next Update that fails, and failWith the error it fails with.
	failOn   int
	failWith error

	// tags maps a client token to the resource the create sent with it made.
	tags map[string]string

	// deletes counts the delete tokens DeleteTokens made, and copies are the tokens deletes took a copy with.
	deletes int
	copies  map[string]bool
}

// Call is a call the handler received.
type Call struct {
	// Method is Read, Create, Update, Delete, Reset, List, Find, or Progress.
	Method     string
	Identifier string

	// Properties are what Create or Find got, ClientToken is the client token they got, and Patch, Token, and State
	// are what Update got.
	Properties  map[string]any
	ClientToken string
	Patch       []map[string]any
	Token       string
	State       string
}

type operation struct {
	op    native.Operation
	waits int

	// reset is true for a delete that is a reset, which keeps the resource when it finishes.
	reset bool
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

func (h *Handler) start(identifier string, op native.Operation) *operation {
	if h.running == nil {
		h.running = map[string]*operation{}
	}
	running := &operation{op: op, waits: h.Waits}
	h.running[identifier] = running
	return running
}

// FailOn makes the next Update fail with err at its call-th AWS call, counting from 0, after the calls before it took
// effect. A failure at the first call returns err. A later one returns a native.Failure with native.OutcomePartial,
// and one at a call after the last, native.OutcomeApplied.
func (h *Handler) FailOn(call int, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failOn, h.failWith = call, err
}

// otherProduct returns a native.OtherProductError when the stored resource belongs to another product.
func (h *Handler) otherProduct(identifier string) error {
	if h.ProductProperty == "" {
		return nil
	}
	props := h.Store.Resource(h.Name, identifier)
	if props == nil || props[h.ProductProperty] == h.Product {
		return nil
	}
	return &native.OtherProductError{Identifier: identifier, Product: fmt.Sprint(props[h.ProductProperty])}
}

// Read returns the resource from the store, without its write-only values unless ReturnsWriteOnly is set.
func (h *Handler) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Read", Identifier: identifier}); err != nil {
		return nil, err
	}
	if err := h.otherProduct(identifier); err != nil {
		return nil, err
	}
	props, ok := h.read(identifier)
	if !ok {
		return nil, native.ErrNotFound
	}
	return props, nil
}

func (h *Handler) read(identifier string) (map[string]any, bool) {
	if h.ReturnsWriteOnly {
		props := h.Store.Resource(h.Name, identifier)
		return props, props != nil
	}
	return h.Store.Read(h.Name, identifier)
}

// Create stores the resource and starts its create. With RecoverByResend, a client token it has seen returns the
// resource the first create made, when the store still has it.
func (h *Handler) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Create", Properties: request.Properties, ClientToken: request.ClientToken}); err != nil {
		return "", err
	}
	if first, ok := h.tags[request.ClientToken]; ok && h.Opts.Recovery == native.RecoverByResend {
		if _, exists := h.Store.Read(h.Name, first); exists {
			return created(first, h.lostResponse("Create"))
		}
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
	if request.ClientToken != "" {
		if h.tags == nil {
			h.tags = map[string]string{}
		}
		h.tags[request.ClientToken] = identifier
	}
	h.start(identifier, native.OperationCreate)
	return created(identifier, h.lostResponse("Create"))
}

// created returns the result of a create: the identifier, or no identifier when the response was lost. A
// *native.Failure is what the handler returns knowing the create made the resource, so it keeps the identifier.
func created(identifier string, lost error) (string, error) {
	var failure *native.Failure
	if lost != nil && !errors.As(lost, &failure) {
		return "", lost
	}
	return identifier, lost
}

// Find returns the resource a create with the client token made, found by its tag, or "" when the store has none.
func (h *Handler) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Find", Properties: request.Properties, ClientToken: request.ClientToken}); err != nil {
		return "", err
	}
	identifier, ok := h.tags[request.ClientToken]
	if !ok {
		return "", nil
	}
	if err := h.otherProduct(identifier); err != nil {
		return "", err
	}
	if _, exists := h.Store.Read(h.Name, identifier); !exists {
		return "", nil
	}
	return identifier, nil
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
	if err := h.record(Call{Method: "Update", Identifier: request.Identifier, Patch: patch, Token: request.Token, State: request.State}); err != nil {
		return err
	}
	if err := h.otherProduct(request.Identifier); err != nil {
		return err
	}
	for _, op := range patch {
		name, _, _ := strings.Cut(strings.TrimPrefix(fmt.Sprint(op["path"]), "/"), "/")
		if slices.Contains(h.CreateOnly, name) {
			return ClientError("InvalidParameterCombination", fmt.Sprintf("%s can't be changed after create", name))
		}
	}
	failOn, failWith := h.failOn, h.failWith
	h.failWith = nil
	for i, op := range patch {
		if failWith != nil && i == failOn {
			return h.failedCall(i, len(patch), failWith)
		}
		if err := h.Store.Patch(h.Name, request.Identifier, []map[string]any{op}); err != nil {
			return native.ErrNotFound
		}
	}
	if failWith != nil {
		return h.failedCall(failOn, len(patch), failWith)
	}
	h.start(request.Identifier, native.OperationUpdate)
	return h.lostResponse("Update")
}

// failedCall returns the error of an update whose call-th of total calls failed with err.
func (h *Handler) failedCall(call int, total int, err error) error {
	switch {
	case call == 0:
		return err
	case call < total:
		return &native.Failure{Message: fmt.Sprintf("call %d of %d failed: %s", call+1, total, err), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomePartial}
	default:
		return &native.Failure{Message: fmt.Sprintf("every change was made, then a call failed: %s", err), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeApplied}
	}
}

// Delete starts deleting the resource.
func (h *Handler) Delete(ctx context.Context, region string, identifier string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Delete", Identifier: identifier}); err != nil {
		return err
	}
	if err := h.otherProduct(identifier); err != nil {
		return err
	}
	if _, ok := h.Store.Read(h.Name, identifier); !ok {
		return native.ErrNotFound
	}
	h.start(identifier, native.OperationDelete)
	return h.lostResponse("Delete")
}

// Reset sets the resource to ResetTo, or removes it when ResetTo is nil, and starts a delete that keeps it.
func (h *Handler) Reset(ctx context.Context, region string, identifier string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.record(Call{Method: "Reset", Identifier: identifier}); err != nil {
		return err
	}
	if err := h.otherProduct(identifier); err != nil {
		return err
	}
	if _, ok := h.Store.Read(h.Name, identifier); !ok {
		return native.ErrNotFound
	}
	if h.ResetTo == nil {
		h.Store.Remove(h.Name, identifier)
		return nil
	}
	h.Store.Put(h.Name, identifier, h.ResetTo)
	h.start(identifier, native.OperationDelete).reset = true
	return h.lostResponse("Reset")
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
		if h.otherProduct(identifier) != nil {
			continue
		}
		props, _ := h.read(identifier)
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
	if err := h.otherProduct(identifier); err != nil {
		return native.Progress{}, err
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
	if op == native.OperationDelete && !running.reset {
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

// DeleteTokens returns h as a handler that sends a delete token (native.DeletePreparer), like a type whose delete
// takes a final snapshot. Each delete that reaches AWS with a token it hasn't seen takes a copy, which Copies counts.
func (h *Handler) DeleteTokens() native.Handler {
	return tokens{h}
}

// Copies returns how many copies deletes with a delete token took.
func (h *Handler) Copies() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.copies)
}

type tokens struct {
	*Handler
}

func (t tokens) DeleteToken(ctx context.Context, region string, identifier string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deletes++
	return fmt.Sprintf("%s-final-%d", identifier, t.deletes), nil
}

func (t tokens) DeleteWithToken(ctx context.Context, region string, identifier string, token string) error {
	if err := t.Delete(ctx, region, identifier); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.copies == nil {
		t.copies = map[string]bool{}
	}
	t.copies[token] = true
	return nil
}

// Scoped returns h as a handler whose resource is the members in the list property member of a parent, which others
// can add members to too (native.ScopedReader), like users added to an IAM group. The store holds one resource per
// parent with every member, and its identifier is the parent's. Create adds the members to the parent, Update adds
// and removes members one call at a time, removals first, and DeleteScoped removes the members the alias managed,
// and the parent once it has none.
func (h *Handler) Scoped(member string) native.Handler {
	return scoped{Handler: h, member: member}
}

type scoped struct {
	*Handler
	member string
}

func (s scoped) members(properties map[string]any) []string {
	out := []string{}
	list, _ := properties[s.member].([]any)
	for _, v := range list {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

func (s scoped) managed(managed []map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, properties := range managed {
		for _, m := range s.members(properties) {
			out[m] = true
		}
	}
	return out
}

func (s scoped) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	props, err := s.Read(ctx, region, identifier)
	if err != nil || managed == nil {
		return props, err
	}
	keep := s.managed(managed)
	found := []any{}
	for _, m := range s.members(props) {
		if keep[m] {
			found = append(found, m)
		}
	}
	if len(found) == 0 {
		return nil, native.ErrNotFound
	}
	props[s.member] = found
	return props, nil
}

func (s scoped) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.record(Call{Method: "Create", Properties: request.Properties, ClientToken: request.ClientToken}); err != nil {
		return "", err
	}
	identifier := fmt.Sprint(request.Properties[s.Store.Identifiers[s.Name]])
	current := s.Store.Resource(s.Name, identifier)
	if current == nil {
		s.Store.Put(s.Name, identifier, request.Properties)
		return created(identifier, s.lostResponse("Create"))
	}
	have := s.members(current)
	list, _ := current[s.member].([]any)
	for _, m := range s.members(request.Properties) {
		if !slices.Contains(have, m) {
			list = append(list, m)
		}
	}
	current[s.member] = list
	s.Store.Put(s.Name, identifier, current)
	return created(identifier, s.lostResponse("Create"))
}

func (s scoped) Update(ctx context.Context, request native.UpdateRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.record(Call{Method: "Update", Identifier: request.Identifier}); err != nil {
		return err
	}
	for _, name := range s.CreateOnly {
		if fmt.Sprint(request.Current[name]) != fmt.Sprint(request.Desired[name]) {
			return ClientError("InvalidParameterCombination", fmt.Sprintf("%s can't be changed after create", name))
		}
	}
	current, desired := s.members(request.Current), s.members(request.Desired)
	type change struct {
		member string
		add    bool
	}
	changes := []change{}
	for _, m := range current {
		if !slices.Contains(desired, m) {
			changes = append(changes, change{m, false})
		}
	}
	removals := len(changes)
	for _, m := range desired {
		if !slices.Contains(current, m) {
			changes = append(changes, change{m, true})
		}
	}
	failOn, failWith := s.failOn, s.failWith
	s.failWith = nil
	fail := func(call int) error {
		if call < removals {
			return &native.Failure{Message: fmt.Sprintf("call %d of %d failed: %s", call+1, len(changes), failWith), ClientError: ucp_aws.IsClientFault(failWith), Outcome: native.OutcomeRejected}
		}
		return s.failedCall(call, len(changes), failWith)
	}
	for i, c := range changes {
		if failWith != nil && i == failOn {
			return fail(i)
		}
		props := s.Store.Resource(s.Name, request.Identifier)
		if props == nil {
			return native.ErrNotFound
		}
		list := []any{}
		for _, m := range s.members(props) {
			if m != c.member {
				list = append(list, m)
			}
		}
		if c.add {
			list = append(list, c.member)
		}
		props[s.member] = list
		s.Store.Put(s.Name, request.Identifier, props)
	}
	if failWith != nil {
		return fail(failOn)
	}
	return s.lostResponse("Update")
}

func (s scoped) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.record(Call{Method: "Delete", Identifier: identifier}); err != nil {
		return err
	}
	props := s.Store.Resource(s.Name, identifier)
	if props == nil {
		return native.ErrNotFound
	}
	remove := s.managed(managed)
	list := []any{}
	for _, m := range s.members(props) {
		if !remove[m] {
			list = append(list, m)
		}
	}
	if len(list) == 0 {
		s.Store.Remove(s.Name, identifier)
	} else {
		props[s.member] = list
		s.Store.Put(s.Name, identifier, props)
	}
	return s.lostResponse("Delete")
}

// Progress reports every operation as finished, because adding and removing members takes effect at once.
func (s scoped) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op != native.OperationDelete {
		if _, ok := s.Store.Read(s.Name, identifier); !ok {
			return native.Progress{}, native.ErrNotFound
		}
	}
	return native.Progress{Done: true}, nil
}
