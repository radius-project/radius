// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package credential

import (
	"errors"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

const (
	azureProviderName = "azure"
	awsProviderName   = "aws"
	clientID          = "00000000-0000-0000-0000-000000000000"
	tenantID          = "00000000-0000-0000-0000-000000000000"
)

var (
	errCredentialNotFound = error(&azcore.ResponseError{ErrorCode: "NotFound"})
	errInternalServer     = errors.New("internal server error")
)

func Test_Credential_Put(t *testing.T) {
	tests := []struct {
		name       string
		planeType  string
		planeName  string
		credential ucp.CredentialResource
		err        error
		setupMocks func(mockCredentialClient MockInterface, planeType string, planeName string)
	}{
		{
			name:      "create azure credential success",
			planeType: AzurePlaneType,
			planeName: AzurePlaneName,
			credential: ucp.CredentialResource{
				Location: to.Ptr(v1.LocationGlobal),
				Type:     to.Ptr(AzureCredential),
				Properties: &ucp.AzureServicePrincipalProperties{
					Storage: &ucp.CredentialStorageProperties{
						Kind: to.Ptr(ucp.CredentialStorageKindInternal),
					},
					ClientID:     to.Ptr(clientID),
					ClientSecret: to.Ptr("cool-client-secret"),
					TenantID:     to.Ptr(tenantID),
				},
			},
			err:        nil,
			setupMocks: setupSuccessPutMocks,
		},
		{
			name:      "create aws credential success",
			planeType: AWSPlaneType,
			planeName: AWSPlaneName,
			credential: ucp.CredentialResource{
				Location: to.Ptr(v1.LocationGlobal),
				Type:     to.Ptr(AWSCredential),
				Properties: &ucp.AWSCredentialProperties{
					Storage: &ucp.CredentialStorageProperties{
						Kind: to.Ptr(ucp.CredentialStorageKindInternal),
					},
					AccessKeyID:     to.Ptr("access-key-id"),
					SecretAccessKey: to.Ptr("secret-access-key"),
				},
			},
			err:        nil,
			setupMocks: setupSuccessPutMocks,
		},
		{
			name: "create unsupported provider credential",
			credential: ucp.CredentialResource{
				Location: to.Ptr(v1.LocationGlobal),
				Type:     to.Ptr(""),
			},
			err:        &ErrUnsupportedCloudProvider{},
			setupMocks: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testcontext.New(t)
			defer cancel()
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockCredentialClient := NewMockInterface(mockCtrl)
			if tt.setupMocks != nil {
				tt.setupMocks(*mockCredentialClient, tt.planeType, tt.planeName)
			}
			cliCredentialClient := UCPCredentialManagementClient{
				CredentialInterface: mockCredentialClient,
			}
			err := cliCredentialClient.Put(ctx, tt.credential)
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
		credentialResource ProviderCredentialConfiguration
		planeType          string
		planeName          string
		err                error
		setupMocks         func(mockCredentialClient MockInterface, planeType string, planeName string)
	}{
		{
			name: "get azure credential success",
			credentialResource: ProviderCredentialConfiguration{
				CloudProviderStatus: CloudProviderStatus{
					Name:    azureProviderName,
					Enabled: true,
				},
			},
			planeType:  AzurePlaneType,
			planeName:  AzurePlaneName,
			err:        nil,
			setupMocks: setupSuccessGetMocks,
		},
		{
			name: "get aws credential success",
			credentialResource: ProviderCredentialConfiguration{
				CloudProviderStatus: CloudProviderStatus{
					Name:    awsProviderName,
					Enabled: true,
				},
			},
			planeType:  AWSPlaneType,
			planeName:  AWSPlaneName,
			err:        nil,
			setupMocks: setupSuccessGetMocks,
		},
		{
			name: "credential not found",
			credentialResource: ProviderCredentialConfiguration{
				CloudProviderStatus: CloudProviderStatus{
					Name:    azureProviderName,
					Enabled: false,
				},
			},
			planeType:  AzurePlaneType,
			planeName:  AzurePlaneName,
			err:        nil,
			setupMocks: setupNotFoundGetMocks,
		},
		{
			name: "credential get failure",
			credentialResource: ProviderCredentialConfiguration{
				CloudProviderStatus: CloudProviderStatus{
					Name:    azureProviderName,
					Enabled: false,
				},
			},
			planeType:  AzurePlaneType,
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
			mockCredentialClient := NewMockInterface(mockCtrl)
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
	mockCredentialClient := NewMockInterface(mockCtrl)
	azureList := []CloudProviderStatus{
		{
			Name:    AzureCredential,
			Enabled: true,
		},
	}
	mockCredentialClient.EXPECT().
		ListCredential(gomock.Any(), AzurePlaneType, AzurePlaneName).
		Return(azureList, nil).
		Times(1)
	awsList := []CloudProviderStatus{
		{
			Name:    AWSCredential,
			Enabled: true,
		},
	}
	mockCredentialClient.EXPECT().
		ListCredential(gomock.Any(), AWSPlaneType, AWSPlaneName).
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
		setupMocks     func(mockCredentialClient MockInterface, planeType string, planeName string)
	}{
		{
			name:           "delete azure credential",
			credentialName: AzureCredential,
			planeType:      AzurePlaneType,
			planeName:      AzurePlaneName,
			err:            nil,
			setupMocks:     setupSuccessDeleteMocks,
		},
		{
			name:           "delete aws credential",
			credentialName: AWSCredential,
			planeType:      AWSPlaneType,
			planeName:      AWSPlaneName,
			err:            nil,
			setupMocks:     setupSuccessDeleteMocks,
		},
		{
			name:           "delete unsupported credential",
			credentialName: AzureCredential,
			planeType:      AzurePlaneType,
			planeName:      AzurePlaneName,
			err:            errInternalServer,
			setupMocks:     setupErrDeleteMocks,
		},
		{
			name:           "delete non existent azure credential",
			credentialName: AzureCredential,
			planeType:      AzurePlaneType,
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
			mockCredentialClient := NewMockInterface(mockCtrl)
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

func setupSuccessPutMocks(mockCredentialClient MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		CreateCredential(gomock.Any(), planeType, planeName, gomock.Any(), gomock.Any()).
		Return(nil).Times(1)
}

func setupSuccessGetMocks(mockCredentialClient MockInterface, planeType string, planeName string) {
	credential := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    azureProviderName,
			Enabled: true,
		},
	}
	if strings.EqualFold(planeType, AzurePlaneType) {
		credential.CloudProviderStatus.Name = azureProviderName
	} else if strings.EqualFold(planeType, AWSPlaneType) {
		credential.CloudProviderStatus.Name = awsProviderName
	}
	mockCredentialClient.EXPECT().
		GetCredential(gomock.Any(), planeType, planeName, defaultSecretName).
		Return(credential, nil).Times(1)
}

func setupNotFoundGetMocks(mockCredentialClient MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		GetCredential(gomock.Any(), planeType, planeName, defaultSecretName).
		Return(ProviderCredentialConfiguration{}, errCredentialNotFound).
		Times(1)
}

func setupErrorGetMocks(mockCredentialClient MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		GetCredential(gomock.Any(), planeType, planeName, defaultSecretName).
		Return(ProviderCredentialConfiguration{}, errInternalServer).
		Times(1)
}

func setupSuccessDeleteMocks(mockCredentialClient MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		DeleteCredential(gomock.Any(), planeType, planeName, defaultSecretName).
		Return(nil).Times(1)
}

func setupNotFoundDeleteMocks(mockCredentialClient MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		DeleteCredential(gomock.Any(), planeType, planeName, defaultSecretName).
		Return(errCredentialNotFound).Times(1)
}

func setupErrDeleteMocks(mockCredentialClient MockInterface, planeType string, planeName string) {
	mockCredentialClient.EXPECT().
		DeleteCredential(gomock.Any(), planeType, planeName, defaultSecretName).
		Return(errInternalServer).Times(1)
}
