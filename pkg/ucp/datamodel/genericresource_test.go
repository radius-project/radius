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

func Test_GenericResourceFromID(t *testing.T) {
	id := resources.MustParse("/planes/test/local/resourceGroups/test-rg/providers/Applications.Test/testResources/my-resource")
	trackingID := resources.MustParse("/planes/test/local/resourceGroups/test-rg/providers/System.Resources/genericResources/asdf")

	actual := GenericResourceFromID(id, trackingID)
	require.Equal(t, trackingID.String(), actual.ID)
	require.Equal(t, trackingID.Type(), actual.Type)
	require.Equal(t, trackingID.Name(), actual.Name)
	require.Equal(t, id.String(), actual.Properties.ID)
	require.Equal(t, id.Type(), actual.Properties.Type)
	require.Equal(t, id.Name(), actual.Properties.Name)
}
