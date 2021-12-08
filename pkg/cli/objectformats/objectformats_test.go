// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"bytes"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/stretchr/testify/require"
)

// These are integration tests that test that our table formatting works well e2e

func Test_FormatApplicationTable(t *testing.T) {
	options := GetApplicationTableFormat()

	// We're just filling in the fields that are read. It's hard to test that something *doesn't* happen.
	obj := radclient.ApplicationResource{
		TrackedResource: radclient.TrackedResource{
			Resource: radclient.Resource{
				Name: to.StringPtr("test-app"),
			},
		},
		Properties: &radclient.ApplicationProperties{
			Status: &radclient.ApplicationStatus{
				HealthState:       to.StringPtr("Healthy"),
				ProvisioningState: to.StringPtr("Provisioned"),
			},
		},
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `APPLICATION
test-app
`
	require.Equal(t, TrimSpaceMulti(expected), TrimSpaceMulti(buffer.String()))
}

func Test_FormatResourceTable(t *testing.T) {
	options := GetResourceTableFormat()

	// We're just filling in the fields that are read. It's hard to test that something *doesn't* happen.
	obj := []radclient.RadiusResource{
		{
			ProxyResource: radclient.ProxyResource{
				Resource: radclient.Resource{
					Name: to.StringPtr("test-resource"),
					Type: to.StringPtr("Microsoft.CustomProviders/resourceProviders/Application/mongodb.com.MongoDBComponent"),
				},
			},
			Properties: map[string]interface{}{
				"status": &radclient.ComponentStatus{
					HealthState:       to.StringPtr("Healthy"),
					ProvisioningState: to.StringPtr("Provisioned"),
				},
			},
		},
		{
			ProxyResource: radclient.ProxyResource{
				Resource: radclient.Resource{
					Name: to.StringPtr("test-azure-resource"),
					Type: to.StringPtr("Microsoft.ServiceBus/namespaces"),
				},
			},
			Properties: map[string]interface{}{
				"status": &radclient.ComponentStatus{
					HealthState:       to.StringPtr("Healthy"),
					ProvisioningState: to.StringPtr("Provisioned"),
				},
			},
		},
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `RESOURCE             TYPE                             PROVISIONING_STATE  HEALTH_STATE
test-resource        mongodb.com.MongoDBComponent     Provisioned         Healthy
test-azure-resource  Microsoft.ServiceBus/namespaces  Provisioned         Healthy
`

	require.Equal(t, TrimSpaceMulti(expected), TrimSpaceMulti(buffer.String()))
}
