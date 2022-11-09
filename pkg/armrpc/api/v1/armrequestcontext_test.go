// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromARMRequest(t *testing.T) {
	req, err := getTestHTTPRequest()
	require.NoError(t, err)

	serviceCtx, _ := FromARMRequest(req, "", LocationGlobal)
	require.Equal(t, "2022-03-15-privatepreview", serviceCtx.APIVersion)
	require.Equal(t, "00000000-0000-0000-0000-000000000001", serviceCtx.ClientTenantID)
	require.Equal(t, "00000000-0000-0000-0000-000000000002", serviceCtx.HomeTenantID)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", serviceCtx.ResourceID.String())
	require.Equal(t, "00000000-0000-0000-0000-000000000000", serviceCtx.ResourceID.ScopeSegments()[0].Name)
	require.Equal(t, "radius-test-rg", serviceCtx.ResourceID.ScopeSegments()[1].Name)
	require.Equal(t, "Applications.Core/environments", serviceCtx.ResourceID.Type())
	require.Equal(t, "env0", serviceCtx.ResourceID.Name())
	require.True(t, len(serviceCtx.OperationID) > 0)
}

func TestSystemData(t *testing.T) {
	req, err := getTestHTTPRequest()
	require.NoError(t, err)
	serviceCtx, _ := FromARMRequest(req, "", LocationGlobal)

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
	req, err := getTestHTTPRequest()
	require.NoError(t, err)
	serviceCtx, err := FromARMRequest(req, "", LocationGlobal)
	require.NoError(t, err)
	ctx := context.Background()
	newCtx := WithARMRequestContext(ctx, serviceCtx)

	sCtx := ARMRequestContextFromContext(newCtx)
	require.NotNil(t, sCtx)
	require.Equal(t, "2022-03-15-privatepreview", sCtx.APIVersion)
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
			req, err := getTestHTTPRequest()

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

func getTestHTTPRequest() (*http.Request, error) {
	jsonData, err := os.ReadFile("./testdata/armrpcheaders.json")
	if err != nil {
		return nil, err
	}

	parsed := map[string]string{}
	if err = json.Unmarshal(jsonData, &parsed); err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, parsed["Referer"], nil)
	for k, v := range parsed {
		req.Header.Add(k, v)
	}
	return req, nil
}
