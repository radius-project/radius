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
	"net/url"
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
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
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
		return route{native: t, handler: t.Handler, typeInfo: &cloudformation.DescribeTypeOutput{
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
	rt := route{native: t, typeInfo: &withSchema}
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

// identifierFor returns the identifier the properties give for a type: the native handler's encoding when t has one,
// and otherwise the primary identifier values joined with "|", as Cloud Control expects. t is nil for a type with no
// native handler. Every path uses it, so a resource is found by the same identifier whichever handler serves it.
func identifierFor(t *native.Type, properties map[string]any, schema string) (string, error) {
	if !hasEncoder(t) {
		return identifierFromProperties(properties, schema)
	}
	return t.Handler.(native.IdentifierEncoder).Identifier(properties)
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

// readNative reads a resource through the native handler. It returns the result in Cloud Control's shape, so the
// rest of the engine treats both handlers the same, and nil when AWS has no such resource.
func readNative(ctx context.Context, h native.Handler, region string, identifier string) (*cloudcontrol.GetResourceOutput, error) {
	properties, err := h.Read(ctx, region, identifier)
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
			return "", "", err
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

// startDeleteCall starts a delete and returns the token the worker polls.
func (e *engine) startDeleteCall(ctx context.Context, r *request, identifier string) (string, error) {
	if r.handler != nil {
		if err := r.handler.Delete(ctx, r.region, identifier); err != nil {
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

// resumeNativeCreate resumes a native first create whose identifier the record saved before the create: AWS has the
// resource only if the create reached it. A native handler has no request to look up, so UCP reads the resource:
//
//   - not found: the create never reached AWS, so it is sent again.
//   - still creating: UCP waits for it, as for any create, and returns 409. The caller deploys again when it finishes.
//     Which attempt's write-only values AWS has is unknown, so the record keeps no fingerprints for them, and the next
//     deploy sends them.
//   - created: the create is finished, and the request continues as an update with the template, which sends every
//     write-only value, because the record has no fingerprints for them.
//   - failed: the request returns the failure, and the record stays, so a DELETE can remove the resource.
func (e *engine) resumeNativeCreate(ctx context.Context, r *request, rec *datamodel.AWSResource, etag string, properties map[string]any, deploy applied, typeInfo *cloudformation.DescribeTypeOutput, parsed *typeSchema, secrets []string) (armrpc_rest.Response, error) {
	identifier := rec.Properties.AWSIdentifier
	progress, err := r.handler.Progress(ctx, r.region, identifier, native.OperationCreate)
	switch {
	case errors.Is(err, native.ErrNotFound):
		return e.create(ctx, r, rec, etag, "", properties, deploy, typeInfo, parsed, secrets)
	case err != nil:
		return ucp_aws.HandleAWSError(ctx, err)
	case !progress.Done:
		rec.Properties.PendingFingerprints = nil
		if _, err := e.track(ctx, r, rec, etag, v1.OperationPut, records.OperationCreate, uuid.New().String()); err != nil {
			return nil, err
		}
		return conflict("A create of the AWS resource %q is still running in AWS. Deploy again when it finishes.", r.id.Name()), nil
	case progress.Failure != nil:
		return failureResponse(r, progress.Failure), nil
	}

	ucplog.FromContextOrDiscard(ctx).Info("Found the AWS resource of an interrupted create", "alias", r.id.Name(), "awsIdentifier", identifier)
	rec.Properties.ClientToken = ""
	current, err := e.readResource(ctx, r, identifier)
	if err != nil {
		return ucp_aws.HandleAWSError(ctx, err)
	}
	if current == nil {
		// Deleted outside Radius since the create: create it again.
		rec.Properties.AWSIdentifier = ""
		return e.create(ctx, r, rec, etag, "", properties, deploy, typeInfo, parsed, secrets)
	}
	return e.update(ctx, r, rec, etag, current, identifier, properties, deploy, typeInfo, parsed, secrets)
}

// identifierError returns the response for an error computing the identifier from a template's properties. A
// handler reports a property value it can't use as a client native.Failure, which gets 400 with its message, as AWS's
// own rejection would.
func identifierError(ctx context.Context, r *request, err error) (armrpc_rest.Response, error) {
	var failure *native.Failure
	if errors.As(err, &failure) {
		return failureResponse(r, failure), nil
	}
	return ucp_aws.HandleAWSError(ctx, err)
}

// failureResponse returns the response for an operation AWS finished with failure.
func failureResponse(r *request, failure interface {
	error
	IsClientError() bool
}) armrpc_rest.Response {
	details := v1.ErrorResponse{Error: &v1.ErrorDetails{Code: v1.CodeInternal, Message: failure.Error(), Target: r.id.String()}}
	if failure.IsClientError() {
		details.Error.Code = v1.CodeInvalid
		return armrpc_rest.NewBadRequestARMResponse(details)
	}
	return armrpc_rest.NewInternalServerErrorARMResponse(details)
}
