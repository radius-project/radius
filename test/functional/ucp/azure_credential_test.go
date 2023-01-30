// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_Azure_Credential_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_Azure_Credential_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		resourceTypePath := "/planes/azure/azurecloud/providers/System.Azure/credentials"
		resourceURL := fmt.Sprintf("%s%s/default?api-version=%s", url, resourceTypePath, ucp.Version)
		collectionURL := fmt.Sprintf("%s%s?api-version=%s", url, resourceTypePath, ucp.Version)
		runAzureCredentialTests(t, resourceURL, collectionURL, roundTripper, getAzureCredentialObject())
	})

	test.Test(t)
}

func runAzureCredentialTests(t *testing.T, resourceUrl string, collectionUrl string, roundTripper http.RoundTripper, credential ucp.AzureCredentialResource) {
	// Create credential operation
	createAzureCredential(t, roundTripper, resourceUrl, credential)
	// Create duplicate credential
	createAzureCredential(t, roundTripper, resourceUrl, credential)
	// List credential operation
	credentialList := listAzureCredential(t, roundTripper, collectionUrl)
	require.Equal(t, len(credentialList), 1)
	assert.DeepEqual(t, credentialList[0], credential)

	// Check for correctness of credential
	createdCredential, statusCode := getAzureCredential(t, roundTripper, resourceUrl)
	require.Equal(t, http.StatusOK, statusCode)
	assert.DeepEqual(t, createdCredential, credential)

	// Delete credential operation
	statusCode, err := deleteAzureCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)

	// Delete non-existent credential
	statusCode, err = deleteAzureCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, statusCode)
}

func createAzureCredential(t *testing.T, roundTripper http.RoundTripper, url string, credential ucp.AzureCredentialResource) {
	body, err := json.Marshal(credential)
	require.NoError(t, err)
	createRequest, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Credential: %s created/updated successfully", url)
}

func getAzureCredential(t *testing.T, roundTripper http.RoundTripper, url string) (ucp.AzureCredentialResource, int) {
	getCredentialRequest, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	result, err := roundTripper.RoundTrip(getCredentialRequest)
	require.NoError(t, err)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)

	credential := ucp.AzureCredentialResource{}
	err = json.Unmarshal(payload, &credential)
	require.NoError(t, err)

	return credential, result.StatusCode
}

func deleteAzureCredential(t *testing.T, roundTripper http.RoundTripper, url string) (int, error) {
	deleteCredentialRequest, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(deleteCredentialRequest)
	return res.StatusCode, err
}

func listAzureCredential(t *testing.T, roundTripper http.RoundTripper, url string) []ucp.AzureCredentialResource {
	listCredentialRequest, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(listCredentialRequest)
	require.NoError(t, err)
	return getAzureCredentialList(t, res)
}

func getAzureCredentialList(t *testing.T, res *http.Response) []ucp.AzureCredentialResource {
	body := res.Body
	defer body.Close()

	var data map[string]any
	err := json.NewDecoder(body).Decode(&data)
	require.NoError(t, err)
	list, ok := data["value"].([]any)
	require.Equal(t, ok, true)
	var credentialList []ucp.AzureCredentialResource
	for _, item := range list {
		s, err := json.Marshal(item)
		require.NoError(t, err)
		credential := ucp.AzureCredentialResource{}
		err = json.Unmarshal(s, &credential)
		require.NoError(t, err)
		credentialList = append(credentialList, credential)
	}
	return credentialList
}

func getAzureCredentialObject() ucp.AzureCredentialResource {
	return ucp.AzureCredentialResource{
		Location: to.Ptr("west-us-2"),
		ID:       to.Ptr("/planes/azure/azurecloud/providers/System.Azure/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.Azure/credentials"),
		Tags: map[string]*string{
			"env": to.Ptr("dev"),
		},
		Properties: &ucp.AzureServicePrincipalProperties{
			ClientID: to.Ptr("00000000-0000-0000-0000-000000000000"),
			TenantID: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:     to.Ptr("ServicePrincipal"),
			Storage: &ucp.InternalCredentialStorageProperties{
				Kind:       to.Ptr(string(v20220901privatepreview.CredentialStorageKindInternal)),
				SecretName: to.Ptr("azure-azurecloud-default"),
			},
		},
	}
}
