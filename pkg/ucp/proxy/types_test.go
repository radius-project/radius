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

package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/stretchr/testify/require"
)

func Test_ConvertHeaderToUCPIDs(t *testing.T) {
	type data []struct {
		name           string
		header         []string
		planeURL       string
		planeID        string
		planeKind      string
		httpScheme     string
		ucpHost        string
		expectedHeader string
	}
	positiveTestData := data{
		{
			name:           LocationHeader,
			header:         []string{"http://localhost:7443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			expectedHeader: "http://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			planeURL:       "http://localhost:7443",
			planeKind:      rest.PlaneKindAzure,
			planeID:        "/planes/test/local",
			httpScheme:     "http",
			ucpHost:        "localhost:9443",
		},
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"http://localhost:7443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			expectedHeader: "http://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			planeURL:       "http://localhost:7443",
			planeKind:      rest.PlaneKindAzure,
			planeID:        "/planes/test/local",
			httpScheme:     "http",
			ucpHost:        "localhost:9443",
		},
		{
			name:           LocationHeader,
			header:         []string{"https://localhost:7443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			expectedHeader: "https://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			planeURL:       "https://localhost:7443",
			planeKind:      rest.PlaneKindAzure,
			planeID:        "/planes/test/local",
			httpScheme:     "https",
			ucpHost:        "localhost:9443",
		},
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"https://example.com"},
			expectedHeader: "https://localhost:9443/planes/test/local/",
			planeURL:       "https://example.com",
			planeKind:      rest.PlaneKindAzure,
			planeID:        "/planes/test/local",
			httpScheme:     "https",
			ucpHost:        "localhost:9443",
		},
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"https://example.com/"},
			expectedHeader: "https://localhost:9443/planes/test/local/",
			planeURL:       "https://example.com",
			planeKind:      rest.PlaneKindAzure,
			planeID:        "/planes/test/local",
			httpScheme:     "https",
			ucpHost:        "localhost:9443",
		},
		{
			name:           AzureAsyncOperationHeader,
			header:         []string{"https://example.com"},
			expectedHeader: "https://localhost:9443/planes/test/local/",
			planeURL:       "https://example.com/",
			planeKind:      rest.PlaneKindAzure,
			planeID:        "/planes/test/local",
			httpScheme:     "https",
			ucpHost:        "localhost:9443",
		},
		{
			name:           LocationHeader,
			header:         []string{"https://localhost:7443/planes/radius/local/resourceGroups/rg/providers/Applications.Core/Containers/test"},
			expectedHeader: "https://localhost:9443/planes/radius/local/resourceGroups/rg/providers/Applications.Core/Containers/test",
			planeURL:       "https://localhost:7443",
			planeKind:      rest.PlaneKindUCPNative,
			planeID:        "/planes/radius/local",
			httpScheme:     "https",
			ucpHost:        "localhost:9443",
		},
	}
	for _, datum := range positiveTestData {
		response := http.Response{
			Header: http.Header{},
		}
		ctx := createTestContext(context.Background(), datum.planeURL, datum.planeID, datum.planeKind, datum.httpScheme, datum.ucpHost)
		err := convertHeaderToUCPIDs(ctx, datum.name, datum.header, &response)
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
		},
	}
	for _, datum := range negativeTestData {
		response := http.Response{
			Header: http.Header{},
		}
		ctx := createTestContext(context.Background(), datum.planeURL, datum.planeID, datum.planeKind, datum.httpScheme, datum.ucpHost)
		err := convertHeaderToUCPIDs(ctx, datum.name, datum.header, &response)
		require.Error(t, err, "%q should have have failed", datum)
		require.Equal(t, fmt.Sprintf("PlaneURL: %s received in the request context does not match the url found in %s header: %s", datum.planeURL, datum.name, datum.header[0]), err.Error())
	}
}

func Test_ConvertHeaderToUCPIDs_NoContextDataSet(t *testing.T) {
	response := http.Response{
		Header: http.Header{},
	}
	err := convertHeaderToUCPIDs(context.Background(), AzureAsyncOperationHeader, []string{"http://example.com/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"}, &response)
	require.Error(t, err, "Should have have failed")
	require.Equal(t, "Could not find ucp request data in Azure-Asyncoperation header", err.Error())
	err = convertHeaderToUCPIDs(context.Background(), LocationHeader, []string{"http://example.com/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"}, &response)
	require.Error(t, err, "Should have have failed")
	require.Equal(t, "Could not find ucp request data in Location header", err.Error())
}

func Test_ConvertHeaderToUCPIDs_WithUCPHost(t *testing.T) {
	type data []struct {
		name       string
		header     []string
		planeURL   string
		planeID    string
		planeKind  string
		httpScheme string
		ucpHost    string
	}
	testData := data{
		{
			name:       LocationHeader,
			header:     []string{"http://localhost:9443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			planeURL:   "http://localhost:7443",
			planeKind:  rest.PlaneKindAzure,
			planeID:    "/planes/test/local",
			httpScheme: "http",
			ucpHost:    "localhost:9443",
		},
	}

	for _, datum := range testData {
		response := http.Response{
			Header: http.Header{},
		}
		response.Header.Set(LocationHeader, "http://localhost:9443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test")
		ctx := createTestContext(context.Background(), datum.planeURL, datum.planeID, datum.planeKind, datum.httpScheme, datum.ucpHost)
		err := convertHeaderToUCPIDs(ctx, datum.name, datum.header, &response)
		require.NoError(t, err, "%q should have not have failed", datum)
		// response.SetHeader converts the header into CanonicalMIME format
		require.Equal(t, datum.header[0], response.Header[textproto.CanonicalMIMEHeaderKey(datum.name)][0])
	}
}

func Test_HasUCPHost(t *testing.T) {
	type data []struct {
		name       string
		header     []string
		planeURL   string
		planeID    string
		planeKind  string
		httpScheme string
		ucpHost    string
		result     bool
	}
	testData := data{
		{
			name:       AzureAsyncOperationHeader,
			header:     []string{"http://localhost:9443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			planeURL:   "http://localhost:9443",
			planeKind:  rest.PlaneKindAzure,
			planeID:    "/planes/test/local",
			httpScheme: "http",
			ucpHost:    "localhost:9443",
			result:     true,
		},
		{
			name:       LocationHeader,
			header:     []string{"https://localhost:9443/planes/radius/local/resourceGroups/rg/providers/Applications.Core/Containers/test"},
			planeURL:   "https://localhost:9443",
			planeKind:  rest.PlaneKindUCPNative,
			planeID:    "/planes/radius/local",
			httpScheme: "https",
			ucpHost:    "localhost:9443",
			result:     true,
		},
		{
			name:       AzureAsyncOperationHeader,
			header:     []string{"http://de-api.radius-system:6443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test"},
			planeURL:   "http://localhost:9443",
			planeKind:  rest.PlaneKindAzure,
			planeID:    "/planes/test/local",
			httpScheme: "http",
			ucpHost:    "localhost:9443",
			result:     false,
		},
		{
			name:       LocationHeader,
			header:     []string{"https://de-api.radius-system:6443/planes/radius/local/resourceGroups/rg/providers/Applications.Core/Containers/test"},
			planeURL:   "https://localhost:9443",
			planeKind:  rest.PlaneKindUCPNative,
			planeID:    "/planes/radius/local",
			httpScheme: "https",
			ucpHost:    "localhost:9443",
			result:     false,
		},
		{
			name:       LocationHeader,
			header:     []string{"https://localhost:9443/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/rg/providers/Applications.Core/Containers/test"},
			planeURL:   "https://localhost:9443",
			planeKind:  rest.PlaneKindUCPNative,
			planeID:    "/planes/radius/local",
			httpScheme: "https",
			ucpHost:    "localhost:9443/apis/api.ucp.dev/v1alpha3",
			result:     true,
		},
	}
	for _, datum := range testData {

		ctx := createTestContext(context.Background(), datum.planeURL, datum.planeID, datum.planeKind, datum.httpScheme, datum.ucpHost)
		hasUCPHost, err := hasUCPHost(ctx.Value(UCPRequestInfoField).(UCPRequestInfo), datum.name, datum.header)
		require.NoError(t, err, "%q should have not have failed", datum)
		require.Equal(t, datum.result, hasUCPHost)
	}
}
