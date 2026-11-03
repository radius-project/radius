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

package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
)

func Test_NewServer(t *testing.T) {
	t.Run("keeps the AWS clients that are set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cc := ucp_aws.NewMockAWSCloudControlClient(ctrl)
		options := &ucp.Options{Config: &ucp.Config{}, AWSClients: ucp_aws.Clients{CloudControl: cc, CloudFormation: ucp_aws.NewMockAWSCloudFormationClient(ctrl), Kafka: ucp_aws.NewMockAWSKafkaClient(ctrl)}}
		host, err := NewServer(t.Context(), options)
		require.NoError(t, err)
		require.NotEmpty(t, host.Services)
		require.Same(t, cc, options.AWSClients.CloudControl)
	})

	t.Run("fails when the AWS configuration cannot be loaded", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "radius-test-profile-that-does-not-exist")
		_, err := NewServer(t.Context(), &ucp.Options{Config: &ucp.Config{}})
		require.Error(t, err)
	})
}

func TestNewServerPolicyRuntime(t *testing.T) {
	catalog, err := authorization.NewCatalog(nil)
	require.NoError(t, err)
	store, err := authorization.NewPolicyStore(inmemory.NewClient(), catalog, nil, nil)
	require.NoError(t, err)
	runtime, err := authorization.NewPolicyRuntime(store, nil)
	require.NoError(t, err)

	for _, tt := range []struct {
		name    string
		enabled bool
		runtime *authorization.PolicyRuntime
		wantErr bool
	}{
		{name: "unconfigured"},
		{name: "configured", enabled: true, runtime: runtime},
		{name: "missing runtime", enabled: true, wantErr: true},
		{name: "unexpected runtime", runtime: runtime, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options := &ucp.Options{
				Config:            &ucp.Config{UserAuthorization: authorization.RuntimeOptions{Enabled: tt.enabled}},
				UserAuthorization: tt.runtime,
				AWSClients: ucp_aws.Clients{
					CloudControl:   ucp_aws.NewMockAWSCloudControlClient(gomock.NewController(t)),
					CloudFormation: ucp_aws.NewMockAWSCloudFormationClient(gomock.NewController(t)),
				},
			}
			host, err := NewServer(t.Context(), options)
			if tt.wantErr {
				require.ErrorContains(t, err, "runtime must match")
				return
			}
			require.NoError(t, err)
			count := 0
			for _, service := range host.Services {
				if service.Name() == runtime.Name() {
					require.Same(t, tt.runtime, service)
					count++
				}
			}
			if tt.enabled {
				require.Equal(t, 1, count)
			} else {
				require.Zero(t, count)
			}
		})
	}
}
