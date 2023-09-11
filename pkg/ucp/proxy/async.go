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

package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// locationHeader is the name of the Location header.
	locationHeader = "Location"

	// azureAsyncOperationHeader is the name of the Azure-AsyncOperation header.
	//
	// This value has manually been canonizalized to speed up processing. DO NOT modify
	// the casing of this value.
	azureAsyncOperationHeader = "Azure-Asyncoperation"
)

// ProcessAsyncOperationHeaders is a ResponderFunc that processes the Azure-AsyncOperation header and
// Location header to match the UCP hostname and scheme based on the Referrer header.
//
// Users of this director should ensure the referrer header is set on the request before proxying.
// The referrer header should contain the original UCP request URL.
//
// The values of the Azure-AsyncOperation and Location headers are rewritten to point to the UCP endpoint.
// If the result header values omit the plane-prefix (eg: /subscriptions/...), then the plane-prefix is
// prepended to the header value. The query string returned by the downstream are preserved.
func ProcessAsyncOperationHeaders(resp *http.Response) error {
	ctx := resp.Request.Context()
	logger := ucplog.FromContextOrDiscard(ctx)

	// If the response is not a 200, 201 or 202, then we don't need to process the headers
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		logger.V(ucplog.LevelDebug).Info("response status code is not 200, 201 or 202, skipping async operation headers")
		return nil
	}

	// We process the response based on the Referrer header. If the referrer header is not present, then we don't
	// need to process the headers because we don't know how.
	referrer := resp.Request.Header.Get(v1.RefererHeader)
	if referrer == "" {
		logger.V(ucplog.LevelDebug).Info("request has no referrer header, skipping async operation headers")
		return nil
	}

	referrerURL, err := url.Parse(referrer)
	if err != nil {
		logger.V(ucplog.LevelDebug).Info("referrer header is not a URL, skipping async operation headers")
		return nil
	}

	// If the referrer is not a UCP request, then we don't need to process the headers
	//
	// We also need to extract a "path base". This is the path prefix that was trimmed from the UCP prefix
	// when the request was proxied. We need to re-add this path base to the header values.
	originalPath := referrerURL.Path
	pathBase := ""
	planesIndex := strings.Index(strings.ToLower(originalPath), "/"+resources.PlanesSegment+"/")
	if planesIndex != -1 && planesIndex != 0 {
		logger.V(ucplog.LevelDebug).Info("referrer header has path base", "pathBase", originalPath[:planesIndex])
		pathBase = originalPath[:planesIndex]
		originalPath = originalPath[planesIndex:]
	}

	planeType, planeName, _, err := resources.ExtractPlanesPrefixFromURLPath(originalPath)
	if err != nil {
		logger.V(ucplog.LevelDebug).Info("referrer header is not a UCP request, skipping async operation headers")
		return nil
	}

	planesPrefix := fmt.Sprintf("/%s/%s/%s", resources.PlanesSegment, planeType, planeName)

	// As per https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/async-operations,
	// rewrite both the Azure-AsyncOperation and Location headers to point to the UCP endpoint.
	for _, header := range []string{azureAsyncOperationHeader, locationHeader} {
		if value, ok := resp.Header[header]; ok {
			result, err := rewriteHeader(value[0], referrerURL, pathBase, planesPrefix)
			if err == nil {
				resp.Header.Set(header, result)
				logger.V(ucplog.LevelDebug).Info("rewrote header", "header", header, "before", value, "after", result, "referrer", referrerURL)
			} else {
				logger.Error(err, "failed to rewrite header", "header", header, "value", value, "referrer", referrerURL)
			}
		}
	}

	return nil
}

func rewriteHeader(header string, referrerURL *url.URL, pathBase string, planesPrefix string) (string, error) {
	// COPY the original URL so that we can modify it
	builder := *referrerURL

	headerURL, err := url.Parse(header)
	if err != nil {
		return "", fmt.Errorf("header value is not a valid URL: %w", err)
	}

	// Some downstreams are *aware* of the UCP path and will return a URL with the UCP path in it.
	//
	// However they are generally not aware of the "path base" that's trimmed from the UCP prefix. We need
	// to fixup the URL they return no matter what.
	if pathBase != "" && strings.HasPrefix(strings.ToLower(headerURL.Path), strings.ToLower(pathBase+planesPrefix)) {
		// Value has same basepath + planes prefix as the referrer, so we can just return the value as-is
		builder.Path = headerURL.Path
	} else if strings.HasPrefix(strings.ToLower(headerURL.Path), strings.ToLower(planesPrefix)) {
		// Value has same planes prefix as the referrer, so we can just return the value with path base.
		builder.Path = pathBase + headerURL.Path
	} else {
		// Value does not have path base or planes prefix, so we need to add the planes prefix and path base.
		builder.Path = pathBase + planesPrefix + headerURL.Path
	}

	builder.RawFragment = ""
	builder.RawQuery = headerURL.RawQuery

	return builder.String(), nil
}
