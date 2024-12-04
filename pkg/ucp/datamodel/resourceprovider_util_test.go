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

package datamodel

import (
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_ResourceProviderIDFromResourceID(t *testing.T) {
	id := resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/foo")

	result, err := ResourceProviderIDFromResourceID(id)
	require.NoError(t, err)

	expected := resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test")
	require.Equal(t, expected, result)
}

func Test_ResourceTypeIDFromResourceID(t *testing.T) {
	id := resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/foo")

	result, err := ResourceTypeIDFromResourceID(id)
	require.NoError(t, err)

	expected := resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources")
	require.Equal(t, expected, result)
}

func Test_ResourceProviderLocationIDFromResourceID(t *testing.T) {
	id := resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/foo")

	result, err := ResourceProviderLocationIDFromResourceID(id, "east")
	require.NoError(t, err)

	expected := resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/locations/east")
	require.Equal(t, expected, result)
}
