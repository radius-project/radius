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

package processors

import (
	"testing"

	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_GetOutputResourcesFromResourcesField(t *testing.T) {
	resourcesField := []*portableresources.ResourceReference{
		{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource1"},
		{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource2"},
	}

	expected := []rpv1.OutputResource{
		{
			LocalID:       "",
			ID:            resources.MustParse(resourcesField[0].ID),
			RadiusManaged: to.Ptr(false),
		},
		{
			LocalID:       "",
			ID:            resources.MustParse(resourcesField[1].ID),
			RadiusManaged: to.Ptr(false),
		},
	}

	actual, err := GetOutputResourcesFromResourcesField(resourcesField)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_GetOutputResourceFromResourceID_Invalid(t *testing.T) {
	resourcesField := []*portableresources.ResourceReference{
		{ID: "/////asdf////"},
	}

	actual, err := GetOutputResourcesFromResourcesField(resourcesField)
	require.Error(t, err)
	require.Empty(t, actual)
	require.IsType(t, &ValidationError{}, err)
	require.Equal(t, "resource id \"/////asdf////\" is invalid", err.Error())
}
