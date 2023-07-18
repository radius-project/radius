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

package providers

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func TestAzureProvider_BuildConfig(t *testing.T) {
	tests := []struct {
		desc           string
		envConfig      *recipes.Configuration
		expectedConfig map[string]any
		expectedErrMsg string
	}{
		{
			desc: "valid config",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{
						Scope: "/subscriptions/test-sub/resourceGroups/test-rg",
					},
				},
			},
			expectedConfig: map[string]any{
				"subscription_id": "test-sub",
				"features":        map[string]any{},
			},
			expectedErrMsg: "",
		},
		{
			desc: "missing Azure provider - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{},
			},
			expectedConfig: map[string]any{
				"features": map[string]any{},
			},
			expectedErrMsg: "",
		},
		{
			desc: "missing Azure provider scope - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{},
				},
			},
			expectedConfig: map[string]any{
				"features": map[string]any{},
			},
			expectedErrMsg: "",
		},
		{
			desc: "invalid Azure provider scope - error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{
						Scope: "invalid",
					},
				},
			},
			expectedConfig: nil,
			expectedErrMsg: "code BadRequest: err Invalid Azure provider scope \"invalid\" is configured on the Environment, error parsing: 'invalid' is not a valid resource id",
		},
		{
			desc: "missing subscription id from scope - error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{
						Scope: "/test-sub/resourceGroups/test-rg",
					},
				},
			},
			expectedConfig: nil,
			expectedErrMsg: "code BadRequest: err Invalid Azure provider scope \"/test-sub/resourceGroups/test-rg\" is configured on the Environment, subscriptionID is required in the scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &azureProvider{}
			config := p.BuildConfig(context.Background(), tt.envConfig)
			require.Equal(t, len(tt.expectedConfig), len(config))
			require.Equal(t, tt.expectedConfig["features"], config["features"])
			require.Equal(t, tt.expectedConfig["subscription_id"], config["subscription_id"])
		})
	}
}
