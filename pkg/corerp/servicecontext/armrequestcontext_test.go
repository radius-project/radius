// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicecontext

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromARMRequest(t *testing.T) {
	req, err := getTestHTTPRequest()
	require.NoError(t, err)

	serviceCtx, err := FromARMRequest(req, "")
	require.Equal(t, "2022-03-15-privatepreview", serviceCtx.APIVersion)
	require.Equal(t, "00000000-0000-0000-0000-000000000001", serviceCtx.ClientTenantID)
	require.Equal(t, "00000000-0000-0000-0000-000000000002", serviceCtx.HomeTenantID)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", serviceCtx.ResourceID.ID)
	require.Equal(t, "00000000-0000-0000-0000-000000000000", serviceCtx.ResourceID.SubscriptionID)
	require.Equal(t, "radius-test-rg", serviceCtx.ResourceID.ResourceGroup)
	require.Equal(t, "Applications.Core/environments", serviceCtx.ResourceID.Types[0].Type)
	require.Equal(t, "env0", serviceCtx.ResourceID.Types[0].Name)
	require.True(t, len(serviceCtx.OperationID) > 0)
}

func TestSystemData(t *testing.T) {
	req, err := getTestHTTPRequest()
	require.NoError(t, err)
	serviceCtx, err := FromARMRequest(req, "")

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
	serviceCtx, err := FromARMRequest(req, "")
	require.NoError(t, err)
	ctx := context.Background()
	newCtx := WithARMRequestContext(ctx, serviceCtx)

	sCtx := ARMRequestContextFromContext(newCtx)
	require.NotNil(t, sCtx)
	require.Equal(t, "2022-03-15-privatepreview", sCtx.APIVersion)
}

func getTestHTTPRequest() (*http.Request, error) {
	jsonData, err := ioutil.ReadFile("./testdata/armrpcheaders.json")
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
