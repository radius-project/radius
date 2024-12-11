/*
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package ucpclient

import (
	"context"
	"fmt"
	"net/http"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpfake "github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
)

type FakeUCPClient struct {
	ResourceProvidersClient v20231001preview.ResourceProvidersClient
	ResourceTypesClient     v20231001preview.ResourceTypesClient
	APIVersionsClient       v20231001preview.APIVersionsClient
	LocationsClient         v20231001preview.LocationsClient
}

func NewFakeUCPClient() (*FakeUCPClient, error) {

	// Create fake servers for each client
	resourceProvidersServer := &ucpfake.ResourceProvidersServer{
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
			resp.AddNonTerminalResponse(http.StatusAccepted, nil)
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
	resourceTypesServer := &ucpfake.ResourceTypesServer{
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

			// Create a PollerResponder to simulate a long-running operation (LRO)
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

	apiVersionsServer := &ucpfake.APIVersionsServer{
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
			resp.AddNonTerminalResponse(http.StatusAccepted, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)
			return
		},
	}

	locationsServer := &ucpfake.LocationsServer{
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
			resp.AddNonTerminalResponse(http.StatusAccepted, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
	}

	// Create individual transports for each fake server
	resourceProvidersTransport := ucpfake.NewResourceProvidersServerTransport(resourceProvidersServer)
	resourceTypesTransport := ucpfake.NewResourceTypesServerTransport(resourceTypesServer)
	apiVersionsTransport := ucpfake.NewAPIVersionsServerTransport(apiVersionsServer)
	locationsTransport := ucpfake.NewLocationsServerTransport(locationsServer)

	// Configure client options with respective transports
	resourceProvidersOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: resourceProvidersTransport,
		},
	}

	resourceTypesOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: resourceTypesTransport,
		},
	}

	apiVersionsOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: apiVersionsTransport,
		},
	}

	locationsOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: locationsTransport,
		},
	}

	credential := &azfake.TokenCredential{}

	resourceProvidersClient, err := v20231001preview.NewResourceProvidersClient(credential, resourceProvidersOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create fake ResourceProvidersClient: %w", err)
	}

	resourceTypesClient, err := v20231001preview.NewResourceTypesClient(credential, resourceTypesOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create fake ResourceTypesClient: %w", err)
	}

	apiVersionsClient, err := v20231001preview.NewAPIVersionsClient(credential, apiVersionsOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create fake APIVersionsClient: %w", err)
	}

	locationsClient, err := v20231001preview.NewLocationsClient(credential, locationsOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create fake LocationsClient: %w", err)
	}

	// Return the FakeUCPClient instance
	return &FakeUCPClient{
		ResourceProvidersClient: *resourceProvidersClient,
		ResourceTypesClient:     *resourceTypesClient,
		APIVersionsClient:       *apiVersionsClient,
		LocationsClient:         *locationsClient,
	}, nil
}
