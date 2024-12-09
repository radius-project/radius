package ucpclient

import (
	"context"
	"net/http"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/require"

	v20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
)

func TestResourceTypesServer(t *testing.T) {
	// Initialize the fake ResourceTypesServer with method implementations.
	srv := &fake.ResourceTypesServer{
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
		BeginDelete: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resourceTypeName string,
			options *v20231001preview.ResourceTypesClientBeginDeleteOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceTypesClientDeleteResponse], errResp azfake.ErrorResponder) {
			// Create the response object
			result := v20231001preview.ResourceTypesClientDeleteResponse{}
			// Create a PollerResponder to simulate a long-running operation (LRO)
			resp.AddNonTerminalResponse(http.StatusAccepted, nil)
			resp.SetTerminalResponse(http.StatusNoContent, result, nil)

			return
		},
		NewListPager: func(
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceTypesClientListOptions,
		) (resp azfake.PagerResponder[v20231001preview.ResourceTypesClientListResponse]) {
			// Simulate paging with two pages of results.
			page1 := v20231001preview.ResourceTypesClientListResponse{
				ResourceTypeResourceListResult: v20231001preview.ResourceTypeResourceListResult{
					Value: []*v20231001preview.ResourceTypeResource{
						{Name: to.Ptr("resourceType1")},
						{Name: to.Ptr("resourceType2")},
					},
					NextLink: to.Ptr("nextPageLink"),
				},
			}
			page2 := v20231001preview.ResourceTypesClientListResponse{
				ResourceTypeResourceListResult: v20231001preview.ResourceTypeResourceListResult{
					Value: []*v20231001preview.ResourceTypeResource{
						{Name: to.Ptr("resourceType3")},
					},
				},
			}
			resp.AddPage(http.StatusOK, page1, nil)
			resp.AddPage(http.StatusOK, page2, nil)
			return
		},
	}

	// Create a fake transport using the ResourceTypesServer.
	transport := fake.NewResourceTypesServerTransport(srv)

	// Set up client options with the fake transport.
	clientOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: transport,
		},
	}

	// Create the ResourceTypesClient with the fake transport and mock credential.
	client, err := v20231001preview.NewResourceTypesClient(&azfake.TokenCredential{}, clientOptions)
	require.NoError(t, err)

	ctx := context.Background()
	planeName := "local"
	resourceProviderName := "testResourceProvider"
	resourceTypeName := "testResourceType"
	resource := v20231001preview.ResourceTypeResource{
		Name: to.Ptr(resourceTypeName),
	}

	// Call BeginCreateOrUpdate and poll until completion.
	pollerResp, err := client.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, resourceTypeName, resource, nil)
	require.NoError(t, err)
	finalResp, err := pollerResp.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, resourceTypeName, *finalResp.Name)

	// Call Get.
	getResp, err := client.Get(ctx, planeName, resourceProviderName, resourceTypeName, nil)
	require.NoError(t, err)
	require.Equal(t, resourceTypeName, *getResp.Name)

	// Call BeginDelete and poll until completion.
	deletePollerResp, err := client.BeginDelete(ctx, planeName, resourceProviderName, resourceTypeName, nil)
	require.NoError(t, err)
	_, err = deletePollerResp.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Call NewListPager.
	pager := client.NewListPager(planeName, resourceProviderName, nil)
	var resources []*v20231001preview.ResourceTypeResource
	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		resources = append(resources, page.Value...)
	}
	require.Len(t, resources, 3)
	require.Equal(t, "resourceType1", *resources[0].Name)
	require.Equal(t, "resourceType2", *resources[1].Name)
	require.Equal(t, "resourceType3", *resources[2].Name)
}
