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

	"github.com/project-radius/radius/pkg/to"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_Azure_Credential_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_Azure_Credential_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		resourceTypePath := "/planes/azure/azuretest/providers/System.Azure/credentials"
		resourceURL := fmt.Sprintf("%s%s/default?api-version=%s", url, resourceTypePath, ucp.Version)
		collectionURL := fmt.Sprintf("%s%s?api-version=%s", url, resourceTypePath, ucp.Version)
		runAzureCredentialTests(t, resourceURL, collectionURL, roundTripper, getAzureTestCredentialObject(), getExpectedAzureTestCredentialObject())
	})

	test.Test(t)
}

func runAzureCredentialTests(t *testing.T, resourceUrl string, collectionUrl string, roundTripper http.RoundTripper, createCredential ucp.AzureCredentialResource, expectedCredential ucp.AzureCredentialResource) {
	// Create credential operation
	createAzureTestCredential(t, roundTripper, resourceUrl, createCredential)

	// Create duplicate credential
	createAzureTestCredential(t, roundTripper, resourceUrl, createCredential)

	// List credential operation
	credentialList := listAzureTestCredential(t, roundTripper, collectionUrl)
	index, err := getIndexOfAzureTestCredential(*expectedCredential.ID, credentialList)
	require.NoError(t, err)
	require.Equal(t, credentialList[index], expectedCredential)

	// Check for correctness of credential
	createdCredential, statusCode := getAzureTestCredential(t, roundTripper, resourceUrl)
	require.Equal(t, http.StatusOK, statusCode)
	assert.DeepEqual(t, createdCredential, expectedCredential)

	// Delete credential operation
	statusCode, err = deleteAzureTestCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)

	// Delete non-existent credential
	statusCode, err = deleteAzureTestCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, statusCode)
}

func createAzureTestCredential(t *testing.T, roundTripper http.RoundTripper, url string, credential ucp.AzureCredentialResource) {
	body, err := json.Marshal(credential)
	require.NoError(t, err)
	createRequest, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Credential: %s created/updated successfully", url)
}

func getAzureTestCredential(t *testing.T, roundTripper http.RoundTripper, url string) (ucp.AzureCredentialResource, int) {
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

func deleteAzureTestCredential(t *testing.T, roundTripper http.RoundTripper, url string) (int, error) {
	deleteCredentialRequest, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(deleteCredentialRequest)
	return res.StatusCode, err
}

func listAzureTestCredential(t *testing.T, roundTripper http.RoundTripper, url string) []ucp.AzureCredentialResource {
	listCredentialRequest, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(listCredentialRequest)
	require.NoError(t, err)
	return getAzureTestCredentialList(t, res)
}

func getAzureTestCredentialList(t *testing.T, res *http.Response) []ucp.AzureCredentialResource {
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

func getAzureTestCredentialObject() ucp.AzureCredentialResource {
	return ucp.AzureCredentialResource{
		Location: to.Ptr("west-us-2"),
		ID:       to.Ptr("/planes/azure/azuretest/providers/System.Azure/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.Azure/credentials"),
		Tags: map[string]*string{
			"env": to.Ptr("dev"),
		},
		Properties: &ucp.AzureServicePrincipalProperties{
			ClientID:     to.Ptr("00000000-0000-0000-0000-000000000000"),
			TenantID:     to.Ptr("00000000-0000-0000-0000-000000000000"),
			ClientSecret: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:         to.Ptr("ServicePrincipal"),
			Storage: &ucp.InternalCredentialStorageProperties{
				Kind:       to.Ptr(string(ucp.CredentialStorageKindInternal)),
				SecretName: to.Ptr("azure-azuretest-default"),
			},
		},
	}
}

func getExpectedAzureTestCredentialObject() ucp.AzureCredentialResource {
	return ucp.AzureCredentialResource{
		Location: to.Ptr("west-us-2"),
		ID:       to.Ptr("/planes/azure/azuretest/providers/System.Azure/credentials/default"),
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
				Kind:       to.Ptr(string(ucp.CredentialStorageKindInternal)),
				SecretName: to.Ptr("azure-azuretest-default"),
			},
		},
	}
}

func getIndexOfAzureTestCredential(testCredentialId string, credentialList []ucp.AzureCredentialResource) (int, error) {
	found := false
	foundCredentials := make([]string, len(credentialList))
	testCredentialIndex := -1

	for index := range credentialList {
		foundCredentials[index] = *credentialList[index].ID
		if *credentialList[index].ID == testCredentialId {
			if !found {
				testCredentialIndex = index
				found = true
			} else {
				return -1, fmt.Errorf("credential %s duplicated in credentialList: %v", testCredentialId, foundCredentials)
			}
		}
	}

	if !found {
		return -1, fmt.Errorf("credential: %s not found in credentialList: %v", testCredentialId, foundCredentials)
	}

	return testCredentialIndex, nil
}
