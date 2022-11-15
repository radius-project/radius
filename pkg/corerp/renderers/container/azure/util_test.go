// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMakeResourceName(t *testing.T) {
	nameTests := []struct {
		name   string
		prefix []string
		out    string
	}{
		{
			"resource",
			nil,
			"resource",
		},
		{
			"resource",
			[]string{"app"},
			"app-resource",
		},
		{
			"Resource",
			[]string{"App"},
			"app-resource",
		},
		{
			"resource",
			[]string{"env", "app"},
			"env-app-resource",
		},
	}

	for _, tt := range nameTests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.out, MakeResourceName(tt.name, tt.prefix...))
		})
	}
}
