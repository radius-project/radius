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

package aws

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/testhost"
)

const (
	aliasVPCPath    = awsProvidersPath + "AWS.EC2/VPC/app-vpc"
	aliasBucketPath = awsProvidersPath + "AWS.S3/Bucket/app-bucket"
	aliasDBPath     = awsProvidersPath + "AWS.RDS/DBInstance/app-db"
)

var aliasSchemas = map[string]string{
	"AWS::EC2::VPC":        `{"primaryIdentifier": ["/properties/VpcId"], "readOnlyProperties": ["/properties/VpcId"], "createOnlyProperties": ["/properties/CidrBlock"]}`,
	"AWS::S3::Bucket":      `{"primaryIdentifier": ["/properties/BucketName"], "createOnlyProperties": ["/properties/BucketName"]}`,
	"AWS::RDS::DBInstance": `{"primaryIdentifier": ["/properties/DBInstanceIdentifier"], "createOnlyProperties": ["/properties/DBInstanceIdentifier"], "writeOnlyProperties": ["/properties/MasterUserPassword"]}`,
}

type ucpResponse struct {
	status  int
	headers http.Header
	body    map[string]any
	raw     string
}

func send(t *testing.T, ucp *testhost.TestHost, method string, path string, owner string, body any) ucpResponse {
	t.Helper()
	var content []byte
	if body != nil {
		var err error
		content, err = json.Marshal(body)
		require.NoError(t, err)
	}
	url := path
	if strings.HasPrefix(path, "/") {
		url = ucp.BaseURL() + path
	}
	request, err := rpctest.NewHTTPRequestWithContent(t.Context(), method, url, content)
	require.NoError(t, err)
	request = request.WithContext(rpctest.NewARMRequestContext(request))
	if owner != "" {
		request.Header.Set(sdkclients.AWSResourceOwnerHeader, owner)
	}
	response, err := ucp.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	out := ucpResponse{status: response.StatusCode, headers: response.Header, raw: string(raw), body: map[string]any{}}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

// deploy sends a PUT and, when UCP accepts it as an operation, polls the operation status until it finishes, the way
// the Deployment Engine does.
func deploy(t *testing.T, ucp *testhost.TestHost, path string, body map[string]any) string {
	t.Helper()
	resp := send(t, ucp, http.MethodPut, path, testOwner, body)
	switch resp.status {
	case http.StatusOK:
		return string(v1.ProvisioningStateSucceeded)
	case http.StatusCreated, http.StatusAccepted:
		return waitForOperation(t, ucp, resp)
	default:
		require.Failf(t, "unexpected status", "%d: %s", resp.status, resp.raw)
		return ""
	}
}

func waitForOperation(t *testing.T, ucp *testhost.TestHost, resp ucpResponse) string {
	t.Helper()
	statusURL := resp.headers.Get("Azure-AsyncOperation")
	require.NotEmpty(t, statusURL)
	state := ""
	require.Eventually(t, func() bool {
		status := send(t, ucp, http.MethodGet, statusURL, "", nil)
		require.Equal(t, http.StatusOK, status.status, status.raw)
		state, _ = status.body["status"].(string)
		return v1.ProvisioningState(state).IsTerminal()
	}, 10*time.Second, 20*time.Millisecond)
	return state
}

func Test_AWSResourceByAlias_EndToEnd(t *testing.T) {
	ucp, cc := initializeFakeAWSTest(t, aliasSchemas)
	cc.Identifiers["AWS::S3::Bucket"] = "BucketName"

	// Deploying a VPC that has only an alias twice leaves one VPC.
	vpc := map[string]any{"properties": map[string]any{"CidrBlock": "10.0.0.0/16"}}
	require.Equal(t, "Succeeded", deploy(t, ucp, aliasVPCPath, vpc))
	require.Equal(t, "Succeeded", deploy(t, ucp, aliasVPCPath, vpc))
	require.Equal(t, 1, cc.Count("AWS::EC2::VPC"))

	cc.Calls()
	resp := send(t, ucp, http.MethodGet, aliasVPCPath, "", nil)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, "10.0.0.0/16", resp.body["properties"].(map[string]any)["CidrBlock"])
	// GET by alias reads AWS by the identifier in the record, not by the alias in the URL.
	calls := cc.Calls()
	require.Len(t, calls, 1)
	require.Equal(t, "GetResource", calls[0].Operation)
	require.Equal(t, "AWS::EC2::VPC", calls[0].TypeName)
	require.Equal(t, "id-1", calls[0].Identifier)

	// Removing a property clears it.
	require.Equal(t, "Succeeded", deploy(t, ucp, aliasBucketPath, map[string]any{"properties": map[string]any{"BucketName": "assets", "Tags": []any{"a"}}}))
	require.Equal(t, "Succeeded", deploy(t, ucp, aliasBucketPath, map[string]any{"properties": map[string]any{"BucketName": "assets"}}))
	require.Equal(t, map[string]any{"BucketName": "assets"}, cc.Resource("AWS::S3::Bucket", "assets"))

	// Another owner cannot change the VPC.
	resp = send(t, ucp, http.MethodPut, aliasVPCPath, "deploy;/planes/radius/local/resourceGroups/other;;", vpc)
	require.Equal(t, http.StatusConflict, resp.status, resp.raw)

	// Deleting by alias deletes the VPC; deleting again does nothing.
	resp = send(t, ucp, http.MethodDelete, aliasVPCPath, testOwner, nil)
	require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
	require.Equal(t, "Succeeded", waitForOperation(t, ucp, resp))
	require.Zero(t, cc.Count("AWS::EC2::VPC"))
	require.Equal(t, http.StatusNoContent, send(t, ucp, http.MethodDelete, aliasVPCPath, testOwner, nil).status)
	require.Equal(t, http.StatusNotFound, send(t, ucp, http.MethodGet, aliasVPCPath, "", nil).status)
}

func Test_AWSResourceByAlias_FailedCreate(t *testing.T) {
	ucp, cc := initializeFakeAWSTest(t, aliasSchemas)
	cc.FailNext = "InvalidRequest"
	require.Equal(t, "Failed", deploy(t, ucp, aliasVPCPath, map[string]any{"properties": map[string]any{"CidrBlock": "bad"}}))

	// The record is gone, so the next deploy starts over.
	require.Equal(t, "Succeeded", deploy(t, ucp, aliasVPCPath, map[string]any{"properties": map[string]any{"CidrBlock": "10.0.0.0/16"}}))
	require.Equal(t, 1, cc.Count("AWS::EC2::VPC"))
}

func Test_AWSResourceByAlias_WriteOnlyValueIsSentButNotEchoed(t *testing.T) {
	ucp, cc := initializeFakeAWSTest(t, aliasSchemas)
	cc.Identifiers["AWS::RDS::DBInstance"] = "DBInstanceIdentifier"

	resp := send(t, ucp, http.MethodPut, aliasDBPath, testOwner, map[string]any{"properties": map[string]any{
		"DBInstanceIdentifier": "app-db",
		"MasterUsername":       "admin",
		"MasterUserPassword":   testPassword,
	}})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.NotContains(t, resp.raw, testPassword)
	require.Equal(t, "Succeeded", waitForOperation(t, ucp, resp))
	calls := cc.Calls()
	require.Contains(t, calls[len(calls)-1].Document, testPassword, "the password must still be sent to AWS")
}

// Test_AWSResourceByAlias_UnchangedWriteOnlyValueIsNotSent redeploys an unchanged password through UCP, and then a
// changed one.
func Test_AWSResourceByAlias_UnchangedWriteOnlyValueIsNotSent(t *testing.T) {
	ucp, cc := initializeFakeAWSTest(t, aliasSchemas)
	cc.Identifiers["AWS::RDS::DBInstance"] = "DBInstanceIdentifier"
	cc.WriteOnly["AWS::RDS::DBInstance"] = []string{"MasterUserPassword"}
	body := func(password string) map[string]any {
		return map[string]any{"properties": map[string]any{"DBInstanceIdentifier": "app-db", "MasterUserPassword": password}}
	}
	require.Equal(t, "Succeeded", deploy(t, ucp, aliasDBPath, body(testPassword)))
	cc.Calls()

	resp := send(t, ucp, http.MethodPut, aliasDBPath, testOwner, body(testPassword))
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	for _, call := range cc.Calls() {
		require.Equal(t, "GetResource", call.Operation)
	}

	require.Equal(t, "Succeeded", deploy(t, ucp, aliasDBPath, body("changed-password")))
	updates := 0
	for _, call := range cc.Calls() {
		if call.Operation == "UpdateResource" {
			updates++
			require.Contains(t, call.Document, "changed-password")
		}
	}
	require.Equal(t, 1, updates)
}

// Test_AWSResourceByAlias_FingerprintKeyMissing starts UCP without the fingerprint key. A PUT that sets a write-only
// value fails with 500 naming the Secret and calls nothing in AWS. Other PUTs work.
func Test_AWSResourceByAlias_FingerprintKeyMissing(t *testing.T) {
	ucp, cc := initializeFakeAWSTestWithKey(t, aliasSchemas, filepath.Join(t.TempDir(), "missing"))

	resp := send(t, ucp, http.MethodPut, aliasDBPath, testOwner, map[string]any{"properties": map[string]any{"DBInstanceIdentifier": "app-db", "MasterUserPassword": testPassword}})
	require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
	require.Contains(t, resp.raw, "radius-aws-fingerprint-key")
	require.NotContains(t, resp.raw, testPassword)
	require.Empty(t, cc.Calls())
	require.Equal(t, http.StatusNotFound, send(t, ucp, http.MethodGet, aliasDBPath, "", nil).status)

	require.Equal(t, "Succeeded", deploy(t, ucp, aliasVPCPath, map[string]any{"properties": map[string]any{"CidrBlock": "10.0.0.0/16"}}))
}

// Test_AWSResource_TransitionalPostRoutes checks that POST /:put and /:delete still work as they did before resource
// records, for Deployment Engine releases that do not deploy by alias: no owner header, no record, and an operation
// whose status comes from Cloud Control.
// Test_AWSResource_TransitionalPostPutKeepsNoFingerprints sends a write-only value through POST /:put to a UCP that has
// no fingerprint key. That route saves no record, so it needs no fingerprint: it works, and nothing is stored.
func Test_AWSResource_TransitionalPostPutKeepsNoFingerprints(t *testing.T) {
	ucp, cc := initializeFakeAWSTestWithKey(t, aliasSchemas, filepath.Join(t.TempDir(), "missing"))
	cc.Identifiers["AWS::RDS::DBInstance"] = "DBInstanceIdentifier"
	body := map[string]any{"properties": map[string]any{"DBInstanceIdentifier": "legacy-db", "MasterUserPassword": testPassword}}

	resp := send(t, ucp, http.MethodPost, awsProvidersPath+"AWS.RDS/DBInstance/:put", "", body)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.NotContains(t, resp.raw, testPassword)
	require.Equal(t, "Succeeded", waitForOperation(t, ucp, resp))

	db, err := ucp.Options().DatabaseProvider.GetClient(t.Context())
	require.NoError(t, err)
	_, err = db.Get(t.Context(), awsProvidersPath+"AWS.RDS/DBInstance/legacy-db")
	require.ErrorIs(t, err, &database.ErrNotFound{})
}

func Test_AWSResource_TransitionalPostRoutes(t *testing.T) {
	ucp, cc := initializeFakeAWSTest(t, aliasSchemas)
	cc.Identifiers["AWS::S3::Bucket"] = "BucketName"
	collection := awsProvidersPath + "AWS.S3/Bucket"
	body := map[string]any{"properties": map[string]any{"BucketName": "legacy"}}

	resp := send(t, ucp, http.MethodPost, collection+"/:put", "", body)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.Equal(t, collection+"/legacy", resp.body["id"])
	require.Equal(t, "Succeeded", waitForOperation(t, ucp, resp))
	require.Equal(t, 1, cc.Count("AWS::S3::Bucket"))
	// No record is saved. GET by the ID reads AWS by the identifier in the URL, as the recipe driver does to find the
	// ARN of an output resource.
	get := send(t, ucp, http.MethodGet, collection+"/legacy", "", nil)
	require.Equal(t, http.StatusOK, get.status, get.raw)
	require.Equal(t, "legacy", get.body["properties"].(map[string]any)["BucketName"])

	resp = send(t, ucp, http.MethodPost, collection+"/:delete", "", body)
	require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
	require.Equal(t, "Succeeded", waitForOperation(t, ucp, resp))
	require.Zero(t, cc.Count("AWS::S3::Bucket"))
}

// Test_AWSResource_TransitionalDeleteWithoutRecord checks that a DELETE of a URL with no record, such as a recipe
// deleting a resource the Deployment Engine created with POST /:put, deletes the resource in AWS by the URL's last
// segment, with or without the owner header. This is transitional, like the POST routes.
func Test_AWSResource_TransitionalDeleteWithoutRecord(t *testing.T) {
	for _, owner := range []string{"", testOwner} {
		t.Run("owner "+strconv.Quote(owner), func(t *testing.T) {
			ucp, cc := initializeFakeAWSTest(t, aliasSchemas)
			cc.Identifiers["AWS::S3::Bucket"] = "BucketName"
			collection := awsProvidersPath + "AWS.S3/Bucket"

			resp := send(t, ucp, http.MethodPost, collection+"/:put", "", map[string]any{"properties": map[string]any{"BucketName": "legacy"}})
			require.Equal(t, http.StatusCreated, resp.status, resp.raw)
			require.Equal(t, "Succeeded", waitForOperation(t, ucp, resp))

			resp = send(t, ucp, http.MethodDelete, collection+"/legacy", owner, nil)
			require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
			require.Equal(t, "Succeeded", waitForOperation(t, ucp, resp))
			require.Zero(t, cc.Count("AWS::S3::Bucket"))

			// AWS no longer has it, so a retry returns 204.
			require.Equal(t, http.StatusNoContent, send(t, ucp, http.MethodDelete, collection+"/legacy", owner, nil).status)
		})
	}
}

// Test_AWSResource_TransitionalPathsSkipManagedResources checks that the transitional paths return 409 for a resource an
// alias's record manages, so they never skip that record's owner check.
func Test_AWSResource_TransitionalPathsSkipManagedResources(t *testing.T) {
	ucp, cc := initializeFakeAWSTest(t, aliasSchemas)
	cc.Identifiers["AWS::S3::Bucket"] = "BucketName"
	collection := awsProvidersPath + "AWS.S3/Bucket"
	body := map[string]any{"properties": map[string]any{"BucketName": "assets"}}

	resp := send(t, ucp, http.MethodPut, collection+"/app-bucket", testOwner, body)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.Equal(t, "Succeeded", waitForOperation(t, ucp, resp))

	for _, r := range []struct{ method, path, owner string }{
		{http.MethodPost, collection + "/:put", ""},
		{http.MethodPost, collection + "/:delete", ""},
		{http.MethodDelete, collection + "/assets", ""},
		{http.MethodDelete, collection + "/assets", testOwner},
	} {
		var reqBody any
		if r.method == http.MethodPost {
			reqBody = body
		}
		resp := send(t, ucp, r.method, r.path, r.owner, reqBody)
		require.Equal(t, http.StatusConflict, resp.status, "%s %s: %s", r.method, r.path, resp.raw)
		require.Contains(t, resp.raw, "app-bucket")
	}
	require.Equal(t, 1, cc.Count("AWS::S3::Bucket"))
	require.Equal(t, http.StatusOK, send(t, ucp, http.MethodGet, collection+"/app-bucket", "", nil).status)
}
