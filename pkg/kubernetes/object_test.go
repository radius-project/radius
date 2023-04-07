// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidObjectName(t *testing.T) {
	tests := []struct {
		in    string
		valid bool
	}{
		{
			"default-corerp-resources-dapr-pubsub-generic",
			true,
		},
		{
			"default.corerp-resources-dapr-pubsub-generic",
			false,
		},
		{
			"default-corerp-resources-dapr-pubsub-generic-dapr-pubsub-generic",
			false,
		},
		{
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.valid, IsValidObjectName(tt.in))
		})
	}
}

func TestIsValidDaprObjectName(t *testing.T) {
	tests := []struct {
		in    string
		valid bool
	}{
		{
			"default.corerp.resources.dapr.pubsub.generic",
			true,
		},
		{
			"default.corerp-resources-dapr-pubsub-generic",
			true,
		},
		{
			"default_corerp_resources_dapr_pubsub_generic",
			false,
		},
		{
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.valid, IsValidDaprObjectName(tt.in))
		})
	}
}
