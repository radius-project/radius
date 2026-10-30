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

	"uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/components/database"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
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
// it again so a worker polls the original request, and returns the response. Without one, AWS never accepted it:
// resume clears it and the request continues. A first create keeps its client token, so the request sends the create
// again with the same token.
func (e *engine) resume(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, method v1.OperationMethod) (*datamodel.AWSResource, string, armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	op := records.PendingOperation(rec)

	if rec.Properties.RequestToken == "" {
		logger.Info("Clearing an AWS operation that never reached AWS", "alias", r.id.Name(), "operation", op)
		records.ClearPending(rec)
		rec.SetProvisioningState(v1.ProvisioningStateFailed)
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

// saveError turns a failed record save into a response: a 409 when another request changed the record, and an error
// otherwise.
func saveError(id resources.ID, err error) (armrpc_rest.Response, error) {
	if errors.Is(err, &database.ErrConcurrency{}) {
		return conflict("Another request changed the AWS resource %q at the same time. Retry.", id.Name()), nil
	}
	return nil, fmt.Errorf("failed to save the record of %s: %w", id.String(), err)
}

// checkIdentifier returns a 409 when another alias's record has the AWS identifier, so two aliases never manage the
// same AWS resource.
func (e *engine) checkIdentifier(ctx context.Context, id resources.ID, identifier string) (armrpc_rest.Response, error) {
	holder, err := e.store.FindByIdentifier(ctx, id, identifier)
	if err != nil {
		return nil, err
	}
	if holder != "" {
		return conflict("The AWS resource %q is already managed under the alias %q. Reference it with existing instead, or delete it from the other alias first.", identifier, records.AliasOf(holder)), nil
	}
	return nil, nil
}

// readResource reads a resource from AWS by its identifier. It returns nil when AWS has no such resource.
func (e *engine) readResource(ctx context.Context, r *request, identifier string) (*cloudcontrol.GetResourceOutput, error) {
	out, err := e.clients.CloudControl.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   aws.String(r.typeName),
		Identifier: aws.String(identifier),
	}, CloudControlRegionOption(r.region))
	if ucp_aws.IsAWSResourceNotFoundError(err) {
		return nil, nil
	}
	return out, err
}

func (e *engine) describeType(ctx context.Context, r *request) (*cloudformation.DescribeTypeOutput, error) {
	return e.clients.CloudFormation.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     cftypes.RegistryTypeResource,
		TypeName: aws.String(r.typeName),
	}, CloudFormationRegionOption(r.region))
}

// put creates or updates the AWS resource under the alias in the URL.
//
//  1. Read the record. A running operation gets 409, unless it is stale or was never queued, and then the request
//     resumes it. A record that still has a client token has a first create that has not finished, so the request
//     sends the create again with that token.
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

	typeInfo, err := e.describeType(ctx, r)
	if err != nil {
		return ucp_aws.HandleAWSError(ctx, err)
	}
	if errResponse, err := putErrorResponse(r.typeName, typeInfo); err != nil {
		return nil, err
	} else if errResponse != nil {
		return errResponse, nil
	}
	schema := aws.ToString(typeInfo.Schema)
	secrets, err := secretProperties(r.typeName, schema)
	if err != nil {
		return nil, err
	}
	parsed, err := parseTypeSchema(typeInfo)
	if err != nil {
		return nil, err
	}
	fingerprints, resp, err := e.fingerprints(r, parsed, body.Properties)
	if resp != nil || err != nil {
		return resp, err
	}

	rec, etag, err := e.store.Get(ctx, r.id)
	if err != nil {
		return nil, err
	}
	if rec != nil {
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
	identifier := rec.Properties.AWSIdentifier
	var current *cloudcontrol.GetResourceOutput
	switch {
	case records.IsFirstCreate(rec):
		// The first create has not finished and nothing is polling it: send it again with the same client token, so
		// Cloud Control returns the first request if it reached AWS.
	case identifier != "":
		current, err = e.readResource(ctx, r, identifier)
		if err != nil {
			return ucp_aws.HandleAWSError(ctx, err)
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
			lookup, err = identifierFromProperties(properties, schema)
			if err != nil {
				return nil, err
			}
		}
		if lookup != "" {
			current, err = e.readResource(ctx, r, lookup)
			if err != nil {
				return ucp_aws.HandleAWSError(ctx, err)
			}
			identifier = lookup
		}
	}

	applied := applied{properties: withoutSecrets(properties, secrets), fingerprints: fingerprints}
	if current != nil {
		return e.update(ctx, r, rec, etag, current, identifier, properties, applied, typeInfo, parsed, secrets)
	}
	return e.create(ctx, r, rec, etag, name, properties, applied, schema, secrets)
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
		if resp, err := e.checkIdentifier(ctx, r.id, identifier); resp != nil || err != nil {
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
	patch, err := awsoperations.GeneratePatchWithOptions(currentState, desiredState, schema, awsoperations.PatchOptions{Removed: removed})
	if err != nil {
		return nil, err
	}
	// AWS never returns write-only values, so the patch sets every one the template sets. Leave out the ones the
	// fingerprints show need not be sent.
	immutable := typeInfo.ProvisioningType == cftypes.ProvisioningTypeImmutable
	patch = withoutUnsentWriteOnly(patch, parsed, immutable, compareFingerprints(rec.Properties.Fingerprints, applied.fingerprints))
	if len(patch) > 0 {
		if errResponse := checkPatch(r.typeName, typeInfo, patch); errResponse != nil {
			return errResponse, nil
		}
	}

	responseProperties := buildResponseProperties(currentProperties, properties, secrets)
	responseBody := map[string]any{"id": r.id.String(), "name": r.id.Name(), "type": r.id.Type(), "properties": responseProperties}

	if len(patch) == 0 {
		// Nothing to change in AWS, but the record takes this deploy's properties: a property the template now sets
		// and later removes is then cleared.
		rec.Properties.LastApplied = applied.properties
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

	// Save the values this deploy applies before calling AWS. The worker makes them last-applied when AWS succeeds.
	rec.Properties.PendingLastApplied = applied.properties
	rec.Properties.PendingFingerprints = applied.fingerprints
	rec.Properties.PendingOperation = records.OperationUpdate
	rec.Properties.RequestToken = ""
	etag, err = e.store.Save(ctx, rec, etag)
	if err != nil {
		return saveError(r.id, err)
	}

	marshaled, err := json.Marshal(&patch)
	if err != nil {
		return nil, err
	}
	logger.Info("Updating AWS resource", "alias", r.id.Name(), "awsIdentifier", identifier)
	response, err := e.clients.CloudControl.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
		TypeName:      aws.String(r.typeName),
		Identifier:    aws.String(identifier),
		PatchDocument: aws.String(string(marshaled)),
	}, CloudControlRegionOption(r.region))
	if err != nil {
		// AWS changed nothing, so the previous last-applied properties stay.
		records.ClearPending(rec)
		if _, saveErr := e.store.Save(ctx, rec, etag); saveErr != nil {
			logger.Error(saveErr, "Failed to clear the pending values after AWS rejected an update", "alias", r.id.Name())
		}
		return ucp_aws.HandleAWSError(ctx, err)
	}

	operationID, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationUpdate, aws.ToString(response.ProgressEvent.RequestToken))
	if err != nil {
		return nil, err
	}
	responseProperties["provisioningState"] = v1.ProvisioningStateProvisioning
	return e.asyncResponse(r.id, responseBody, 201, operationID), nil
}

// create creates the AWS resource. The record is saved with a client token before AWS is called, so a retry sends
// the same token and Cloud Control does not create a second resource.
func (e *engine) create(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, name string, properties map[string]any, applied applied, schema string, secrets []string) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// name takes over a resource that already exists. When AWS has none, create one only if the identifier properties
	// give that same name; otherwise the template asked for a resource that is not there.
	known, err := identifierFromProperties(properties, schema)
	if err != nil {
		return nil, err
	}
	if name != "" && known != name {
		return badRequest(fmt.Sprintf("name %q does not match an AWS resource of type %s. Set name only to take over an existing resource, or remove it and set the identifier properties to create one.", name, r.typeName)), nil
	}
	if resp, err := e.checkIdentifier(ctx, r.id, known); resp != nil || err != nil {
		return resp, err
	}

	if !records.IsFirstCreate(rec) {
		rec.Properties.ClientToken = uuid.New().String()
	}
	// Save the identifier the properties give before the create, so the identifier check also finds a create that is
	// still running. AWS assigns the others, and the worker saves them when the create finishes.
	rec.Properties.AWSIdentifier = known
	rec.Properties.RequestToken = ""
	rec.Properties.PendingLastApplied = applied.properties
	rec.Properties.PendingFingerprints = applied.fingerprints
	rec.Properties.PendingOperation = records.OperationCreate
	rec.SetProvisioningState(v1.ProvisioningStateAccepted)
	etag, err = e.store.Save(ctx, rec, etag)
	if err != nil {
		return saveError(r.id, err)
	}

	desiredState, err := json.Marshal(properties)
	if err != nil {
		return nil, err
	}
	logger.Info("Creating AWS resource", "alias", r.id.Name())
	response, err := e.clients.CloudControl.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     aws.String(r.typeName),
		DesiredState: aws.String(string(desiredState)),
		ClientToken:  aws.String(rec.Properties.ClientToken),
	}, CloudControlRegionOption(r.region))
	if err != nil {
		if ucp_aws.IsClientFault(err) {
			// AWS rejected the create, so nothing exists: remove the record, so the next deploy starts over.
			if delErr := e.store.Delete(ctx, r.id); delErr != nil {
				logger.Error(delErr, "Failed to remove the record after AWS rejected a create", "alias", r.id.Name())
			}
		}
		// Otherwise AWS may have accepted the create. The record keeps its client token, so the next deploy sends
		// the create again with it and Cloud Control returns the first request instead of creating a second resource.
		return ucp_aws.HandleAWSError(ctx, err)
	}
	if id := aws.ToString(response.ProgressEvent.Identifier); id != "" {
		rec.Properties.AWSIdentifier = id
	}

	operationID, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationCreate, aws.ToString(response.ProgressEvent.RequestToken))
	if err != nil {
		return nil, err
	}

	responseProperties := buildResponseProperties(nil, properties, secrets)
	responseProperties["provisioningState"] = v1.ProvisioningStateProvisioning
	responseBody := map[string]any{"id": r.id.String(), "name": r.id.Name(), "type": r.id.Type(), "properties": responseProperties}
	return e.asyncResponse(r.id, responseBody, 201, operationID), nil
}

// track saves the operation AWS accepted on the record, queues it, and then marks the record in progress. The request
// token is saved before queuing, so a request that finds the operation never queued polls it instead of starting
// over.
func (e *engine) track(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, method v1.OperationMethod, op string, requestToken string) (uuid.UUID, error) {
	operationID, err := uuid.Parse(requestToken)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("AWS Cloud Control returned the request token %q, which is not a UUID: %w", requestToken, err)
	}

	rec.Properties.RequestToken = requestToken
	rec.Properties.PendingOperation = op
	rec.SetProvisioningState(v1.ProvisioningStateAccepted)
	etag, err = e.store.Save(ctx, rec, etag)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("AWS accepted the request for the alias %q, but UCP failed to save it. Deploy again to continue it: %w", r.id.Name(), err)
	}

	if err := e.queue(ctx, r.id, method, operationID); err != nil {
		// Nothing polls the operation, so it is not in progress: the next request polls the request token.
		return uuid.UUID{}, err
	}

	rec.SetProvisioningState(records.StateOf(op))
	if _, err := e.store.Save(ctx, rec, etag); err != nil && !errors.Is(err, &database.ErrConcurrency{}) {
		// The worker marks the record in progress when it starts the operation, and a changed record means it did.
		ucplog.FromContextOrDiscard(ctx).Error(err, "Failed to mark an AWS operation in progress", "alias", r.id.Name())
	}
	return operationID, nil
}

// queue queues the operation on UCP's async backend. The Cloud Control request token is the operation ID.
func (e *engine) queue(ctx context.Context, id resources.ID, method v1.OperationMethod, operationID uuid.UUID) error {
	if e.sm == nil {
		return errors.New("async operations are not available: the status manager is not configured")
	}

	sCtx := &v1.ARMRequestContext{}
	if armCtx := v1.ARMRequestContextFromContext(ctx); armCtx != nil {
		copied := *armCtx
		sCtx = &copied
	}
	sCtx.ResourceID = id
	sCtx.OperationID = operationID
	sCtx.OperationType = v1.OperationType{Type: records.OperationType, Method: method}

	return e.sm.QueueAsyncOperation(ctx, sCtx, statusmanager.QueueOperationOptions{
		OperationTimeout: ucp_aws.OperationTimeout,
		RetryAfter:       v1.DefaultRetryAfterDuration,
	})
}

func (e *engine) asyncResponse(id resources.ID, body any, code int, operationID uuid.UUID) armrpc_rest.Response {
	return armrpc_rest.NewAsyncOperationResponse(body, v1.LocationGlobal, code, id, operationID, "", id.RootScope(), e.pathBase)
}

// delete deletes the AWS resource under the alias in the URL. It checks the owner, deletes the resource in AWS by the
// record's identifier, and the worker removes the record when AWS finishes. When the record has no identifier yet, or
// AWS says the resource does not exist, only the record is removed. An alias with no record returns 204 without
// calling AWS, so a retried DELETE never deletes a different resource that happens to have the alias as its
// identifier.
func (e *engine) delete(ctx context.Context, httpReq *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
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
	rec, etag, resp, err = e.checkRecord(ctx, r, rec, etag, owner, v1.OperationDelete)
	if resp != nil || err != nil {
		return resp, err
	}

	identifier := rec.Properties.AWSIdentifier
	if identifier == "" {
		// AWS has not assigned an identifier yet, so there is nothing in AWS UCP can find to delete.
		return e.removeRecord(ctx, r.id)
	}

	logger.Info("Deleting AWS resource", "alias", r.id.Name(), "awsIdentifier", identifier)
	response, err := e.clients.CloudControl.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   aws.String(r.typeName),
		Identifier: aws.String(identifier),
	}, CloudControlRegionOption(r.region))
	if ucp_aws.IsAWSResourceNotFoundError(err) {
		return e.removeRecord(ctx, r.id)
	} else if err != nil {
		return ucp_aws.HandleAWSError(ctx, err)
	}

	records.ClearPending(rec)
	operationID, err := e.track(ctx, r, rec, etag, v1.OperationDelete, records.OperationDelete, aws.ToString(response.ProgressEvent.RequestToken))
	if err != nil {
		return nil, err
	}
	return e.asyncResponse(r.id, map[string]any{}, 202, operationID), nil
}

// removeRecord removes the record when AWS has nothing to delete. When that fails, the request fails, and the caller
// deletes again.
func (e *engine) removeRecord(ctx context.Context, id resources.ID) (armrpc_rest.Response, error) {
	if err := e.store.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("the AWS resource for the alias %q is deleted, but UCP failed to remove its record. Delete again: %w", id.Name(), err)
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
	if rec == nil || rec.Properties.AWSIdentifier == "" {
		return armrpc_rest.NewNotFoundResponse(r.id), nil
	}

	current, err := e.readResource(ctx, r, rec.Properties.AWSIdentifier)
	if err != nil {
		return ucp_aws.HandleAWSError(ctx, err)
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
	return armrpc_rest.NewOKResponse(map[string]any{
		"id":         r.id.String(),
		"name":       r.id.Name(),
		"type":       r.id.Type(),
		"properties": properties,
	}), nil
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
