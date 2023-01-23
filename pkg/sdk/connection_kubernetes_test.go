// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sdk

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// Note: we're not implementing our own validation of Kubernetes config, so we're
// also not testing those cases in details. We assume the Kubernetes libraries do
// a good enough job.

func Test_NewKubernetesConnectionFromConfig_Valid(t *testing.T) {
	// We're being fairly detailed with the verification of the roundtripper here
	// because a regression will result in some really hard to understand error messages.

	config := &rest.Config{
		Host: "https://example.com",
	}

	expectedEndpoint := "https://example.com/apis/api.ucp.dev/v1alpha3"
	expectedInnerRoundTripper, err := rest.TransportFor(config)
	require.NoError(t, err)

	expectedRoundTripper := newLocationRewriteRoundTripper(expectedEndpoint, expectedInnerRoundTripper)

	connection, err := NewKubernetesConnectionFromConfig(config)
	require.NoError(t, err)

	require.IsType(t, &kubernetesConnection{}, connection)
	kubernetesConnection := connection.(*kubernetesConnection)
	require.Equal(t, expectedEndpoint, kubernetesConnection.endpoint)

	roundTripper := kubernetesConnection.roundTripper
	require.Equal(t, expectedRoundTripper, roundTripper)

	require.Equal(t, &http.Client{Transport: expectedRoundTripper}, connection.Client())
	require.Equal(t, expectedEndpoint, connection.Endpoint())
}
