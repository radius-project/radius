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
		credentialResourceID := "/planes/azure/azurecloud/providers/System.Azure/credentials/default"
		credentialCollectionID := "/planes/azure/azurecloud/providers/System.Azure/credentials"
		credentialResourceURL := fmt.Sprintf("%s%s?api-version=%s", url, credentialResourceID, ucp.Version)
		credentialCollectionURL := fmt.Sprintf("%s%s?api-version=%s", url, credentialCollectionID, ucp.Version)
		credentialOperations(t, credentialResourceURL, credentialCollectionURL, roundTripper, getAzureCredentialObject())
	})

	test.Test(t)
}

func Test_AWS_Credential_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_AWS_Credential_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		credentialResourceID := "/planes/aws/awscloud/providers/System.AWS/credentials/default"
		credentialCollectionID := "/planes/aws/awscloud/providers/System.AWS/credentials"
		credentialResourceURL := fmt.Sprintf("%s%s?api-version=%s", url, credentialResourceID, ucp.Version)
		credentialCollectionURL := fmt.Sprintf("%s%s?api-version=%s", url, credentialCollectionID, ucp.Version)
		credentialOperations(t, credentialResourceURL, credentialCollectionURL, roundTripper, getAWSCredentialObject())
	})

	test.Test(t)
}

func credentialOperations(t *testing.T, resourceUrl string, collectionUrl string, roundTripper http.RoundTripper, credential ucp.CredentialResource) {
	// Create credential operation
	createCredential(t, roundTripper, resourceUrl, credential)
	// Create duplicate credential
	createCredential(t, roundTripper, resourceUrl, credential)
	// List credential operation
	credentialList := listCredential(t, roundTripper, collectionUrl)
	require.Equal(t, len(credentialList), 1)
	assert.DeepEqual(t, credentialList[0], credential)

	// // Check for correctness of credential
	credential, statusCode := getCredential(t, roundTripper, resourceUrl)
	require.Equal(t, http.StatusOK, statusCode)
	assert.DeepEqual(t, credential, credential)

	// Delete credential operation
	statusCode, err := deleteCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)

	// Delete non-existent credential
	statusCode, err = deleteCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, statusCode)
}

func createCredential(t *testing.T, roundTripper http.RoundTripper, url string, credential ucp.CredentialResource) {
	body, err := json.Marshal(credential)
	require.NoError(t, err)
	createRequest, err := http.NewRequest(
		http.MethodPut,
		url,
		bytes.NewBuffer(body))
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err, "")

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Credential: %s created/updated successfully", url)
}

func getCredential(t *testing.T, roundTripper http.RoundTripper, url string) (ucp.CredentialResource, int) {
	getCredentialRequest, err := http.NewRequest(
		http.MethodGet,
		url,
		nil,
	)
	require.NoError(t, err, "")

	result, err := roundTripper.RoundTrip(getCredentialRequest)
	require.NoError(t, err, "")

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)

	credential := ucp.CredentialResource{}
	err = json.Unmarshal(payload, &credential)
	require.NoError(t, err)

	return credential, result.StatusCode
}

func deleteCredential(t *testing.T, roundTripper http.RoundTripper, url string) (int, error) {
	deleteCredentialRequest, err := http.NewRequest(
		http.MethodDelete,
		url,
		nil,
	)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(deleteCredentialRequest)
	return res.StatusCode, err
}

func listCredential(t *testing.T, roundTripper http.RoundTripper, url string) []ucp.CredentialResource {
	listCredentialRequest, err := http.NewRequest(
		http.MethodGet,
		url,
		nil,
	)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(listCredentialRequest)
	require.NoError(t, err)
	return getCredentialList(t, res)
}

func getCredentialList(t *testing.T, res *http.Response) []ucp.CredentialResource {
	var data map[string]interface{}
	body := res.Body
	defer body.Close()
	err := json.NewDecoder(body).Decode(&data)
	require.NoError(t, err)
	list, ok := data["value"].([]interface{})
	require.Equal(t, ok, true)
	var credentialList []ucp.CredentialResource
	for _, item := range list {
		s, err := json.Marshal(item)
		require.NoError(t, err)
		credential := ucp.CredentialResource{}
		err = json.Unmarshal(s, &credential)
		require.NoError(t, err)
		credentialList = append(credentialList, credential)
	}
	return credentialList
}

func getAzureCredentialObject() ucp.CredentialResource {
	return ucp.CredentialResource{
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
			Kind:     to.Ptr("azure.com.serviceprincipal"),
			Storage: &ucp.InternalCredentialStorageProperties{
				Kind:       to.Ptr(v20220901privatepreview.CredentialStorageKindInternal),
				SecretName: to.Ptr("azure_azurecloud_default"),
			},
		},
	}
}

func getAWSCredentialObject() ucp.CredentialResource {
	return ucp.CredentialResource{
		Location: to.Ptr("west-us-2"),
		ID:       to.Ptr("/planes/aws/awscloud/providers/System.AWS/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.AWS/credentials"),
		Tags: map[string]*string{
			"env": to.Ptr("dev"),
		},
		Properties: &v20220901privatepreview.AWSCredentialProperties{
			AccessKeyID:     to.Ptr("00000000-0000-0000-0000-000000000000"),
			SecretAccessKey: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:            to.Ptr("aws.com.iam"),
			Storage: &v20220901privatepreview.InternalCredentialStorageProperties{
				Kind:       to.Ptr(v20220901privatepreview.CredentialStorageKindInternal),
				SecretName: to.Ptr("aws_awscloud_default"),
			},
		},
	}
}
