// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azuretest

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/test"
)

func NewTestOptions(t *testing.T) Options {

	auth, err := armauth.GetArmAuthorizer()
	require.NoError(t, err, "failed to authenticate with azure")

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	require.NoErrorf(t, err, "failed to obtain Azure credentials")

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	env, err := cli.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	az, err := environments.RequireAzureCloud(env)
	require.NoError(t, err, "environment was not azure cloud")

	_, radiusConnection, err := kubernetes.CreateAPIServerConnection(az.Context, az.APIServerBaseURL)
	require.NoError(t, err, "failed to create API Server connection")

	radiusBaseURL, radiusRoundTripper, err := kubernetes.GetBaseUrlAndRoundTripper(az.APIServerBaseURL, "api.radius.dev", az.Context)
	require.NoError(t, err, "failed to create API Server round-tripper")

	return Options{
		TestOptions:      test.NewTestOptions(t),
		ARMAuthorizer:    auth,
		ARMConnection:    arm.NewDefaultConnection(azcred, nil),
		RadiusBaseURL:    radiusBaseURL,
		RadiusConnection: radiusConnection,
		RadiusSender:     autorest.SenderFunc(radiusRoundTripper.RoundTrip),
		Environment:      az,
	}
}

type Options struct {
	test.TestOptions
	ARMAuthorizer    autorest.Authorizer
	ARMConnection    *arm.Connection
	RadiusBaseURL    string
	RadiusConnection *arm.Connection
	RadiusSender     autorest.Sender
	Environment      *environments.AzureCloudEnvironment
}
