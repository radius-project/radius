// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import (
	"context"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/project-radius/radius/test/ucp/httpbaseline"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestContext(ctx context.Context, planeURL string, planeID string, httpScheme string) context.Context {
	ctx = context.WithValue(ctx, PlaneUrlField, planeURL)
	ctx = context.WithValue(ctx, PlaneIdField, planeID)
	ctx = context.WithValue(ctx, HttpSchemeField, httpScheme)
	return ctx
}

func Test_ARM_Baselines(t *testing.T) {
	baselines, err := readBaselines()
	require.NoError(t, err)

	downstream, err := url.Parse("http://example.com")
	require.NoError(t, err)

	for _, baseline := range baselines {
		t.Run(baseline.Name, func(t *testing.T) {
			require.NoError(t, baseline.Error, "failed to read baseline")

			ctx, cancel := testcontext.New(t)
			defer cancel()

			// Create a "downstream" that will respond according to the test
			// setup and will capture the downstream request for comparison.
			capture := baseline.DownstreamResponse.CreateRoundTripper()
			options := ReverseProxyOptions{
				RoundTripper: capture,
				ProxyAddress: "localhost:9443",
			}
			pp := NewARMProxy(options, downstream, nil)

			w := httptest.NewRecorder()
			ctx = createTestContext(ctx, "http://example.com", "/planes/example/local", "http")
			req := baseline.UpstreamRequest.ToTestRequest(ctx)

			// Send the request
			pp.ServeHTTP(w, req)

			resp, err := httpbaseline.NewResponse(w.Result())
			require.NoError(t, err)

			// Now we should compare the upstream response and downstream request.
			assert.Equal(t, baseline.DownstreamRequest, capture.Request, "downstream request does not match expected")
			assert.Equal(t, baseline.UpstreamResponse, *resp, "upstream response does not match expected")
		})
	}
}

func readBaselines() ([]baseline, error) {
	baselines := []baseline{}
	base := filepath.Join(".", "testdata", "arm")
	dirs, err := ioutil.ReadDir(base)
	if err != nil {
		return nil, err
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		baselines = append(baselines, readBaseline(filepath.Join(base, dir.Name())))
	}

	return baselines, nil
}

func readBaseline(path string) baseline {
	b := baseline{
		Name:          filepath.Base(path),
		DirectoryPath: path,
	}

	// NOTE: if we have an error reading the baseline, we don't halt, we include that info.
	downstreamRequest, err := httpbaseline.ReadRequestFromFile(filepath.Join(path, "downstream-request.json"))
	if err != nil {
		b.Error = err
		return b
	}
	b.DownstreamRequest = downstreamRequest

	downstreamResponse, err := httpbaseline.ReadResponseFromFile(filepath.Join(path, "downstream-response.json"))
	if err != nil {
		b.Error = err
		return b
	}
	b.DownstreamResponse = downstreamResponse

	upstreamRequest, err := httpbaseline.ReadRequestFromFile(filepath.Join(path, "upstream-request.json"))
	if err != nil {
		b.Error = err
		return b
	}
	b.UpstreamRequest = upstreamRequest

	upstreamResponse, err := httpbaseline.ReadResponseFromFile(filepath.Join(path, "upstream-response.json"))
	if err != nil {
		b.Error = err
		return b
	}
	b.UpstreamResponse = upstreamResponse

	return b
}

type baseline struct {
	Name          string
	DirectoryPath string
	Error         error

	DownstreamRequest  httpbaseline.Request
	DownstreamResponse httpbaseline.Response
	UpstreamRequest    httpbaseline.Request
	UpstreamResponse   httpbaseline.Response
}
