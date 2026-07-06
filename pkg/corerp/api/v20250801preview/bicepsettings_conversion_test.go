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

package v20250801preview

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	bicepSettingsID       = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/bicepSettings/bc"
	bicepSettingsName     = "bc"
	bicepSettingsType     = "Radius.Core/bicepSettings"
	bicepSettingsLocation = "global"
)

func TestBicepSettings_ConvertTo_EmptyRegistryAuthentications(t *testing.T) {
	src := newVersionedBicepSettings(nil)

	dm, err := src.ConvertTo()
	require.NoError(t, err)

	bc, ok := dm.(*datamodel.BicepSettings)
	require.True(t, ok)
	require.Equal(t, bicepSettingsID, bc.ID)
	require.Empty(t, bc.Properties.RegistryAuthentications)
}

func TestBicepSettings_ConvertTo_BasicAuth(t *testing.T) {
	src := newVersionedBicepSettings(map[string]*BicepRegistryAuthentication{
		"corp.acr.io": {
			AuthenticationMethod: to.Ptr(BicepAuthenticationMethodBasicAuth),
			BasicAuthSecretID:    to.Ptr("/planes/radius/local/.../secret"),
		},
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	bc := dm.(*datamodel.BicepSettings)

	require.Len(t, bc.Properties.RegistryAuthentications, 1)
	auth := bc.Properties.RegistryAuthentications["corp.acr.io"]
	require.Equal(t, "BasicAuth", auth.AuthenticationMethod)
	require.Equal(t, "/planes/radius/local/.../secret", auth.BasicAuthSecretId)
	require.Empty(t, auth.AzureWiClientId)
	require.Empty(t, auth.AzureWiTenantId)
	require.Empty(t, auth.AwsIamRoleArn)
}

func TestBicepSettings_ConvertTo_AzureWI(t *testing.T) {
	src := newVersionedBicepSettings(map[string]*BicepRegistryAuthentication{
		"corp.acr.io": {
			AuthenticationMethod: to.Ptr(BicepAuthenticationMethodAzureWI),
			AzureWiClientID:      to.Ptr("client-id"),
			AzureWiTenantID:      to.Ptr("tenant-id"),
		},
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	bc := dm.(*datamodel.BicepSettings)

	auth := bc.Properties.RegistryAuthentications["corp.acr.io"]
	require.Equal(t, "AzureWI", auth.AuthenticationMethod)
	require.Equal(t, "client-id", auth.AzureWiClientId)
	require.Equal(t, "tenant-id", auth.AzureWiTenantId)
	require.Empty(t, auth.BasicAuthSecretId)
}

func TestBicepSettings_ConvertTo_AwsIrsa(t *testing.T) {
	src := newVersionedBicepSettings(map[string]*BicepRegistryAuthentication{
		"corp.ecr.aws": {
			AuthenticationMethod: to.Ptr(BicepAuthenticationMethodAwsIrsa),
			AwsIamRoleArn:        to.Ptr("arn:aws:iam::123:role/MyRole"),
		},
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	bc := dm.(*datamodel.BicepSettings)

	auth := bc.Properties.RegistryAuthentications["corp.ecr.aws"]
	require.Equal(t, "AwsIrsa", auth.AuthenticationMethod)
	require.Equal(t, "arn:aws:iam::123:role/MyRole", auth.AwsIamRoleArn)
	require.Empty(t, auth.BasicAuthSecretId)
}

func TestBicepSettings_ConvertTo_NilEntrySkipped(t *testing.T) {
	src := newVersionedBicepSettings(map[string]*BicepRegistryAuthentication{
		"corp.acr.io": {
			AuthenticationMethod: to.Ptr(BicepAuthenticationMethodBasicAuth),
			BasicAuthSecretID:    to.Ptr("/planes/.../s1"),
		},
		"empty.acr.io": nil,
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	bc := dm.(*datamodel.BicepSettings)

	require.Len(t, bc.Properties.RegistryAuthentications, 1)
	_, has := bc.Properties.RegistryAuthentications["corp.acr.io"]
	require.True(t, has)
}

func TestBicepSettings_ConvertFrom_Wrong_Type(t *testing.T) {
	dst := &BicepSettingsResource{}
	err := dst.ConvertFrom(&datamodel.Environment{})
	require.Error(t, err)
	require.Equal(t, v1.ErrInvalidModelConversion, err)
}

func TestBicepSettings_RoundTrip_Identity(t *testing.T) {
	original := newVersionedBicepSettings(map[string]*BicepRegistryAuthentication{
		"basic.acr.io": {
			AuthenticationMethod: to.Ptr(BicepAuthenticationMethodBasicAuth),
			BasicAuthSecretID:    to.Ptr("/planes/.../basic-secret"),
		},
		"azure.acr.io": {
			AuthenticationMethod: to.Ptr(BicepAuthenticationMethodAzureWI),
			AzureWiClientID:      to.Ptr("client-id"),
			AzureWiTenantID:      to.Ptr("tenant-id"),
		},
		"aws.ecr.io": {
			AuthenticationMethod: to.Ptr(BicepAuthenticationMethodAwsIrsa),
			AwsIamRoleArn:        to.Ptr("arn:aws:iam::123:role/MyRole"),
		},
	})

	dm, err := original.ConvertTo()
	require.NoError(t, err)

	roundTripped := &BicepSettingsResource{}
	require.NoError(t, roundTripped.ConvertFrom(dm))

	require.Len(t, roundTripped.Properties.RegistryAuthentications, 3)

	basic := roundTripped.Properties.RegistryAuthentications["basic.acr.io"]
	require.Equal(t, BicepAuthenticationMethodBasicAuth, *basic.AuthenticationMethod)
	require.Equal(t, "/planes/.../basic-secret", *basic.BasicAuthSecretID)
	require.Nil(t, basic.AzureWiClientID)
	require.Nil(t, basic.AzureWiTenantID)
	require.Nil(t, basic.AwsIamRoleArn)

	azure := roundTripped.Properties.RegistryAuthentications["azure.acr.io"]
	require.Equal(t, BicepAuthenticationMethodAzureWI, *azure.AuthenticationMethod)
	require.Equal(t, "client-id", *azure.AzureWiClientID)
	require.Equal(t, "tenant-id", *azure.AzureWiTenantID)
	require.Nil(t, azure.BasicAuthSecretID)
	require.Nil(t, azure.AwsIamRoleArn)

	aws := roundTripped.Properties.RegistryAuthentications["aws.ecr.io"]
	require.Equal(t, BicepAuthenticationMethodAwsIrsa, *aws.AuthenticationMethod)
	require.Equal(t, "arn:aws:iam::123:role/MyRole", *aws.AwsIamRoleArn)
	require.Nil(t, aws.BasicAuthSecretID)
	require.Nil(t, aws.AzureWiClientID)
	require.Nil(t, aws.AzureWiTenantID)
}

// TestBicepSettings_ConvertFrom_TwoEntriesAreDistinct guards against pointer
// aliasing in fromBicepRegistryAuthDataModel. Iterating a map yields the same
// loop variable address; if the converter takes the address of that variable
// instead of a fresh copy, all entries end up sharing the same backing
// storage and one host's secret leaks into the other.
func TestBicepSettings_ConvertFrom_TwoEntriesAreDistinct(t *testing.T) {
	dm := &datamodel.BicepSettings{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: bicepSettingsID, Name: bicepSettingsName, Type: bicepSettingsType, Location: bicepSettingsLocation,
			},
		},
		Properties: datamodel.BicepSettingsResourceProperties{
			RegistryAuthentications: map[string]datamodel.BicepRegistryAuthentication{
				"hostA": {
					AuthenticationMethod: "BasicAuth",
					BasicAuthSecretId:    "secretA",
				},
				"hostB": {
					AuthenticationMethod: "BasicAuth",
					BasicAuthSecretId:    "secretB",
				},
			},
		},
	}

	versioned := &BicepSettingsResource{}
	require.NoError(t, versioned.ConvertFrom(dm))

	hostA := versioned.Properties.RegistryAuthentications["hostA"]
	hostB := versioned.Properties.RegistryAuthentications["hostB"]

	require.Equal(t, "secretA", *hostA.BasicAuthSecretID)
	require.Equal(t, "secretB", *hostB.BasicAuthSecretID)
	// Pointers must not alias — otherwise mutating one would change the other.
	require.NotSame(t, hostA.BasicAuthSecretID, hostB.BasicAuthSecretID)
	require.NotSame(t, hostA.AuthenticationMethod, hostB.AuthenticationMethod)
}

// newVersionedBicepSettings builds a BicepSettingsResource with the required
// tracked-resource fields populated and the supplied registry auth map.
func newVersionedBicepSettings(auths map[string]*BicepRegistryAuthentication) *BicepSettingsResource {
	return &BicepSettingsResource{
		ID:       to.Ptr(bicepSettingsID),
		Name:     to.Ptr(bicepSettingsName),
		Type:     to.Ptr(bicepSettingsType),
		Location: to.Ptr(bicepSettingsLocation),
		Properties: &BicepSettingsProperties{
			ProvisioningState:       to.Ptr(ProvisioningStateSucceeded),
			RegistryAuthentications: auths,
		},
	}
}
