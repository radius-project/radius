// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"bytes"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/stretchr/testify/require"
)

// These are integration tests that test that our table formatting works well e2e

func Test_FormatApplicationTable(t *testing.T) {
	options := GetApplicationTableFormat()

	// We're just filling in the fields that are read. It's hard to test that something *doesn't* happen.
	obj := radclientv3.ApplicationResource{
		TrackedResource: radclientv3.TrackedResource{
			Resource: radclientv3.Resource{
				Name: to.StringPtr("test-app"),
			},
		},
		Properties: &radclientv3.ApplicationProperties{
			Status: &radclientv3.ApplicationStatus{
				HealthState:       to.StringPtr("Healthy"),
				ProvisioningState: to.StringPtr("Provisioned"),
			},
		},
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `APPLICATION  PROVISIONING_STATE  HEALTH_STATE
test-app     Provisioned         Healthy
`
	require.Equal(t, TrimSpaceMulti(expected), TrimSpaceMulti(buffer.String()))
}

func Test_FormatResourceTable(t *testing.T) {
	options := GetResourceTableFormat()

	// We're just filling in the fields that are read. It's hard to test that something *doesn't* happen.
	obj := radclientv3.RadiusResource{
		ProxyResource: radclientv3.ProxyResource{
			Resource: radclientv3.Resource{
				Name: to.StringPtr("test-resource"),
				Type: to.StringPtr("my/very/CoolResource"),
			},
		},
		Properties: map[string]interface{}{
			"status": &radclientv3.ComponentStatus{
				HealthState:       to.StringPtr("Healthy"),
				ProvisioningState: to.StringPtr("Provisioned"),
			},
		},
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `RESOURCE       TYPE          PROVISIONING_STATE  HEALTH_STATE
test-resource  CoolResource  Provisioned         Healthy
`
	require.Equal(t, TrimSpaceMulti(expected), TrimSpaceMulti(buffer.String()))
}
