// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourceOperationsClientPrepare(t *testing.T) {
	resourceClient := NewResourceDeploymentOperationsClientWithBaseURI("http://localhost:5050")

	request, err := resourceClient.ListPreparer(context.TODO(), "/resourcegroups/my-rg/providers/Microsoft.Resources/deployments/my-deployment", nil)
	require.NoError(t, err)

	require.Equal(t, "/resourcegroups/my-rg/providers/Microsoft.Resources/deployments/my-deployment/operations", request.URL.Path)
}
