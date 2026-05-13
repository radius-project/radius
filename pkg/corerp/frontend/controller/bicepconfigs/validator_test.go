// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bicepconfigs

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name        string
		auths       map[string]datamodel.BicepRegistryAuthentication
		wantReject  bool
		wantMsgPart string
	}{
		{
			name:  "no registry authentications is accepted",
			auths: nil,
		},
		{
			name: "BasicAuth with secret is accepted",
			auths: map[string]datamodel.BicepRegistryAuthentication{
				"corp.acr.io": {AuthenticationMethod: "BasicAuth", BasicAuthSecretId: "/some/secret"},
			},
		},
		{
			name: "BasicAuth without secret is rejected",
			auths: map[string]datamodel.BicepRegistryAuthentication{
				"corp.acr.io": {AuthenticationMethod: "BasicAuth"},
			},
			wantReject:  true,
			wantMsgPart: "basicAuthSecretId is required",
		},
		{
			name: "AzureWI with both ids is accepted",
			auths: map[string]datamodel.BicepRegistryAuthentication{
				"corp.acr.io": {
					AuthenticationMethod: "AzureWI",
					AzureWiClientId:      "client-id",
					AzureWiTenantId:      "tenant-id",
				},
			},
		},
		{
			name: "AzureWI missing tenant is rejected",
			auths: map[string]datamodel.BicepRegistryAuthentication{
				"corp.acr.io": {AuthenticationMethod: "AzureWI", AzureWiClientId: "client-id"},
			},
			wantReject:  true,
			wantMsgPart: "azureWiClientId and azureWiTenantId are required",
		},
		{
			name: "AwsIrsa with role arn is accepted",
			auths: map[string]datamodel.BicepRegistryAuthentication{
				"corp.acr.io": {AuthenticationMethod: "AwsIrsa", AwsIamRoleArn: "arn:aws:iam::123:role/r"},
			},
		},
		{
			name: "AwsIrsa missing role arn is rejected",
			auths: map[string]datamodel.BicepRegistryAuthentication{
				"corp.acr.io": {AuthenticationMethod: "AwsIrsa"},
			},
			wantReject:  true,
			wantMsgPart: "awsIamRoleArn is required",
		},
		{
			name: "unknown auth method is rejected",
			auths: map[string]datamodel.BicepRegistryAuthentication{
				"corp.acr.io": {AuthenticationMethod: "OAuth2"},
			},
			wantReject:  true,
			wantMsgPart: "unsupported authenticationMethod",
		},
		{
			name: "method omitted is accepted (field is optional)",
			auths: map[string]datamodel.BicepRegistryAuthentication{
				"corp.acr.io": {},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &datamodel.BicepConfig{
				Properties: datamodel.BicepConfigResourceProperties{
					RegistryAuthentications: tc.auths,
				},
			}

			resp, err := ValidateRequest(context.Background(), r, nil, nil)
			require.NoError(t, err)

			if !tc.wantReject {
				require.Nil(t, resp, "expected accept (nil rest.Response)")
				return
			}
			require.NotNil(t, resp, "expected validation failure")

			badReq, ok := resp.(*rest.BadRequestResponse)
			require.True(t, ok, "expected *rest.BadRequestResponse, got %T", resp)
			require.NotNil(t, badReq.Body.Error)
			require.Contains(t, badReq.Body.Error.Message, tc.wantMsgPart)
		})
	}
}
