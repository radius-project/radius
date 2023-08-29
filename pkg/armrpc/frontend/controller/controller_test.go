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

package controller

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

func TestUpdateSystemData(t *testing.T) {
	testSystemData := []struct {
		name     string
		old      v1.SystemData
		new      v1.SystemData
		expected v1.SystemData
	}{
		{
			name: "new systemdata",
			old:  v1.SystemData{},
			new: v1.SystemData{
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
			expected: v1.SystemData{
				CreatedBy:          "fake@hotmail.com",
				CreatedByType:      "User",
				CreatedAt:          "2022-03-22T18:57:52.6857175Z",
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
		},
		{
			name: "update systemdata",
			old: v1.SystemData{
				CreatedBy:          "test@hotmail.com",
				CreatedByType:      "User",
				CreatedAt:          "2022-03-21T18:57:52.6857175Z",
				LastModifiedBy:     "test@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-21T18:57:52.6857175Z",
			},
			new: v1.SystemData{
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
			expected: v1.SystemData{
				CreatedBy:          "test@hotmail.com",
				CreatedByType:      "User",
				CreatedAt:          "2022-03-21T18:57:52.6857175Z",
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
		},
		{
			name: "empty new systemdata",
			old: v1.SystemData{
				CreatedBy:          "test@hotmail.com",
				CreatedByType:      "User",
				CreatedAt:          "2022-03-21T18:57:52.6857175Z",
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
			new: v1.SystemData{},
			expected: v1.SystemData{
				CreatedBy:          "test@hotmail.com",
				CreatedByType:      "User",
				CreatedAt:          "2022-03-21T18:57:52.6857175Z",
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
		},
	}
	for _, tc := range testSystemData {
		t.Run(tc.name, func(t *testing.T) {
			actual := UpdateSystemData(tc.old, tc.new)
			require.Equal(t, tc.expected, actual)
		})
	}
}
