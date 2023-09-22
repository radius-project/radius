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

package v20231001preview

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func Test_GenericResource_VersionedToDataModel(t *testing.T) {
	versioned := &GenericResource{}
	dm, err := versioned.ConvertTo()
	require.Equal(t, errors.New("the GenericResource type does not support conversion from versioned models"), err)
	require.Nil(t, dm)
}

func Test_GenericResource_DataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *GenericResource
		err      error
	}{
		{
			filename: "genericresource_datamodel.json",
			expected: &GenericResource{
				ID:   to.Ptr("/planes/radius/local/resourcegroups/rg1/providers/Applications.Core/applications/test-app"),
				Type: to.Ptr("Applications.Core/applications"),
				Name: to.Ptr("test-app"),
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			data := &datamodel.GenericResource{}
			err := json.Unmarshal(rawPayload, data)
			require.NoError(t, err)

			versioned := &GenericResource{}

			err = versioned.ConvertFrom(data)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, versioned)
			}
		})
	}
}
