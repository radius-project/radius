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
	"testing"

	"github.com/radius-project/radius/pkg/to"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateResourceProvider_Success(t *testing.T) {
	ctx := context.Background()
	resourceProviderName := "MyCompany2.CompanyName2"
	resource := &ucpv20231001.ResourceProviderResource{
		Name: to.Ptr(resourceProviderName),
	}

	// Setup fake UCP server
	server, err := NewFakeUCPServer()
	require.NoError(t, err)

	// Setup UCP client with fake servers
	client, err := NewTestUCPClient(server)
	require.NoError(t, err)

	updatedResource, err := client.CreateOrUpdateResourceProvider(ctx, planeName, resourceProviderName, resource)
	require.NoError(t, err)
	require.Equal(t, resourceProviderName, *updatedResource.Name)
}

func TestCreateOrUpdateResourceProvider_Failure(t *testing.T) {
	ctx := context.Background()
	resourceProviderName := "MyCompany2.CompanyName2"
	resource := &ucpv20231001.ResourceProviderResource{
		Name: to.Ptr(resourceProviderName),
	}

	// Setup UCP client with fake servers
	client := UCPClient{}

	_, err := client.CreateOrUpdateResourceProvider(ctx, planeName, resourceProviderName, resource)
	require.Error(t, err)
	require.ErrorContains(t, err, "ResourceProvidersClient is nil")
}
