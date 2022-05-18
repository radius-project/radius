// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-10-01/resources"
	"github.com/stretchr/testify/require"
)

func TestUCPClientPrepare(t *testing.T) {
	ucpClient := NewResourceDeploymentClientWithBaseURI("http://localhost:5050")

	request, err := ucpClient.ResourceCreateOrUpdatePreparer(context.TODO(), "/resourcegroups/my-rg/providers/Microsoft.Resources/deployments/my-deployment", resources.Deployment{})
	require.NoError(t, err)

	require.Equal(t, "/resourcegroups/my-rg/providers/Microsoft.Resources/deployments/my-deployment", request.URL.Path)
}
