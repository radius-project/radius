// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sdk

import (
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

const (
	locationHeader            = "Location"
	azureAsyncOperationHeader = "Azure-AsyncOperation"
)

// TODO: replace this with use of the HTTP Referrer header.

var _ http.RoundTripper = (*locationRewriteRoundTripper)(nil)

// locationRewriteRoundTripper rewrites the value of the HTTP Location and Azure-AsyncOperation header
// on responses to match the expected externally routable URL scheme, host, and port.
//
// There is a blocking behavior bug when combining the ARM-RPC protocol and a Kubernetes APIService.
// Kubernetes does not forward the original hostname when proxying requests (we get the wrong value in
// X-Forwarded-Host). See: https://github.com/kubernetes/kubernetes/issues/107435
//
// ARM-RPC requires the Location header to contain a fully-qualified absolute URL (it must start
// with http://... or https://...). Combining this requirement with the broken behavior of APIService
// proxying means that we generate the wrong URL.
//
// So this is a temporary solution, until we can solve this at the protocol level. We rewrite the Location
// header on the client.
type locationRewriteRoundTripper struct {
	// RoundTripper is the inner http.RoundTripper that sends the request.
	RoundTripper http.RoundTripper

	// Scheme is the externally routable scheme segment of the URL (usually https).
	Scheme string

	// Authority is the externally routable authority segment of the URL (host:port).
	Authority string
}

// RoundTrip is the implementation of http.RoundTripper.
func (t *locationRewriteRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	// Send the request and then rewrite response headers.
	res, err := t.RoundTripper.RoundTrip(request)
	if err != nil {
		return nil, err
	}

	for _, headerName := range []string{locationHeader, azureAsyncOperationHeader} {
		values, ok := res.Header[textproto.CanonicalMIMEHeaderKey(headerName)]
		if ok && len(values) > 0 {
			// Headers can be multi-value but the cases we care about only have a single value.
			rewritten := t.rewrite(values[0], t.Scheme, t.Authority)
			if rewritten != nil {
				res.Header[textproto.CanonicalMIMEHeaderKey(headerName)] = []string{rewritten.String()}
			}
		}
	}

	return res, nil
}

func (t *locationRewriteRoundTripper) rewrite(value string, scheme string, host string) *url.URL {
	// OK we have a value, try to parse as a URL and then rewrite.
	u, err := url.Parse(value)
	if err != nil {
		// If we fail to parse the value just skip rewiting. Our usage should always be valid.
		return nil
	}

	if u.Scheme == "" {
		// If we don't have a fully-qualified URL then just skip rewriting. Our usage should always be fully-qualified.
		return nil
	}

	if scheme != "" {
		u.Scheme = scheme
	}
	u.Host = host

	return u
}

// newLocationRewriteRoundTripper creates a new roundtripper for the given URL or authority value.
func newLocationRewriteRoundTripper(endpoint string, inner http.RoundTripper) *locationRewriteRoundTripper {
	// NOTE: while we get the value from RESTConfig.Host - it's NOT always a host:port combo. Sometimes
	// it is a URL including the scheme portion. JUST FOR FUN.
	//
	// We do our best to handle all of those cases here and degrade silently if we can't.
	if strings.Contains(endpoint, "://") {
		// If we get here this is likely a fully-qualified URL.
		u, err := url.Parse(endpoint)
		if err != nil {
			// We failed to parse this as a URL, just treat it as a hostname then.
			return &locationRewriteRoundTripper{RoundTripper: inner, Authority: endpoint}
		}

		// OK we have a URL
		return &locationRewriteRoundTripper{RoundTripper: inner, Authority: u.Host, Scheme: u.Scheme}
	}

	// If we get here it's likely not a fully-qualified URL. Treat it as a hostname.
	return &locationRewriteRoundTripper{RoundTripper: inner, Authority: endpoint}
}
