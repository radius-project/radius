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
		prefix string
		name   string
		out    string
	}{
		{
			"",
			"resource",
			"resource",
		},
		{
			"app",
			"resource",
			"app-resource",
		},
		{
			"app",
			"Resource",
			"app-resource",
		},
	}

	for _, tt := range nameTests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.out, MakeResourceName(tt.prefix, tt.name, Separator))
		})
	}
}
