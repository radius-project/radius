// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"testing"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/stretchr/testify/require"
)

func TestUpdateSystemData(t *testing.T) {
	testSystemData := []struct {
		name     string
		old      armrpcv1.SystemData
		new      armrpcv1.SystemData
		expected armrpcv1.SystemData
	}{
		{
			name: "new systemdata",
			old:  armrpcv1.SystemData{},
			new: armrpcv1.SystemData{
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
			expected: armrpcv1.SystemData{
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
			old: armrpcv1.SystemData{
				CreatedBy:          "test@hotmail.com",
				CreatedByType:      "User",
				CreatedAt:          "2022-03-21T18:57:52.6857175Z",
				LastModifiedBy:     "test@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-21T18:57:52.6857175Z",
			},
			new: armrpcv1.SystemData{
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
			expected: armrpcv1.SystemData{
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
			old: armrpcv1.SystemData{
				CreatedBy:          "test@hotmail.com",
				CreatedByType:      "User",
				CreatedAt:          "2022-03-21T18:57:52.6857175Z",
				LastModifiedBy:     "fake@hotmail.com",
				LastModifiedByType: "User",
				LastModifiedAt:     "2022-03-22T18:57:52.6857175Z",
			},
			new: armrpcv1.SystemData{},
			expected: armrpcv1.SystemData{
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
