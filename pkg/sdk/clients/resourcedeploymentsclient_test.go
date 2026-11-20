/*
Copyright 2026 The Radius Authors.

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

package clients

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/stretchr/testify/require"
)

func TestGetDeploymentForRecovery(t *testing.T) {
	const resourceID = "/planes/radius/local/resourceGroups/default/providers/Microsoft.Resources/deployments/recovery"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, resourceID, r.URL.Path)
		require.Equal(t, DeploymentsClientAPIVersion, r.URL.Query().Get("api-version"))
		require.Zero(t, r.ContentLength)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"properties":{"provisioningState":"Succeeded","outputResources":[{"id":"/resources/example"}]}}`))
		require.NoError(t, err)
	}))
	defer server.Close()
	client, err := NewResourceDeploymentsClient(&Options{
		Cred: &aztoken.AnonymousCredential{}, BaseURI: server.URL,
		ARMClientOptions: &arm.ClientOptions{ClientOptions: policy.ClientOptions{InsecureAllowCredentialWithHTTP: true}},
	})
	require.NoError(t, err)
	response, err := client.GetDeployment(t.Context(), resourceID, DeploymentsClientAPIVersion)
	require.NoError(t, err)
	require.Equal(t, armdeployments.ProvisioningStateSucceeded, *response.Properties.ProvisioningState)
	require.Len(t, response.Properties.OutputResources, 1)

	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	client, err = NewResourceDeploymentsClient(&Options{
		Cred: &aztoken.AnonymousCredential{}, BaseURI: missing.URL,
		ARMClientOptions: &arm.ClientOptions{ClientOptions: policy.ClientOptions{InsecureAllowCredentialWithHTTP: true}},
	})
	require.NoError(t, err)
	_, err = client.GetDeployment(t.Context(), resourceID, DeploymentsClientAPIVersion)
	var responseError *azcore.ResponseError
	require.ErrorAs(t, err, &responseError)
	require.Equal(t, http.StatusNotFound, responseError.StatusCode)
}
