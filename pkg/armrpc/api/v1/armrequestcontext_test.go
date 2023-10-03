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

package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestFromARMRequest(t *testing.T) {
	headerTests := []struct {
		desc       string
		refererUrl string
		resourceID string
	}{
		{
			"With referer header",
			"https://radapp.io/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-RG/providers/Applications.Core/environments/Env0?api-version=2023-10-01-preview",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-RG/providers/Applications.Core/environments/Env0",
		},
		{
			"Without referer header",
			"",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/radius-test-rg/providers/applications.core/environments/env0",
		},
		{
			"With referer path base",
			"https://radapp.io/apis/api.ucp.dev/v1alpha3/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-RG/providers/Applications.Core/environments/Env0?api-version=2023-10-01-preview",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-RG/providers/Applications.Core/environments/Env0",
		},
	}

	for _, tt := range headerTests {
		t.Run(tt.desc, func(t *testing.T) {
			req, err := getTestHTTPRequest("./testdata/armrpcheaders.json")
			require.NoError(t, err)

			if tt.refererUrl == "" {
				req.Header.Del(RefererHeader)
			} else {
				req.Header.Set(RefererHeader, tt.refererUrl)
			}

			rid, err := resources.ParseResource(tt.resourceID)
			require.NoError(t, err)

			serviceCtx, err := FromARMRequest(req, "", LocationGlobal)
			require.NoError(t, err)
			require.Equal(t, "2023-10-01-preview", serviceCtx.APIVersion)
			require.Equal(t, "00000000-0000-0000-0000-000000000001", serviceCtx.ClientTenantID)
			require.Equal(t, "00000000-0000-0000-0000-000000000002", serviceCtx.HomeTenantID)
			require.Equal(t, tt.resourceID, serviceCtx.ResourceID.String())
			require.Equal(t, rid.ScopeSegments()[0].Name, serviceCtx.ResourceID.ScopeSegments()[0].Name)
			require.Equal(t, rid.ScopeSegments()[1].Name, serviceCtx.ResourceID.ScopeSegments()[1].Name)
			require.Equal(t, rid.Type(), serviceCtx.ResourceID.Type())
			require.Equal(t, rid.Name(), serviceCtx.ResourceID.Name())
			require.True(t, len(serviceCtx.OperationID) > 0)
		})
	}
}

func TestSystemData(t *testing.T) {
	req, err := getTestHTTPRequest("./testdata/armrpcheaders.json")
	require.NoError(t, err)
	serviceCtx, err := FromARMRequest(req, "", LocationGlobal)
	require.NoError(t, err)

	sysData := serviceCtx.SystemData()
	require.NotNil(t, sysData)
	require.Equal(t, "", sysData.CreatedAt)
	require.Equal(t, "", sysData.CreatedBy)
	require.Equal(t, "", sysData.CreatedByType)
	require.Equal(t, "2022-03-22T18:54:52.6857175Z", sysData.LastModifiedAt)
	require.Equal(t, "fake@hotmail.com", sysData.LastModifiedBy)
	require.Equal(t, "User", sysData.LastModifiedByType)
}

func TestFromContext(t *testing.T) {
	t.Run("ARMRequestContext is injected", func(t *testing.T) {
		req, err := getTestHTTPRequest("./testdata/armrpcheaders.json")
		require.NoError(t, err)
		serviceCtx, err := FromARMRequest(req, "", LocationGlobal)
		require.NoError(t, err)
		ctx := context.Background()
		newCtx := WithARMRequestContext(ctx, serviceCtx)

		sCtx := ARMRequestContextFromContext(newCtx)
		require.NotNil(t, sCtx)
		require.Equal(t, "2023-10-01-preview", sCtx.APIVersion)
	})

	t.Run("ARMRequestContext is not injected", func(t *testing.T) {
		require.Panics(t, func() {
			ARMRequestContextFromContext(context.Background())
		})
	})
}

func TestTopQueryParam(t *testing.T) {
	topQueryParamCases := []struct {
		desc        string
		qpKey       string
		qpValue     string
		expectedTop int
		shouldFail  bool
	}{
		{"no-top-query-param", "top", "", DefaultQueryItemCount, false},
		{"invalid-top-query-param", "top", "xyz", 0, true},
		{"out-of-bounds-top-query-param", "top", "100000", 0, true},
		{"out-of-bounds-top-query-param", "top", "-100", 0, true},
	}

	for _, tt := range topQueryParamCases {
		t.Run(tt.desc, func(t *testing.T) {
			req, err := getTestHTTPRequest("./testdata/armrpcheaders.json")

			q := req.URL.Query()
			q.Add(tt.qpKey, tt.qpValue)
			req.URL.RawQuery = q.Encode()

			require.NoError(t, err)
			serviceCtx, err := FromARMRequest(req, "", LocationGlobal)

			if tt.shouldFail {
				require.NotNil(t, err)
				require.Nil(t, serviceCtx)
			} else {
				require.Nil(t, err)
				require.NotNil(t, serviceCtx)
				require.Equal(t, tt.expectedTop, serviceCtx.Top)
			}
		})
	}
}

func getTestHTTPRequest(headerFile string) (*http.Request, error) {
	jsonData, err := os.ReadFile(headerFile)
	if err != nil {
		return nil, err
	}

	parsed := map[string]string{}
	if err = json.Unmarshal(jsonData, &parsed); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, strings.ToLower(parsed["Referer"]), nil)
	if err != nil {
		return nil, err
	}
	for k, v := range parsed {
		req.Header.Add(k, v)
	}
	return req, nil
}

func TestParsePathBase(t *testing.T) {
	prefixTests := []struct {
		desc        string
		refererPath string
		prefix      string
		resourceID  string
	}{
		{
			"With api prefix",
			"/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/radius-test-RG/providers/Applications.Core/environments/Env0",
			"/apis/api.ucp.dev/v1alpha3",
			"/planes/radius/local/resourceGroups/radius-test-RG/providers/Applications.Core/environments/Env0",
		},
		{
			"Without api prefix header",
			"/planes/radius/local/resourceGroups/radius-test-RG/providers/Applications.Core/environments/Env0",
			"",
			"/planes/radius/local/resourceGroups/radius-test-RG/providers/Applications.Core/environments/Env0",
		},
		{
			"Empty path",
			"",
			"",
			"",
		},
		{
			"With api prefix (/subscription/ path)",
			"/apis/api.ucp.dev/v1alpha3/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/radius-test-rg/providers/applications.core/environments/env0",
			"/apis/api.ucp.dev/v1alpha3",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/radius-test-rg/providers/applications.core/environments/env0",
		},
		{
			"Without api prefix (/subscription/ path)",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/radius-test-rg/providers/applications.core/environments/env0",
			"",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/radius-test-rg/providers/applications.core/environments/env0",
		},
		{
			"With api prefix (AWS path)",
			"/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1",
			"/apis/api.ucp.dev/v1alpha3",
			"/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1",
		},
		{
			"Without api prefix (AWS path)",
			"/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1",
			"",
			"/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1",
		},
	}

	for _, tt := range prefixTests {
		t.Run(tt.desc, func(t *testing.T) {
			pathPrefix := ParsePathBase(tt.refererPath)
			require.Equal(t, pathPrefix, tt.prefix)

			path := strings.TrimPrefix(tt.refererPath, pathPrefix)
			require.Equal(t, path, tt.resourceID)
		})
	}
}
