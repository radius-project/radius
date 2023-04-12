// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/connections"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/to"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

const (
	testAccessKeyId     = "TEST-ACCESS-KEY-ID"
	testSecretAccessKey = "TEST-SECRET-ACCESS-KEY"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name: "Valid AWS command",
			Input: []string{
				"--access-key-id", testAccessKeyId,
				"--secret-access-key", testSecretAccessKey,
			},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
		{
			Name: "AWS command with fallback workspace",
			Input: []string{
				"--access-key-id", testAccessKeyId,
				"--secret-access-key", testSecretAccessKey,
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: radcli.LoadEmptyConfig(t)},
		},
		{
			Name: "AWS command with too many positional args",
			Input: []string{
				"letsgoooooo",
				"--access-key-id", testAccessKeyId,
				"--secret-access-key", testSecretAccessKey,
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
		{
			Name: "AWS command without IAM access key id",
			Input: []string{
				"--access-key-id", "",
				"--secret-access-key", testSecretAccessKey,
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
		{
			Name: "AWS command without IAM secret access key",
			Input: []string{
				"--access-key-id", testAccessKeyId,
				"--secret-access-key", "",
			},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: configWithWorkspace},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Create aws provider", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			expectedPut := ucp.AWSCredentialResource{
				Location: to.Ptr(v1.LocationGlobal),
				Type:     to.Ptr(cli_credential.AWSCredential),
				Properties: &ucp.AWSAccessKeyCredentialProperties{
					Storage: &ucp.CredentialStorageProperties{
						Kind: to.Ptr(string(ucp.CredentialStorageKindInternal)),
					},
					AccessKeyID:     to.Ptr(testAccessKeyId),
					SecretAccessKey: to.Ptr(testSecretAccessKey),
				},
			}

			client := cli_credential.NewMockCredentialManagementClient(ctrl)
			client.EXPECT().
				PutAWS(gomock.Any(), expectedPut).
				Return(nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{CredentialManagementClient: client},
				Output:            outputSink,
				Workspace: &workspaces.Workspace{
					Connection: map[string]any{
						"kind":    workspaces.KindKubernetes,
						"context": "my-context",
					},
					Source: workspaces.SourceUserConfig,
				},
				Format:          "table",
				AccessKeyID:     testAccessKeyId,
				SecretAccessKey: testSecretAccessKey,
				KubeContext:     "my-context",
			}
			err := runner.Run(context.Background())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "Configuring credential for cloud provider %q for Radius installation %q...",
					Params: []any{"aws", "Kubernetes (context=my-context)"},
				},
				output.LogOutput{
					Format: "Successfully Configured credential for cloud provider %q",
					Params: []any{"aws"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})
}
