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

	"github.com/radius-project/radius/pkg/ucp"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
)

func Test_NewServer(t *testing.T) {
	t.Run("keeps the AWS clients that are set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		cc := ucp_aws.NewMockAWSCloudControlClient(ctrl)
		options := &ucp.Options{Config: &ucp.Config{}, AWSClients: ucp_aws.Clients{CloudControl: cc, CloudFormation: ucp_aws.NewMockAWSCloudFormationClient(ctrl)}}
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
