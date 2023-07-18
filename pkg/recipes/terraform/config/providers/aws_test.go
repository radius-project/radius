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

func TestAWSProvider_BuildConfig(t *testing.T) {
	tests := []struct {
		desc           string
		envConfig      *recipes.Configuration
		expectedConfig map[string]any
	}{
		{
			desc: "valid config",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "/planes/aws/aws/accounts/0000/regions/test-region",
					},
				},
			},
			expectedConfig: map[string]any{
				"region": "test-region",
			},
		},
		{
			desc: "missing AWS provider config",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{},
			},
			expectedConfig: nil,
		},
		{
			desc: "missing AWS provider scope",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{},
				},
			},
			expectedConfig: nil,
		},
		{
			desc: "invalid AWS provider scope",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "invalid",
					},
				},
			},
			expectedConfig: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &awsProvider{}
			config, err := p.BuildConfig(context.Background(), tt.envConfig)
			require.NoError(t, err)
			require.Equal(t, len(tt.expectedConfig), len(config))
			require.Equal(t, tt.expectedConfig["region"], config["region"])
		})
	}
}
