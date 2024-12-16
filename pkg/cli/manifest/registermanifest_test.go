/*
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package manifest

import (
	"context"
	"net/http"
	"testing"

	// armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpfake "github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/stretchr/testify/require"
)

func NewTestClientFactory() (*v20231001preview.ClientFactory, error) {

	// Create fake servers for each client
	resourceProvidersServer := ucpfake.ResourceProvidersServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resource v20231001preview.ResourceProviderResource,
			options *v20231001preview.ResourceProvidersClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceProvidersClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Simulate successful creation
			result := v20231001preview.ResourceProvidersClientCreateOrUpdateResponse{
				ResourceProviderResource: resource,
			}
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
		Get: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientGetOptions,
		) (resp azfake.Responder[v20231001preview.ResourceProvidersClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.ResourceProvidersClientGetResponse{
				ResourceProviderResource: v20231001preview.ResourceProviderResource{
					Name: to.Ptr(resourceProviderName),
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}

	// Create other fake servers similarly
	resourceTypesServer := ucpfake.ResourceTypesServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resourceTypeName string,
			resource v20231001preview.ResourceTypeResource,
			options *v20231001preview.ResourceTypesClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceTypesClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20231001preview.ResourceTypesClientCreateOrUpdateResponse{
				ResourceTypeResource: resource,
			}

			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
		Get: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resourceTypeName string,
			options *v20231001preview.ResourceTypesClientGetOptions,
		) (resp azfake.Responder[v20231001preview.ResourceTypesClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.ResourceTypesClientGetResponse{
				ResourceTypeResource: v20231001preview.ResourceTypeResource{
					Name: to.Ptr(resourceTypeName),
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}

	apiVersionsServer := ucpfake.APIVersionsServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resourceTypeName string,
			apiVersionName string, // Added missing parameter
			resource v20231001preview.APIVersionResource,
			options *v20231001preview.APIVersionsClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.APIVersionsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Simulate successful creation
			result := v20231001preview.APIVersionsClientCreateOrUpdateResponse{
				APIVersionResource: resource,
			}
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)
			return
		},
	}

	locationsServer := ucpfake.LocationsServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			locationName string,
			resource v20231001preview.LocationResource,
			options *v20231001preview.LocationsClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.LocationsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Simulate successful creation
			result := v20231001preview.LocationsClientCreateOrUpdateResponse{
				LocationResource: resource,
			}
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
	}

	serverFactory := ucpfake.ServerFactory{
		ResourceProvidersServer: resourceProvidersServer,
		ResourceTypesServer:     resourceTypesServer,
		APIVersionsServer:       apiVersionsServer,
		LocationsServer:         locationsServer,
	}

	serverFactoryTransport := ucpfake.NewServerFactoryTransport(&serverFactory)

	clientOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: serverFactoryTransport,
		},
	}

	clientFactory, err := v20231001preview.NewClientFactory(&azfake.TokenCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	return clientFactory, err
}

func TestRegisterDirectory(t *testing.T) {
	tests := []struct {
		name                     string
		clientFactory            func() (*v20231001preview.ClientFactory, error)
		planeName                string
		directoryPath            string
		expectError              bool
		expectedErrorMessage     string
		expectedResourceProvider string
	}{
		{
			name: "Success",
			clientFactory: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory()
			},
			planeName:                "local",
			directoryPath:            "testdata2",
			expectError:              false,
			expectedErrorMessage:     "",
			expectedResourceProvider: "MyCompany2.CompanyName2",
		},
		{
			name: "EmptyDirectoryPath",
			clientFactory: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory()
			},
			planeName:                "local",
			directoryPath:            "",
			expectError:              true,
			expectedErrorMessage:     "invalid manifest directory",
			expectedResourceProvider: "",
		},
		{
			name: "InvalidDirectoryPath",
			clientFactory: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory()
			},
			planeName:                "local",
			directoryPath:            "#^$/invalid",
			expectError:              true,
			expectedErrorMessage:     "failed to access manifest path #^$/invalid: stat #^$/invalid: no such file or directory",
			expectedResourceProvider: "",
		},
		{
			name: "FilePathInsteadOfDirectory",
			clientFactory: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory()
			},
			planeName:                "local",
			directoryPath:            "testdata/valid.yaml",
			expectError:              true,
			expectedErrorMessage:     "manifest path testdata/valid.yaml is not a directory path",
			expectedResourceProvider: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory, err := tt.clientFactory()
			require.NoError(t, err, "Failed to create client factory")
			logger := func(message string) {}

			err = RegisterDirectory(context.Background(), *clientFactory, tt.planeName, tt.directoryPath, logger)
			if tt.expectError {
				require.Error(t, err, "Expected an error but got none")
				require.Contains(t, err.Error(), tt.expectedErrorMessage, "Error message does not match")
			} else {
				require.NoError(t, err, "Did not expect an error but got one")

				// Verify the expected resource provider exists
				if tt.expectedResourceProvider != "" {
					rp, err := clientFactory.NewResourceProvidersClient().Get(context.Background(), tt.planeName, tt.expectedResourceProvider, nil)
					require.NoError(t, err, "Failed to retrieve the expected resource provider")
					require.Equal(t, to.Ptr(tt.expectedResourceProvider), rp.Name, "Resource provider name does not match")
				}
			}
		})
	}
}

func TestRegisterFile(t *testing.T) {
	tests := []struct {
		name                     string
		clientFactory            func() (*v20231001preview.ClientFactory, error)
		planeName                string
		filePath                 string
		expectError              bool
		expectedErrorMessage     string
		expectedResourceProvider string
	}{
		{
			name: "Success",
			clientFactory: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory()
			},
			planeName:                "local",
			filePath:                 "testdata2",
			expectError:              false,
			expectedErrorMessage:     "",
			expectedResourceProvider: "MyCompany2.CompanyName2",
		},
		{
			name: "EmptyDirectoryPath",
			clientFactory: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory()
			},
			planeName:                "local",
			filePath:                 "",
			expectError:              true,
			expectedErrorMessage:     "invalid manifest directory",
			expectedResourceProvider: "",
		},
		{
			name: "InvalidDirectoryPath",
			clientFactory: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory()
			},
			planeName:                "local",
			filePath:                 "#^$/invalid",
			expectError:              true,
			expectedErrorMessage:     "failed to access manifest path #^$/invalid: stat #^$/invalid: no such file or directory",
			expectedResourceProvider: "",
		},
		{
			name: "FilePathInsteadOfDirectory",
			clientFactory: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory()
			},
			planeName:                "local",
			filePath:                 "testdata/valid.yaml",
			expectError:              true,
			expectedErrorMessage:     "manifest path testdata/valid.yaml is not a directory path",
			expectedResourceProvider: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory, err := tt.clientFactory()
			require.NoError(t, err, "Failed to create client factory")
			logger := func(message string) {}

			err = RegisterDirectory(context.Background(), *clientFactory, tt.planeName, tt.filePath, logger)
			if tt.expectError {
				require.Error(t, err, "Expected an error but got none")
				require.Contains(t, err.Error(), tt.expectedErrorMessage, "Error message does not match")
			} else {
				require.NoError(t, err, "Did not expect an error but got one")

				// Verify the expected resource provider exists
				if tt.expectedResourceProvider != "" {
					rp, err := clientFactory.NewResourceProvidersClient().Get(context.Background(), tt.planeName, tt.expectedResourceProvider, nil)
					require.NoError(t, err, "Failed to retrieve the expected resource provider")
					require.Equal(t, to.Ptr(tt.expectedResourceProvider), rp.Name, "Resource provider name does not match")
				}
			}
		})
	}
}
