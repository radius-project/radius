// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
)

func newTestARMContext() context.Context {
	return v1.WithARMRequestContext(context.Background(), &v1.ARMRequestContext{})
}

func TestPrepareRadiusResource_OldResource_Nil(t *testing.T) {
	newResource := &TestResourceDataModel{}
	resp, err := PrepareRadiusResource(newTestARMContext(), newResource, nil, &controller.Options{})

	require.Nil(t, resp)
	require.NoError(t, err)
}

func TestPrepareRadiusResource_UnmatchedLinks(t *testing.T) {
	oldResource := &TestResourceDataModel{Properties: &TestResourceDataModelProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0",
			Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0",
		},
	}}
	newResource := &TestResourceDataModel{Properties: &TestResourceDataModelProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0",
			Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0",
		},
	}}

	resp, err := PrepareRadiusResource(newTestARMContext(), newResource, oldResource, &controller.Options{})
	require.NoError(t, err)
	require.Nil(t, resp)

	// Ensure that unmatched application id returns the error.
	newResource.Properties.BasicResourceProperties.Application = "invalid"
	resp, err = PrepareRadiusResource(newTestARMContext(), newResource, oldResource, &controller.Options{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestPrepareRadiusResource_DeepCopy(t *testing.T) {
	oldResource := &TestResourceDataModel{Properties: &TestResourceDataModelProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0",
			Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0",
			Status: rp.ResourceStatus{
				OutputResources: []outputresource.OutputResource{
					{
						LocalID: "testID",
					},
				},
			},
		},
	}}
	newResource := &TestResourceDataModel{Properties: &TestResourceDataModelProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0",
			Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0",
		},
	}}
	resp, err := PrepareRadiusResource(newTestARMContext(), newResource, oldResource, &controller.Options{})
	require.NoError(t, err)
	require.Nil(t, resp)
	require.Equal(t, "testID", newResource.Properties.BasicResourceProperties.Status.OutputResources[0].LocalID)
}
