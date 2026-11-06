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
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/wI2L/jsondiff"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/aws/schemas"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_aws "github.com/radius-project/radius/pkg/ucp/resources/aws"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// nativeTimeoutMargin is how much longer UCP's async operation runs than the handler's timeout, so the worker stops
// waiting first and reports the state it waited for.
const nativeTimeoutMargin = 5 * time.Minute

// route is the handler that serves a request.
type route struct {
	// native is the type's native handler and schema, or nil when no native handler serves the type.
	native *native.Type

	// handler serves the request: native.Handler when it does, and nil for Cloud Control.
	handler native.Handler

	// typeInfo is the type's information, with the native schema in place of CloudFormation's. It is nil when no
	// native handler serves the type, because then routing reads nothing from AWS.
	typeInfo *cloudformation.DescribeTypeOutput

	// writeOnly are the paths of the type's write-only properties, which UCP removes from every resource AWS returns.
	// routeType sets them from the native schema, and put from the schema it uses. They are nil for a Cloud Control
	// type otherwise, because Cloud Control never returns write-only values.
	writeOnly []string
}

// routeType picks the handler for typeName in region:
//
//  1. A hand-written type, which has no CloudFormation type, always uses its native handler. UCP never asks
//     CloudFormation about it.
//  2. A type with a native handler uses it only when CloudFormation's DescribeType reports it NON_PROVISIONABLE in the
//     region, meaning Cloud Control can't create it there. Otherwise Cloud Control serves it. DescribeType results are
//     cached, so this adds no AWS calls to a deploy, and a region that gains Cloud Control support switches without a
//     release. A failed DescribeType fails the request: UCP never guesses the handler.
//  3. Any other type uses Cloud Control, and routing calls nothing.
func routeType(ctx context.Context, clients ucp_aws.Clients, typeName string, region string) (route, error) {
	t := clients.Native.Lookup(typeName)
	if t == nil {
		return route{}, nil
	}
	if t.HandWritten {
		return route{native: t, handler: t.Handler, writeOnly: t.WriteOnly, typeInfo: &cloudformation.DescribeTypeOutput{
			TypeName:         aws.String(typeName),
			Schema:           aws.String(t.Schema),
			ProvisioningType: cftypes.ProvisioningTypeFullyMutable,
		}}, nil
	}

	info, err := describeType(ctx, clients, typeName, region)
	if err != nil {
		return route{}, err
	}
	// The cache shares its output, so change a copy.
	withSchema := *info
	withSchema.Schema = aws.String(t.Schema)
	rt := route{native: t, typeInfo: &withSchema, writeOnly: t.WriteOnly}
	if info.ProvisioningType == cftypes.ProvisioningTypeNonProvisionable {
		rt.handler = t.Handler
	}
	return rt, nil
}

func describeType(ctx context.Context, clients ucp_aws.Clients, typeName string, region string) (*cloudformation.DescribeTypeOutput, error) {
	return clients.CloudFormation.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     cftypes.RegistryTypeResource,
		TypeName: aws.String(typeName),
	}, CloudFormationRegionOption(region))
}

// route picks the handler that serves r, as routeType does, and saves it on r.
func (e *engine) route(ctx context.Context, r *request) error {
	rt, err := routeType(ctx, e.clients, r.typeName, r.region)
	if err != nil {
		return err
	}
	r.route = rt
	return nil
}

// continueWith makes the handler that started the record's operation serve the request, when the record has one
// that has not finished. It fails when a native handler started it and UCP no longer has one for the type.
func continueWith(r *request, rec *datamodel.AWSResource) error {
	if rec.Properties.PendingOperation == "" && rec.Properties.RequestToken == "" && !records.IsFirstCreate(rec) {
		return nil
	}
	if rec.Properties.Handler != records.HandlerNative {
		r.handler = nil
		return nil
	}
	if r.native == nil {
		return fmt.Errorf("a native handler started the operation on the AWS resource %q, but UCP has no native handler for %s", r.id.Name(), r.typeName)
	}
	r.handler = r.native.Handler
	return nil
}

// handlerName returns the record's Handler for the handler that serves r.
func handlerName(r *request) string {
	if r.handler != nil {
		return records.HandlerNative
	}
	return ""
}

// operationType returns the async operation type UCP queues the operation of r under.
func operationType(r *request) string {
	if r.handler != nil {
		return records.NativeOperationType
	}
	return records.OperationType
}

// operationTimeout returns how long UCP's async operation for op runs.
func operationTimeout(r *request, op string) time.Duration {
	if r.handler == nil {
		return ucp_aws.OperationTimeout
	}
	return r.handler.Options().Timeout(native.Operation(op)) + nativeTimeoutMargin
}

// isNotFound reports whether err means AWS has no such resource, from either handler.
func isNotFound(err error) bool {
	return ucp_aws.IsAWSResourceNotFoundError(err) || errors.Is(err, native.ErrNotFound)
}

// isGoneForDelete reports whether err means a DELETE has nothing of its type to delete: AWS has no such resource, or
// the identifier now names a resource of another product (native.OtherProductError), so the resource the alias
// managed is gone. The DELETE removes only the record and never touches the other product's resource. A PUT or GET
// still fails with 409 for another product.
func isGoneForDelete(err error) bool {
	var other *native.OtherProductError
	return isNotFound(err) || errors.As(err, &other)
}

// identifierFor returns the identifier the properties give for a type: the native handler's encoding when t has one,
// and otherwise the primary identifier values joined with "|", as Cloud Control expects. t is nil for a type with no
// native handler. Every path uses it, so a resource is found by the same identifier whichever handler serves it.
func identifierFor(t *native.Type, properties map[string]any, schema string) (string, error) {
	if !hasEncoder(t) {
		return identifierFromProperties(properties, schema)
	}
	return t.Handler.(native.IdentifierEncoder).Identifier(properties)
}

// resolveIdentifier returns the one form of identifier that the native handler of t declares (native.IdentifierResolver),
// with either handler, as an identifier encoding applies with either handler. It returns identifier when the type has
// no native handler, or one form, or there is no identifier.
func resolveIdentifier(ctx context.Context, t *native.Type, region string, identifier string) (string, error) {
	if t == nil || identifier == "" {
		return identifier, nil
	}
	resolver, ok := t.Handler.(native.IdentifierResolver)
	if !ok {
		return identifier, nil
	}
	return resolver.ResolveIdentifier(ctx, region, identifier)
}

// segmentEscaper escapes the characters that would split a resource ID segment, and "%", so the escaping can be
// reversed.
var segmentEscaper = strings.NewReplacer("%", "%25", "/", "%2F")

// addressSegment returns the last segment of the resource ID UCP returns for an AWS identifier, in a list or
// `existing`, with either handler. An AWS identifier can contain "/", such as an S3 ARN, or be "..", so it is escaped
// into one segment, and identifierOfSegment reverses it. Only "%", "/", and a whole "." or ".." are escaped, so an
// identifier without them, such as "a|b", keeps the ID it had before.
func addressSegment(identifier string) string {
	switch identifier {
	case ".":
		return "%2E"
	case "..":
		return "%2E%2E"
	}
	return segmentEscaper.Replace(identifier)
}

// identifierOfID returns the AWS identifier in the last segment of id, an AWS resource ID UCP returned, for a POST
// /:get on collection. id must name a resource of the same type in the same account and region.
func identifierOfID(collection resources.ID, id string) (string, armrpc_rest.Response) {
	parsed, err := resources.ParseResource(id)
	if err != nil || !strings.EqualFold(parsed.RootScope(), collection.RootScope()) || !strings.EqualFold(parsed.Type(), collection.Type()) {
		return "", badRequest(fmt.Sprintf("The %s header must be the ID of an AWS resource of type %s in %s.", sdkclients.AWSResourceIDHeader, collection.Type(), collection.RootScope()))
	}
	return identifierOfSegment(parsed.Name()), nil
}

// identifierOfSegment returns the AWS identifier in a resource ID segment addressSegment made. A segment that isn't
// valid escaping is returned unchanged.
func identifierOfSegment(segment string) string {
	if identifier, err := url.PathUnescape(segment); err == nil {
		return identifier
	}
	return segment
}

// hasEncoder reports whether t's handler declares its own identifier encoding.
func hasEncoder(t *native.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Handler.(native.IdentifierEncoder)
	return ok
}

// withGeneratedName returns properties with a generated name when a native handler serves r, declares a name property
// (native.Options.NameProperty), and the template leaves it out. The name comes from the record's client token, which
// identifies the create: every attempt of one create sends the same name. properties is not changed, so the
// last-applied properties stay the template's, without the name, and a later deploy that still leaves it out doesn't
// clear it.
func withGeneratedName(r *request, rec *datamodel.AWSResource, properties map[string]any) (map[string]any, error) {
	if r.handler == nil || r.native.NameRules == nil {
		return properties, nil
	}
	rules := r.native.NameRules
	if value, ok := properties[rules.Property]; ok && value != nil {
		return properties, nil
	}
	name, err := rules.Generate(rec.Properties.ClientToken)
	if err != nil {
		return nil, err
	}
	out := maps.Clone(properties)
	if out == nil {
		out = map[string]any{}
	}
	out[rules.Property] = name
	return out, nil
}

// identifierRequired returns the 400 for a create whose identifier the template doesn't give, of a type whose native
// handler finds a lost create by its identifier.
func identifierRequired(r *request, schema string) (armrpc_rest.Response, error) {
	paths, err := getPrimaryIdentifiersFromSchema(schema)
	if err != nil {
		return nil, err
	}
	return badRequest(fmt.Sprintf("Set %s of the AWS resource %q. UCP needs the identifier of an AWS resource of type %s before it creates one, so it can find the resource if the create is interrupted.", strings.Join(awsoperations.PropertyPaths(paths), ", "), r.id.Name(), r.typeName)), nil
}

// readNative reads a resource through the native handler. It returns the result in Cloud Control's shape, so the
// rest of the engine treats both handlers the same, and nil when AWS has no such resource. A handler that reads only
// the members it manages (native.ScopedReader) gets managed.
func readNative(ctx context.Context, h native.Handler, region string, identifier string, managed []map[string]any) (*cloudcontrol.GetResourceOutput, error) {
	var properties map[string]any
	var err error
	if scoped, ok := h.(native.ScopedReader); ok {
		properties, err = scoped.ReadScoped(ctx, region, identifier, managed)
	} else {
		properties, err = h.Read(ctx, region, identifier)
	}
	if errors.Is(err, native.ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	state, err := json.Marshal(properties)
	if err != nil {
		return nil, err
	}
	return &cloudcontrol.GetResourceOutput{
		TypeName:            aws.String(h.TypeName()),
		ResourceDescription: &cctypes.ResourceDescription{Identifier: aws.String(identifier), Properties: aws.String(string(state))},
	}, nil
}

// startUpdate starts an update and returns the token the worker polls: Cloud Control's request token, or an operation
// ID UCP makes for a native handler.
func (e *engine) startUpdate(ctx context.Context, r *request, identifier string, current map[string]any, desired map[string]any, patch jsondiff.Patch) (string, error) {
	if r.handler != nil {
		err := r.handler.Update(ctx, native.UpdateRequest{Region: r.region, Identifier: identifier, Current: current, Desired: desired, Patch: patch})
		if err != nil {
			return "", err
		}
		return uuid.New().String(), nil
	}

	marshaled, err := json.Marshal(&patch)
	if err != nil {
		return "", err
	}
	response, err := e.clients.CloudControl.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
		TypeName:      aws.String(r.typeName),
		Identifier:    aws.String(identifier),
		PatchDocument: aws.String(string(marshaled)),
	}, CloudControlRegionOption(r.region))
	if err != nil {
		return "", err
	}
	return aws.ToString(response.ProgressEvent.RequestToken), nil
}

// startCreate starts a create with the record's client token. It returns the identifier, when AWS gives it now, and
// the token the worker polls.
func (e *engine) startCreate(ctx context.Context, r *request, rec *datamodel.AWSResource, properties map[string]any) (string, string, error) {
	if r.handler != nil {
		identifier, err := r.handler.Create(ctx, native.CreateRequest{Region: r.region, Properties: properties, ClientToken: rec.Properties.ClientToken})
		if err != nil {
			// A create that failed partway can return the identifier of the resource it left.
			return identifier, "", err
		}
		return identifier, uuid.New().String(), nil
	}

	desiredState, err := json.Marshal(properties)
	if err != nil {
		return "", "", err
	}
	response, err := e.clients.CloudControl.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     aws.String(r.typeName),
		DesiredState: aws.String(string(desiredState)),
		ClientToken:  aws.String(rec.Properties.ClientToken),
	}, CloudControlRegionOption(r.region))
	if err != nil {
		return "", "", err
	}
	return aws.ToString(response.ProgressEvent.Identifier), aws.ToString(response.ProgressEvent.RequestToken), nil
}

// prepareDelete saves on the record the value a native handler sends with every attempt to delete the resource
// (native.DeletePreparer), before the first delete call, so a retried delete sends the same value. It returns the new
// ETag, or etag when there is nothing to save.
func (e *engine) prepareDelete(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, identifier string) (string, error) {
	preparer, ok := r.handler.(native.DeletePreparer)
	if !ok || rec.Properties.DeleteToken != "" || onDelete(r) == schemas.OnDeleteReset {
		return etag, nil
	}
	token, err := preparer.DeleteToken(ctx, r.region, identifier)
	if err != nil || token == "" {
		return etag, err
	}
	rec.Properties.DeleteToken = token
	saved, err := e.store.Save(ctx, rec, etag)
	if err != nil {
		rec.Properties.DeleteToken = ""
		return etag, fmt.Errorf("failed to save the delete token of %s: %w", r.id.String(), err)
	}
	return saved, nil
}

// startDeleteCall starts a delete and returns the token the worker polls. For a type whose onDelete metadata is reset,
// the native handler resets the resource instead (native.Resetter). A native handler that removes only the members it
// manages (native.ScopedDeleter) gets the record's managed values, and one that sends the same value on every attempt
// (native.DeletePreparer) gets deleteToken. Where Cloud Control serves the type, Cloud Control deletes it as
// CloudFormation defines.
func (e *engine) startDeleteCall(ctx context.Context, r *request, identifier string, deleteToken string) (string, error) {
	if r.handler != nil {
		var err error
		resetter, resets := r.handler.(native.Resetter)
		scoped, isScoped := r.handler.(native.ScopedDeleter)
		preparer, prepares := r.handler.(native.DeletePreparer)
		switch {
		case resets && onDelete(r) == schemas.OnDeleteReset:
			err = resetter.Reset(ctx, r.region, identifier)
		case prepares && deleteToken != "":
			err = preparer.DeleteWithToken(ctx, r.region, identifier, deleteToken)
		case isScoped:
			err = scoped.DeleteScoped(ctx, r.region, identifier, r.managed)
		default:
			err = r.handler.Delete(ctx, r.region, identifier)
		}
		if err != nil {
			return "", err
		}
		return uuid.New().String(), nil
	}

	response, err := e.clients.CloudControl.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   aws.String(r.typeName),
		Identifier: aws.String(identifier),
	}, CloudControlRegionOption(r.region))
	if err != nil {
		return "", err
	}
	return aws.ToString(response.ProgressEvent.RequestToken), nil
}

// resumeNativeCreate resumes a native first create that may have reached AWS. A native handler has no request to look
// up, so UCP finds the resource as the handler declares (native.Recovery):
//
//   - The record has the identifier, saved before the create: UCP reads AWS by it.
//   - RecoverBySearch: the handler's Find looks for the resource with the lost create's values, the pending
//     properties, because the template may have changed since. If AWS has none, the create is sent again with the
//     template. If another alias manages the resource Find returns, the request gets 409.
//   - RecoverByResend: the create is sent again with the template. AWS may return the first resource, with the lost
//     create's values, so UCP treats the result like a found resource rather than a new create. For a handler whose
//     resource is some members of a parent, UCP first saves the members of both (see saveResendMembers). When the
//     create fails but returns an identifier, the record keeps it (see identifierToKeep), and the request fails.
//
// When UCP reads the resource, it continues by what AWS shows:
//
//   - not found: the create never reached AWS, so it is sent again.
//   - still creating: UCP waits for it, as for any create, and returns 409. The caller deploys again when it finishes.
//     Which attempt's write-only values AWS has is unknown, so the record keeps no fingerprints for them, and the next
//     deploy sends them.
//   - created: the create is finished, and the request continues as an update with the template, which sends every
//     write-only value, because the record has no fingerprints for them.
//   - failed, and AWS changed nothing (native.OutcomeRejected), such as a GraphQL schema AppSync couldn't create: the
//     create is sent again with this deploy's template.
//   - failed otherwise: the request returns the failure, and the record stays, so a DELETE can remove the resource.
func (e *engine) resumeNativeCreate(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, properties map[string]any, deploy applied, typeInfo *cloudformation.DescribeTypeOutput, parsed *typeSchema, secrets []string) (armrpc_rest.Response, error) {
	identifier := rec.Properties.AWSIdentifier
	if identifier == "" {
		var err error
		if r.handler.Options().Recovery == native.RecoverByResend {
			var resp armrpc_rest.Response
			etag, resp, err = e.saveResendMembers(ctx, r, rec, etag, deploy, parsed)
			if resp != nil || err != nil {
				return resp, err
			}
			identifier, err = e.resendCreate(ctx, r, rec, properties)
		} else {
			identifier, err = e.findCreate(ctx, r, rec, rec.Properties.PendingLastApplied)
		}
		if err != nil {
			if found := e.identifierToKeep(ctx, r, identifier); found != "" {
				e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
					rec.Properties.AWSIdentifier = found
				})
			}
			return identifierError(ctx, r, err)
		}
		if identifier == "" {
			return e.create(ctx, r, rec, etag, "", properties, deploy, typeInfo, parsed, secrets)
		}
		// A search by values AWS keeps unique can find a resource another alias manages.
		if resp, err := e.checkIdentifier(ctx, r, identifier); resp != nil || err != nil {
			return resp, err
		}
		rec.Properties.AWSIdentifier = identifier
	}
	progress, err := r.handler.Progress(ctx, r.region, identifier, native.OperationCreate)
	switch {
	case errors.Is(err, native.ErrNotFound):
		return e.create(ctx, r, rec, etag, "", properties, deploy, typeInfo, parsed, secrets)
	case err != nil:
		return handleError(ctx, r.id, err)
	case !progress.Done:
		rec.Properties.PendingFingerprints = nil
		if _, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationCreate, uuid.New().String()); err != nil {
			return nil, err
		}
		return conflict("A create of the AWS resource %q is still running in AWS. Deploy again when it finishes.", r.id.Name()), nil
	case progress.Failure != nil && progress.Failure.Outcome == native.OutcomeRejected:
		return e.create(ctx, r, rec, etag, "", properties, deploy, typeInfo, parsed, secrets)
	case progress.Failure != nil:
		return failureResponse(r.id, progress.Failure), nil
	}

	ucplog.FromContextOrDiscard(ctx).Info("Found the AWS resource of an interrupted create", "alias", r.id.Name(), "awsIdentifier", identifier)
	// The create may have applied some or all of its values, so Radius owns them, and the update below clears one the
	// template no longer sets.
	records.KeepPending(rec)
	rec.Properties.ClientToken = ""
	current, err := e.readResource(ctx, r, identifier)
	if err != nil {
		return handleError(ctx, r.id, err)
	}
	if current == nil {
		// Deleted outside Radius since the create: create it again.
		rec.Properties.AWSIdentifier = ""
		return e.create(ctx, r, rec, etag, "", properties, deploy, typeInfo, parsed, secrets)
	}
	return e.update(ctx, r, rec, etag, current, identifier, properties, deploy, typeInfo, parsed, secrets)
}

// findCreate finds the resource of the record's first create with the handler's Find, for a handler whose Recovery is
// RecoverBySearch. It returns "" when AWS has no such resource, and for a handler that finds a lost create another
// way.
func (e *engine) findCreate(ctx context.Context, r *request, rec *datamodel.AWSResource, properties map[string]any) (string, error) {
	finder, ok := r.handler.(native.Finder)
	if !ok || r.handler.Options().Recovery != native.RecoverBySearch {
		return "", nil
	}
	properties, err := withGeneratedName(r, rec, properties)
	if err != nil {
		return "", err
	}
	return finder.Find(ctx, native.CreateRequest{Region: r.region, Properties: properties, ClientToken: rec.Properties.ClientToken})
}

// recoverNativeCreate finds the AWS resource of a native first create that may have reached AWS, before a DELETE,
// with the steps a PUT takes (see resumeNativeCreate). The template is not available, so a search or a create sent
// again uses the pending properties, which have no write-only values:
//
//   - The record has the identifier: UCP reads AWS by it.
//   - RecoverBySearch: the handler's Find looks for the resource.
//   - RecoverByResend: the create is sent again with the record's client token, which returns the first resource.
//     When AWS never had the create, this creates the resource, and the delete then removes it.
//
// Then, by what AWS shows:
//
//   - no such resource, or a resource of another product: nothing of the type exists, so only the record is removed.
//   - still creating: UCP waits for it, as for any create, and returns 409. The caller deletes again when it finishes.
//   - created, or failed with a resource left: the record gets the identifier, the pending values count as applied,
//     as for an interrupted update, and the delete continues.
//   - another alias manages the resource found: the record is kept, and the DELETE gets 409.
//   - the resent create fails but returns an identifier: the record keeps it (see identifierToKeep), and the DELETE
//     returns the error. The next DELETE reads AWS by the identifier and deletes the resource.
//   - a call fails: the record is kept, and the DELETE returns the error.
func (e *engine) recoverNativeCreate(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string) (string, armrpc_rest.Response, error) {
	keepCreate := func() {
		e.release(ctx, r, rec, etag, func(rec *datamodel.AWSResource) {
			rec.Properties.PendingOperation = records.OperationCreate
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
	}
	keep := func(err error) (string, armrpc_rest.Response, error) {
		keepCreate()
		resp, err := identifierError(ctx, r, err)
		return "", resp, err
	}
	removed := func() (string, armrpc_rest.Response, error) {
		resp, err := e.removeRecord(ctx, r, etag)
		return "", resp, err
	}

	identifier := rec.Properties.AWSIdentifier
	if identifier == "" {
		var err error
		pending := rec.Properties.PendingLastApplied
		if r.handler.Options().Recovery == native.RecoverByResend {
			identifier, err = e.resendCreate(ctx, r, rec, pending)
		} else {
			identifier, err = e.findCreate(ctx, r, rec, pending)
		}
		var other *native.OtherProductError
		if errors.As(err, &other) {
			return removed()
		} else if err != nil {
			rec.Properties.AWSIdentifier = e.identifierToKeep(ctx, r, identifier)
			return keep(err)
		}
		if identifier == "" {
			return removed()
		}
		// A search by values AWS keeps unique can find a resource another alias manages, which this DELETE must not
		// delete.
		held, err := e.checkIdentifier(ctx, r, identifier)
		if held != nil || err != nil {
			keepCreate()
			return "", held, err
		}
	}

	progress, err := r.handler.Progress(ctx, r.region, identifier, native.OperationCreate)
	switch {
	case isGoneForDelete(err):
		return removed()
	case err != nil:
		return keep(err)
	}
	rec.Properties.AWSIdentifier = identifier
	if !progress.Done {
		// Which write-only values AWS has is unknown, so the record keeps no fingerprints for them.
		rec.Properties.PendingFingerprints = nil
		if _, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationCreate, uuid.New().String()); err != nil {
			return "", nil, err
		}
		return "", conflict("A create of the AWS resource %q is still running in AWS. Delete again when it finishes.", r.id.Name()), nil
	}

	ucplog.FromContextOrDiscard(ctx).Info("Found the AWS resource of an interrupted create", "alias", r.id.Name(), "awsIdentifier", identifier)
	// The create may have applied some or all of its values, so Radius owns them. If the delete then fails, a later
	// deploy clears one the template no longer sets.
	records.KeepPending(rec)
	rec.Properties.ClientToken = ""
	etag, err = e.store.Save(ctx, rec, etag)
	if err != nil {
		resp, err := saveError(r.id, err)
		return "", resp, err
	}
	return etag, nil, nil
}

// saveResendMembers saves the record before a create is sent again with the template, for a handler whose resource is
// some members of a parent (native.ScopedReader). The create sent again may add the template's members and then fail,
// so the pending values become the template's, with the members of the lost create added, and a DELETE removes the
// members of both. Which attempt's write-only values AWS has is unknown, so the record keeps no fingerprints for them.
//
// A template that changes a create-only property of the lost create, such as the parent, gets 400 and nothing is
// saved or sent: sending the create again would add members to another parent, and the record would lose the lost
// create's members.
func (e *engine) saveResendMembers(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, deploy applied, parsed *typeSchema) (string, armrpc_rest.Response, error) {
	if _, scoped := r.handler.(native.ScopedReader); !scoped {
		return etag, nil, nil
	}
	patch, err := jsondiff.Compare(rec.Properties.PendingLastApplied, deploy.properties)
	if err != nil {
		return "", nil, err
	}
	// The pending values have no write-only values, so the template's aren't changes.
	patch = slices.DeleteFunc(patch, func(op jsondiff.Operation) bool {
		_, ok := parsed.writeOnlyPattern(op.Path)
		return ok
	})
	if resp := createOnlyChanged(r, parsed, patch); resp != nil {
		return "", resp, nil
	}
	pending := maps.Clone(deploy.properties)
	addMembers(pending, rec.Properties.PendingLastApplied)
	rec.Properties.PendingLastApplied = pending
	rec.Properties.PendingFingerprints = nil
	etag, err = e.store.Save(ctx, rec, etag)
	if err != nil {
		resp, err := saveError(r.id, err)
		return "", resp, err
	}
	return etag, nil, nil
}

// identifierToKeep returns the identifier a resent create returned along with an error, so the record can keep it:
// AWS may have made the resource before the error, such as when a later call of a create that takes more than one
// call failed. The next request then reads AWS by the identifier instead of sending the failing create again. It
// returns "" when there is no identifier, or when another alias manages it.
func (e *engine) identifierToKeep(ctx context.Context, r *request, identifier string) string {
	if identifier == "" {
		return ""
	}
	held, err := e.checkIdentifier(ctx, r, identifier)
	if err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "Failed to check the identifier a failed create returned", "alias", r.id.Name(), "awsIdentifier", identifier)
	}
	if held != nil || err != nil {
		return ""
	}
	return identifier
}

// resendCreate sends the record's first create again with its client token, for a handler whose Recovery is
// RecoverByResend, and returns the identifier of the resource AWS has. That resource may have the values of the first
// create, not properties, so the caller reads it and reconciles it as a found resource.
func (e *engine) resendCreate(ctx context.Context, r *request, rec *datamodel.AWSResource, properties map[string]any) (string, error) {
	properties, err := withGeneratedName(r, rec, properties)
	if err != nil {
		return "", err
	}
	identifier, err := r.handler.Create(ctx, native.CreateRequest{Region: r.region, Properties: properties, ClientToken: rec.Properties.ClientToken})
	if err == nil && identifier == "" {
		return "", fmt.Errorf("the native handler for %s sent the create of the AWS resource %q again, but returned no identifier", r.typeName, r.id.Name())
	}
	return identifier, err
}

// identifierError returns the response for an error computing the identifier from a template's properties, including
// a generated name, or finding a lost create. A handler reports a property value it can't use as a client
// native.Failure, which gets 400 with its message, as AWS's own rejection would, and an identifier of another product
// gets 409 (see handleError).
func identifierError(ctx context.Context, r *request, err error) (armrpc_rest.Response, error) {
	return handleError(ctx, r.id, err)
}

// failureResponse returns the response for an operation on the resource id that AWS finished with failure.
func failureResponse(id resources.ID, failure interface {
	error
	IsClientError() bool
}) armrpc_rest.Response {
	details := v1.ErrorResponse{Error: &v1.ErrorDetails{Code: v1.CodeInternal, Message: failure.Error(), Target: id.String()}}
	if failure.IsClientError() {
		details.Error.Code = v1.CodeInvalid
		return armrpc_rest.NewBadRequestARMResponse(details)
	}
	return armrpc_rest.NewInternalServerErrorARMResponse(details)
}

// onDelete returns what deleting a resource of r's type does, from its metadata.
func onDelete(r *request) schemas.OnDelete {
	if r.native == nil {
		return schemas.OnDeleteDelete
	}
	return r.native.Metadata.OnDelete
}

// findsCreateByResend reports whether the handler that serves r finds a lost create, whose identifier AWS assigns, by
// sending the create again: Cloud Control (see recoverCreate), and a native handler whose Recovery is RecoverByResend.
func findsCreateByResend(r *request) bool {
	return r.handler == nil || r.handler.Options().Recovery == native.RecoverByResend
}

// rejected returns the 400 for a DELETE of a type whose onDelete metadata is reject.
func rejected(r *request) armrpc_rest.Response {
	return badRequest(fmt.Sprintf("Radius can't delete the AWS resource %q, because AWS doesn't let it delete a resource of type %s. Nothing was changed.", r.id.Name(), r.typeName))
}

// equivalentFor returns the native handler's rule for values AWS returns in another form, or nil when there is none.
// It applies with either handler, because both read the same resource.
func equivalentFor(r *request) func(path string, template any, aws any) bool {
	if r.native == nil {
		return nil
	}
	return r.native.Handler.Options().Equivalent
}

// outcomeOf returns how much of a start call that failed with err AWS applied: the outcome a native handler reports
// with a *native.Failure, OutcomeRejected for a client fault, such as an invalid property, and for a resource of
// another product, which the handler refuses before it changes anything, and OutcomeUnknown otherwise, because a
// transport or server error does not show that AWS changed nothing.
func outcomeOf(err error) native.Outcome {
	var failure *native.Failure
	if errors.As(err, &failure) {
		return failure.Outcome
	}
	var other *native.OtherProductError
	if errors.As(err, &other) || ucp_aws.IsClientFault(err) {
		return native.OutcomeRejected
	}
	return native.OutcomeUnknown
}

// handleError returns the response for an error from AWS or a native handler of the resource id: 409 for an
// identifier of another product, the failure a native handler reports, and otherwise as ucp_aws.HandleAWSError does.
func handleError(ctx context.Context, id resources.ID, err error) (armrpc_rest.Response, error) {
	var other *native.OtherProductError
	if errors.As(err, &other) {
		return conflict("The AWS identifier %q belongs to another product (%s), not to the AWS type %s, so UCP changed nothing. Use an identifier of that type.", other.Identifier, other.Product, resources_aws.ToAWSResourceType(id)), nil
	}
	var failure *native.Failure
	if errors.As(err, &failure) {
		return failureResponse(id, failure), nil
	}
	return ucp_aws.HandleAWSError(ctx, err)
}
