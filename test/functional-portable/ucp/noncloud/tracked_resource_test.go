/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ucp

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/sdk"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	corerptest "github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_TrackedResources(t *testing.T) {
	log := func(message string, obj any) {
		j, err := json.MarshalIndent(&obj, "", "  ")
		require.NoError(t, err)
		t.Logf("%s:\n\n%+v", message, string(j))
	}

	ctx := testcontext.New(t)
	options := corerptest.NewRPTestOptions(t)
	resourceGroupID := resources.MustParse("/planes/radius/local/resourcegroups/test-" + uuid.New().String())

	rgc, err := ucp.NewResourceGroupsClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(options.Connection))
	require.NoError(t, err)
	rc, err := ucp.NewResourcesClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(options.Connection))
	require.NoError(t, err)
	ac, err := corerp.NewApplicationsClient(resourceGroupID.String(), &aztoken.AnonymousCredential{}, sdk.NewClientOptions(options.Connection))
	require.NoError(t, err)
	exc, err := corerp.NewExtendersClient(resourceGroupID.String(), &aztoken.AnonymousCredential{}, sdk.NewClientOptions(options.Connection))
	require.NoError(t, err)

	rg, err := rgc.CreateOrUpdate(ctx, "radius", "local", resourceGroupID.Name(), ucp.ResourceGroupResource{Location: to.Ptr(v1.LocationGlobal)}, nil)
	require.NoError(t, err)
	log("Created resource group", rg)

	t.Run("Resource group starts empty", func(t *testing.T) {
		resources := []*ucp.GenericResource{}
		pager := rc.NewListPager("radius", "local", resourceGroupID.Name(), nil)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			require.NoError(t, err)
			log("Got resource page", page)
			resources = append(resources, page.Value...)
		}
		require.Empty(t, resources)
	})

	t.Run("Create resources", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			a, err := ac.CreateOrUpdate(ctx, fmt.Sprintf("app-%d", i), corerp.ApplicationResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &corerp.ApplicationProperties{
					Environment: to.Ptr(options.Workspace.Environment),
				},
			}, nil)
			require.NoError(t, err)
			log("Got application", a)

			// We're using extender here because its operations are asynchronous.
			poller, err := exc.BeginCreateOrUpdate(ctx, fmt.Sprintf("ex-%d", i), corerp.ExtenderResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &corerp.ExtenderProperties{
					Environment:          to.Ptr(options.Workspace.Environment),
					Application:          to.Ptr(*a.ID),
					ResourceProvisioning: to.Ptr(corerp.ResourceProvisioningManual),
				},
			}, nil)
			require.NoError(t, err)

			ex, err := poller.PollUntilDone(ctx, nil)
			require.NoError(t, err)
			log("Got extender", ex)
		}
	})

	t.Run("Resource group contains resources", func(t *testing.T) {
		expected := []*ucp.GenericResource{}

		for i := 0; i < 3; i++ {
			expected = append(expected, &ucp.GenericResource{
				ID:   to.Ptr(resourceGroupID.Append(resources.TypeSegment{Type: "Applications.Core/applications", Name: fmt.Sprintf("app-%d", i)}).String()),
				Name: to.Ptr(fmt.Sprintf("app-%d", i)),
				Type: to.Ptr("Applications.Core/applications"),
			})
			expected = append(expected, &ucp.GenericResource{
				ID:   to.Ptr(resourceGroupID.Append(resources.TypeSegment{Type: "Applications.Core/extenders", Name: fmt.Sprintf("ex-%d", i)}).String()),
				Name: to.Ptr(fmt.Sprintf("ex-%d", i)),
				Type: to.Ptr("Applications.Core/extenders"),
			})
		}

		sort.Slice(expected, func(i, j int) bool {
			return *expected[i].ID < *expected[j].ID
		})

		require.EventuallyWithT(t, func(t *assert.CollectT) {
			resources := []*ucp.GenericResource{}
			pager := rc.NewListPager("radius", "local", resourceGroupID.Name(), nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				require.NoError(t, err)
				log("Got resource page", page)
				resources = append(resources, page.Value...)
			}

			sort.Slice(resources, func(i, j int) bool {
				return *resources[i].ID < *resources[j].ID
			})
			assert.Equal(t, expected, resources)
		}, time.Second*30, time.Millisecond*500)
	})

	t.Run("Delete resources", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			// Delete in reverse order to make sure the extender is deleted before the application it
			// belongs to.
			poller, err := exc.BeginDelete(ctx, fmt.Sprintf("ex-%d", i), nil)
			require.NoError(t, err)

			_, err = poller.PollUntilDone(ctx, nil)
			require.NoError(t, err)

			_, err = ac.Delete(ctx, fmt.Sprintf("app-%d", i), nil)
			require.NoError(t, err)
		}
	})

	t.Run("Resource group is empty again", func(t *testing.T) {
		require.EventuallyWithT(t, func(t *assert.CollectT) {
			resources := []*ucp.GenericResource{}
			pager := rc.NewListPager("radius", "local", resourceGroupID.Name(), nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				require.NoError(t, err)
				log("Got resource page", page)
				resources = append(resources, page.Value...)
			}
			assert.Empty(t, resources)
		}, time.Second*30, time.Millisecond*500)
	})

	t.Run("Delete resource group", func(t *testing.T) {
		_, err := rgc.Delete(ctx, "radius", "local", resourceGroupID.Name(), nil)
		require.NoError(t, err)
	})
}
