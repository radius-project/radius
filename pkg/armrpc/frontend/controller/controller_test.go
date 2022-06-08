// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
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
