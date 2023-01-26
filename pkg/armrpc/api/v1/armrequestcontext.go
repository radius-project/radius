// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// The below contants are the headers in request from ARM.
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md#proxy-request-header-modifications
const (
	// APIVersionParameterName is the query string parameter for the api version.
	APIVersionParameterName = "api-version"

	// SkipTokenParameterName is the query string parameter for the skip token which is used for pagination purposes.
	SkipTokenParameterName = "skipToken"

	// TopParameterName is an optional query parameter that defines the number of records requested by the client.
	TopParameterName = "top"
)

// The constants below define the default, max, and min values for the number of records to be returned by the server.
const (
	// MaxQueryItemCount represents the default value for the maximum number of records to be returned by the server.
	MaxQueryItemCount = 20

	// DefaultQueryItemCount represents the default value for the number of records to be returned by the server.
	DefaultQueryItemCount = 10

	// MinQueryItemCount represents the default value for the minimum number of records to be returned by the server.
	MinQueryItemCount = 5
)

var (
	// AcceptLanguageHeader is the standard http header used so that we don't have to pass in the http request.
	AcceptLanguageHeader = "Accept-Language"

	// HostHeader is the standard http header Host used to indicate the target host name.
	HostHeader = "Host"

	// RefererHeader is the full URI that the client connected to (which will be different than the RP URI, since it will have the public
	// hostname instead of the RP hostname). This value can be used in generating FQDN for Location headers or other requests since RPs
	// should not reference their endpoint name.
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

	// ClientObjectIDHeader is the object Id of the client JWT making the request. Not all users have object Id.
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

	// IfMatch HTTP request header makes a request conditional. The resource is returned only if the
	// condition (tag or wildcard in this case)in the If-Match is met.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/Addendum.md#etags-for-resources
	IfMatch = http.CanonicalHeaderKey("If-Match")

	// IfNoneMatch HTTP request header also makes a request conditional. The resource is returned only
	// if the condition (tag or wildcard in this case) in the If-None-Match is not met.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/Addendum.md#etags-for-resources
	IfNoneMatch = http.CanonicalHeaderKey("If-None-Match")
)

var (
	// ErrTopQueryParamOutOfBounds represents the error of top query parameter being out of defined bounds.
	ErrTopQueryParamOutOfBounds = errors.New("top query parameter is not within the limits")
)

// ARMRequestContext represents the service context including proxy request header values.
type ARMRequestContext struct {
	// ResourceID represents arm resource ID extracted from resource id.
	ResourceID resources.ID

	// ClientRequestID represents the client request id from arm request.
	ClientRequestID string
	// CorrelationID represents the request corrleation id from arm request.
	CorrelationID string
	// OperationID represents the unique id per operation, which will be used as async operation id later.
	OperationID uuid.UUID
	// OperationType represents the type of the operation.
	OperationType string
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
	// RawSystemMetadata is the raw system metadata from arm request. SystemData() returns unmarshalled system metadata
	RawSystemMetadata string
	// Location represents the location of the resource.
	Location string

	// IfMatch receives "*" or an ETag - No support for multiple ETags for now
	IfMatch string
	// IfNoneMatch receives "*" or an ETag - No support for multiple ETags for now
	IfNoneMatch string

	// SkipToken
	SkipToken string
	// Top is the maximum number of records to be returned by the server. The validation will be handled downstream.
	Top int

	// HTTPMethod represents the original method.
	HTTPMethod string
	// OriginalURL represents the original URL of the request.
	OrignalURL url.URL
}

// FromARMRequest extracts proxy request headers from http.Request.
func FromARMRequest(r *http.Request, pathBase, location string) (*ARMRequestContext, error) {
	log := ucplog.FromContextOrDiscard(r.Context())
	refererUri := r.Header.Get(RefererHeader)
	refererURL, err := url.Parse(refererUri)
	if refererUri == "" || err != nil {
		refererURL = r.URL
	}

	if pathBase == "" {
		pathPrefix := GetBaseIndex(refererURL.Path)
		pathBase = refererURL.Path[:pathPrefix]
	}
	path := strings.TrimPrefix(refererURL.Path, pathBase)
	rID, err := resources.ParseByMethod(path, r.Method)
	if err != nil {
		log.V(ucplog.Debug).Info(fmt.Sprintf("URL was not a valid resource id: %v", refererURL.Path))
		// do not stop extracting headers. handler needs to care invalid resource id.
	}

	queryItemCount, err := getQueryItemCount(r.URL.Query().Get(TopParameterName))
	if err != nil {
		log.V(ucplog.Debug).Info(fmt.Sprintf("Error parsing top query parameter: %v", r.URL.Query()))
		return nil, err
	}

	rpcCtx := &ARMRequestContext{
		ResourceID:      rID,
		ClientRequestID: r.Header.Get(ClientRequestIDHeader),
		CorrelationID:   r.Header.Get(CorrelationRequestIDHeader),
		OperationID:     uuid.New(), // TODO: this is temp. implementation. Revisit to have the right generation logic when implementing async request processor.
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
		Location:          location,

		IfMatch:     r.Header.Get(IfMatch),
		IfNoneMatch: r.Header.Get(IfNoneMatch),

		SkipToken: r.URL.Query().Get(SkipTokenParameterName),
		Top:       queryItemCount,

		HTTPMethod: r.Method,
		OrignalURL: *r.URL,
	}

	if route := mux.CurrentRoute(r); route != nil {
		rpcCtx.OperationType = route.GetName()
	}

	return rpcCtx, nil
}

// SystemData returns unmarshalled RawSystemMetaData.
func (rc ARMRequestContext) SystemData() *SystemData {
	if rc.RawSystemMetadata == "" {
		return &SystemData{}
	}

	systemDataProp := &SystemData{}
	if err := json.Unmarshal([]byte(rc.RawSystemMetadata), systemDataProp); err != nil {
		return &SystemData{}
	}

	return systemDataProp
}

// getQueryItemCount function returns the number of records requested.
// The default value is defined above.
// If there is a top query parameter, we use that instead of the default one.
// This function also checks if the top parameter is within the defined limits.
func getQueryItemCount(topQueryParam string) (int, error) {
	if topQueryParam == "" {
		return DefaultQueryItemCount, nil
	}

	topParam, err := strconv.Atoi(topQueryParam)

	if err != nil {
		return topParam, err
	}

	if topParam > MaxQueryItemCount || topParam < MinQueryItemCount {
		return topParam, ErrTopQueryParamOutOfBounds
	}

	return topParam, err
}

// ARMRequestContextFromContext extracts ARMRPContext from http context.
func ARMRequestContextFromContext(ctx context.Context) *ARMRequestContext {
	return ctx.Value(armContextKey).(*ARMRequestContext)
}

// WithARMRequestContext injects ARMRequestContext into the given http context.
func WithARMRequestContext(ctx context.Context, armctx *ARMRequestContext) context.Context {
	return context.WithValue(ctx, armContextKey, armctx)
}

func GetBaseIndex(path string) int {
	normalized := strings.ToLower(path)
	idx := strings.Index(normalized, "/planes/")
	if idx >= 0 {
		return idx
	}
	idx = strings.Index(normalized, "/subscriptions/")
	if idx >= 0 {
		return idx
	}
	return 0

}
