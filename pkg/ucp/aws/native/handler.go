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

// Package native defines native handlers: code that serves an AWS resource type by calling the service's own API
// through the AWS SDK, for types AWS Cloud Control can't create in a region and types with no CloudFormation type.
// A native handler replaces only the calls to Cloud Control. UCP keeps the same record, owner check, comparison, and
// operation status for both, so a record works with either handler.
//
// A handler starts an operation and returns. UCP makes its own operation ID, and the UCP worker calls Progress until
// the operation finishes. Progress only reads AWS, so a restarted worker can call it again.
package native

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wI2L/jsondiff"
)

// Operation is an operation on a resource. The values match records.OperationCreate, OperationUpdate, and
// OperationDelete.
type Operation string

const (
	OperationCreate Operation = "CREATE"
	OperationUpdate Operation = "UPDATE"
	OperationDelete Operation = "DELETE"
)

// ErrNotFound is returned, or wrapped, by Read, Update, Delete, and Progress when AWS has no resource with the
// identifier.
var ErrNotFound = errors.New("the AWS resource does not exist")

// OtherProductError is returned, or wrapped, by a handler whose service API also serves other products, when the
// identifier names a resource of another product, such as an Aurora cluster for a DocumentDB handler. The handler
// checks the product before every Read, Update, Delete, Reset, Progress, and Find, and leaves other products out of
// List. UCP fails a PUT, GET, or `existing` with 409, naming the identifier, and changes nothing. A DELETE treats it as
// ErrNotFound: the resource the alias managed is gone, so UCP removes only the record and never touches the other
// product's resource.
type OtherProductError struct {
	// Identifier is the identifier the request used, and Product is the product AWS reports for it, such as
	// aurora-postgresql.
	Identifier string
	Product    string
}

// Error returns a message that names the identifier and the product.
func (e *OtherProductError) Error() string {
	return fmt.Sprintf("the AWS identifier %q belongs to another product (%s)", e.Identifier, e.Product)
}

// Handler serves one AWS resource type through the service's own API. Properties use the names in the type's schema,
// in CloudFormation format. region is the AWS region of the request.
type Handler interface {
	// TypeName returns the type the handler serves, such as AWS::DocDB::DBCluster.
	TypeName() string

	// Options returns how UCP uses the handler.
	Options() Options

	// Read returns the properties of the resource with the identifier, or ErrNotFound. Some service APIs return
	// secrets, so UCP removes the schema's write-only values from what Read and List return before it uses or returns
	// them.
	Read(ctx context.Context, region string, identifier string) (map[string]any, error)

	// Create starts creating a resource and returns its identifier. It may return "" when the identifier is known
	// before the create, because UCP already has it. Options().Recovery says how a handler makes a create safe to
	// send again. A create that fails after AWS made the resource, such as a create that makes several calls, returns
	// a *Failure with the identifier when it knows it, so UCP saves it on the record.
	Create(ctx context.Context, request CreateRequest) (string, error)

	// Update starts updating the resource.
	Update(ctx context.Context, request UpdateRequest) error

	// Delete starts deleting the resource with the identifier. It returns ErrNotFound when AWS has no such resource.
	Delete(ctx context.Context, region string, identifier string) error

	// List returns every resource of the type in the region. UCP calls it only when Options().List is ListRegion.
	List(ctx context.Context, region string) ([]Resource, error)

	// Progress reads how far op on the resource has got. It returns ErrNotFound when AWS has no such resource, which
	// finishes a delete.
	Progress(ctx context.Context, region string, identifier string, op Operation) (Progress, error)
}

// IdentifierEncoder is implemented by a handler whose identifier is not its primary identifier values joined with
// "|", such as a type whose identifier values can contain "|". UCP uses it for create, take-over, `existing`,
// recovery, and delete, with either handler, so every path finds the resource by the same identifier.
type IdentifierEncoder interface {
	// Identifier returns the identifier the properties give, or "" when they don't give all of it. A property value
	// it can't use is a *Failure with ClientError set, which the caller gets as 400 with its message. It is the AWS
	// identifier itself and may contain any character, such as the "/" of an S3 ARN; UCP escapes it only where it is
	// the last segment of a resource ID. For a type that has a CloudFormation type, it must be the identifier Cloud
	// Control accepts, because Cloud Control serves the type in some regions.
	Identifier(properties map[string]any) (string, error)
}

// Resetter is implemented by a handler whose type's metadata sets onDelete to reset, such as an account setting. On
// DELETE, UCP calls Reset instead of Delete, then waits with Progress for OperationDelete, which reports Done when the
// setting is back to AWS's default, or ErrNotFound when the setting then reads as absent. UCP removes the record.
type Resetter interface {
	// Reset starts setting the resource with the identifier back to AWS's default. It returns ErrNotFound when AWS has
	// no such resource.
	Reset(ctx context.Context, region string, identifier string) error
}

// DeletePreparer is implemented by a handler whose delete must send the same value on every attempt, so a retried
// delete never takes a second copy of the data, such as the name of a DocumentDB cluster's final snapshot or the
// idempotency token of an FSx delete. Before the first delete call, UCP calls DeleteToken and saves the value on the
// record, then calls DeleteWithToken instead of Delete with that value on every attempt. A create or update clears the
// value, and so does a delete whose Progress reports that it failed, so the next delete gets a new one. A DeleteWithToken
// call that returns an error, a timeout, or an error while waiting keeps it, because the delete may still be running.
type DeletePreparer interface {
	// DeleteToken returns the value to send with every attempt to delete the resource with the identifier. UCP calls
	// it once per delete, so it may use the current time.
	DeleteToken(ctx context.Context, region string, identifier string) (string, error)

	// DeleteWithToken starts deleting the resource with the identifier, as Delete does, sending token.
	DeleteWithToken(ctx context.Context, region string, identifier string, token string) error
}

// Finder is implemented by a handler whose Options().Recovery is RecoverBySearch.
type Finder interface {
	// Find returns the identifier of the resource a create sent with request made, or "" when AWS has none. It finds
	// the resource by the tag Create added with request.ClientToken, or by values AWS keeps unique. When it can't tell,
	// it returns an error, never "".
	Find(ctx context.Context, request CreateRequest) (string, error)
}

// CreateRequest is a create to start.
type CreateRequest struct {
	Region string

	// Properties are the template's properties, including write-only values, and the name UCP generated when the
	// template leaves out Options().NameProperty. When UCP sends a create again to find a resource before a delete,
	// the template is not available, so Properties are those of the last create without write-only values.
	Properties map[string]any

	// ClientToken is a UUID saved on the record before the first create, and the same on every attempt to create
	// this resource. It identifies the operation: UCP generates names from it, and a handler sends it as the API's
	// idempotency token or tags the create with it. UCP may call Create more than once with the same ClientToken,
	// including from concurrent requests, and each call must return the same resource without creating a second one.
	ClientToken string
}

// Recovery is how UCP finds a create it lost after AWS accepted it, such as when UCP stopped before it saved the
// result. The next PUT, or a DELETE, finds the resource instead of creating a second one.
type Recovery int

const (
	// RecoverByIdentifier means the identifier is known before the create, because the template sets it or UCP
	// generates it (see Options.NameProperty). UCP saves it on the record before the create and reads AWS by it: if
	// AWS has the resource, UCP waits for it, and otherwise it sends the create again. A create whose identifier the
	// template doesn't give fails with 400 before UCP calls AWS.
	RecoverByIdentifier Recovery = iota

	// RecoverByResend means sending the create again returns the first resource: AWS assigns the identifier and the
	// API takes an idempotency token, which the handler sets to CreateRequest.ClientToken, or the call is safe to
	// repeat, such as adding a user to a group. UCP sends the create again, possibly from two requests at once, before
	// it reserves the record, so Create with the same ClientToken must return the same resource every time.
	RecoverByResend

	// RecoverBySearch means AWS assigns the identifier and the API takes no idempotency token. The handler tags the
	// create with CreateRequest.ClientToken, or relies on values AWS keeps unique, and implements Finder. UCP calls
	// Find, and sends the create again only when AWS has no such resource.
	RecoverBySearch
)

// UpdateRequest is an update to start.
type UpdateRequest struct {
	Region     string
	Identifier string

	// Current is what Read returned, without write-only values, and Desired is the template's properties, including
	// write-only values.
	Current map[string]any
	Desired map[string]any

	// Patch is the change from Current to Desired that UCP computed, relative to the properties object. It leaves
	// out write-only values whose fingerprint shows they are unchanged, and clears properties the template no longer
	// sets. A handler applies Patch, and reads Desired only for context.
	Patch jsondiff.Patch
}

// Resource is a resource List returns.
type Resource struct {
	Identifier string
	Properties map[string]any
}

// Progress is how far an operation has got.
type Progress struct {
	// Done is true when the operation finished.
	Done bool

	// Failure describes why a finished operation failed, or is nil when it succeeded.
	Failure *Failure

	// State is what AWS reports now, such as "creating", and Waiting is the state UCP waits for, such as "available".
	// Both name the states in the error when UCP stops waiting at the timeout.
	State   string
	Waiting string
}

// Outcome is how much of a failed operation AWS applied. UCP sets the record's pending values by it:
//
//   - OutcomeRejected: AWS changed nothing. The pending values are dropped, so the previous last-applied values stay.
//   - OutcomePartial: AWS applied some of the operation, such as an operation that makes several calls failing after
//     the first. The pending values are kept as possibly applied, so a later deploy that removes one clears it. The
//     next deploy reads AWS and makes only the calls still needed.
//   - OutcomeUnknown: UCP stopped before it saw the result, or AWS's answer doesn't say. The same as OutcomePartial.
//   - OutcomeApplied: AWS applied all of it, but the operation still failed, such as a call after the last change.
//     The pending values become last-applied.
type Outcome int

const (
	OutcomeUnknown Outcome = iota
	OutcomeRejected
	OutcomePartial
	OutcomeApplied
)

// Failure is why an operation failed. A handler returns it from Progress, or as the error of a start call (Create,
// Update, Delete, or Reset) to say how much of the operation AWS applied. A start call that returns another error is
// OutcomeRejected when AWS rejected the call as a client fault, and OutcomeUnknown otherwise.
type Failure struct {
	// Message is shown to the user. It names properties, never their values, because values can hold secrets.
	Message string

	// ClientError is true when the request caused the failure, such as an invalid property value, rather than AWS.
	ClientError bool

	// Outcome is how much of the operation AWS applied. The zero value, OutcomeUnknown, keeps the pending values as
	// possibly applied.
	Outcome Outcome
}

// Error returns the message.
func (f *Failure) Error() string {
	return f.Message
}

// IsClientError reports whether the request caused the failure.
func (f *Failure) IsClientError() bool {
	return f.ClientError
}

// ListSupport is how a type can be listed.
type ListSupport int

const (
	// ListNone means the type can't be listed.
	ListNone ListSupport = iota

	// ListRegion means every resource of the type in a region can be listed.
	ListRegion

	// ListParent means the type can be listed only under a parent resource, such as the objects in an S3 bucket.
	ListParent
)

// DefaultTimeout is how long UCP waits for an operation when the handler sets no timeout for it.
const DefaultTimeout = 2 * time.Hour

// Options describe how UCP uses a handler.
type Options struct {
	// List is how the type can be listed.
	List ListSupport

	// Recovery is how UCP finds a create it lost after AWS accepted it.
	Recovery Recovery

	// NameProperty is the top-level property that names the resource when AWS lets the caller choose the name, or ""
	// when there is none. When the template leaves it out, UCP generates a name from CreateRequest.ClientToken that
	// fits the property's rules in the schema (see NameRules), and sends it with the create. The property stays
	// optional, because the same template must also work where Cloud Control serves the type, and Cloud Control
	// generates its own name.
	NameProperty string

	// Equivalent, when set, reports whether a value AWS returns means the same as the template's, for values AWS
	// returns in another form that the schema's propertyTransform can't express, such as an engine version 5.0 that
	// comes back as 5.0.0. Such values don't count as changes. path is the property's path relative to the properties
	// object, with "/" between segments, such as EngineVersion or Config/Version. UCP uses the schema's
	// propertyTransform first, so prefer it where it can express the rule. UCP doesn't call Equivalent for a value that
	// leaves out something the last deploy set, such as a field inside a list item, so a rule that lets AWS's value
	// hold more than the template, such as defaults AWS fills in, never hides a removal.
	Equivalent func(path string, template any, aws any) bool

	// CreateTimeout, UpdateTimeout, and DeleteTimeout are how long UCP waits for each operation. Zero means
	// DefaultTimeout.
	CreateTimeout time.Duration
	UpdateTimeout time.Duration
	DeleteTimeout time.Duration
}

// Timeout returns how long UCP waits for op.
func (o Options) Timeout(op Operation) time.Duration {
	var timeout time.Duration
	switch op {
	case OperationCreate:
		timeout = o.CreateTimeout
	case OperationUpdate:
		timeout = o.UpdateTimeout
	case OperationDelete:
		timeout = o.DeleteTimeout
	}
	if timeout <= 0 {
		return DefaultTimeout
	}
	return timeout
}
