/*
Copyright 2023 The Radius Authors.

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

package ucp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/stretchr/testify/require"
)

func Test_AWS_Credential_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_AWS_Credential_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		resourceTypePath := "/planes/aws/awstest/providers/System.AWS/credentials"
		resourceURL := fmt.Sprintf("%s%s/default?api-version=%s", url, resourceTypePath, ucp.Version)
		collectionURL := fmt.Sprintf("%s%s?api-version=%s", url, resourceTypePath, ucp.Version)
		runAWSCredentialTests(t, resourceURL, collectionURL, roundTripper, getAWSTestCredentialObject(), getExpectedAWSTestCredentialObject())
	})

	test.Test(t)
}

func runAWSCredentialTests(t *testing.T, resourceUrl string, collectionUrl string, roundTripper http.RoundTripper, createCredential ucp.AwsCredentialResource, expectedCredential ucp.AwsCredentialResource) {
	// Create credential operation
	createAWSTestCredential(t, roundTripper, resourceUrl, createCredential)

	// Create duplicate credential
	createAWSTestCredential(t, roundTripper, resourceUrl, createCredential)

	// List credential operation
	credentialList := listAWSTestCredential(t, roundTripper, collectionUrl)

	index, err := getIndexOfAWSTestCredential(*expectedCredential.ID, credentialList)
	require.NoError(t, err)
	require.Equal(t, credentialList[index], expectedCredential)

	// Check for correctness of credential
	createdCredential, statusCode := getAWSTestCredential(t, roundTripper, resourceUrl)

	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, createdCredential, expectedCredential)

	// Delete credential operation
	statusCode, err = deleteAWSTestCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)

	// Delete non-existent credential
	statusCode, err = deleteAWSTestCredential(t, roundTripper, resourceUrl)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, statusCode)
}

func createAWSTestCredential(t *testing.T, roundTripper http.RoundTripper, url string, credential ucp.AwsCredentialResource) {
	body, err := json.Marshal(credential)
	require.NoError(t, err)
	createRequest, err := NewUCPRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Credential: %s created/updated successfully", url)
}

func getAWSTestCredential(t *testing.T, roundTripper http.RoundTripper, url string) (ucp.AwsCredentialResource, int) {
	getCredentialRequest, err := NewUCPRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	result, err := roundTripper.RoundTrip(getCredentialRequest)
	require.NoError(t, err)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)

	credential := ucp.AwsCredentialResource{}
	err = json.Unmarshal(payload, &credential)
	require.NoError(t, err)

	return credential, result.StatusCode
}

func deleteAWSTestCredential(t *testing.T, roundTripper http.RoundTripper, url string) (int, error) {
	deleteCredentialRequest, err := NewUCPRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(deleteCredentialRequest)
	return res.StatusCode, err
}

func listAWSTestCredential(t *testing.T, roundTripper http.RoundTripper, url string) []ucp.AwsCredentialResource {
	listCredentialRequest, err := NewUCPRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	res, err := roundTripper.RoundTrip(listCredentialRequest)
	require.NoError(t, err)
	return getAWSTestCredentialList(t, res)
}

func getAWSTestCredentialList(t *testing.T, res *http.Response) []ucp.AwsCredentialResource {
	body := res.Body
	defer body.Close()

	var data map[string]any
	err := json.NewDecoder(body).Decode(&data)
	require.NoError(t, err)
	list, ok := data["value"].([]any)
	require.Equal(t, ok, true)
	var credentialList []ucp.AwsCredentialResource
	for _, item := range list {
		s, err := json.Marshal(item)
		require.NoError(t, err)
		credential := ucp.AwsCredentialResource{}
		err = json.Unmarshal(s, &credential)
		require.NoError(t, err)
		credentialList = append(credentialList, credential)
	}
	return credentialList
}

func getAWSTestCredentialObject() ucp.AwsCredentialResource {
	return ucp.AwsCredentialResource{
		Location: to.Ptr("global"),
		ID:       to.Ptr("/planes/aws/awstest/providers/System.AWS/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.AWS/credentials"),
		Tags: map[string]*string{
			"env": to.Ptr("dev"),
		},
		Properties: &ucp.AwsAccessKeyCredentialProperties{
			AccessKeyID:     to.Ptr("00000000-0000-0000-0000-000000000000"),
			SecretAccessKey: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:            to.Ptr(ucp.AWSCredentialKindAccessKey),
			Storage: &ucp.InternalCredentialStorageProperties{
				Kind:       to.Ptr(ucp.CredentialStorageKindInternal),
				SecretName: to.Ptr("aws-awstest-default"),
			},
		},
	}
}

func getExpectedAWSTestCredentialObject() ucp.AwsCredentialResource {
	return ucp.AwsCredentialResource{
		Location: to.Ptr("global"),
		ID:       to.Ptr("/planes/aws/awstest/providers/System.AWS/credentials/default"),
		Name:     to.Ptr("default"),
		Type:     to.Ptr("System.AWS/credentials"),
		Tags: map[string]*string{
			"env": to.Ptr("dev"),
		},
		Properties: &ucp.AwsAccessKeyCredentialProperties{
			AccessKeyID: to.Ptr("00000000-0000-0000-0000-000000000000"),
			Kind:        to.Ptr(ucp.AWSCredentialKindAccessKey),
			Storage: &ucp.InternalCredentialStorageProperties{
				Kind:       to.Ptr(ucp.CredentialStorageKindInternal),
				SecretName: to.Ptr("aws-awstest-default"),
			},
		},
	}
}

func getIndexOfAWSTestCredential(testCredentialId string, credentialList []ucp.AwsCredentialResource) (int, error) {
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
