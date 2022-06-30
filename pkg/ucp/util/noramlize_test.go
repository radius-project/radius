// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeString(t *testing.T) {
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
			normalized := NormalizeStringToLower(tc.in)
			require.Equal(t, tc.out, normalized)
		})
	}
}
