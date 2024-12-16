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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

type UCPClientImpl struct {
	Options *policy.ClientOptions
}

func NewUCPClient(UCPConnection sdk.Connection) (*UCPClient, error) {
	if UCPConnection == nil {
		return nil, errors.New("UCP connection is nil")
	}

	clientOptions := sdk.NewClientOptions(UCPConnection)

	resourceProvidersClient, err := v20231001preview.NewResourceProvidersClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create ResourceProvidersClient: %w", err)
	}

	resourceTypesClient, err := v20231001preview.NewResourceTypesClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create ResourceTypeResource: %w", err)
	}

	apiVersionsClient, err := v20231001preview.NewAPIVersionsClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create APIVersionResource: %w", err)
	}

	locationsClient, err := v20231001preview.NewLocationsClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create LocationResource: %w", err)
	}

	return &UCPClient{
		ResourceProvidersClient: resourceProvidersClient,
		ResourceTypesClient:     resourceTypesClient,
		APIVersionsClient:       apiVersionsClient,
		LocationsClient:         locationsClient,
	}, nil
}

func (u *UCPClient) RegisterManifests(ctx context.Context, manifestDirectory string) error {

	// loop thru files in the directory
	//manifestDirectory := u.Options.Config.Manifests.ManifestDirectory
	if manifestDirectory == "" {
		return fmt.Errorf("invalid manifest directory")
	}

	// List all files in the manifestDirectory
	files, err := os.ReadDir(manifestDirectory)
	if err != nil {
		return err
	}

	// Iterate over each file in the directory
	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue // Skip directories - TBD: check if want to include subdirectories
		}
		filePath := filepath.Join(manifestDirectory, fileInfo.Name())

		// Read the manifest file
		resourceProvider, err := manifest.ReadFile(filePath)
		if err != nil {
			return err
		}

		_, err = u.CreateOrUpdateResourceProvider(ctx, planeName, resourceProvider.Name, &v20231001preview.ResourceProviderResource{
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
			_, err := u.CreateOrUpdateResourceType(ctx, planeName, resourceProvider.Name, resourceTypeName, &v20231001preview.ResourceTypeResource{
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
				_, err := u.CreateOrUpdateAPIVersion(ctx, planeName, resourceProvider.Name, resourceTypeName, apiVersionName, &v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{},
				})
				if err != nil {
					return err
				}

				locationResourceType.APIVersions[apiVersionName] = map[string]any{}
			}

			locationResource.Properties.ResourceTypes[resourceTypeName] = locationResourceType
		}

		_, err = u.CreateOrUpdateLocation(ctx, planeName, resourceProvider.Name, v1.LocationGlobal, &locationResource)
		if err != nil {
			return err
		}

		_, err = u.GetResourceProvider(ctx, planeName, resourceProvider.Name)
		if err != nil {
			return err
		}
	}
	return nil
}
