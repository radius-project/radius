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

package cmd

import (
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestCreateEnvProviders(t *testing.T) {
	var nilAzureProvider *azure.Provider = nil
	var nilAWSProvider *aws.Provider = nil

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
			err:       errors.New("internal error: cannot create environment with invalid type 'azure.Provider'"),
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
			out: corerp.Providers{},
			err: clierrors.Message("Only one azure provider can be configured to a scope."),
		},
		{
			name: "only aws provider",
			providers: []any{
				&aws.Provider{AccountID: "0", Region: "westus"},
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
				&aws.Provider{AccountID: "0", Region: "westus"},
				&aws.Provider{AccountID: "1", Region: "eastus"},
			},
			out: corerp.Providers{},
			err: clierrors.Message("Only one aws provider can be configured to a scope."),
		},
		{
			name: "azure and aws providers",
			providers: []any{
				&azure.Provider{SubscriptionID: "testSubs", ResourceGroup: "testRG"},
				&aws.Provider{AccountID: "0", Region: "westus"},
			},
			out: corerp.Providers{
				Azure: &corerp.ProvidersAzure{
					Scope: to.Ptr("/subscriptions/testSubs/resourceGroups/testRG"),
				},
				Aws: &corerp.ProvidersAws{
					Scope: to.Ptr("/planes/aws/aws/accounts/0/regions/westus"),
				},
			},
			err: nil,
		},
		{
			name: "skip typed nil value",
			providers: []any{
				nilAzureProvider,
				nilAWSProvider,
			},
			out: corerp.Providers{},
			err: nil,
		},
	}

	for _, tc := range providersTests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := CreateEnvProviders(tc.providers)
			require.Equal(t, tc.out, provider)
			require.Equal(t, tc.err, err)
		})
	}
}
