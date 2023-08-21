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

package frontend

import (
	"context"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0",
			Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0",
		},
	}}
	newResource := &TestResourceDataModel{Properties: &TestResourceDataModelProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0",
			Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0",
			Status: rpv1.ResourceStatus{
				OutputResources: []rpv1.OutputResource{
					{
						LocalID: "testID",
					},
				},
			},
		},
	}}
	newResource := &TestResourceDataModel{Properties: &TestResourceDataModelProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0",
			Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0",
		},
	}}
	resp, err := PrepareRadiusResource(newTestARMContext(), newResource, oldResource, &controller.Options{})
	require.NoError(t, err)
	require.Nil(t, resp)
	require.Equal(t, "testID", newResource.Properties.BasicResourceProperties.Status.OutputResources[0].LocalID)
}
