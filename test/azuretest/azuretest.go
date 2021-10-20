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
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/kubernetes"
	"github.com/stretchr/testify/require"
	k8s "k8s.io/client-go/kubernetes"
)

func NewTestOptions(t *testing.T) TestOptions {
	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	auth, err := armauth.GetArmAuthorizer()
	require.NoError(t, err, "failed to authenticate with azure")

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	require.NoErrorf(t, err, "failed to obtain Azure credentials")
	con := arm.NewDefaultConnection(azcred, nil)

	env, err := cli.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	az, err := environments.RequireAzureCloud(env)
	require.NoError(t, err, "environment was not azure cloud")

	k8sconfig, err := kubernetes.ReadKubeConfig()
	require.NoError(t, err, "failed to read k8s config")

	k8s, _, err := kubernetes.CreateTypedClient(k8sconfig.CurrentContext)
	require.NoError(t, err, "failed to create kubernetes client")

	return TestOptions{
		ConfigFilePath: config.ConfigFileUsed(),
		ARMAuthorizer:  auth,
		ARMConnection:  con,
		Environment:    az,
		K8sClient:      k8s,
	}
}

type TestOptions struct {
	ConfigFilePath string
	ARMAuthorizer  autorest.Authorizer
	ARMConnection  *arm.Connection
	Environment    *environments.AzureCloudEnvironment
	K8sClient      *k8s.Clientset
}
