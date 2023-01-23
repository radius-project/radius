// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/golang/mock/gomock"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

const (
	azureProviderName   = "azure"
	awsProviderName     = "aws"
	testClientSecret    = "testAzureSecret"
	validClientID       = "3b4e017f-f31b-4b0b-93d0-ee06f85b33ee"
	invalidClientID     = "invalid_test_client_id"
	validTenantID       = "72f988bf-86f1-41af-91ab-2d7cd011db47"
	invalidTenantID     = "invalid_tenant_id"
	testAccessKeyId     = "test_access_key_id"
	testSecretAccessKey = "test_secret_access_key"
)

var (
	errInvalidClientID      = fmt.Errorf(fmt.Sprintf(ValidInfoTemplate, "azure client id"))
	errInvalidTenantID      = fmt.Errorf(ValidInfoTemplate, "azure tenant id")
	errEmptyClientSecret    = fmt.Errorf(infoRequiredTemplate, "azure client secret")
	errEmptyAccessKeyId     = fmt.Errorf(infoRequiredTemplate, "aws access key id")
	errEmptySecretAccessKey = fmt.Errorf(infoRequiredTemplate, "aws secret access key")
	errCredentialNotFound   = error(&azcore.ResponseError{ErrorCode: "NotFound"})
	errInternalServer       = errors.New("internal server error")
)

func Test_Credential_Put(t *testing.T) {
	azureCredentialResource := cli_credential.ProviderCredentialResource{
		Name: "azure",
	}

	awsCredentialResource := cli_credential.ProviderCredentialResource{
		Name: "aws",
	}

	invalidCredentialResource := cli_credential.ProviderCredentialResource{
		Name: "invalid",
	}

	tests := []struct {
		name           string
		planeType      string
		planeName      string
		providerConfig cli_credential.ProviderCredentialConfiguration
		err            error
		setupMocks     func(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string)
	}{
		{
			name:      "create azure credential success",
			planeType: cli_credential.AzurePlaneType,
			planeName: AzurePlaneName,
			providerConfig: cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: azureCredentialResource,
				AzureCredentials: &cli_credential.ServicePrincipalCredentials{
					ClientID:     validClientID,
					ClientSecret: testClientSecret,
					TenantID:     validTenantID,
				},
			},
			err:        nil,
			setupMocks: setupSuccessPutMocks,
		},
		{
			name: "create azure credential invalid clientId",
			providerConfig: cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: azureCredentialResource,
				AzureCredentials: &cli_credential.ServicePrincipalCredentials{
					ClientID:     invalidClientID,
					ClientSecret: testClientSecret,
					TenantID:     validTenantID,
				},
			},
			err:        errInvalidClientID,
			setupMocks: nil,
		},
		{
			name: "create azure credential invalid tenantId",
			providerConfig: cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: azureCredentialResource,
				AzureCredentials: &cli_credential.ServicePrincipalCredentials{
					ClientID:     validClientID,
					ClientSecret: testClientSecret,
					TenantID:     invalidTenantID,
				},
			},
			err:        errInvalidTenantID,
			setupMocks: nil,
		},
		{
			name: "create azure credential empty secret",
			providerConfig: cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: azureCredentialResource,
				AzureCredentials: &cli_credential.ServicePrincipalCredentials{
					ClientID:     validClientID,
					ClientSecret: "",
					TenantID:     validTenantID,
				},
			},
			err:        errEmptyClientSecret,
			setupMocks: nil,
		},
		{
			name:      "create aws credential success",
			planeType: cli_credential.AWSPlaneType,
			planeName: AWSPlaneName,
			providerConfig: cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: awsCredentialResource,
				AWSCredentials: &cli_credential.IAMCredentials{
					AccessKeyID:     testAccessKeyId,
					SecretAccessKey: testSecretAccessKey,
				},
			},
			err:        nil,
			setupMocks: setupSuccessPutMocks,
		},
		{
			name: "create aws credential with empty accessKeyID",
			providerConfig: cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: awsCredentialResource,
				AWSCredentials: &cli_credential.IAMCredentials{
					AccessKeyID:     "",
					SecretAccessKey: testSecretAccessKey,
				},
			},
			err:        errEmptyAccessKeyId,
			setupMocks: nil,
		},
		{
			name: "create aws credential with empty secretAccessKey",
			providerConfig: cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: awsCredentialResource,
				AWSCredentials: &cli_credential.IAMCredentials{
					AccessKeyID:     testAccessKeyId,
					SecretAccessKey: "",
				},
			},
			err:        errEmptySecretAccessKey,
			setupMocks: nil,
		},
		{
			name: "create unsupported provider credential",
			providerConfig: cli_credential.ProviderCredentialConfiguration{
				ProviderCredentialResource: invalidCredentialResource,
			},
			err:        &cli_credential.ErrUnsupportedCloudProvider{},
			setupMocks: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testcontext.New(t)
			defer cancel()
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockCredentialClient := cli_credential.NewMockInterface(mockCtrl)
			if tt.setupMocks != nil {
				tt.setupMocks(*mockCredentialClient, tt.planeType, tt.planeName)
			}
			cliCredentialClient := UCPCredentialManagementClient{
				CredentialInterface: mockCredentialClient,
			}
			err := cliCredentialClient.Put(ctx, tt.providerConfig)
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.Equal(t, tt.err, err)
			}
		})
	}
}

func Test_Credential_Get(t *testing.T) {
	tests := []struct {
		name               string
		credentialResource cli_credential.ProviderCredentialResource
		planeType          string
		planeName          string
		err                error
		setupMocks         func(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string)
	}{
		{
			name: "get azure credential success",
			credentialResource: cli_credential.ProviderCredentialResource{
				Name:    azureProviderName,
				Enabled: true,
			},
			planeType:  cli_credential.AzurePlaneType,
			planeName:  AzurePlaneName,
			err:        nil,
			setupMocks: setupSuccessGetMocks,
		},
		{
			name: "get aws credential success",
			credentialResource: cli_credential.ProviderCredentialResource{
				Name:    awsProviderName,
				Enabled: true,
			},
			planeType:  cli_credential.AWSPlaneType,
			planeName:  AWSPlaneName,
			err:        nil,
			setupMocks: setupSuccessGetMocks,
		},
		{
			name: "credential not found",
			credentialResource: cli_credential.ProviderCredentialResource{
				Name:    azureProviderName,
				Enabled: false,
			},
			planeType:  cli_credential.AzurePlaneType,
			planeName:  AzurePlaneName,
			err:        nil,
			setupMocks: setupNotFoundGetMocks,
		},
		{
			name: "credential get failure",
			credentialResource: cli_credential.ProviderCredentialResource{
				Name:    azureProviderName,
				Enabled: false,
			},
			planeType:  cli_credential.AzurePlaneType,
			planeName:  AzurePlaneName,
			err:        errInternalServer,
			setupMocks: setupErrorGetMocks,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testcontext.New(t)
			defer cancel()
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockCredentialClient := cli_credential.NewMockInterface(mockCtrl)
			if tt.setupMocks != nil {
				tt.setupMocks(*mockCredentialClient, tt.planeType, tt.planeName)
			}
			cliCredentialClient := UCPCredentialManagementClient{
				CredentialInterface: mockCredentialClient,
			}
			resp, err := cliCredentialClient.Get(ctx, tt.credentialResource.Name)
			if tt.err == nil {
				require.NoError(t, err)
				require.Equal(t, resp.Name, tt.credentialResource.Name)
				require.Equal(t, resp.Enabled, tt.credentialResource.Enabled)
			} else {
				require.Equal(t, tt.err, err)
			}
		})
	}
}

func Test_Credential_List(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockCredentialClient := cli_credential.NewMockInterface(mockCtrl)
	azureList := []cli_credential.ProviderCredentialResource{
		{
			Name:    azureCredential,
			Enabled: true,
		},
	}
	mockCredentialClient.EXPECT().
		ListCredential(gomock.Any(), cli_credential.AzurePlaneType, AzurePlaneName).
		Return(azureList, nil).
		Times(1)
	awsList := []cli_credential.ProviderCredentialResource{
		{
			Name:    awsCredential,
			Enabled: true,
		},
	}
	mockCredentialClient.EXPECT().
		ListCredential(gomock.Any(), cli_credential.AWSPlaneType, AWSPlaneName).
		Return(awsList, nil).
		Times(1)

	cliCredentialClient := UCPCredentialManagementClient{
		CredentialInterface: mockCredentialClient,
	}
	resp, err := cliCredentialClient.List(ctx)
	require.NoError(t, err)
	require.Equal(t, len(resp), 2)
}

func Test_Credential_Delete(t *testing.T) {
	tests := []struct {
		name           string
		credentialName string
		planeType      string
		planeName      string
		err            error
		setupMocks     func(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string)
	}{
		{
			name:           "delete azure credential",
			credentialName: azureCredential,
			planeType:      cli_credential.AzurePlaneType,
			planeName:      AzurePlaneName,
			err:            nil,
			setupMocks:     setupSuccessDeleteMocks,
		},
		{
			name:           "delete aws credential",
			credentialName: awsCredential,
			planeType:      cli_credential.AWSPlaneType,
			planeName:      AWSPlaneName,
			err:            nil,
			setupMocks:     setupSuccessDeleteMocks,
		},
		{
			name:           "delete unsupported credential",
			credentialName: azureCredential,
			planeType:      cli_credential.AzurePlaneType,
			planeName:      AzurePlaneName,
			err:            errInternalServer,
			setupMocks:     setupErrDeleteMocks,
		},
		{
			name:           "delete non existent azure credential",
			credentialName: azureCredential,
			planeType:      cli_credential.AzurePlaneType,
			planeName:      AzurePlaneName,
			err:            nil,
			setupMocks:     setupNotFoundDeleteMocks,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testcontext.New(t)
			defer cancel()
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockCredentialClient := cli_credential.NewMockInterface(mockCtrl)
			if tt.setupMocks != nil {
				tt.setupMocks(*mockCredentialClient, tt.planeType, tt.planeName)
			}
			cliCredentialClient := UCPCredentialManagementClient{
				CredentialInterface: mockCredentialClient,
			}
			isDeleted, err := cliCredentialClient.Delete(ctx, tt.credentialName)
			if tt.err == nil {
				require.NoError(t, err)
				require.Equal(t, isDeleted, true)
			} else {
				require.Equal(t, err, tt.err)
			}
		})
	}
}

func setupSuccessPutMocks(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		CreateCredential(gomock.Any(), planeType, planeName, gomock.Any(), gomock.Any()).
		Return(nil).Times(1)
}

func setupSuccessGetMocks(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		GetCredential(gomock.Any(), planeType, planeName, gomock.Any()).
		Return(nil).Times(1)
}

func setupNotFoundGetMocks(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		GetCredential(gomock.Any(), planeType, planeName, gomock.Any()).
		Return(errCredentialNotFound).
		Times(1)
}

func setupErrorGetMocks(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		GetCredential(gomock.Any(), planeType, planeName, gomock.Any()).
		Return(errInternalServer).
		Times(1)
}

func setupSuccessDeleteMocks(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		DeleteCredential(gomock.Any(), planeType, planeName, gomock.Any()).
		Return(nil).Times(1)
}

func setupNotFoundDeleteMocks(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		DeleteCredential(gomock.Any(), planeType, planeName, gomock.Any()).
		Return(errCredentialNotFound).Times(1)
}

func setupErrDeleteMocks(mockCredentialClient cli_credential.MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		DeleteCredential(gomock.Any(), planeType, planeName, gomock.Any()).
		Return(errInternalServer).Times(1)
}
