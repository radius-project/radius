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

func TestResourceProvidersServer(t *testing.T) {
	// Initialize the fake ResourceProvidersServer with method implementations.
	srv := &fake.ResourceProvidersServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resource v20231001preview.ResourceProviderResource,
			options *v20231001preview.ResourceProvidersClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceProvidersClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20231001preview.ResourceProvidersClientCreateOrUpdateResponse{
				ResourceProviderResource: resource,
			}

			// resp.AddNonTerminalResponse(http.StatusCreated, nil)
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
		BeginDelete: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientBeginDeleteOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceProvidersClientDeleteResponse], errResp azfake.ErrorResponder) {
			// Simulate a delete operation with a final response.
			result := v20231001preview.ResourceProvidersClientDeleteResponse{}

			// resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)
			return
		},
		// Updated NewListPager method
		NewListPager: func(
			planeName string,
			options *v20231001preview.ResourceProvidersClientListOptions,
		) (resp azfake.PagerResponder[v20231001preview.ResourceProvidersClientListResponse]) {
			// Simulate paging with two pages of results.
			page1 := v20231001preview.ResourceProvidersClientListResponse{
				ResourceProviderResourceListResult: v20231001preview.ResourceProviderResourceListResult{
					Value: []*v20231001preview.ResourceProviderResource{
						{Name: to.Ptr("resourceProvider1")},
						{Name: to.Ptr("resourceProvider2")},
					},
					NextLink: to.Ptr("nextPageLink"),
				},
			}
			page2 := v20231001preview.ResourceProvidersClientListResponse{
				ResourceProviderResourceListResult: v20231001preview.ResourceProviderResourceListResult{
					Value: []*v20231001preview.ResourceProviderResource{
						{Name: to.Ptr("resourceProvider3")},
					},
				},
			}
			resp.AddPage(http.StatusOK, page1, nil)
			resp.AddPage(http.StatusOK, page2, nil)
			return
		},
		GetProviderSummary: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientGetProviderSummaryOptions,
		) (resp azfake.Responder[v20231001preview.ResourceProvidersClientGetProviderSummaryResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: v20231001preview.ResourceProviderSummary{
					Name: to.Ptr(resourceProviderName),
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}

	// Create a fake transport using the ResourceProvidersServer.
	transport := fake.NewResourceProvidersServerTransport(srv)
	clientOptions := &armpolicy.ClientOptions{ClientOptions: policy.ClientOptions{
		Transport: transport,
	}}

	// Create the ResourceProvidersClient with the fake transport.
	client, err := v20231001preview.NewResourceProvidersClient(&azfake.TokenCredential{}, clientOptions)
	require.NoError(t, err)

	ctx := context.Background()
	planeName := planeName
	resourceProviderName := "testResourceProvider"
	resource := v20231001preview.ResourceProviderResource{
		Name: to.Ptr(resourceProviderName),
	}

	// Call BeginCreateOrUpdate and poll until completion.
	pollerResp, err := client.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, resource, nil)
	require.NoError(t, err)
	_, err = pollerResp.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Call Get.
	getResp, err := client.Get(ctx, planeName, resourceProviderName, nil)
	require.NoError(t, err)
	require.Equal(t, resourceProviderName, *getResp.Name)

	// Call BeginDelete and poll until completion.
	deletePollerResp, err := client.BeginDelete(ctx, planeName, resourceProviderName, nil)
	require.NoError(t, err)
	_, err = deletePollerResp.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Call NewListPager.
	pager := client.NewListPager(planeName, nil)
	var resources []*v20231001preview.ResourceProviderResource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		resources = append(resources, page.Value...)
	}

	require.Len(t, resources, 3)
	require.Equal(t, "resourceProvider1", *resources[0].Name)
	require.Equal(t, "resourceProvider2", *resources[1].Name)
	require.Equal(t, "resourceProvider3", *resources[2].Name)

	// Call GetProviderSummary.
	summaryResp, err := client.GetProviderSummary(ctx, planeName, resourceProviderName, nil)
	require.NoError(t, err)
	require.Equal(t, resourceProviderName, *summaryResp.Name)
}
