// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azuretest

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/test/utils"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
)

func NewTestOptions(t *testing.T) TestOptions {
	config, err := rad.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	auth, err := azure.GetResourceManagerEndpointAuthorizer()
	require.NoError(t, err, "failed to authenticate with azure")

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	require.NoErrorf(t, err, "failed to obtain Azure credentials")
	con := armcore.NewDefaultConnection(azcred, nil)

	env, err := rad.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	az, err := environments.RequireAzureCloud(env)
	require.NoError(t, err, "environment was not azure cloud")

	k8s, err := utils.GetKubernetesClient()
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
	ARMConnection  *armcore.Connection
	Environment    *environments.AzureCloudEnvironment
	K8sClient      *kubernetes.Clientset
}
