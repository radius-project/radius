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

package credential

import (
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/testcontext"
)

const (
	azureProviderName = "azure"
	awsProviderName   = "aws"
	clientID          = "00000000-0000-0000-0000-000000000000"
	tenantID          = "00000000-0000-0000-0000-000000000000"
	credentialName    = "default"
)

var (
	errCredentialNotFound = error(&azcore.ResponseError{ErrorCode: "NotFound"})
	errInternalServer     = errors.New("internal server error")
)

func Test_AzureCredential_Put(t *testing.T) {
	tests := []struct {
		name       string
		planeType  string
		planeName  string
		credential ucp.AzureCredentialResource
		err        error
		setupMocks func(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string)
	}{
		{
			name:      "create azure credential success",
			planeType: AzurePlaneType,
			planeName: AzurePlaneName,
			credential: ucp.AzureCredentialResource{
				Name:     to.Ptr(azureProviderName),
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
			setupMocks: setupSuccessPutAzureMocks,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testcontext.NewWithCancel(t)
			t.Cleanup(cancel)

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			azMockCredentialClient := NewMockAzureCredentialManagementClientInterface(mockCtrl)
			awsMockCredentialClient := NewMockAWSCredentialManagementClientInterface(mockCtrl)
			if tt.setupMocks != nil {
				tt.setupMocks(*azMockCredentialClient, *awsMockCredentialClient, tt.planeType, tt.planeName)
			}
			cliCredentialClient := UCPCredentialManagementClient{
				AzClient:  azMockCredentialClient,
				AWSClient: awsMockCredentialClient,
			}
			err := cliCredentialClient.PutAzure(ctx, tt.credential)
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.Equal(t, tt.err, err)
			}
		})
	}
}

func Test_AWSCredential_Put(t *testing.T) {
	tests := []struct {
		name       string
		planeType  string
		planeName  string
		credential ucp.AwsCredentialResource
		err        error
		setupMocks func(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string)
	}{
		{
			name:      "create aws credential success",
			planeType: AWSPlaneType,
			planeName: AWSPlaneName,
			credential: ucp.AwsCredentialResource{
				Name:     to.Ptr(awsProviderName),
				Location: to.Ptr(v1.LocationGlobal),
				Type:     to.Ptr(AWSCredential),
				Properties: &ucp.AwsAccessKeyCredentialProperties{
					Storage: &ucp.CredentialStorageProperties{
						Kind: to.Ptr(ucp.CredentialStorageKindInternal),
					},
					AccessKeyID:     to.Ptr("access-key-id"),
					SecretAccessKey: to.Ptr("secret-access-key"),
				},
			},
			err:        nil,
			setupMocks: setupSuccessPutAWSMocks,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testcontext.NewWithCancel(t)
			t.Cleanup(cancel)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			azMockCredentialClient := NewMockAzureCredentialManagementClientInterface(mockCtrl)
			awsMockCredentialClient := NewMockAWSCredentialManagementClientInterface(mockCtrl)
			if tt.setupMocks != nil {
				tt.setupMocks(*azMockCredentialClient, *awsMockCredentialClient, tt.planeType, tt.planeName)
			}
			cliCredentialClient := UCPCredentialManagementClient{
				AzClient:  azMockCredentialClient,
				AWSClient: awsMockCredentialClient,
			}
			err := cliCredentialClient.PutAWS(ctx, tt.credential)
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
		setupMocks         func(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string)
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
			setupMocks: setupSuccessGetAzureMocks,
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
			setupMocks: setupSuccessGetAWSMocks,
		},
		{
			name: "credential not found azure",
			credentialResource: ProviderCredentialConfiguration{
				CloudProviderStatus: CloudProviderStatus{
					Name:    azureProviderName,
					Enabled: false,
				},
			},
			planeType:  AzurePlaneType,
			planeName:  AzurePlaneName,
			err:        nil,
			setupMocks: setupNotFoundAzureGetMocks,
		},
		{
			name: "credential not found aws",
			credentialResource: ProviderCredentialConfiguration{
				CloudProviderStatus: CloudProviderStatus{
					Name:    awsProviderName,
					Enabled: false,
				},
			},
			planeType:  AWSPlaneType,
			planeName:  AWSPlaneName,
			err:        nil,
			setupMocks: setupNotFoundAWSGetMocks,
		},
		{
			name: "credential get failure azure",
			credentialResource: ProviderCredentialConfiguration{
				CloudProviderStatus: CloudProviderStatus{
					Name:    azureProviderName,
					Enabled: false,
				},
			},
			planeType:  AzurePlaneType,
			planeName:  AzurePlaneName,
			err:        errInternalServer,
			setupMocks: setupErrorAzureGetMocks,
		},
		{
			name: "credential get failure aws",
			credentialResource: ProviderCredentialConfiguration{
				CloudProviderStatus: CloudProviderStatus{
					Name:    awsProviderName,
					Enabled: false,
				},
			},
			planeType:  AWSPlaneType,
			planeName:  AWSPlaneName,
			err:        errInternalServer,
			setupMocks: setupErrorAWSGetMocks,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testcontext.NewWithCancel(t)
			t.Cleanup(cancel)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			azMockCredentialClient := NewMockAzureCredentialManagementClientInterface(mockCtrl)
			awsMockCredentialClient := NewMockAWSCredentialManagementClientInterface(mockCtrl)
			if tt.setupMocks != nil {
				tt.setupMocks(*azMockCredentialClient, *awsMockCredentialClient, tt.planeType, tt.planeName)
			}
			cliCredentialClient := UCPCredentialManagementClient{
				AzClient:  azMockCredentialClient,
				AWSClient: awsMockCredentialClient,
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
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	azMockCredentialClient := NewMockAzureCredentialManagementClientInterface(mockCtrl)
	awsMockCredentialClient := NewMockAWSCredentialManagementClientInterface(mockCtrl)

	azureList := []CloudProviderStatus{
		{
			Name:    AzureCredential,
			Enabled: true,
		},
	}
	azMockCredentialClient.EXPECT().
		List(gomock.Any()).
		Return(azureList, nil).
		Times(1)
	awsList := []CloudProviderStatus{
		{
			Name:    AWSCredential,
			Enabled: true,
		},
	}
	awsMockCredentialClient.EXPECT().
		List(gomock.Any()).
		Return(awsList, nil).
		Times(1)

	cliCredentialClient := UCPCredentialManagementClient{
		AzClient:  azMockCredentialClient,
		AWSClient: awsMockCredentialClient,
	}
	resp, err := cliCredentialClient.List(ctx)
	require.NoError(t, err)
	require.Equal(t, len(resp), 2)
}

func Test_Credential_Azure_Show(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	azMockCredentialClient := NewMockAzureCredentialManagementClientInterface(mockCtrl)

	expectedAzProvider := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    azureProviderName,
			Enabled: true,
		},
	}

	azMockCredentialClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(expectedAzProvider, nil).Times(1)
	cliCredentialClient := UCPCredentialManagementClient{
		AzClient: azMockCredentialClient,
	}
	azProvider, err := cliCredentialClient.Get(ctx, azureProviderName)
	require.NoError(t, err)
	require.Equal(t, azProvider, expectedAzProvider)
}

func Test_Credential_AWS_Show(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	AWSMockCredentialClient := NewMockAWSCredentialManagementClientInterface(mockCtrl)

	expectedAWSProvider := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    awsProviderName,
			Enabled: true,
		},
	}

	AWSMockCredentialClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(expectedAWSProvider, nil).Times(1)
	cliCredentialClient := UCPCredentialManagementClient{
		AWSClient: AWSMockCredentialClient,
	}
	AWSProvider, err := cliCredentialClient.Get(ctx, awsProviderName)
	require.NoError(t, err)
	require.Equal(t, AWSProvider, expectedAWSProvider)
}

func Test_Credential_Delete(t *testing.T) {
	tests := []struct {
		name           string
		credentialName string
		planeType      string
		planeName      string
		isDeleted      bool
		err            error
		setupMocks     func(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string)
	}{
		{
			name:           "delete azure credential",
			credentialName: AzureCredential,
			planeType:      AzurePlaneType,
			planeName:      AzurePlaneName,
			err:            nil,
			isDeleted:      true,
			setupMocks:     setupSuccessAzureDeleteMocks,
		},
		{
			name:           "delete aws credential",
			credentialName: AWSCredential,
			planeType:      AWSPlaneType,
			planeName:      AWSPlaneName,
			err:            nil,
			isDeleted:      true,
			setupMocks:     setupSuccessAWSDeleteMocks,
		},
		{
			name:           "delete unsupported azure credential",
			credentialName: AzureCredential,
			planeType:      AzurePlaneType,
			planeName:      AzurePlaneName,
			err:            errInternalServer,
			setupMocks:     setupErrorAzureDeleteMocks,
		},
		{
			name:           "delete unsupported aws credential",
			credentialName: AWSCredential,
			planeType:      AWSPlaneType,
			planeName:      AWSPlaneName,
			err:            errInternalServer,
			setupMocks:     setupErrorAWSDeleteMocks,
		},
		{
			name:           "delete non existent azure credential",
			credentialName: AzureCredential,
			planeType:      AzurePlaneType,
			planeName:      AzurePlaneName,
			err:            nil,
			isDeleted:      false,
			setupMocks:     setupNotFoundAzureDeleteMocks,
		},
		{
			name:           "delete non existent aws credential",
			credentialName: AWSCredential,
			planeType:      AWSPlaneType,
			planeName:      AWSPlaneName,
			err:            nil,
			isDeleted:      false,
			setupMocks:     setupNotFoundAWSDeleteMocks,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testcontext.NewWithCancel(t)
			t.Cleanup(cancel)

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			azMockCredentialClient := NewMockAzureCredentialManagementClientInterface(mockCtrl)
			awsMockCredentialClient := NewMockAWSCredentialManagementClientInterface(mockCtrl)
			if tt.setupMocks != nil {
				tt.setupMocks(*azMockCredentialClient, *awsMockCredentialClient, tt.planeType, tt.planeName)
			}
			cliCredentialClient := UCPCredentialManagementClient{
				AzClient:  azMockCredentialClient,
				AWSClient: awsMockCredentialClient,
			}
			isDeleted, err := cliCredentialClient.Delete(ctx, tt.credentialName)
			if tt.err == nil {
				require.NoError(t, err)
				require.Equal(t, isDeleted, tt.isDeleted)
			} else {
				require.Equal(t, err, tt.err)
			}
		})
	}
}

func setupSuccessPutAzureMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAzure.EXPECT().
		Put(gomock.Any(), gomock.Any()).
		Return(nil).Times(1)
}
func setupSuccessPutAWSMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAWS.EXPECT().
		Put(gomock.Any(), gomock.Any()).
		Return(nil).Times(1)
}

func setupSuccessGetAzureMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	credential := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    azureProviderName,
			Enabled: true,
		},
	}
	mockAzure.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(credential, nil).Times(1)
}

func setupSuccessGetAWSMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	credential := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    awsProviderName,
			Enabled: true,
		},
	}
	mockAWS.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(credential, nil).Times(1)
}

func setupNotFoundAzureGetMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAzure.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(ProviderCredentialConfiguration{}, errCredentialNotFound).
		Times(1)
}

func setupNotFoundAWSGetMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAWS.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(ProviderCredentialConfiguration{}, errCredentialNotFound).
		Times(1)
}

func setupErrorAzureGetMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAzure.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(ProviderCredentialConfiguration{}, errInternalServer).
		Times(1)
}

func setupErrorAWSGetMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAWS.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(ProviderCredentialConfiguration{}, errInternalServer).
		Times(1)
}

func setupSuccessAzureDeleteMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAzure.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(true, nil).Times(1)
}

func setupSuccessAWSDeleteMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAWS.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(true, nil).Times(1)
}

func setupNotFoundAzureDeleteMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAzure.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(false, nil).Times(1)
}

func setupNotFoundAWSDeleteMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAWS.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(false, nil).Times(1)
}

func setupErrorAzureDeleteMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAzure.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(false, errInternalServer).Times(1)
}

func setupErrorAWSDeleteMocks(mockAzure MockAzureCredentialManagementClientInterface, mockAWS MockAWSCredentialManagementClientInterface, planeType string, planeName string) {
	mockAWS.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(false, errInternalServer).Times(1)
}
