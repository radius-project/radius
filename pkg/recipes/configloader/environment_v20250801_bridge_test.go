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

package configloader

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	v20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	tfConfigName = "tfcfg"
	bcConfigName = "bccfg"
	tfConfigID   = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/terraformSettings/" + tfConfigName
	bcConfigID   = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/bicepSettings/" + bcConfigName
)

// fakeArmOptions builds an arm.ClientOptions whose Transport routes all
// terraformSettings / bicepSettings requests to the supplied fake servers.
func fakeArmOptions(tfSrv fake.TerraformSettingsServer, bcSrv fake.BicepSettingsServer) *arm.ClientOptions {
	return &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
				TerraformSettingsServer: tfSrv,
				BicepSettingsServer:     bcSrv,
			}),
		},
	}
}

// minimalEnv builds an environment resource with the fields getConfigurationV20250801
// requires (Kubernetes namespace) plus the supplied terraformSettings / bicepSettings refs.
func minimalEnv(tfRef, bcRef string) *v20250801.EnvironmentResource {
	return &v20250801.EnvironmentResource{
		Properties: &v20250801.EnvironmentProperties{
			Providers: &v20250801.Providers{
				Kubernetes: &v20250801.ProvidersKubernetes{
					Namespace: to.Ptr(envNamespace),
				},
			},
			Simulated:         to.Ptr(false),
			TerraformSettings: to.Ptr(tfRef),
			BicepSettings:     to.Ptr(bcRef),
		},
	}
}

func TestGetConfigurationV20250801_TerraformCredentialsAndEnvAndProviderInstallation(t *testing.T) {
	tfSrv := fake.TerraformSettingsServer{
		Get: func(ctx context.Context, rootScope string, name string, opts *v20250801.TerraformSettingsClientGetOptions) (resp azfake.Responder[v20250801.TerraformSettingsClientGetResponse], errResp azfake.ErrorResponder) {
			require.Equal(t, tfConfigName, name)
			resp.SetResponse(http.StatusOK, v20250801.TerraformSettingsClientGetResponse{
				TerraformSettingsResource: v20250801.TerraformSettingsResource{
					ID:       to.Ptr(tfConfigID),
					Name:     to.Ptr(tfConfigName),
					Type:     to.Ptr("Radius.Core/terraformSettings"),
					Location: to.Ptr("global"),
					Properties: &v20250801.TerraformSettingsProperties{
						Terraformrc: &v20250801.TerraformrcConfig{
							ProviderInstallation: &v20250801.TerraformProviderInstallation{
								NetworkMirror: &v20250801.TerraformProviderMirror{
									URL:     to.Ptr("https://mirror.example.com/"),
									Include: to.SliceOfPtrs("hashicorp/aws"),
								},
							},
							Credentials: map[string]*v20250801.TerraformCredentialConfig{
								"app.terraform.io":     {Secret: to.Ptr("/planes/.../secretA")},
								"registry.example.com": {Secret: to.Ptr("/planes/.../secretB")},
							},
						},
						Env: map[string]*string{
							"TF_LOG":      to.Ptr("DEBUG"),
							"TF_LOG_PATH": to.Ptr("/tmp/tf.log"),
						},
					},
				},
			}, nil)
			return
		},
	}

	armOpts := fakeArmOptions(tfSrv, fake.BicepSettingsServer{})

	env := minimalEnv(tfConfigID, "")
	// Clear the bicepSettings pointer since we don't want to fetch it in this test.
	env.Properties.BicepSettings = nil

	cfg, err := getConfigurationV20250801(context.Background(), env, armOpts)
	require.NoError(t, err)

	// Credentials map is bridged 1:1.
	require.Len(t, cfg.RecipeConfig.Terraform.Credentials, 2)
	require.Equal(t, "/planes/.../secretA", cfg.RecipeConfig.Terraform.Credentials["app.terraform.io"].Secret)
	require.Equal(t, "/planes/.../secretB", cfg.RecipeConfig.Terraform.Credentials["registry.example.com"].Secret)

	// Env map is bridged into AdditionalProperties.
	require.Len(t, cfg.RecipeConfig.Env.AdditionalProperties, 2)
	require.Equal(t, "DEBUG", cfg.RecipeConfig.Env.AdditionalProperties["TF_LOG"])
	require.Equal(t, "/tmp/tf.log", cfg.RecipeConfig.Env.AdditionalProperties["TF_LOG_PATH"])

	// ProviderInstallation forwarded by reference.
	require.NotNil(t, cfg.RecipeConfig.Terraform.ProviderInstallation)
	require.NotNil(t, cfg.RecipeConfig.Terraform.ProviderInstallation.NetworkMirror)
	require.Equal(t, "https://mirror.example.com/", cfg.RecipeConfig.Terraform.ProviderInstallation.NetworkMirror.URL)
	require.Equal(t, []string{"hashicorp/aws"}, cfg.RecipeConfig.Terraform.ProviderInstallation.NetworkMirror.Include)
}

func TestGetConfigurationV20250801_BicepBasicAuthMapped(t *testing.T) {
	bcSrv := fake.BicepSettingsServer{
		Get: func(ctx context.Context, rootScope string, name string, opts *v20250801.BicepSettingsClientGetOptions) (resp azfake.Responder[v20250801.BicepSettingsClientGetResponse], errResp azfake.ErrorResponder) {
			require.Equal(t, bcConfigName, name)
			method := v20250801.BicepAuthenticationMethodBasicAuth
			resp.SetResponse(http.StatusOK, v20250801.BicepSettingsClientGetResponse{
				BicepSettingsResource: v20250801.BicepSettingsResource{
					ID:       to.Ptr(bcConfigID),
					Name:     to.Ptr(bcConfigName),
					Type:     to.Ptr("Radius.Core/bicepSettings"),
					Location: to.Ptr("global"),
					Properties: &v20250801.BicepSettingsProperties{
						RegistryAuthentications: map[string]*v20250801.BicepRegistryAuthentication{
							"corp.acr.io": {
								AuthenticationMethod: &method,
								BasicAuthSecretID:    to.Ptr("/planes/.../basic-secret"),
							},
						},
					},
				},
			}, nil)
			return
		},
	}

	armOpts := fakeArmOptions(fake.TerraformSettingsServer{}, bcSrv)

	env := minimalEnv("", bcConfigID)
	env.Properties.TerraformSettings = nil

	cfg, err := getConfigurationV20250801(context.Background(), env, armOpts)
	require.NoError(t, err)

	require.Len(t, cfg.RecipeConfig.Bicep.Authentication, 1)
	require.Equal(t, "/planes/.../basic-secret", cfg.RecipeConfig.Bicep.Authentication["corp.acr.io"].Secret)
}

func TestGetConfigurationV20250801_BicepEntriesWithoutBasicAuthSecretAreSkipped(t *testing.T) {
	// Documents the silent-skip behavior: AzureWI / AwsIrsa entries are accepted
	// by the schema but not yet wired into the driver, so they never reach
	// RecipeConfig.Bicep.Authentication. Only entries with a non-empty
	// BasicAuthSecretId survive the bridge.
	bcSrv := fake.BicepSettingsServer{
		Get: func(ctx context.Context, rootScope string, name string, opts *v20250801.BicepSettingsClientGetOptions) (resp azfake.Responder[v20250801.BicepSettingsClientGetResponse], errResp azfake.ErrorResponder) {
			basic := v20250801.BicepAuthenticationMethodBasicAuth
			azure := v20250801.BicepAuthenticationMethodAzureWI
			aws := v20250801.BicepAuthenticationMethodAwsIrsa
			resp.SetResponse(http.StatusOK, v20250801.BicepSettingsClientGetResponse{
				BicepSettingsResource: v20250801.BicepSettingsResource{
					ID:       to.Ptr(bcConfigID),
					Name:     to.Ptr(bcConfigName),
					Type:     to.Ptr("Radius.Core/bicepSettings"),
					Location: to.Ptr("global"),
					Properties: &v20250801.BicepSettingsProperties{
						RegistryAuthentications: map[string]*v20250801.BicepRegistryAuthentication{
							"basic.acr.io": {
								AuthenticationMethod: &basic,
								BasicAuthSecretID:    to.Ptr("/planes/.../basic-secret"),
							},
							"azure.acr.io": {
								AuthenticationMethod: &azure,
								AzureWiClientID:      to.Ptr("client-id"),
								AzureWiTenantID:      to.Ptr("tenant-id"),
							},
							"aws.ecr.io": {
								AuthenticationMethod: &aws,
								AwsIamRoleArn:        to.Ptr("arn:aws:iam::123:role/MyRole"),
							},
						},
					},
				},
			}, nil)
			return
		},
	}

	armOpts := fakeArmOptions(fake.TerraformSettingsServer{}, bcSrv)

	env := minimalEnv("", bcConfigID)
	env.Properties.TerraformSettings = nil

	cfg, err := getConfigurationV20250801(context.Background(), env, armOpts)
	require.NoError(t, err)

	// Only the BasicAuth entry survives.
	require.Len(t, cfg.RecipeConfig.Bicep.Authentication, 1)
	require.Equal(t, "/planes/.../basic-secret", cfg.RecipeConfig.Bicep.Authentication["basic.acr.io"].Secret)
	_, hasAzure := cfg.RecipeConfig.Bicep.Authentication["azure.acr.io"]
	require.False(t, hasAzure, "AzureWI entry should be silently skipped")
	_, hasAws := cfg.RecipeConfig.Bicep.Authentication["aws.ecr.io"]
	require.False(t, hasAws, "AwsIrsa entry should be silently skipped")
}

func TestGetConfigurationV20250801_BicepAllEntriesSkipped_LeavesAuthNil(t *testing.T) {
	// When every entry lacks BasicAuthSecretId the bridge must not synthesize an
	// empty Authentication map (the legacy code only sets Bicep when authMap > 0).
	bcSrv := fake.BicepSettingsServer{
		Get: func(ctx context.Context, rootScope string, name string, opts *v20250801.BicepSettingsClientGetOptions) (resp azfake.Responder[v20250801.BicepSettingsClientGetResponse], errResp azfake.ErrorResponder) {
			azure := v20250801.BicepAuthenticationMethodAzureWI
			resp.SetResponse(http.StatusOK, v20250801.BicepSettingsClientGetResponse{
				BicepSettingsResource: v20250801.BicepSettingsResource{
					ID:       to.Ptr(bcConfigID),
					Name:     to.Ptr(bcConfigName),
					Type:     to.Ptr("Radius.Core/bicepSettings"),
					Location: to.Ptr("global"),
					Properties: &v20250801.BicepSettingsProperties{
						RegistryAuthentications: map[string]*v20250801.BicepRegistryAuthentication{
							"azure.acr.io": {
								AuthenticationMethod: &azure,
								AzureWiClientID:      to.Ptr("client-id"),
								AzureWiTenantID:      to.Ptr("tenant-id"),
							},
						},
					},
				},
			}, nil)
			return
		},
	}

	armOpts := fakeArmOptions(fake.TerraformSettingsServer{}, bcSrv)

	env := minimalEnv("", bcConfigID)
	env.Properties.TerraformSettings = nil

	cfg, err := getConfigurationV20250801(context.Background(), env, armOpts)
	require.NoError(t, err)

	require.Empty(t, cfg.RecipeConfig.Bicep.Authentication, "expected no auth map when no usable entries")
}

func TestGetConfigurationV20250801_TerraformFetchError_IsWrapped(t *testing.T) {
	tfSrv := fake.TerraformSettingsServer{
		Get: func(ctx context.Context, rootScope string, name string, opts *v20250801.TerraformSettingsClientGetOptions) (resp azfake.Responder[v20250801.TerraformSettingsClientGetResponse], errResp azfake.ErrorResponder) {
			errResp.SetError(errors.New("network unreachable"))
			return
		},
	}

	armOpts := fakeArmOptions(tfSrv, fake.BicepSettingsServer{})

	env := minimalEnv(tfConfigID, "")
	env.Properties.BicepSettings = nil

	_, err := getConfigurationV20250801(context.Background(), env, armOpts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to fetch terraformSettings")
	require.Contains(t, err.Error(), tfConfigID)
}

func TestGetConfigurationV20250801_BicepFetchError_IsWrapped(t *testing.T) {
	bcSrv := fake.BicepSettingsServer{
		Get: func(ctx context.Context, rootScope string, name string, opts *v20250801.BicepSettingsClientGetOptions) (resp azfake.Responder[v20250801.BicepSettingsClientGetResponse], errResp azfake.ErrorResponder) {
			errResp.SetError(fmt.Errorf("bicepSettings not reachable"))
			return
		},
	}

	armOpts := fakeArmOptions(fake.TerraformSettingsServer{}, bcSrv)

	env := minimalEnv("", bcConfigID)
	env.Properties.TerraformSettings = nil

	_, err := getConfigurationV20250801(context.Background(), env, armOpts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to fetch bicepSettings")
	require.Contains(t, err.Error(), bcConfigID)
}
