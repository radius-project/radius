// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ConvertHeaderToUCPIDs(t *testing.T) {
	type data []struct {
		name           string
		header         []string
		planeURL       string
		planeID        string
		httpScheme     string
		expectedHeader string
	}
	positiveTestData := data{
		{
			name:           LocationHeader,
			header:         []string{"http://localhost:7443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			expectedHeader: "http://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			planeURL:       "http://localhost:7443",
			planeID:        "/planes/test/local",
			httpScheme:     "http",
		},
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"http://localhost:7443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			expectedHeader: "http://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			planeURL:       "http://localhost:7443",
			planeID:        "/planes/test/local",
			httpScheme:     "http",
		},
		{
			name:           LocationHeader,
			header:         []string{"https://localhost:7443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			expectedHeader: "https://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			planeURL:       "https://localhost:7443",
			planeID:        "/planes/test/local",
			httpScheme:     "https",
		},
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"https://example.com"},
			expectedHeader: "https://localhost:9443/planes/test/local/",
			planeURL:       "https://example.com",
			planeID:        "/planes/test/local",
			httpScheme:     "https",
		},
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"https://example.com/"},
			expectedHeader: "https://localhost:9443/planes/test/local/",
			planeURL:       "https://example.com",
			planeID:        "/planes/test/local",
			httpScheme:     "https",
		},
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"https://example.com"},
			expectedHeader: "https://localhost:9443/planes/test/local/",
			planeURL:       "https://example.com/",
			planeID:        "/planes/test/local",
			httpScheme:     "https",
		},
	}
	for _, datum := range positiveTestData {
		response := http.Response{
			Header: http.Header{},
		}
		ctx := createTestContext(context.Background(), datum.planeURL, datum.planeID, datum.httpScheme)
		err := convertHeaderToUCPIDs(ctx, "localhost:9443", datum.name, datum.header, &response)
		require.NoError(t, err, "%q should have not have failed", datum)
		// response.SetHeader converts the header into CanonicalMIME format
		require.Equal(t, datum.expectedHeader, response.Header[textproto.CanonicalMIMEHeaderKey(datum.name)][0])
	}

	negativeTestData := data{
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"http://example.com/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			expectedHeader: "",
			planeURL:       "https://localhost:7443",
		},
		{
			name:           LocationHeader,
			header:         []string{"http://example.com/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			expectedHeader: "",
			planeURL:       "http://localhost:7443",
			planeID:        "",
		},
	}
	for _, datum := range negativeTestData {
		response := http.Response{
			Header: http.Header{},
		}
		ctx := createTestContext(context.Background(), datum.planeURL, datum.planeID, datum.httpScheme)
		err := convertHeaderToUCPIDs(ctx, "localhost:9443", datum.name, datum.header, &response)
		require.Error(t, err, "%q should have have failed", datum)
		require.Equal(t, fmt.Sprintf("PlaneURL: %s received in the request context does not match the url found in %s header", datum.planeURL, datum.name), err.Error())
	}
}

func Test_ConvertHeaderToUCPIDs_NoContextDataSet(t *testing.T) {
	response := http.Response{
		Header: http.Header{},
	}
	err := convertHeaderToUCPIDs(context.Background(), "localhost:9443", AzureAsyncOperationHeader, []string{"http://example.com/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"}, &response)
	require.Error(t, err, "Should have have failed")
	require.Equal(t, "Could not find plane URL data in Azure-Asyncoperation header", err.Error())
	err = convertHeaderToUCPIDs(context.Background(), "localhost:9443", LocationHeader, []string{"http://example.com/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"}, &response)
	require.Error(t, err, "Should have have failed")
	require.Equal(t, "Could not find plane URL data in Location header", err.Error())
}
