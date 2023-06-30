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

package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

var (
	// ContentTypeHeaderKey is the header key of Content-Type
	ContentTypeHeaderKey = http.CanonicalHeaderKey("Content-Type")

	// DefaultScheme is the default scheme used if there is no scheme in the URL.
	DefaultSheme = "http"
)

var (
	// ErrUnsupportedContentType represents the error of unsupported content-type.
	ErrUnsupportedContentType = errors.New("unsupported Content-Type")
	// ErrRequestedResourceDoesNotExist represents the error of resource that is requested not existing.
	ErrRequestedResourceDoesNotExist = errors.New("requested resource does not exist")
	// ErrETagsDoNotMatch represents the error of the eTag of the resource and the requested etag not matching.
	ErrETagsDoNotMatch = errors.New("etags do not match")
	// ErrResourceAlreadyExists represents the error of the resource being already existent at the moment.
	ErrResourceAlreadyExists = errors.New("resource already exists")
)

// ReadJSONBody extracts the content from request.
//
// # Function Explanation
//
// ReadJSONBody reads the body of the request if the content type is "application/json".
// It returns the body as a byte array or an error if the content type is not supported or an error
// occurs while reading the body.
func ReadJSONBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()

	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get(ContentTypeHeaderKey)))
	if i := strings.Index(contentType, ";"); i > -1 {
		contentType = contentType[0:i]
	}

	if contentType != "application/json" {
		return nil, ErrUnsupportedContentType
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}
	return data, nil
}

// ValidateETag receives an ARMRequestContect and gathers the values in the If-Match and/or
// If-None-Match headers and then checks to see if the etag of the resource matches what is requested.
//
// # Function Explanation
// ValidateETag checks the If-Match and If-None-Match headers of the ARMRequestContext against the provided etag,
// and returns an error if the etag does not match either header.
func ValidateETag(armRequestContext v1.ARMRequestContext, etag string) error {
	ifMatchETag := armRequestContext.IfMatch
	ifMatchCheck := checkIfMatchHeader(ifMatchETag, etag)
	if ifMatchCheck != nil {
		return ifMatchCheck
	}

	ifNoneMatchETag := armRequestContext.IfNoneMatch
	ifNoneMatchCheck := checkIfNoneMatchHeader(ifNoneMatchETag, etag)
	if ifNoneMatchCheck != nil {
		return ifNoneMatchCheck
	}

	return nil
}

// checkIfMatchHeader function checks if the etag of the resource matches
// the one provided in the if-match header
func checkIfMatchHeader(ifMatchETag string, etag string) error {
	if ifMatchETag == "" {
		return nil
	}

	if etag == "" {
		return ErrRequestedResourceDoesNotExist
	}

	if ifMatchETag != "*" && ifMatchETag != etag {
		return ErrETagsDoNotMatch
	}

	return nil
}

// checkIfNoneMatchHeader function checks if the etag of the resource matches
// the one provided in the if-none-match header
func checkIfNoneMatchHeader(ifNoneMatchETag string, etag string) error {
	if ifNoneMatchETag == "*" && etag != "" {
		return ErrResourceAlreadyExists
	}

	return nil
}

// GetURLFromReqWithQueryParameters function builds a URL from the request and query parameters
func GetURLFromReqWithQueryParameters(req *http.Request, qps url.Values) *url.URL {
	url := url.URL{
		Host:     req.Host,
		Scheme:   req.URL.Scheme,
		Path:     req.URL.Path,
		RawQuery: qps.Encode(),
	}

	if url.Scheme == "" {
		url.Scheme = DefaultSheme
	}

	return &url
}

// GetNextLinkUrl function returns the URL string by building a URL from the request and the pagination token.
func GetNextLinkURL(ctx context.Context, req *http.Request, paginationToken string) string {
	if paginationToken == "" {
		return ""
	}

	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	qps := url.Values{}
	qps.Add("api-version", serviceCtx.APIVersion)
	qps.Add("skipToken", paginationToken)
	qps.Add("top", strconv.Itoa(serviceCtx.Top))

	return GetURLFromReqWithQueryParameters(req, qps).String()
}
