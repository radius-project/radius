// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicecontext

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
)

// The below contants are the headers in request from ARM.
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md#proxy-request-header-modifications
const (
	// APIVersionParameterName is the query string parameter for the api version.
	APIVersionParameterName = "api-version"

	// AcceptLanguageHeader is the standard http header used so that we don't have to pass in the http request.
	AcceptLanguageHeader = "Accept-Language"

	// HostHeader is the standard http header Host used to indicate the target host name.
	HostHeader = "Host"

	// RefererHeader is the full URI that the client connected to (which will be different than the RP URI, since it will have the public hostname instead of the RP hostname). This value can be used in generating FQDN for Location headers or other requests since RPs should not reference their endpoint name.
	RefererHeader = "Referer"

	// ContentTypeHeader is the standard http header Content-Type.
	ContentTypeHeader = "Content-Type"

	// CorrelationRequestIDHeader is the http header identifying a set of related operations that the request belongs to, in the form of a GUID.
	CorrelationRequestIDHeader = "X-Ms-Correlation-Request-Id"

	// ClientRequestIDHeader is the http header identifying the request, in the form of a GUID with no decoration.
	ClientRequestIDHeader = "X-Ms-Client-Request-Id"

	// ClientReturnClientRequestIDHeader indicates if a client-request-id should be included in the response. Default is false.
	ClientReturnClientRequestIDHeader = "X-Ms-Return-Client-Request-Id"

	// ClientApplicationIDHeader is the app Id of the client JWT making the request.
	ClientApplicationIDHeader = "X-Ms-Client-App-Id"

	// ClientObjectIDHeader is the object Id of the client JWT making the request. Not all users have object Id. For CSP (reseller) scenarios for example, object Id is not available.
	ClientObjectIDHeader = "X-Ms-Client-Object-Id"

	// ClientPrincipalNameHeader is the principal name / UPN of the client JWT making the request.
	ClientPrincipalNameHeader = "X-Ms-Client-Principal-Name"

	// ClientPrincipalIDHeader is the principal Id of the client JWT making the request.
	ClientPrincipalIDHeader = "X-Ms-Client-Principal-Id"

	// HomeTenantIDHeader is the tenant id of the service principal backed by the identity
	HomeTenantIDHeader = "X-Ms-Home-Tenant-Id"

	// ClientTenantIDHeader is the tenant id of the client
	ClientTenantIDHeader = "X-Ms-Client-Tenant-Id"

	// ARMResourceSystemDataHeader is the http header to the provider on resource write and resource action calls in JSON format.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-contracts.md#properties
	ARMResourceSystemDataHeader = "X-Ms-Arm-Resource-System-Data"

	// TraceparentHeader is W3C trace parent header.
	TraceparentHeader = "Traceparent"
)

// ARMRPCContext is the context which includes ARM RPC.
type ARMRPCContext struct {
	// ResourceID represents arm resource ID extracted from resource id.
	ResourceID azresources.ResourceID

	// ClientRequestID represents the client request id from arm request.
	ClientRequestID string
	// CorrelationID represents the request corrleation id from arm request.
	CorrelationID string
	// OperationID represents the unique id per operation, which will be used as async operation id later.
	OperationID string
	// Traceparent represents W3C trace prarent header for distributed tracing.
	Traceparent string

	// HomeTenantID represents the tenant id of the service principal.
	HomeTenantID string
	// ClientTenantID represents the tenant id of the client.
	ClientTenantID string

	// The properties of the client identities.
	ClientApplicationID string
	ClientObjectID      string
	ClientPrincipalName string
	ClientPrincipalID   string

	// APIVersion represents api-version of incoming arm request.
	APIVersion string
	// AcceptLanguage represents the supported language of the arm request.
	AcceptLanguage string
	// ClientReferer represents the URI the client connected to.
	ClientReferer string
	// UserAgent represents the user agent name of the arm request.
	UserAgent string
	// RawSystemMetadata is the raw system metadata from arm request. SystemData returns unmarshalled system metadata
	RawSystemMetadata string
}

// FromRequest extracts ARM RPC
func FromRequest(r *http.Request, prefix string) (*ARMRPCContext, error) {
	path := strings.TrimPrefix(r.URL.Path, prefix)
	azID, _ := azresources.Parse(path)

	rpcCtx := &ARMRPCContext{
		ResourceID:      azID,
		ClientRequestID: r.Header.Get(ClientRequestIDHeader),
		CorrelationID:   r.Header.Get(CorrelationRequestIDHeader),
		OperationID:     uuid.NewString(),
		Traceparent:     r.Header.Get(TraceparentHeader),

		HomeTenantID:        r.Header.Get(HomeTenantIDHeader),
		ClientTenantID:      r.Header.Get(ClientTenantIDHeader),
		ClientApplicationID: r.Header.Get(ClientApplicationIDHeader),
		ClientObjectID:      r.Header.Get(ClientObjectIDHeader),
		ClientPrincipalName: r.Header.Get(ClientPrincipalIDHeader),
		ClientPrincipalID:   r.Header.Get(ClientPrincipalIDHeader),

		APIVersion:        r.URL.Query().Get(APIVersionParameterName),
		AcceptLanguage:    r.Header.Get(AcceptLanguageHeader),
		ClientReferer:     r.Header.Get(RefererHeader),
		UserAgent:         r.UserAgent(),
		RawSystemMetadata: r.Header.Get(ARMResourceSystemDataHeader),
	}

	return rpcCtx, nil
}

// SystemData returns unmarshalled SystemMetaData.
func (rc ARMRPCContext) SystemData() *armrpcv1.SystemData {
	if rc.RawSystemMetadata == "" {
		return nil
	}

	systemDataProp := &armrpcv1.SystemData{}
	if err := json.Unmarshal([]byte(rc.RawSystemMetadata), systemDataProp); err != nil {
		return nil
	}

	return systemDataProp
}

// FromContext extracts ARMRPContext from http context.
func FromContext(ctx context.Context) *ARMRPCContext {
	return ctx.Value(armContextKey).(*ARMRPCContext)
}

// WithARMRPCContext injects ARMRPCContext into the given http context.
func WithARMRPCContext(ctx context.Context, armctx *ARMRPCContext) context.Context {
	return context.WithValue(ctx, armContextKey, armctx)
}
