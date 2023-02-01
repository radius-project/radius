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

func Test_AWS_Credential_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_AWS_Credential_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		resourceTypePath := "/planes/aws/aws/providers/System.AWS/credentials"
		resourceURL := fmt.Sprintf("%s%s/default?api-version=%s", url, resourceTypePath, ucp.Version)
		collectionURL := fmt.Sprintf("%s%s?api-version=%s", url, resourceTypePath, ucp.Version)
		runAWSCredentialTests(t, resourceURL, collectionURL, roundTripper, getAWSCredentialObject(), getExpectedAWSCredentialObject())
	})

	test.Test(t)
}
func runAWSCredentialTests(t *testing.T, resourceUrl string, collectionUrl string, roundTripper http.RoundTripper, createCredential ucp.AWSCredentialResource, expectedCredential ucp.AWSCredentialResource) {
	// Create credential operation
	createAWSCredential(t, roundTripper, resourceUrl, createCredential)
	// Create duplicate credential
	createAWSCredential(t, roundTripper, resourceUrl, createCredential)
	// List credential operation
	credentialList := listAWSCredential(t, roundTripper, collectionUrl)
	require.Equal(t, len(credentialList), 1)
	assert.DeepEqual(t, credentialList[0], expectedCredential)

	// Check for correctness of credential
	createdCredential, statusCode := getAWSCredential(t, roundTripper, resourceUrl)

	require.Equal(t, http.StatusOK, statusCode)
	assert.DeepEqual(t, createdCredential, expectedCredential)

	// Delete credential operation
	statusCode, err := deleteAWSCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)

	// Delete non-existent credential
	statusCode, err = deleteAWSCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, statusCode)
}

func createAWSCredential(t *testing.T, roundTripper http.RoundTripper, url string, credential ucp.AWSCredentialResource) {
	body, err := json.Marshal(credential)
	require.NoError(t, err)
	createRequest, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Credential: %s created/updated successfully", url)
}

func getAWSCredential(t *testing.T, roundTripper http.RoundTripper, url string) (ucp.AWSCredentialResource, int) {
	getCredentialRequest, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	result, err := roundTripper.RoundTrip(getCredentialRequest)
	require.NoError(t, err)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)

	credential := ucp.AWSCredentialResource{}
	err = json.Unmarshal(payload, &credential)
	require.NoError(t, err)

	return credential, result.StatusCode
}

func deleteAWSCredential(t *testing.T, roundTripper http.RoundTripper, url string) (int, error) {
	deleteCredentialRequest, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(deleteCredentialRequest)
	return res.StatusCode, err
}

func listAWSCredential(t *testing.T, roundTripper http.RoundTripper, url string) []ucp.AWSCredentialResource {
	listCredentialRequest, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(listCredentialRequest)
	require.NoError(t, err)
	return getAWSCredentialList(t, res)
}

func getAWSCredentialList(t *testing.T, res *http.Response) []ucp.AWSCredentialResource {
	body := res.Body
	defer body.Close()

	var data map[string]any
	err := json.NewDecoder(body).Decode(&data)
	require.NoError(t, err)
	list, ok := data["value"].([]any)
	require.Equal(t, ok, true)
	var credentialList []ucp.AWSCredentialResource
	for _, item := range list {
		s, err := json.Marshal(item)
		require.NoError(t, err)
		credential := ucp.AWSCredentialResource{}
		err = json.Unmarshal(s, &credential)
		require.NoError(t, err)
		credentialList = append(credentialList, credential)
	}
	return credentialList
}

func getAWSCredentialObject() ucp.AWSCredentialResource {
	return ucp.AWSCredentialResource{
		Location: to.Ptr("west-us-2"),
		ID:       to.Ptr("/planes/aws/aws/providers/System.AWS/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.AWS/credentials"),
		Tags: map[string]*string{
			"env": to.Ptr("dev"),
		},
		Properties: &v20220901privatepreview.AWSAccessKeyCredentialProperties{
			AccessKeyID:     to.Ptr("00000000-0000-0000-0000-000000000000"),
			SecretAccessKey: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:            to.Ptr("AccessKey"),
			Storage: &v20220901privatepreview.InternalCredentialStorageProperties{
				Kind:       to.Ptr(string(v20220901privatepreview.CredentialStorageKindInternal)),
				SecretName: to.Ptr("aws-awscloud-default"),
			},
		},
	}
}

func getExpectedAWSCredentialObject() ucp.AWSCredentialResource {
	return ucp.AWSCredentialResource{
		Location: to.Ptr("west-us-2"),
		ID:       to.Ptr("/planes/aws/aws/providers/System.AWS/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.AWS/credentials"),
		Tags: map[string]*string{
			"env": to.Ptr("dev"),
		},
		Properties: &v20220901privatepreview.AWSAccessKeyCredentialProperties{
			AccessKeyID: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:        to.Ptr("AccessKey"),
			Storage: &v20220901privatepreview.InternalCredentialStorageProperties{
				Kind:       to.Ptr(string(v20220901privatepreview.CredentialStorageKindInternal)),
				SecretName: to.Ptr("aws-awscloud-default"),
			},
		},
	}
}
