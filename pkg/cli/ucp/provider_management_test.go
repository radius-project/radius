// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

// import (
// 	"context"
// 	"testing"

// 	"github.com/project-radius/radius/pkg/cli/clients"
// 	"github.com/stretchr/testify/require"
// )

// const (
// 	azureProviderName = "azure"
// 	testClientID      = "testClientId"
// 	testTenantID      = "testTenantId"
// 	testClientSecret  = "testAzureSecret"
// )

// func TestPut(t *testing.T) {
// 	azureProviderResource := clients.CloudProviderResource{
// 		Name:    azureProviderName,
// 		Enabled: true,
// 	}
// 	azureCredentials := clients.ServicePrincipalCredentials{
// 		ClientID:     testClientID,
// 		TenantID:     testTenantID,
// 		ClientSecret: testClientSecret,
// 	}
// 	tests := []struct {
// 		providerConfig clients.CloudProviderConfiguration
// 		err            error
// 	}{
// 		{
// 			providerConfig: clients.CloudProviderConfiguration{
// 				CloudProviderResource: azureProviderResource,
// 				AzureCredentials:      &azureCredentials,
// 			},
// 			err: nil,
// 		},
// 	}

// 	for _, tt := range tests {
// 		ctrl := gomock.NewController(t)
// 		azureCredentialClient := 
// 		providerClient := UCPCloudProviderManagementClient{}
// 		err := providerClient.createCredentialConfiguration(context.Background(), providerClient, azureProviderName, tt.providerConfig)
// 		if tt.err == nil {
// 			require.NoError(t, err)
// 		} else {
// 			require.Equal(t, tt.err, err)
// 		}
// 	}
// }
