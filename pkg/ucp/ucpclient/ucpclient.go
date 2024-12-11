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

	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

/*
// UCPClient is a client for interacting with the UCP API.
type UCPClient interface {
	// CreateOrUpdateResourceProvider creates or updates a resource provider in the configured scope.
	CreateOrUpdateResourceProvider(ctx context.Context, planeName string, providerNamespace string, resource *ucpv20231001.ResourceProviderResource) (ucpv20231001.ResourceProviderResource, error)

	// CreateOrUpdateResourceType creates or updates a resource type in the configured scope.
	GetResourceProvider(ctx context.Context, planeName string, resourceProviderName string) (ucpv20231001.ResourceProviderResource, error)

	// CreateOrUpdateResourceType creates or updates a resource type in the configured scope.
	CreateOrUpdateResourceType(ctx context.Context, planeName string, providerNamespace string, resourceTypeName string, resource *ucpv20231001.ResourceTypeResource) (ucpv20231001.ResourceTypeResource, error)

	// CreateOrUpdateAPIVersion creates or updates an API version in the configured scope.
	CreateOrUpdateAPIVersion(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, resource *ucpv20231001.APIVersionResource) (ucpv20231001.APIVersionResource, error)

	// CreateOrUpdateLocation creates or updates a resource provider location in the configured scope.
	CreateOrUpdateLocation(ctx context.Context, planeName string, resourceProviderName string, locationName string, resource *ucpv20231001.LocationResource) (ucpv20231001.LocationResource, error)

	RegisterManifests(ctx context.Context) error

	// ... Add other methods as needed ...
}
*/

const planeName = "local"

// UCPClient holds instances of each specific client.
type UCPClient struct {
	ResourceProvidersClient *ucpv20231001.ResourceProvidersClient
	ResourceTypesClient     *ucpv20231001.ResourceTypesClient
	APIVersionsClient       *ucpv20231001.APIVersionsClient
	LocationsClient         *ucpv20231001.LocationsClient
	// Add other clients as needed
}

//var _ UCPClient = (*UCPClientFactory)(nil)

// CreateOrUpdateResourceProvider creates or updates a resource provider in the configured scope.
func (u *UCPClient) CreateOrUpdateResourceProvider(ctx context.Context, planeName string, resourceProviderName string, resource *ucpv20231001.ResourceProviderResource) (ucpv20231001.ResourceProviderResource, error) {
	if u.ResourceProvidersClient == nil {
		return ucpv20231001.ResourceProviderResource{}, fmt.Errorf("ResourceProvidersClient is nil")
	}

	poller, err := u.ResourceProvidersClient.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, *resource, &ucpv20231001.ResourceProvidersClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, fmt.Errorf("begin create or update failed: %w", err)
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, fmt.Errorf("poll until done failed: %w", err)
	}

	return response.ResourceProviderResource, nil
}

// CreateOrUpdateResourceType creates or updates a resource type in the configured scope.
func (u *UCPClient) CreateOrUpdateResourceType(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, resource *ucpv20231001.ResourceTypeResource) (ucpv20231001.ResourceTypeResource, error) {
	if u.ResourceTypesClient == nil {
		return ucpv20231001.ResourceTypeResource{}, fmt.Errorf("ResourceTypeResource is nil")
	}

	poller, err := u.ResourceTypesClient.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, resourceTypeName, *resource, &ucpv20231001.ResourceTypesClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return ucpv20231001.ResourceTypeResource{}, err
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return ucpv20231001.ResourceTypeResource{}, err
	}

	return response.ResourceTypeResource, nil
}

// CreateOrUpdateAPIVersion creates or updates an API version in the configured scope.
func (u *UCPClient) CreateOrUpdateAPIVersion(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, resource *ucpv20231001.APIVersionResource) (ucpv20231001.APIVersionResource, error) {
	if u.APIVersionsClient == nil {
		return ucpv20231001.APIVersionResource{}, fmt.Errorf("APIVersionResource is nil")
	}

	poller, err := u.APIVersionsClient.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, resourceTypeName, apiVersionName, *resource, &ucpv20231001.APIVersionsClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return ucpv20231001.APIVersionResource{}, err
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return ucpv20231001.APIVersionResource{}, err
	}

	return response.APIVersionResource, nil
}

// CreateOrUpdateLocation creates or updates a resource provider location in the configured scope.
func (u *UCPClient) CreateOrUpdateLocation(ctx context.Context, planeName string, resourceProviderName string, locationName string, resource *ucpv20231001.LocationResource) (ucpv20231001.LocationResource, error) {
	if u.LocationsClient == nil {
		return ucpv20231001.LocationResource{}, fmt.Errorf("LocationResource is nil")
	}

	poller, err := u.LocationsClient.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, locationName, *resource, &ucpv20231001.LocationsClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return ucpv20231001.LocationResource{}, err
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return ucpv20231001.LocationResource{}, err
	}

	return response.LocationResource, nil
}

// GetResourceProvider gets the resource provider with the specified name in the configured scope.
func (u *UCPClient) GetResourceProvider(ctx context.Context, planeName string, resourceProviderName string) (ucpv20231001.ResourceProviderResource, error) {
	if u.ResourceProvidersClient == nil {
		return ucpv20231001.ResourceProviderResource{}, fmt.Errorf("ResourceProvidersClient is nil")
	}

	response, err := u.ResourceProvidersClient.Get(ctx, planeName, resourceProviderName, &ucpv20231001.ResourceProvidersClientGetOptions{})
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, err
	}

	return response.ResourceProviderResource, nil
}
