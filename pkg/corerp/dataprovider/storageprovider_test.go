// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeResourceType(t *testing.T) {
	testrt := []struct {
		in  string
		out string
	}{
		{"applications.core/environments", "applicationscore-environments"},
		{"applications.core/provider", "applicationscore-provider"},
		{"applications.connector/provider", "applicationsconnector-provider"},
	}

	for _, tc := range testrt {
		t.Run(tc.in, func(t *testing.T) {
			normalized := NormalizeResourceType(tc.in)
			require.Equal(t, tc.out, normalized)
		})
	}
}
