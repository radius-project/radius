package v20231001preview_test

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

func TestLocationsServer(t *testing.T) {
	// Initialize the fake LocationsServer with method implementations.
	srv := &fake.LocationsServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			locationName string,
			resource v20231001preview.LocationResource,
			options *v20231001preview.LocationsClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.LocationsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Create the response object.
			result := v20231001preview.LocationsClientCreateOrUpdateResponse{
				LocationResource: resource,
			}

			// Create a PollerResponder to simulate a long-running operation (LRO).
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
		Get: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			locationName string,
			options *v20231001preview.LocationsClientGetOptions,
		) (resp azfake.Responder[v20231001preview.LocationsClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.LocationsClientGetResponse{
				LocationResource: v20231001preview.LocationResource{
					Name: to.Ptr(locationName),
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
		BeginDelete: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			locationName string,
			options *v20231001preview.LocationsClientBeginDeleteOptions,
		) (resp azfake.PollerResponder[v20231001preview.LocationsClientDeleteResponse], errResp azfake.ErrorResponder) {
			// Create the response object.
			result := v20231001preview.LocationsClientDeleteResponse{}

			resp.AddNonTerminalResponse(http.StatusNoContent, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)
			return
		},
		NewListPager: func(
			planeName string,
			resourceProviderName string,
			options *v20231001preview.LocationsClientListOptions,
		) (resp azfake.PagerResponder[v20231001preview.LocationsClientListResponse]) {
			// Simulate paging with two pages of results.
			page1 := v20231001preview.LocationsClientListResponse{
				LocationResourceListResult: v20231001preview.LocationResourceListResult{
					Value: []*v20231001preview.LocationResource{
						{Name: to.Ptr("location1")},
						{Name: to.Ptr("location2")},
					},
					NextLink: to.Ptr("nextPageLink"),
				},
			}
			page2 := v20231001preview.LocationsClientListResponse{
				LocationResourceListResult: v20231001preview.LocationResourceListResult{
					Value: []*v20231001preview.LocationResource{
						{Name: to.Ptr("location3")},
					},
				},
			}
			resp.AddPage(http.StatusOK, page1, nil)
			resp.AddPage(http.StatusOK, page2, nil)
			return
		},
	}

	// Create a fake transport using the LocationsServer.
	transport := fake.NewLocationsServerTransport(srv)

	// Set up client options with the fake transport.
	clientOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: transport,
		},
	}

	// Create the LocationsClient with the fake transport and mock credential.
	client, err := v20231001preview.NewLocationsClient(&azfake.TokenCredential{}, clientOptions)
	require.NoError(t, err)

	ctx := context.Background()
	planeName := "testPlane"
	resourceProviderName := "testResourceProvider"
	locationName := "testLocation"
	resource := v20231001preview.LocationResource{
		Name: to.Ptr(locationName),
	}

	// Call BeginCreateOrUpdate and poll until completion.
	pollerResp, err := client.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, locationName, resource, nil)
	require.NoError(t, err)
	finalResp, err := pollerResp.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, locationName, *finalResp.Name)

	// Call Get.
	getResp, err := client.Get(ctx, planeName, resourceProviderName, locationName, nil)
	require.NoError(t, err)
	require.Equal(t, locationName, *getResp.Name)

	// Call BeginDelete and poll until completion.
	deletePollerResp, err := client.BeginDelete(ctx, planeName, resourceProviderName, locationName, nil)
	require.NoError(t, err)
	_, err = deletePollerResp.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Call NewListPager.
	pager := client.NewListPager(planeName, resourceProviderName, nil)
	var locations []*v20231001preview.LocationResource
	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		locations = append(locations, page.Value...)
	}
	require.Len(t, locations, 3)
	require.Equal(t, "location1", *locations[0].Name)
	require.Equal(t, "location2", *locations[1].Name)
	require.Equal(t, "location3", *locations[2].Name)
}
