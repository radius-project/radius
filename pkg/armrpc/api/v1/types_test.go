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

package v1

import (
	"testing"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestOperationType_String(t *testing.T) {
	opTypeTests := []struct {
		in  OperationType
		out string
	}{
		{
			in:  OperationType{Type: "applications.core/environments", Method: OperationPut},
			out: "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
		},
		{
			in:  OperationType{Type: "applications.core/environments", Method: "ListSecret"},
			out: "APPLICATIONS.CORE/ENVIRONMENTS|LISTSECRET",
		},
	}

	for _, tt := range opTypeTests {
		require.Equal(t, tt.out, tt.in.String())
	}
}

func TestOperationType_ParseOperationType(t *testing.T) {
	opTypeTests := []struct {
		in     string
		out    OperationType
		parsed bool
	}{
		{
			in:     "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
			out:    OperationType{Type: "APPLICATIONS.CORE/ENVIRONMENTS", Method: OperationPut},
			parsed: true,
		},
		{
			in:     "APPLICATIONS.CORE/ENVIRONMENTS|LISTSECRET",
			out:    OperationType{Type: "APPLICATIONS.CORE/ENVIRONMENTS", Method: "LISTSECRET"},
			parsed: true,
		},
		{
			in:     "APPLICATIONS.CORE/ENVIRONMENTS",
			out:    OperationType{},
			parsed: false,
		},
	}

	for _, tt := range opTypeTests {
		actual, ok := ParseOperationType(tt.in)
		require.Equal(t, tt.out, actual)
		require.Equal(t, tt.parsed, ok)
	}
}

func TestBaseResource_UpdateMetadata(t *testing.T) {
	oldResource := BaseResource{
		TrackedResource: TrackedResource{
			ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-RG/providers/Applications.Core/environments/EnVironMent0",
			Name:     "EnVironMent0",
			Type:     "Applications.Core/environment",
			Location: "global",
		},
	}

	newResourceID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/environment0"

	newResource := BaseResource{}

	armCtx := &ARMRequestContext{Location: "global"}
	var err error
	armCtx.ResourceID, err = resources.ParseResource(newResourceID)
	require.NoError(t, err)

	t.Run("update metadata from incoming request", func(t *testing.T) {
		newResource.UpdateMetadata(armCtx, nil)
		require.Equal(t, newResourceID, newResource.ID)
		require.Equal(t, "environment0", newResource.Name)
		require.Equal(t, "Applications.Core/environments", newResource.Type)
	})

	t.Run("update metadata from oldResource", func(t *testing.T) {
		newResource.UpdateMetadata(armCtx, &oldResource)
		require.Equal(t, oldResource.ID, newResource.ID)
		require.Equal(t, oldResource.Name, newResource.Name)
		require.Equal(t, oldResource.Type, newResource.Type)
	})
}
