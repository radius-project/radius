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
		expectedErrMsg string
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
			expectedErrMsg: "",
		},
		{
			desc: "missing AWS provider config - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{},
			},
			expectedConfig: nil,
			expectedErrMsg: "",
		},
		{
			desc: "missing AWS provider scope - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{},
				},
			},
			expectedConfig: nil,
			expectedErrMsg: "",
		},
		{
			desc: "invalid AWS provider scope - error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "invalid",
					},
				},
			},
			expectedConfig: nil,
			expectedErrMsg: "code BadRequest: err Invalid AWS provider scope \"invalid\" is configured on the Environment, error parsing: 'invalid' is not a valid resource id",
		},
		{
			desc: "missing region from scope - error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "/planes/aws/aws/accounts/0000/test-region",
					},
				},
			},
			expectedConfig: nil,
			expectedErrMsg: "code BadRequest: err Invalid AWS provider scope \"/planes/aws/aws/accounts/0000/test-region\" is configured on the Environment, region is required in the scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &awsProvider{}
			config := p.BuildConfig(context.Background(), tt.envConfig)
			require.Equal(t, len(tt.expectedConfig), len(config))
			require.Equal(t, tt.expectedConfig["region"], config["region"])
		})
	}
}
