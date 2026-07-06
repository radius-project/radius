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
	tfConfigID       = "/planes/radius/local/resourceGroups/rg/providers/Radius.Core/terraformSettings/tf"
	tfConfigName     = "tf"
	tfConfigType     = "Radius.Core/terraformSettings"
	tfConfigLocation = "global"
)

func TestTerraformSettings_ConvertTo_Empty(t *testing.T) {
	src := &TerraformSettingsResource{
		ID:       to.Ptr(tfConfigID),
		Name:     to.Ptr(tfConfigName),
		Type:     to.Ptr(tfConfigType),
		Location: to.Ptr(tfConfigLocation),
		Properties: &TerraformSettingsProperties{
			ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
		},
	}

	dm, err := src.ConvertTo()
	require.NoError(t, err)

	tc, ok := dm.(*datamodel.TerraformSettings)
	require.True(t, ok)
	require.Equal(t, tfConfigID, tc.ID)
	require.Nil(t, tc.Properties.Terraformrc.ProviderInstallation)
	require.Empty(t, tc.Properties.Terraformrc.Credentials)
	require.Empty(t, tc.Properties.Env)
	require.Empty(t, tc.Properties.ReferencedBy)
}

func TestTerraformSettings_ConvertTo_NetworkMirrorOnly(t *testing.T) {
	src := newVersionedTerraformSettings(&TerraformrcConfig{
		ProviderInstallation: &TerraformProviderInstallation{
			NetworkMirror: &TerraformProviderMirror{
				URL:     to.Ptr("https://mirror.example.com/"),
				Include: to.SliceOfPtrs("hashicorp/aws"),
				Exclude: to.SliceOfPtrs("hashicorp/google"),
			},
		},
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	tc := dm.(*datamodel.TerraformSettings)

	require.NotNil(t, tc.Properties.Terraformrc.ProviderInstallation)
	require.NotNil(t, tc.Properties.Terraformrc.ProviderInstallation.NetworkMirror)
	require.Equal(t, "https://mirror.example.com/", tc.Properties.Terraformrc.ProviderInstallation.NetworkMirror.URL)
	require.Equal(t, []string{"hashicorp/aws"}, tc.Properties.Terraformrc.ProviderInstallation.NetworkMirror.Include)
	require.Equal(t, []string{"hashicorp/google"}, tc.Properties.Terraformrc.ProviderInstallation.NetworkMirror.Exclude)
	require.Nil(t, tc.Properties.Terraformrc.ProviderInstallation.Direct)
}

func TestTerraformSettings_ConvertTo_DirectOnly(t *testing.T) {
	src := newVersionedTerraformSettings(&TerraformrcConfig{
		ProviderInstallation: &TerraformProviderInstallation{
			Direct: &TerraformProviderDirect{
				Include: to.SliceOfPtrs("hashicorp/google"),
				Exclude: to.SliceOfPtrs("hashicorp/aws"),
			},
		},
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	tc := dm.(*datamodel.TerraformSettings)

	require.NotNil(t, tc.Properties.Terraformrc.ProviderInstallation)
	require.Nil(t, tc.Properties.Terraformrc.ProviderInstallation.NetworkMirror)
	require.NotNil(t, tc.Properties.Terraformrc.ProviderInstallation.Direct)
	require.Equal(t, []string{"hashicorp/google"}, tc.Properties.Terraformrc.ProviderInstallation.Direct.Include)
	require.Equal(t, []string{"hashicorp/aws"}, tc.Properties.Terraformrc.ProviderInstallation.Direct.Exclude)
}

func TestTerraformSettings_ConvertTo_Both(t *testing.T) {
	src := newVersionedTerraformSettings(&TerraformrcConfig{
		ProviderInstallation: &TerraformProviderInstallation{
			NetworkMirror: &TerraformProviderMirror{URL: to.Ptr("https://mirror.example.com/")},
			Direct:        &TerraformProviderDirect{Include: to.SliceOfPtrs("hashicorp/google")},
		},
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	tc := dm.(*datamodel.TerraformSettings)

	require.NotNil(t, tc.Properties.Terraformrc.ProviderInstallation.NetworkMirror)
	require.NotNil(t, tc.Properties.Terraformrc.ProviderInstallation.Direct)
}

func TestTerraformSettings_ConvertTo_MultipleCredentialHosts(t *testing.T) {
	src := newVersionedTerraformSettings(&TerraformrcConfig{
		Credentials: map[string]*TerraformCredentialConfig{
			"app.terraform.io":     {Secret: to.Ptr("/planes/radius/local/.../secretA")},
			"registry.example.com": {Secret: to.Ptr("/planes/radius/local/.../secretB")},
			"private.example.com":  {Secret: to.Ptr("/planes/radius/local/.../secretC")},
		},
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	tc := dm.(*datamodel.TerraformSettings)

	require.Len(t, tc.Properties.Terraformrc.Credentials, 3)
	require.Equal(t, "/planes/radius/local/.../secretA", tc.Properties.Terraformrc.Credentials["app.terraform.io"].Secret)
	require.Equal(t, "/planes/radius/local/.../secretB", tc.Properties.Terraformrc.Credentials["registry.example.com"].Secret)
	require.Equal(t, "/planes/radius/local/.../secretC", tc.Properties.Terraformrc.Credentials["private.example.com"].Secret)
}

func TestTerraformSettings_ConvertTo_NilCredentialEntrySkipped(t *testing.T) {
	// A nil entry in the Credentials map should not produce a panic; the
	// converter should silently skip it.
	src := newVersionedTerraformSettings(&TerraformrcConfig{
		Credentials: map[string]*TerraformCredentialConfig{
			"app.terraform.io":     {Secret: to.Ptr("/planes/.../s1")},
			"registry.example.com": nil,
		},
	})

	dm, err := src.ConvertTo()
	require.NoError(t, err)
	tc := dm.(*datamodel.TerraformSettings)

	require.Len(t, tc.Properties.Terraformrc.Credentials, 1)
	_, has := tc.Properties.Terraformrc.Credentials["app.terraform.io"]
	require.True(t, has)
}

func TestTerraformSettings_ConvertFrom_Wrong_Type(t *testing.T) {
	dst := &TerraformSettingsResource{}
	err := dst.ConvertFrom(&datamodel.Environment{})
	require.Error(t, err)
	require.Equal(t, v1.ErrInvalidModelConversion, err)
}

func TestTerraformSettings_RoundTrip_Identity(t *testing.T) {
	original := newVersionedTerraformSettings(&TerraformrcConfig{
		ProviderInstallation: &TerraformProviderInstallation{
			NetworkMirror: &TerraformProviderMirror{
				URL:     to.Ptr("https://mirror.example.com/"),
				Include: to.SliceOfPtrs("hashicorp/aws", "hashicorp/google"),
				Exclude: to.SliceOfPtrs("hashicorp/random"),
			},
			Direct: &TerraformProviderDirect{
				Include: to.SliceOfPtrs("hashicorp/local"),
			},
		},
		Credentials: map[string]*TerraformCredentialConfig{
			"app.terraform.io":     {Secret: to.Ptr("/planes/.../s1")},
			"registry.example.com": {Secret: to.Ptr("/planes/.../s2")},
		},
	})
	original.Properties.Env = map[string]*string{
		"TF_LOG":      to.Ptr("DEBUG"),
		"TF_LOG_PATH": to.Ptr("/tmp/tf.log"),
	}

	dm, err := original.ConvertTo()
	require.NoError(t, err)

	roundTripped := &TerraformSettingsResource{}
	require.NoError(t, roundTripped.ConvertFrom(dm))

	// Provider installation
	rt := roundTripped.Properties.Terraformrc
	require.Equal(t, "https://mirror.example.com/", *rt.ProviderInstallation.NetworkMirror.URL)
	require.ElementsMatch(t,
		[]string{"hashicorp/aws", "hashicorp/google"},
		ptrsToStrings(rt.ProviderInstallation.NetworkMirror.Include))
	require.ElementsMatch(t, []string{"hashicorp/random"}, ptrsToStrings(rt.ProviderInstallation.NetworkMirror.Exclude))
	require.ElementsMatch(t, []string{"hashicorp/local"}, ptrsToStrings(rt.ProviderInstallation.Direct.Include))

	// Credentials
	require.Len(t, rt.Credentials, 2)
	require.Equal(t, "/planes/.../s1", *rt.Credentials["app.terraform.io"].Secret)
	require.Equal(t, "/planes/.../s2", *rt.Credentials["registry.example.com"].Secret)

	// Env
	require.Len(t, roundTripped.Properties.Env, 2)
	require.Equal(t, "DEBUG", *roundTripped.Properties.Env["TF_LOG"])
	require.Equal(t, "/tmp/tf.log", *roundTripped.Properties.Env["TF_LOG_PATH"])
}

func TestTerraformSettings_CredentialsAreIndependentEntries(t *testing.T) {
	// Guards against pointer-aliasing bugs in the credentials map: each host's
	// Secret must point to its own value, not share storage across iterations.
	dm := &datamodel.TerraformSettings{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: tfConfigID, Name: tfConfigName, Type: tfConfigType, Location: tfConfigLocation,
			},
		},
		Properties: datamodel.TerraformSettingsResourceProperties{
			Terraformrc: datamodel.TerraformrcConfig{
				Credentials: map[string]datamodel.TerraformCredentialConfig{
					"hostA": {Secret: "secretA"},
					"hostB": {Secret: "secretB"},
				},
			},
		},
	}

	versioned := &TerraformSettingsResource{}
	require.NoError(t, versioned.ConvertFrom(dm))

	creds := versioned.Properties.Terraformrc.Credentials
	require.Len(t, creds, 2)
	require.Equal(t, "secretA", *creds["hostA"].Secret)
	require.Equal(t, "secretB", *creds["hostB"].Secret)
	// The two pointers must not alias.
	require.NotSame(t, creds["hostA"].Secret, creds["hostB"].Secret)
}

// newVersionedTerraformSettings builds a TerraformSettingsResource with the
// required tracked-resource fields populated and the supplied .terraformrc.
func newVersionedTerraformSettings(rc *TerraformrcConfig) *TerraformSettingsResource {
	return &TerraformSettingsResource{
		ID:       to.Ptr(tfConfigID),
		Name:     to.Ptr(tfConfigName),
		Type:     to.Ptr(tfConfigType),
		Location: to.Ptr(tfConfigLocation),
		Properties: &TerraformSettingsProperties{
			ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
			Terraformrc:       rc,
		},
	}
}

func ptrsToStrings(in []*string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out
}
