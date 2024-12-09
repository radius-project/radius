/*
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package registermanifests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/server"
)

// ResourceProvidersClient represents
type UCPClient struct {
	resourceProviderClient *ucpv20231001.ResourceProvidersClient
	resourceTypeClient     *ucpv20231001.ResourceTypesClient
	locationResourceClient *ucpv20231001.LocationsClient
	apiVersionClient       *ucpv20231001.APIVersionsClient
}

// CreateOrUpdateResourceProvider creates or updates a resource provider in the configured scope.
func (r *UCPClient) CreateOrUpdateResourceProvider(ctx context.Context, planeName string, resourceProviderName string, resource *ucpv20231001.ResourceProviderResource) (ucpv20231001.ResourceProviderResource, error) {
	poller, err := r.resourceProviderClient.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, *resource, &ucpv20231001.ResourceProvidersClientBeginCreateOrUpdateOptions{})
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
func (r *UCPClient) CreateOrUpdateResourceType(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, resource *ucpv20231001.ResourceTypeResource) (ucpv20231001.ResourceTypeResource, error) {

	poller, err := r.resourceTypeClient.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, resourceTypeName, *resource, &ucpv20231001.ResourceTypesClientBeginCreateOrUpdateOptions{})
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
func (r *UCPClient) CreateOrUpdateAPIVersion(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, resource *ucpv20231001.APIVersionResource) (ucpv20231001.APIVersionResource, error) {

	poller, err := r.apiVersionClient.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, resourceTypeName, apiVersionName, *resource, &ucpv20231001.APIVersionsClientBeginCreateOrUpdateOptions{})
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
func (r *UCPClient) CreateOrUpdateLocation(ctx context.Context, planeName string, resourceProviderName string, locationName string, resource *ucpv20231001.LocationResource) (ucpv20231001.LocationResource, error) {
	poller, err := r.locationResourceClient.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, locationName, *resource, &ucpv20231001.LocationsClientBeginCreateOrUpdateOptions{})
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
func (r *UCPClient) GetResourceProvider(ctx context.Context, planeName string, resourceProviderName string) (ucpv20231001.ResourceProviderResource, error) {

	response, err := r.resourceProviderClient.Get(ctx, planeName, resourceProviderName, &ucpv20231001.ResourceProvidersClientGetOptions{})
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, err
	}

	return response.ResourceProviderResource, nil
}

/*
// Assuming sdk.Connection interface requires a Connect method.
type MyConnection struct {
	Endpoint string
}

// Implement the Connect method
func (c *MyConnection) Connect(ctx context.Context) error {
	if c.Endpoint == "" {
		return errors.New("endpoint is empty")
	}
	// Implement connection logic here...
	return nil
}

// Implement the Client method
func (c *MyConnection) Client(ctx context.Context) error {

	return nil
}
*/

// Compile-time assertion to ensure MyConnection implements sdk.Connection
//var _ sdk.Connection = (*MyConnection)(nil)

func RegisterManifests(ctx context.Context, options *server.Options) error {
	//options.Config.
	connection, err := sdk.NewDirectConnection("http://localhost:9000/apis/api.ucp.dev/v1alpha3")
	if err != nil {
		return err
	}
	co := sdk.NewClientOptions(connection)

	rp, err := ucpv20231001.NewResourceProvidersClient(&aztoken.AnonymousCredential{}, co)
	if err != nil {
		return err
	}

	rt, err := ucpv20231001.NewResourceTypesClient(&aztoken.AnonymousCredential{}, co)
	if err != nil {
		return err
	}

	apiV, err := ucpv20231001.NewAPIVersionsClient(&aztoken.AnonymousCredential{}, co)
	if err != nil {
		return err
	}

	loc, err := ucpv20231001.NewLocationsClient(&aztoken.AnonymousCredential{}, co)
	if err != nil {
		return err
	}

	ucpClient := &UCPClient{
		resourceProviderClient: rp,
		resourceTypeClient:     rt,
		apiVersionClient:       apiV,
		locationResourceClient: loc,
	}

	// loop thru files in the directory
	manifestDirectory := options.Config.Manifests.ManifestDirectory
	if manifestDirectory == "" {
		return errors.New("manifest directory path is empty")
	}

	// List all files in the manifestDirectory
	files, err := os.ReadDir(manifestDirectory)
	if err != nil {
		return err
	}

	// Iterate over each file in the directory
	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue // Skip directories
		}
		filePath := filepath.Join(manifestDirectory, fileInfo.Name())
		// Read the manifest file
		resourceProvider, err := manifest.ReadFile(filePath)
		if err != nil {
			return err
		}

		_, err = ucpClient.CreateOrUpdateResourceProvider(ctx, "local", resourceProvider.Name, &v20231001preview.ResourceProviderResource{
			Location:   to.Ptr(v1.LocationGlobal),
			Properties: &v20231001preview.ResourceProviderProperties{},
		})
		if err != nil {
			return err
		}

		// The location resource contains references to all of the resource types and API versions that the resource provider supports.
		// We're instantiating the struct here so we can update it as we loop.
		locationResource := v20231001preview.LocationResource{
			Properties: &v20231001preview.LocationProperties{
				ResourceTypes: map[string]*v20231001preview.LocationResourceType{},
			},
		}

		for resourceTypeName, resourceType := range resourceProvider.Types {
			_, err := ucpClient.CreateOrUpdateResourceType(ctx, "local", resourceProvider.Name, resourceTypeName, &v20231001preview.ResourceTypeResource{
				Properties: &v20231001preview.ResourceTypeProperties{
					DefaultAPIVersion: resourceType.DefaultAPIVersion,
				},
			})
			if err != nil {
				return err
			}

			locationResourceType := &v20231001preview.LocationResourceType{
				APIVersions: map[string]map[string]any{},
			}

			for apiVersionName := range resourceType.APIVersions {
				_, err := ucpClient.CreateOrUpdateAPIVersion(ctx, "local", resourceProvider.Name, resourceTypeName, apiVersionName, &v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{},
				})
				if err != nil {
					return err
				}

				locationResourceType.APIVersions[apiVersionName] = map[string]any{}
			}

			locationResource.Properties.ResourceTypes[resourceTypeName] = locationResourceType
		}

		_, err = ucpClient.CreateOrUpdateLocation(ctx, "local", resourceProvider.Name, v1.LocationGlobal, &locationResource)
		if err != nil {
			return err
		}

		_, err = ucpClient.GetResourceProvider(ctx, "local", resourceProvider.Name)
		if err != nil {
			return err
		}
	}
	return nil
}
