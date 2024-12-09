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
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/server"
)

type UCPClientImpl struct {
	ManifestDirectory string
	Options           *policy.ClientOptions
}

func NewUCPClient(options *server.Options) (UCPClient, error) {
	if options.UCPConnection == nil {
		return nil, errors.New("UCP connection is nil")
	}

	clientOptions := sdk.NewClientOptions(options.UCPConnection)

	return &UCPClientImpl{
		ManifestDirectory: options.Config.Manifests.ManifestDirectory,
		Options:           clientOptions,
	}, nil
}

var _ UCPClient = (*UCPClientImpl)(nil)

func (u *UCPClientImpl) RegisterManifests(ctx context.Context) error {

	// loop thru files in the directory
	//manifestDirectory := u.Options.Config.Manifests.ManifestDirectory
	if u.ManifestDirectory == "" {
		return errors.New("manifest directory path is empty")
	}

	// List all files in the manifestDirectory
	files, err := os.ReadDir(u.ManifestDirectory)
	if err != nil {
		return err
	}

	// Iterate over each file in the directory
	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue // Skip directories
		}
		filePath := filepath.Join(u.ManifestDirectory, fileInfo.Name())
		// Read the manifest file
		resourceProvider, err := manifest.ReadFile(filePath)
		if err != nil {
			return err
		}

		_, err = u.CreateOrUpdateResourceProvider(ctx, "local", resourceProvider.Name, &v20231001preview.ResourceProviderResource{
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
			_, err := u.CreateOrUpdateResourceType(ctx, "local", resourceProvider.Name, resourceTypeName, &v20231001preview.ResourceTypeResource{
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
				_, err := u.CreateOrUpdateAPIVersion(ctx, "local", resourceProvider.Name, resourceTypeName, apiVersionName, &v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{},
				})
				if err != nil {
					return err
				}

				locationResourceType.APIVersions[apiVersionName] = map[string]any{}
			}

			locationResource.Properties.ResourceTypes[resourceTypeName] = locationResourceType
		}

		_, err = u.CreateOrUpdateLocation(ctx, "local", resourceProvider.Name, v1.LocationGlobal, &locationResource)
		if err != nil {
			return err
		}

		_, err = u.GetResourceProvider(ctx, "local", resourceProvider.Name)
		if err != nil {
			return err
		}
	}
	return nil
}
