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
	"reflect"
	"slices"
	"strings"
	"time"

	"uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/aws/extraproperties"
	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/components/database"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// engine holds the logic the AWS resource controllers share. Every AWS resource UCP deploys has a record
// (datamodel.AWSResource) in UCP's data store, under the resource ID that ends in the template's alias. The record
// remembers which AWS resource the alias refers to and what Radius last applied to it.
type engine struct {
	clients  ucp_aws.Clients
	store    *records.Store
	sm       statusmanager.StatusManager
	pathBase string

	// fingerprintKey returns the key for fingerprints of write-only values. Only PUT uses it.
	fingerprintKey FingerprintKeyFunc
}

func newEngine(opts armrpc_controller.Options, clients ucp_aws.Clients, fingerprintKey FingerprintKeyFunc) *engine {
	return &engine{clients: clients, store: records.NewStore(opts.DatabaseClient), sm: opts.StatusManager, pathBase: opts.PathBase, fingerprintKey: fingerprintKey}
}

// request is an AWS resource request after the common checks.
type request struct {
	id       resources.ID
	typeName string
	region   string

	// route is the handler that serves the request. put, get, and delete set it.
	route

	// managed are the property sets the alias may have applied, which a native handler that reads only the members it
	// manages gets (native.ScopedReader). put, get, and startDelete set them from the record. They are nil when UCP
	// has no record.
	managed []map[string]any
}

// managedValues returns the property sets the alias may have applied: the record's last-applied and possibly applied
// properties, and desired when it isn't nil.
func managedValues(rec *datamodel.AWSResource, desired map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, properties := range []map[string]any{rec.Properties.LastApplied, rec.Properties.PendingLastApplied, desired} {
		if properties != nil {
			out = append(out, properties)
		}
	}
	return out
}

// ownManaged returns managedValues(rec, desired) without the members another alias's record may hold, for a native
// handler that names its members (native.Members), so a read, an update, and a delete never treat them as this
// alias's. A desired that lists such a member gets 409 naming that alias, before anything is reserved or sent to AWS.
// identifier is the AWS identifier the request acts on, the record's or a take-over's name, which names the parent the
// handler acts on. An alias's record holds the members of its last-applied and possibly applied properties, so a
// member stays held until the deploy that removes it, or the delete, finishes. A record being deleted still holds its
// members against a deploy, but not against another delete, so of two aliases that list a member, the one deleted
// last removes it.
//
// The read and the reservation that follows are separate steps, so two aliases that add the same member at the same
// moment can both hold it. Each then leaves it out of its delete while the other isn't being deleted, and a redeploy
// of either that lists it gets 409 until the other removes it.
func (e *engine) ownManaged(ctx context.Context, r *request, rec *datamodel.AWSResource, identifier string, desired map[string]any) ([]map[string]any, armrpc_rest.Response, error) {
	managed := managedValues(rec, desired)
	if r.handler == nil || r.native == nil || r.handler.Options().Members == nil {
		return managed, nil, nil
	}
	m := r.handler.Options().Members
	others, err := e.store.Others(ctx, r.id, r.native.Metadata.IdentityScope)
	if err != nil {
		return nil, nil, err
	}
	// held maps a parent to its members and the alias that holds each, and kept to those a delete must leave.
	held := map[string]map[string]string{}
	kept := map[string]map[string]bool{}
	for _, other := range others {
		deleting := records.IsRunning(other) && records.PendingOperation(other) == records.OperationDelete
		for _, properties := range managedValues(other, nil) {
			parent, members := m.Of(other.Properties.AWSIdentifier, properties)
			if held[parent] == nil {
				held[parent], kept[parent] = map[string]string{}, map[string]bool{}
			}
			for _, member := range members {
				held[parent][member] = other.ID
				kept[parent][member] = kept[parent][member] || !deleting
			}
		}
	}
	if desired != nil {
		parent, members := m.Of(identifier, desired)
		for _, member := range members {
			if holder, ok := held[parent][member]; ok {
				return nil, conflict("%q in %s of %q is already managed under the alias %q. Remove it from this template, or from the other alias first.", member, m.List, parent, records.AliasOf(holder)), nil
			}
		}
	}
	out := []map[string]any{}
	for _, properties := range managed {
		parent, _ := m.Of(identifier, properties)
		out = append(out, m.Without(properties, kept[parent]))
	}
	return out, nil, nil
}

// keepPending treats the values of an interrupted operation as applied, as records.KeepPending does. For a native
// handler whose resource is some members of a parent (native.ScopedReader), a member list keeps the members of both
// the last deploy and the interrupted one: the operation may have stopped before it removed the old members, so Radius
// still owns them until a later deploy removes them.
func keepPending(r *request, rec *datamodel.AWSResource) {
	if _, scoped := r.handler.(native.ScopedReader); scoped {
		addMembers(rec.Properties.PendingLastApplied, rec.Properties.LastApplied)
	}
	records.KeepPending(rec)
}

// addMembers adds to each list in properties the members of the same list in previous that it doesn't have.
func addMembers(properties map[string]any, previous map[string]any) {
	for name, value := range properties {
		list, ok := value.([]any)
		earlier, wasList := previous[name].([]any)
		if !ok || !wasList {
			continue
		}
		list = slices.Clone(list)
		for _, member := range earlier {
			if !slices.ContainsFunc(list, func(v any) bool { return reflect.DeepEqual(v, member) }) {
				list = append(list, member)
			}
		}
		properties[name] = list
	}
}

// begin reads the region and rejects legacy and deprecated types. A non-nil response ends the request.
func (e *engine) begin(ctx context.Context, httpReq *http.Request) (*request, armrpc_rest.Response) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(httpReq.URL.Path, e.pathBase)
	if errResponse != nil {
		return nil, errResponse
	}
	typeName := serviceCtx.ResourceTypeInAWSFormat()
	if errResponse := unsupportedTypeResponse(typeName); errResponse != nil {
		return nil, errResponse
	}
	return &request{id: serviceCtx.ResourceID, typeName: typeName, region: region}, nil
}

// ownerFromRequest returns the owner the caller sent. PUT and DELETE require it, so a missing header returns a 400.
// Reads do not need it.
func ownerFromRequest(req *http.Request) (string, armrpc_rest.Response) {
	owner := strings.TrimSpace(req.Header.Get(sdkclients.AWSResourceOwnerHeader))
	if owner == "" {
		return "", badRequest(fmt.Sprintf("The %s header is required to create, update, or delete an AWS resource.", sdkclients.AWSResourceOwnerHeader))
	}
	return owner, nil
}

// validateAlias rejects an alias that contains "/", because the alias is the last segment of the resource ID.
func validateAlias(req *http.Request) armrpc_rest.Response {
	path := req.URL.EscapedPath()
	last := path[strings.LastIndex(path, "/")+1:]
	if strings.Contains(strings.ToLower(last), "%2f") {
		return badRequest("The alias of an AWS resource must not contain '/', because it is the last segment of the resource ID.")
	}
	return nil
}

// checkOwner returns a 409 when the record belongs to another owner. Owners are Radius IDs, which are not
// case-sensitive.
func checkOwner(rec *datamodel.AWSResource, owner string) armrpc_rest.Response {
	if strings.EqualFold(rec.Properties.Owner, owner) {
		return nil
	}
	return conflict("The alias %q is already used by %s. Choose another alias, or reference the resource with existing.", rec.Name, rec.Properties.Owner)
}

func conflict(format string, args ...any) armrpc_rest.Response {
	return armrpc_rest.NewConflictResponse(fmt.Sprintf(format, args...))
}

// mustResume reports whether the request must resume an operation on the record before it does anything else: a
// running operation that is stale, or one AWS accepted that UCP saved but never queued.
func mustResume(rec *datamodel.AWSResource) bool {
	if records.IsRunning(rec) {
		return records.IsStale(rec)
	}
	return rec.Properties.RequestToken != ""
}

// checkRecord runs the checks every PUT and DELETE makes on an existing record: the owner, an operation in progress,
// and an interrupted operation, which it resumes. It returns the record to continue with, or a response that ends
// the request.
func (e *engine) checkRecord(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, owner string, method v1.OperationMethod) (*datamodel.AWSResource, string, armrpc_rest.Response, error) {
	if resp := checkOwner(rec, owner); resp != nil {
		return nil, "", resp, nil
	}
	if records.IsRunning(rec) && !records.IsStale(rec) {
		return nil, "", conflict("Another operation on the AWS resource %q is still running. Retry when it finishes.", r.id.Name()), nil
	}
	if mustResume(rec) {
		return e.resume(ctx, r, rec, etag, method)
	}
	return rec, etag, nil, nil
}

// resume handles a stale or never-queued operation. With a request token, AWS accepted the operation: resume queues
// it again so a worker polls the original request, and returns the response. Without one, UCP reserved the record
// but may or may not have reached AWS, and resume recovers by operation:
//
//   - update: the properties of the interrupted update may now be in AWS, so they count as applied and Radius owns
//     them. A later deploy that omits one clears it. The request continues.
//   - delete: the delete is sent again. A DELETE gets its response; a PUT gets 409 and retries when it finishes.
//   - create: the record keeps its client token and pending values, and the request continues. A PUT sends the create
//     again with the same token, and a DELETE recovers the create first (see recoverCreate).
func (e *engine) resume(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, method v1.OperationMethod) (*datamodel.AWSResource, string, armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	op := records.PendingOperation(rec)

	if rec.Properties.RequestToken == "" {
		logger.Info("Recovering an AWS operation that UCP reserved but did not track", "alias", r.id.Name(), "operation", op)
		switch op {
		case records.OperationDelete:
			resp, err := e.startDelete(ctx, r, rec, etag)
			if err != nil || method == v1.OperationDelete || !deleteStarted(resp) {
				return nil, "", resp, err
			}
			return nil, "", conflict("An interrupted delete of the AWS resource %q was resumed. Retry when it finishes.", r.id.Name()), nil
		case records.OperationCreate:
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		default:
			keepPending(r, rec)
			rec.SetProvisioningState(v1.ProvisioningStateFailed)
		}
		etag, err := e.store.Save(ctx, rec, etag)
		if err != nil {
			resp, err := saveError(r.id, err)
			return nil, "", resp, err
		}
		return rec, etag, nil, nil
	}

	logger.Info("Resuming an AWS operation nothing is polling", "alias", r.id.Name(), "operation", op, "awsIdentifier", rec.Properties.AWSIdentifier)
	queueMethod := v1.OperationPut
	if op == records.OperationDelete {
		queueMethod = v1.OperationDelete
	}
	operationID, err := e.track(ctx, r, rec, etag, queueMethod, op, rec.Properties.RequestToken)
	if err != nil {
		resp, err := saveError(r.id, err)
		return nil, "", resp, err
	}

	if queueMethod != method {
		return nil, "", conflict("An interrupted %s of the AWS resource %q was resumed. Retry when it finishes.", strings.ToLower(op), r.id.Name()), nil
	}
	if method == v1.OperationDelete {
		return nil, "", e.asyncResponse(r.id, map[string]any{}, 202, operationID), nil
	}
	body := map[string]any{"id": r.id.String(), "name": r.id.Name(), "type": r.id.Type(), "properties": map[string]any{"provisioningState": v1.ProvisioningStateProvisioning}}
	return nil, "", e.asyncResponse(r.id, body, 201, operationID), nil
}

// deleteStarted reports whether startDelete deleted the resource or started its delete, rather than failing.
func deleteStarted(resp armrpc_rest.Response) bool {
	switch resp.(type) {
	case *armrpc_rest.NoContentResponse, *armrpc_rest.AsyncOperationResponse:
		return true
	default:
		return false
	}
}

// reserve saves the record with operation op in progress before UCP calls AWS, so a competing PUT or DELETE gets 409
// until the operation finishes, or until it goes stale and the next request recovers it (see resume). It returns the
// new ETag, or a response that ends the request.
func (e *engine) reserve(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, op string) (string, armrpc_rest.Response, error) {
	rec.Properties.PendingOperation = op
	rec.Properties.RequestToken = ""
	rec.Properties.Handler = handlerName(r)
	if op != records.OperationDelete {
		rec.Properties.DeleteToken = ""
	}
	rec.SetProvisioningState(records.StateOf(op))
	etag, err := e.store.Save(ctx, rec, etag)
	if err != nil {
		resp, err := saveError(r.id, err)
		return "", resp, err
	}
	return etag, nil, nil
}

// release undoes a reservation after AWS rejected the call, so the next request does not wait for it to go stale.
// change sets the state to save. A failed save is logged: the reservation then goes stale and is recovered.
func (e *engine) release(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, change func(rec *datamodel.AWSResource)) {
	change(rec)
	if _, err := e.store.Save(ctx, rec, etag); err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "Failed to release an AWS operation that AWS did not accept", "alias", r.id.Name())
	}
}

// saveError turns a failed record save into a response: a 409 when another request changed the record, and an error
// otherwise.
func saveError(id resources.ID, err error) (armrpc_rest.Response, error) {
	if errors.Is(err, &database.ErrConcurrency{}) {
		return conflict("Another request changed the AWS resource %q at the same time. Retry.", id.Name()), nil
	}
	return nil, fmt.Errorf("failed to save the record of %s: %w", id.String(), err)
}

// checkIdentifier returns a 409 when another alias's record has the AWS identifier, in the type's identity scope, so
// two aliases never manage the same AWS resource. For a type whose metadata sets identityScope to account, that
// includes an alias in another region.
func (e *engine) checkIdentifier(ctx context.Context, r *request, identifier string) (armrpc_rest.Response, error) {
	scope := schemas.IdentityScopeRegion
	if r.native != nil {
		scope = r.native.Metadata.IdentityScope
	}
	holder, err := e.store.FindByIdentifier(ctx, r.id, identifier, scope)
	if err != nil {
		return nil, err
	}
	if holder != "" {
		return conflict("The AWS resource %q is already managed under the alias %q. Reference it with existing instead, or delete it from the other alias first.", identifier, records.AliasOf(holder)), nil
	}
	return nil, nil
}

// readResource reads a resource from AWS by its identifier, through the handler that serves r, without its write-only
// values. It returns nil when AWS has no such resource.
func (e *engine) readResource(ctx context.Context, r *request, identifier string) (*cloudcontrol.GetResourceOutput, error) {
	var out *cloudcontrol.GetResourceOutput
	var err error
	if r.handler != nil {
		out, err = readNative(ctx, r.handler, r.region, identifier, r.managed)
	} else {
		out, err = e.clients.CloudControl.GetResource(ctx, &cloudcontrol.GetResourceInput{
			TypeName:   aws.String(r.typeName),
			Identifier: aws.String(identifier),
		}, CloudControlRegionOption(r.region))
		if ucp_aws.IsAWSResourceNotFoundError(err) {
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return out, removeWriteOnly(out, r.writeOnly)
}

// put creates or updates the AWS resource under the alias in the URL.
//
//  1. Read the record. A running operation gets 409, unless it is stale or was never queued, and then the request
//     resumes it. A record that still has a client token has a first create that has not finished, so the request
//     sends the create again with that token (see recoverReplay when the template differs from the first create).
//  2. With no AWS resource in the record, look for one by the top-level name, then by the identifier in the
//     properties. If AWS has it, take it over. Otherwise save the record with a client token, and with the identifier
//     when the properties give it, and create. Before taking over or creating a resource whose identifier is known,
//     check that no other alias's record has it.
//  3. With an AWS resource, compute the update from the template, the last-applied properties and fingerprints, and
//     AWS. When nothing differs, save the new last-applied properties and fingerprints and return 200 without calling
//     AWS.
//  4. Save the new last-applied properties and fingerprints as pending values before calling AWS. Then save the
//     operation on the record, queue it, and return 201. The worker waits for AWS and updates the record.
//
// Each AWS call goes to the handler routeType picks: Cloud Control, or a native handler. An operation that has not
// finished continues with the handler that started it (see continueWith).
//
// The fingerprints of write-only values are made before step 1, so a request that needs the fingerprint key fails
// without it before UCP saves anything or calls Cloud Control.
func (e *engine) put(ctx context.Context, httpReq *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	r, resp := e.begin(ctx, httpReq)
	if resp != nil {
		return resp, nil
	}
	if resp := validateAlias(httpReq); resp != nil {
		return resp, nil
	}
	owner, resp := ownerFromRequest(httpReq)
	if resp != nil {
		return resp, nil
	}
	body, err := readRequestBody(httpReq)
	if err != nil {
		return badRequest("failed to read request body"), nil
	}
	name, resp := body.nameString()
	if resp != nil {
		return resp, nil
	}

	if err := e.route(ctx, r); err != nil {
		return handleError(ctx, r.id, err)
	}
	if name, err = resolveIdentifier(ctx, r.native, r.region, name); err != nil {
		return handleError(ctx, r.id, err)
	}
	typeInfo := r.typeInfo
	if typeInfo == nil {
		typeInfo, err = describeType(ctx, e.clients, r.typeName, r.region)
		if err != nil {
			return handleError(ctx, r.id, err)
		}
		if errResponse, err := putErrorResponse(r.typeName, typeInfo); err != nil {
			return nil, err
		} else if errResponse != nil {
			return errResponse, nil
		}
	}
	schema := aws.ToString(typeInfo.Schema)
	// UCP adds extra properties only to its read responses, so they are never sent to AWS or stored.
	extras, err := extraPropertyNames(r.typeName, schema)
	if err != nil {
		return nil, err
	}
	body.Properties = extraproperties.Without(extras, body.Properties)
	secrets, err := secretProperties(r.typeName, schema)
	if err != nil {
		return nil, err
	}
	parsed, err := parseTypeSchema(typeInfo)
	if err != nil {
		return nil, err
	}
	r.writeOnly = awsoperations.PropertyPaths(parsed.WriteOnlyProperties)
	fingerprints, resp, err := e.fingerprints(r, parsed, body.Properties)
	if resp != nil || err != nil {
		return resp, err
	}

	rec, etag, err := e.store.Get(ctx, r.id)
	if err != nil {
		return nil, err
	}
	if rec != nil {
		if err := continueWith(r, rec); err != nil {
			return nil, err
		}
		rec, etag, resp, err = e.checkRecord(ctx, r, rec, etag, owner, v1.OperationPut)
		if resp != nil || err != nil {
			return resp, err
		}
		if name != "" && rec.Properties.AWSIdentifier != "" && name != rec.Properties.AWSIdentifier {
			return conflict("The alias %q already refers to the AWS resource %q, so it cannot take over %q.", r.id.Name(), rec.Properties.AWSIdentifier, name), nil
		}
	} else {
		rec = records.NewRecord(r.id, owner)
	}

	properties := body.Properties
	// A lost create is found in the parent its properties name, whatever the name (see resumeNativeCreate).
	target := rec.Properties.AWSIdentifier
	if target == "" && !records.IsFirstCreate(rec) {
		target = name
	}
	if r.managed, resp, err = e.ownManaged(ctx, r, rec, target, properties); resp != nil || err != nil {
		return resp, err
	}
	identifier := rec.Properties.AWSIdentifier
	var current *cloudcontrol.GetResourceOutput
	applied := applied{properties: withoutSecrets(properties, secrets), fingerprints: fingerprints}
	switch {
	case records.IsFirstCreate(rec) && r.handler != nil:
		return e.resumeNativeCreate(ctx, r, rec, etag, properties, applied, typeInfo, parsed, secrets)
	case records.IsFirstCreate(rec):
		// The first create has not finished and nothing is polling it: send it again with the same client token, so
		// Cloud Control returns the first request if it reached AWS.
	case identifier != "":
		current, err = e.readResource(ctx, r, identifier)
		if err != nil {
			return handleError(ctx, r.id, err)
		}
		if current == nil {
			// Deleted outside Radius: create it again and update the record.
			logger.Info("The AWS resource of an alias no longer exists; creating it again", "alias", r.id.Name(), "awsIdentifier", identifier)
			rec.Properties.AWSIdentifier = ""
		}
	default:
		// No AWS resource under this alias yet. Take over one that already exists: first by name, then by the
		// identifier in the properties.
		lookup := name
		if lookup == "" {
			lookup, err = identifierFor(r.native, properties, schema)
			if err != nil {
				return identifierError(ctx, r, err)
			}
			if lookup, err = resolveIdentifier(ctx, r.native, r.region, lookup); err != nil {
				return handleError(ctx, r.id, err)
			}
		}
		if lookup != "" {
			current, err = e.readResource(ctx, r, lookup)
			if err != nil {
				return handleError(ctx, r.id, err)
			}
			identifier = lookup
		}
	}

	if current != nil {
		return e.update(ctx, r, rec, etag, current, identifier, properties, applied, typeInfo, parsed, secrets)
	}
	return e.create(ctx, r, rec, etag, name, properties, applied, typeInfo, parsed, secrets)
}

// applied is what a successful deploy records: the properties without write-only and secret values, and the
// fingerprints of the write-only values.
type applied struct {
	properties   map[string]any
	fingerprints *datamodel.AWSFingerprints
}

// update updates the AWS resource current, which AWS has under identifier. It takes the resource over when the
// record has no AWS identifier yet.
func (e *engine) update(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, current *cloudcontrol.GetResourceOutput, identifier string, properties map[string]any, applied applied, typeInfo *cloudformation.DescribeTypeOutput, parsed *typeSchema, secrets []string) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if found := aws.ToString(current.ResourceDescription.Identifier); found != "" {
		identifier = found
	}
	if rec.Properties.AWSIdentifier == "" {
		if resp, err := e.checkIdentifier(ctx, r, identifier); resp != nil || err != nil {
			return resp, err
		}
		logger.Info("Taking over an existing AWS resource", "alias", r.id.Name(), "awsIdentifier", identifier)
	}
	rec.Properties.AWSIdentifier = identifier

	currentState := []byte(aws.ToString(current.ResourceDescription.Properties))
	currentProperties := map[string]any{}
	if err := json.Unmarshal(currentState, &currentProperties); err != nil {
		return nil, err
	}

	schema := []byte(aws.ToString(typeInfo.Schema))
	removed := []string{}
	if records.HasApplied(rec) {
		removed = awsoperations.RemovedPaths(rec.Properties.LastApplied, properties)
	}
	createOnly, err := awsoperations.RemovedCreateOnly(currentProperties, schema, removed)
	if err != nil {
		return nil, err
	}
	if len(createOnly) > 0 {
		return badRequest(fmt.Sprintf("These create-only properties of the AWS resource %q were removed from the template, but AWS cannot clear them after create: %s. Set them again, or delete the resource and deploy again.", r.id.Name(), strings.Join(createOnly, ", "))), nil
	}

	desiredState, err := json.Marshal(properties)
	if err != nil {
		return nil, err
	}
	patch, err := awsoperations.GeneratePatchWithOptions(currentState, desiredState, schema, awsoperations.PatchOptions{
		Removed:       removed,
		Equivalent:    equivalentFor(r, rec.Properties.LastApplied != nil),
		LastApplied:   rec.Properties.LastApplied,
		Unresolved:    rec.Properties.UnresolvedPaths,
		FailUncertain: r.native != nil && r.native.Handler.Options().DisruptiveUpdate,
	})
	var uncertain *awsoperations.UncertainError
	if errors.As(err, &uncertain) {
		paths := strings.Join(uncertain.Paths, ", ")
		return conflict("UCP can't tell whether AWS added the values at %s of the AWS resource %q beyond the template's, or an earlier deploy set them, and an update can restart or interrupt a resource of type %s. UCP changed nothing. To keep AWS's values, set %s to the value AWS reports. After that deploy, UCP can tell what a later template removes.", paths, r.id.Name(), r.typeName, paths), nil
	} else if err != nil {
		return nil, err
	}
	// AWS never returns write-only values, so the patch sets every one the template sets. Leave out the ones the
	// fingerprints show need not be sent.
	immutable := typeInfo.ProvisioningType == cftypes.ProvisioningTypeImmutable
	if previous := rec.Properties.Fingerprints; applied.fingerprints != nil && (previous == nil || previous.Partial) {
		// UCP took the resource over, or the record is older than fingerprints, so it does not know every write-only
		// value the resource has. That stays true after this deploy.
		applied.fingerprints.Partial = true
	}
	patch = withoutUnsentWriteOnly(patch, parsed, immutable, compareFingerprints(rec.Properties.Fingerprints, applied.fingerprints))
	if len(patch) > 0 {
		if errResponse := checkRecordPatch(r.typeName, typeInfo, patch); errResponse != nil {
			return errResponse, nil
		}
		if errResponse := createOnlyChanged(r, parsed, patch); errResponse != nil {
			return errResponse, nil
		}
	}

	responseProperties := buildResponseProperties(currentProperties, properties, secrets)
	responseBody := map[string]any{"id": r.id.String(), "name": r.id.Name(), "type": r.id.Type(), "properties": responseProperties}

	if len(patch) == 0 {
		// Nothing to change in AWS, but the record takes this deploy's properties: a property the template now sets
		// and later removes is then cleared. Equivalent wasn't used at unresolved paths, so AWS has the template's
		// values there, and they are resolved.
		rec.Properties.LastApplied = applied.properties
		rec.Properties.UnresolvedPaths = nil
		rec.Properties.Fingerprints = applied.fingerprints
		records.ClearPending(rec)
		rec.Properties.RequestToken = ""
		rec.SetProvisioningState(v1.ProvisioningStateSucceeded)
		if _, err := e.store.Save(ctx, rec, etag); err != nil {
			return saveError(r.id, err)
		}
		responseProperties["provisioningState"] = v1.ProvisioningStateSucceeded
		return armrpc_rest.NewOKResponse(responseBody), nil
	}

	// Reserve the record with the values this deploy applies before calling AWS. The worker makes them last-applied
	// when AWS succeeds.
	prior := rec.ProvisioningState()
	rec.Properties.PendingLastApplied = applied.properties
	rec.Properties.PendingFingerprints = applied.fingerprints
	etag, resp, err := e.reserve(ctx, r, rec, etag, records.OperationUpdate)
	if resp != nil || err != nil {
		return resp, err
	}

	logger.Info("Updating AWS resource", "alias", r.id.Name(), "awsIdentifier", identifier, "handler", handlerName(r))
	requestToken, err := e.startUpdate(ctx, r, identifier, currentProperties, properties, patch)
	if err != nil {
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			// When AWS rejected the update, it changed nothing, and the previous last-applied properties stay. A
			// transport or server error, or a native update that failed partway, does not show that AWS changed
			// nothing, so Radius owns the pending properties, as when an interrupted update is recovered, and a later
			// deploy that omits one clears it.
			records.Settle(rec, outcomeOf(err))
			rec.SetProvisioningState(prior)
		})
		return handleError(ctx, r.id, err)
	}

	operationID, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationUpdate, requestToken)
	if err != nil {
		return nil, err
	}
	responseProperties["provisioningState"] = v1.ProvisioningStateProvisioning
	return e.asyncResponse(r.id, responseBody, 201, operationID), nil
}

// create creates the AWS resource. The record is saved with a client token before AWS is called, so a retry sends
// the same token and Cloud Control does not create a second resource.
func (e *engine) create(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, name string, properties map[string]any, applied applied, typeInfo *cloudformation.DescribeTypeOutput, parsed *typeSchema, secrets []string) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	schema := aws.ToString(typeInfo.Schema)

	// A record that already has a client token resumes a create that AWS may have accepted, so this request is a replay.
	// Otherwise this is a new create, with a new client token, which also seeds a generated name.
	replay := records.IsFirstCreate(rec)
	if !replay {
		rec.Properties.ClientToken = uuid.New().String()
	}
	properties, err := withGeneratedName(r, rec, properties)
	if err != nil {
		return identifierError(ctx, r, err)
	}

	// name takes over a resource that already exists. When AWS has none, create one only if the identifier properties
	// give that same name; otherwise the template asked for a resource that is not there.
	known, err := identifierFor(r.native, properties, schema)
	if err != nil {
		return identifierError(ctx, r, err)
	}
	if known, err = resolveIdentifier(ctx, r.native, r.region, known); err != nil {
		return handleError(ctx, r.id, err)
	}
	if name != "" && known != name {
		return badRequest(fmt.Sprintf("name %q does not match an AWS resource of type %s. Set name only to take over an existing resource, or remove it and set the identifier properties to create one.", name, r.typeName)), nil
	}
	if known == "" && r.handler != nil && r.handler.Options().Recovery == native.RecoverByIdentifier {
		return identifierRequired(r, schema)
	}
	if resp, err := e.checkIdentifier(ctx, r, known); resp != nil || err != nil {
		return resp, err
	}

	// Save the identifier the properties give before the create, so the identifier check also finds a create that is
	// still running. AWS assigns the others, and the worker saves them when the create finishes.
	rec.Properties.AWSIdentifier = known
	previous := pendingOf(rec)
	rec.Properties.PendingLastApplied = applied.properties
	rec.Properties.PendingFingerprints = applied.fingerprints
	if replay {
		// Until AWS answers, UCP cannot tell which attempt's write-only values AWS has, so the reservation saves no
		// fingerprints. A request that finds the reservation stale, or after a failed save, then sends every value that
		// can change. The fingerprints are set again once AWS shows which create it has.
		rec.Properties.PendingFingerprints = nil
	}
	etag, resp, err := e.reserve(ctx, r, rec, etag, records.OperationCreate)
	if resp != nil || err != nil {
		return resp, err
	}

	logger.Info("Creating AWS resource", "alias", r.id.Name(), "handler", handlerName(r))
	created, requestToken, err := e.startCreate(ctx, r, rec, properties)
	if err != nil {
		outcome := outcomeOf(err)
		if !replay && outcome == native.OutcomeRejected {
			// AWS rejected the first attempt, so nothing exists: remove the record, so the next deploy starts over.
			delErr := e.store.DeleteIfUnchanged(ctx, r.id, etag)
			if delErr == nil {
				return handleError(ctx, r.id, err)
			}
			logger.Error(delErr, "Failed to remove the record after AWS rejected a create", "alias", r.id.Name())
		}
		if replay {
			if token, ok := ucp_aws.ConflictingRequestToken(err, rec.Properties.ClientToken); ok {
				return e.recoverReplay(ctx, r, rec, etag, token, previous, properties, applied, typeInfo, parsed, secrets)
			}
		}
		// Otherwise AWS may have accepted the create. A client fault on a replay, such as throttling or a client token
		// conflict UCP cannot read, does not show that the original create failed. A native create that failed partway,
		// or after its last change, left a resource. The record keeps its client token, so the next deploy finds the
		// resource (see resumeNativeCreate), or sends the create again with it and Cloud Control returns the first
		// request instead of creating a second resource.
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			if created != "" {
				rec.Properties.AWSIdentifier = created
			}
			switch {
			case outcome == native.OutcomeApplied && rec.Properties.AWSIdentifier != "":
				// The native handler made every change, and UCP has the identifier, so the create is finished: its
				// values and fingerprints are last-applied, and the next deploy updates the resource as usual.
				records.Settle(rec, outcome)
				rec.Properties.ClientToken = ""
				rec.SetProvisioningState(v1.ProvisioningStateFailed)
				return
			case replay && outcome == native.OutcomeRejected:
				// AWS did nothing with a replay it rejected, so the pending values still describe the create it may
				// have.
				restorePending(rec, previous)
			}
			// After any other error, AWS may have the create from this attempt or an earlier one, and the pending
			// fingerprints stay unknown.
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
		return handleError(ctx, r.id, err)
	}
	// AWS returned the request for this body, so its fingerprints describe the create AWS has.
	rec.Properties.PendingFingerprints = applied.fingerprints
	if created != "" {
		rec.Properties.AWSIdentifier = created
	}

	operationID, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationCreate, requestToken)
	if err != nil {
		return nil, err
	}

	responseProperties := buildResponseProperties(nil, properties, secrets)
	responseProperties["provisioningState"] = v1.ProvisioningStateProvisioning
	responseBody := map[string]any{"id": r.id.String(), "name": r.id.Name(), "type": r.id.Type(), "properties": responseProperties}
	return e.asyncResponse(r.id, responseBody, 201, operationID), nil
}

// pendingOf returns the pending values of the record's operation in progress.
func pendingOf(rec *datamodel.AWSResource) applied {
	return applied{properties: rec.Properties.PendingLastApplied, fingerprints: rec.Properties.PendingFingerprints}
}

// restorePending sets the record's pending values back to p, the result of pendingOf.
func restorePending(rec *datamodel.AWSResource, p applied) {
	rec.Properties.PendingLastApplied = p.properties
	rec.Properties.PendingFingerprints = p.fingerprints
}

// recoverReplay finishes a resumed first create that AWS answered with ClientTokenConflictException naming the request
// token of the original create. The template differs from the original create's body, for example in write-only
// values, so AWS did not return the original request. The record gets back previous, the pending values it had before
// this request, which describe a create that may have reached AWS, and UCP reads the original request's status:
//
//   - succeeded: the pending values and fingerprints become last-applied, as when the worker finishes a create, the
//     record gets the identifier, and the request continues as an update with the template. The update compares the
//     template's write-only values with those fingerprints, as for any other update.
//   - failed: nothing exists, so the record is removed, as when the worker finds a create failed, and the request
//     returns the failure. The next deploy starts over.
//   - still running: UCP tracks the create and returns 409, and the caller deploys again when it finishes.
//   - reading the status fails: the record is kept, and the request returns the error.
func (e *engine) recoverReplay(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, requestToken string, previous applied, properties map[string]any, deploy applied, typeInfo *cloudformation.DescribeTypeOutput, parsed *typeSchema, secrets []string) (armrpc_rest.Response, error) {
	restorePending(rec, previous)
	status, err := ucp_aws.GetRequestStatus(ctx, e.clients.CloudControl, r.region, requestToken)
	if err != nil {
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
		return handleError(ctx, r.id, err)
	}
	switch {
	case !status.Done:
		if _, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationCreate, requestToken); err != nil {
			return nil, err
		}
		return conflict("A create of the AWS resource %q is still running in AWS. Deploy again when it finishes.", r.id.Name()), nil
	case !status.Succeeded:
		if err := e.store.DeleteIfUnchanged(ctx, r.id, etag); err != nil {
			return saveError(r.id, err)
		}
		return failureResponse(r.id, status), nil
	}

	rec.Properties.AWSIdentifier = status.Identifier
	records.ApplyPending(rec)
	etag, err = e.store.Save(ctx, rec, etag)
	if err != nil {
		return saveError(r.id, err)
	}
	current, err := e.readResource(ctx, r, status.Identifier)
	if err != nil {
		return handleError(ctx, r.id, err)
	}
	if current == nil {
		// Deleted outside Radius since the create: create it again.
		rec.Properties.AWSIdentifier = ""
		return e.create(ctx, r, rec, etag, "", properties, deploy, typeInfo, parsed, secrets)
	}
	return e.update(ctx, r, rec, etag, current, status.Identifier, properties, deploy, typeInfo, parsed, secrets)
}

// track saves the operation AWS accepted on the record, still in progress, and queues it. The request token is saved
// before queuing, so a request that finds the operation never queued polls it instead of starting over.
func (e *engine) track(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, method v1.OperationMethod, op string, requestToken string) (uuid.UUID, error) {
	operationID, err := uuid.Parse(requestToken)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("AWS Cloud Control returned the request token %q, which is not a UUID: %w", requestToken, err)
	}

	rec.Properties.RequestToken = requestToken
	rec.Properties.PendingOperation = op
	rec.SetProvisioningState(records.StateOf(op))
	etag, err = e.store.Save(ctx, rec, etag)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("AWS accepted the request for the alias %q, but UCP failed to save it. Deploy again to continue it: %w", r.id.Name(), err)
	}

	if err := e.queue(ctx, r, method, operationID, operationTimeout(r, op)); err != nil {
		// Nothing polls the operation, so it is not in progress: the next request polls the request token right away.
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
		return uuid.UUID{}, err
	}
	return operationID, nil
}

// queue queues the operation on UCP's async backend, which runs it for at most timeout. The Cloud Control request
// token, or the ID UCP made for a native operation, is the operation ID.
func (e *engine) queue(ctx context.Context, r *request, method v1.OperationMethod, operationID uuid.UUID, timeout time.Duration) error {
	if e.sm == nil {
		return errors.New("async operations are not available: the status manager is not configured")
	}

	sCtx := &v1.ARMRequestContext{}
	if armCtx := v1.ARMRequestContextFromContext(ctx); armCtx != nil {
		copied := *armCtx
		sCtx = &copied
	}
	sCtx.ResourceID = r.id
	sCtx.OperationID = operationID
	sCtx.OperationType = v1.OperationType{Type: operationType(r), Method: method}

	return e.sm.QueueAsyncOperation(ctx, sCtx, statusmanager.QueueOperationOptions{
		OperationTimeout: timeout,
		RetryAfter:       v1.DefaultRetryAfterDuration,
	})
}

func (e *engine) asyncResponse(id resources.ID, body any, code int, operationID uuid.UUID) armrpc_rest.Response {
	return armrpc_rest.NewAsyncOperationResponse(body, v1.LocationGlobal, code, id, operationID, "", id.RootScope(), e.pathBase)
}

// delete deletes the AWS resource under the alias in the URL. It checks the owner and deletes the resource in AWS by
// the record's identifier (see startDelete). An alias with no record returns 204 without calling AWS, so a retried
// DELETE never deletes a different resource that happens to have the alias as its identifier.
func (e *engine) delete(ctx context.Context, httpReq *http.Request) (armrpc_rest.Response, error) {
	r, resp := e.begin(ctx, httpReq)
	if resp != nil {
		return resp, nil
	}
	owner, resp := ownerFromRequest(httpReq)
	if resp != nil {
		return resp, nil
	}

	rec, etag, err := e.store.Get(ctx, r.id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return armrpc_rest.NewNoContentResponse(), nil
	}
	if err := e.route(ctx, r); err != nil {
		return handleError(ctx, r.id, err)
	}
	if err := continueWith(r, rec); err != nil {
		return nil, err
	}
	rec, etag, resp, err = e.checkRecord(ctx, r, rec, etag, owner, v1.OperationDelete)
	if resp != nil || err != nil {
		return resp, err
	}
	return e.startDelete(ctx, r, rec, etag)
}

// startDelete reserves the record and deletes its AWS resource, and the worker removes the record when AWS finishes.
// A record with no identifier and no client token has nothing in AWS, so only the record is removed. One with a client
// token has a create that may have reached AWS, which recoverCreate, or recoverNativeCreate for a native handler, finds
// first. When AWS has no resource, or the identifier now names a resource of another product, only the record is
// removed (see isGoneForDelete).
//
// The type's onDelete metadata can change that: nothing removes only the record, leave keeps the record and returns
// 204, and reset has the native handler reset the resource instead of deleting it (see startDeleteCall). Neither
// nothing nor leave calls AWS. reject returns 400 and changes nothing when AWS has the resource, and removes the record
// when it doesn't: a record whose create may have reached AWS finds the resource first, as for any delete, except that
// it never sends the create again, so a create only a resend could find removes only the record.
func (e *engine) startDelete(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	managed, refused, err := e.ownManaged(ctx, r, rec, rec.Properties.AWSIdentifier, nil)
	if refused != nil || err != nil {
		return refused, err
	}
	r.managed = managed
	switch onDelete(r) {
	case schemas.OnDeleteNothing:
		logger.Info("Removing the record of an AWS resource without changing it in AWS", "alias", r.id.Name(), "awsIdentifier", rec.Properties.AWSIdentifier)
		return e.removeRecord(ctx, r, etag)
	case schemas.OnDeleteLeave:
		logger.Info("Leaving an AWS resource and its record as they are", "alias", r.id.Name(), "awsIdentifier", rec.Properties.AWSIdentifier)
		return armrpc_rest.NewNoContentResponse(), nil
	}
	if rec.Properties.AWSIdentifier == "" && !records.IsFirstCreate(rec) {
		return e.removeRecord(ctx, r, etag)
	}
	// A native first create is found below, with the steps a PUT takes. Any other record with an identifier is read by it,
	// including a Cloud Control first create, whose identifier the template gave.
	if onDelete(r) == schemas.OnDeleteReject && rec.Properties.AWSIdentifier != "" && (r.handler == nil || !records.IsFirstCreate(rec)) {
		current, err := e.readResource(ctx, r, rec.Properties.AWSIdentifier)
		switch {
		case isGoneForDelete(err), err == nil && current == nil:
			return e.removeRecord(ctx, r, etag)
		case err != nil:
			return handleError(ctx, r.id, err)
		}
		return rejected(r), nil
	}
	if onDelete(r) == schemas.OnDeleteReject && rec.Properties.AWSIdentifier == "" && records.IsFirstCreate(rec) && findsCreateByResend(r) {
		// The only way to find this create is to send it again, which creates the resource when AWS never had it, and a
		// DELETE never creates one. Nothing Radius can show it made exists, so only the record is removed.
		logger.Info("Removing the record of an interrupted create that only a resend could find", "alias", r.id.Name())
		return e.removeRecord(ctx, r, etag)
	}

	// A resumed delete finds the record still marked in progress; if AWS rejects the delete, it is released as Failed.
	prior := rec.ProvisioningState()
	if records.IsRunning(rec) {
		prior = v1.ProvisioningStateFailed
	}
	etag, resp, err := e.reserve(ctx, r, rec, etag, records.OperationDelete)
	if resp != nil || err != nil {
		return resp, err
	}
	switch {
	case records.IsFirstCreate(rec) && r.handler != nil:
		etag, resp, err = e.recoverNativeCreate(ctx, r, rec, etag)
		if resp != nil || err != nil {
			return resp, err
		}
	case rec.Properties.AWSIdentifier == "":
		etag, resp, err = e.recoverCreate(ctx, r, rec, etag)
		if resp != nil || err != nil {
			return resp, err
		}
	}
	if onDelete(r) == schemas.OnDeleteReject {
		// The create reached AWS, and the record now has its identifier. recoverNativeCreate already kept the create's
		// values as possibly applied, so a later deploy that leaves one out clears it.
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			records.ClearPending(rec)
			rec.SetProvisioningState(prior)
		})
		return rejected(r), nil
	}

	identifier := rec.Properties.AWSIdentifier
	etag, err = e.prepareDelete(ctx, r, rec, etag, identifier)
	if err != nil {
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			records.ClearPending(rec)
			rec.SetProvisioningState(prior)
		})
		return handleError(ctx, r.id, err)
	}
	logger.Info("Deleting AWS resource", "alias", r.id.Name(), "awsIdentifier", identifier, "handler", handlerName(r))
	requestToken, err := e.startDeleteCall(ctx, r, identifier, rec.Properties.DeleteToken)
	if isGoneForDelete(err) {
		return e.removeRecord(ctx, r, etag)
	} else if err != nil {
		if outcomeOf(err) == native.OutcomeApplied {
			// The native handler made every change, so the delete or reset is done, as when the worker finds it so.
			delErr := e.store.DeleteIfUnchanged(ctx, r.id, etag)
			if delErr == nil {
				return handleError(ctx, r.id, err)
			}
			logger.Error(delErr, "Failed to remove the record after AWS applied a delete", "alias", r.id.Name())
		}
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			records.ClearPending(rec)
			rec.SetProvisioningState(prior)
		})
		return handleError(ctx, r.id, err)
	}

	operationID, err := e.track(ctx, r, rec, etag, v1.OperationDelete, records.OperationDelete, requestToken)
	if err != nil {
		return nil, err
	}
	return e.asyncResponse(r.id, map[string]any{}, 202, operationID), nil
}

// recoverCreate finds the AWS resource of a first create that may have reached AWS, before a DELETE: the record has a
// client token but no identifier, for example because UCP stopped after AWS accepted the create and before it saved
// the request. It sends the create again with the record's client token, so Cloud Control returns the original
// request when AWS accepted it, and reads that request's status:
//
//   - succeeded: the record gets the identifier, and the delete continues.
//   - failed: nothing exists, so only the record is removed.
//   - AWS rejects the resend: the record is kept, and the DELETE returns the error, because a rejected replay does not
//     show that the original create failed.
//   - still running: UCP tracks the create and returns 409, and the caller deletes again when it finishes.
//
// The resend sends the pending properties, which have no write-only values. Cloud Control returns the original request
// only when the body matches it exactly, so for a type with write-only properties it returns
// ClientTokenConflictException instead, naming the original request. UCP reads that request's status the same way.
//
// When AWS never had the create, the resend creates the resource, and the delete then removes it. Cloud Control keeps a
// client token for 36 hours, so after that a resend cannot find the original request and creates a second resource.
// Because the resend may be what creates the resource, the pending fingerprints are dropped: they describe values this
// create may not have set.
func (e *engine) recoverCreate(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string) (string, armrpc_rest.Response, error) {
	rec.Properties.PendingFingerprints = nil
	desired := rec.Properties.PendingLastApplied
	if desired == nil {
		desired = map[string]any{}
	}
	desiredState, err := json.Marshal(desired)
	if err != nil {
		return "", nil, err
	}

	response, err := e.clients.CloudControl.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     aws.String(r.typeName),
		DesiredState: aws.String(string(desiredState)),
		ClientToken:  aws.String(rec.Properties.ClientToken),
	}, CloudControlRegionOption(r.region))
	var requestToken string
	if err == nil {
		requestToken = aws.ToString(response.ProgressEvent.RequestToken)
	} else if token, ok := ucp_aws.ConflictingRequestToken(err, rec.Properties.ClientToken); ok {
		requestToken = token
	} else {
		// The resend is a replay, so an error, even a client fault such as throttling, does not show that the original
		// create failed. Keep the record so the next DELETE tries again.
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			rec.Properties.PendingOperation = records.OperationCreate
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
		resp, err := handleError(ctx, r.id, err)
		return "", resp, err
	}

	status, err := ucp_aws.GetRequestStatus(ctx, e.clients.CloudControl, r.region, requestToken)
	if err != nil {
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			rec.Properties.PendingOperation = records.OperationCreate
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
		resp, err := handleError(ctx, r.id, err)
		return "", resp, err
	}
	switch {
	case !status.Done:
		if _, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationCreate, requestToken); err != nil {
			return "", nil, err
		}
		return "", conflict("A create of the AWS resource %q is still running in AWS. Delete again when it finishes.", r.id.Name()), nil
	case !status.Succeeded:
		resp, err := e.removeRecord(ctx, r, etag)
		return "", resp, err
	}

	rec.Properties.AWSIdentifier = status.Identifier
	rec.Properties.ClientToken = ""
	etag, err = e.store.Save(ctx, rec, etag)
	if err != nil {
		resp, err := saveError(r.id, err)
		return "", resp, err
	}
	return etag, nil, nil
}

// removeRecord removes the record when AWS has nothing to delete. It removes the record only if it is unchanged since
// the request read it at etag, so it never removes a record another request saved meanwhile: that request keeps the
// alias, and this one gets 409. When the removal fails otherwise, the request fails, and the caller deletes again.
func (e *engine) removeRecord(ctx context.Context, r *request, etag string) (armrpc_rest.Response, error) {
	if err := e.store.DeleteIfUnchanged(ctx, r.id, etag); errors.Is(err, &database.ErrConcurrency{}) {
		return saveError(r.id, err)
	} else if err != nil {
		return nil, fmt.Errorf("the AWS resource for the alias %q is deleted, but UCP failed to remove its record. Delete again: %w", r.id.Name(), err)
	}
	return armrpc_rest.NewNoContentResponse(), nil
}

// get reads the AWS resource under the alias in the URL: the record, then AWS by the identifier in the record. It
// always reads AWS, so the result shows changes made outside Radius. It does not check the owner, because reading
// another owner's resource does not change it.
func (e *engine) get(ctx context.Context, httpReq *http.Request) (armrpc_rest.Response, error) {
	r, resp := e.begin(ctx, httpReq)
	if resp != nil {
		return resp, nil
	}

	rec, _, err := e.store.Get(ctx, r.id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return noRecordResponse(r.id), nil
	}
	if rec.Properties.AWSIdentifier == "" {
		return armrpc_rest.NewNotFoundResponse(r.id), nil
	}
	if err := e.route(ctx, r); err != nil {
		return handleError(ctx, r.id, err)
	}
	if r.managed, resp, err = e.ownManaged(ctx, r, rec, rec.Properties.AWSIdentifier, nil); resp != nil || err != nil {
		return resp, err
	}

	current, err := e.readResource(ctx, r, rec.Properties.AWSIdentifier)
	if err != nil {
		return handleError(ctx, r.id, err)
	}
	if current == nil {
		return armrpc_rest.NewNotFoundResponse(r.id), nil
	}

	properties := map[string]any{}
	if current.ResourceDescription.Properties != nil {
		if err := json.Unmarshal([]byte(*current.ResourceDescription.Properties), &properties); err != nil {
			return nil, err
		}
	}
	if extraproperties.Names(r.typeName) != nil {
		// Only types with extra properties need the schema here. DescribeType answers are cached, and a type with a
		// native handler already has its schema from routing.
		typeInfo := r.typeInfo
		if typeInfo == nil {
			typeInfo, err = describeType(ctx, e.clients, r.typeName, r.region)
			if err != nil {
				return ucp_aws.HandleAWSError(ctx, err)
			}
		}
		if resp, err := addExtraProperties(ctx, e.clients.Kafka, r.typeName, r.region, aws.ToString(typeInfo.Schema), properties); resp != nil || err != nil {
			return resp, err
		}
	}
	return armrpc_rest.NewOKResponse(map[string]any{
		"id":         r.id.String(),
		"name":       r.id.Name(),
		"type":       r.id.Type(),
		"properties": properties,
	}), nil
}

// noRecordResponse returns the 404 for an AWS resource ID that has no record. Its code tells it apart from a record
// whose AWS resource is gone, so a caller can read a resource Radius did not deploy with POST /:get instead, and never
// does so for an alias, which could read an unrelated resource whose AWS identifier matches the alias.
func noRecordResponse(id resources.ID) armrpc_rest.Response {
	return &armrpc_rest.NotFoundResponse{Body: v1.ErrorResponse{Error: &v1.ErrorDetails{
		Code:    sdkclients.AWSResourceNoRecordCode,
		Message: fmt.Sprintf("UCP has no record of the AWS resource %q. Deploy it by alias, or reference it with existing and name or properties.", id.String()),
		Target:  id.String(),
	}}}
}

// identifierFromProperties returns the AWS identifier the properties give, or "" when they do not set every primary
// identifier property.
func identifierFromProperties(properties map[string]any, schema string) (string, error) {
	identifier, err := getPrimaryIdentifierFromMultiIdentifiers(properties, schema)
	if errors.Is(err, &ucp_aws.AWSMissingPropertyError{}) {
		return "", nil
	}
	return identifier, err
}

// withoutSecrets returns a copy of properties without write-only and other secret values, which are never stored.
func withoutSecrets(properties map[string]any, secrets []string) map[string]any {
	out, _ := deepCopy(properties).(map[string]any)
	removePaths(out, secrets)
	return out
}

// requestBody is a request body of the form {"name": ..., "properties": {...}}.
type requestBody struct {
	Name       json.RawMessage `json:"name"`
	Properties map[string]any  `json:"properties"`
}

func readRequestBody(req *http.Request) (*requestBody, error) {
	defer req.Body.Close()
	body := &requestBody{}
	if err := json.NewDecoder(req.Body).Decode(body); err != nil {
		return nil, err
	}
	if body.Properties == nil {
		body.Properties = map[string]any{}
	}
	return body, nil
}

// nameString returns the top-level name, which names an AWS resource that already exists. It returns "" when the
// body has none, and a 400 when it is not a string.
func (b *requestBody) nameString() (string, armrpc_rest.Response) {
	if len(b.Name) == 0 || string(b.Name) == "null" {
		return "", nil
	}
	var name string
	if err := json.Unmarshal(b.Name, &name); err != nil {
		return "", badRequest("name must be a string")
	}
	return strings.TrimSpace(name), nil
}
