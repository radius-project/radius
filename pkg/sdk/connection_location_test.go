// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sdk

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_newLocationRewriteRoundTripper_Authority(t *testing.T) {
	endpoint := "example.com"
	innerRoundTripper := http.DefaultTransport

	expected := &locationRewriteRoundTripper{
		RoundTripper: innerRoundTripper,
		Scheme:       "",
		Authority:    "example.com",
	}

	actual := newLocationRewriteRoundTripper(endpoint, innerRoundTripper)
	require.Equal(t, expected, actual)
}

func Test_newLocationRewriteRoundTripper_URL(t *testing.T) {
	endpoint := "http://example.com/some/path"
	innerRoundTripper := http.DefaultTransport

	expected := &locationRewriteRoundTripper{
		RoundTripper: innerRoundTripper,
		Scheme:       "http",
		Authority:    "example.com",
	}

	actual := newLocationRewriteRoundTripper(endpoint, innerRoundTripper)
	require.Equal(t, expected, actual)
}

func Test_locationRewriteRoundTripper_RoundTrip(t *testing.T) {
	mockRoundTripper := &MockTransport{
		Response: &http.Response{
			Header: http.Header{
				http.CanonicalHeaderKey(locationHeader):            []string{"https://other-host.com/location"},
				http.CanonicalHeaderKey(azureAsyncOperationHeader): []string{"https://other-host.com/async-operation"},
			},
		},
	}

	roundTripper := newLocationRewriteRoundTripper("http://example.com", mockRoundTripper)
	response, err := roundTripper.RoundTrip(&http.Request{}) // Request doesn't matter
	require.NoError(t, err)

	require.Equal(t, "http://example.com/location", response.Header.Get(locationHeader))
	require.Equal(t, "http://example.com/async-operation", response.Header.Get(azureAsyncOperationHeader))
}

var _ http.RoundTripper = (*MockTransport)(nil)

type MockTransport struct {
	Response *http.Response
}

// RoundTrip implements http.RoundTripper
func (mt *MockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return mt.Response, nil
}
