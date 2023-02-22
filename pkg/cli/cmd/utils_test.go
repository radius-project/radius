// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"testing"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestCreateEnvProviders(t *testing.T) {
	providersTests := []struct {
		name      string
		providers []any
		out       corerp.Providers
		err       error
	}{
		{
			name:      "empty providers",
			providers: []any{},
			out:       corerp.Providers{},
			err:       nil,
		},
		{
			name:      "invalid provider types",
			providers: []any{azure.Provider{}},
			out:       corerp.Providers{},
			err:       &cli.FriendlyError{Message: "Internal error: cannot create environement with the invalid provider type"},
		},
		{
			name: "skip nil provider",
			providers: []any{
				nil,
				&azure.Provider{SubscriptionID: "testSubs", ResourceGroup: "testRG"},
			},
			out: corerp.Providers{
				Azure: &corerp.ProvidersAzure{
					Scope: to.Ptr("/subscriptions/testSubs/resourceGroups/testRG"),
				},
			},
			err: nil,
		},
		{
			name: "only azure provider",
			providers: []any{
				&azure.Provider{SubscriptionID: "testSubs", ResourceGroup: "testRG"},
			},
			out: corerp.Providers{
				Azure: &corerp.ProvidersAzure{
					Scope: to.Ptr("/subscriptions/testSubs/resourceGroups/testRG"),
				},
			},
			err: nil,
		},
		{
			name: "multiple azure providers",
			providers: []any{
				&azure.Provider{SubscriptionID: "testSubs", ResourceGroup: "testRG"},
				&azure.Provider{SubscriptionID: "testSub2", ResourceGroup: "testRG2"},
			},
			out: corerp.Providers{
				Azure: &corerp.ProvidersAzure{
					Scope: to.Ptr("/subscriptions/testSubs/resourceGroups/testRG"),
				},
			},
			err: &cli.FriendlyError{Message: "Only one azure provider can be configured to a scope"},
		},
		{
			name: "only aws provider",
			providers: []any{
				&aws.Provider{AccountId: "0", TargetRegion: "westus"},
			},
			out: corerp.Providers{
				Aws: &corerp.ProvidersAws{
					Scope: to.Ptr("/planes/aws/aws/accounts/0/regions/westus"),
				},
			},
			err: nil,
		},
		{
			name: "multiple aws providers",
			providers: []any{
				&aws.Provider{AccountId: "0", TargetRegion: "westus"},
				&aws.Provider{AccountId: "1", TargetRegion: "eastus"},
			},
			out: corerp.Providers{
				Aws: &corerp.ProvidersAws{
					Scope: to.Ptr("/planes/aws/aws/accounts/0/regions/westus"),
				},
			},
			err: &cli.FriendlyError{Message: "Only one aws provider can be configured to a scope"},
		},
	}

	for _, tc := range providersTests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := CreateEnvProviders(tc.providers)
			require.Equal(t, tc.out, provider)
			require.ErrorIs(t, err, tc.err)
		})
	}
}
