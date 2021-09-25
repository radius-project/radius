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
	"github.com/Azure/radius/pkg/azure/radclientv3"
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

	expected := `APPLICATION  PROVISIONING_STATE  HEALTH_STATE
test-app     Provisioned         Healthy
`
	require.Equal(t, expected, buffer.String())
}

func Test_FormatComponentTable(t *testing.T) {
	options := GetComponentTableFormat()

	// We're just filling in the fields that are read. It's hard to test that something *doesn't* happen.
	obj := radclient.ComponentResource{
		TrackedResource: radclient.TrackedResource{
			Resource: radclient.Resource{
				Name: to.StringPtr("test-component"),
			},
		},
		Kind: to.StringPtr("radius.dev/TestComponent@v1alpha1"),
		Properties: &radclient.ComponentProperties{
			Status: &radclient.ComponentStatus{
				HealthState:       to.StringPtr("Healthy"),
				ProvisioningState: to.StringPtr("Provisioned"),
			},
		},
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `COMPONENT       KIND                               PROVISIONING_STATE  HEALTH_STATE
test-component  radius.dev/TestComponent@v1alpha1  Provisioned         Healthy
`
	require.Equal(t, expected, buffer.String())
}

func Test_FormatResourceTable(t *testing.T) {
	options := GetResourceTableFormat()

	// We're just filling in the fields that are read. It's hard to test that something *doesn't* happen.
	obj := radclientv3.RadiusResource{
		ProxyResource: radclientv3.ProxyResource{
			Resource: radclientv3.Resource{
				Name: to.StringPtr("test-component"),
				Type: to.StringPtr("my/very/CoolResource"),
			},
		},
		Properties: &radclientv3.BasicComponentProperties{
			Status: &radclientv3.ComponentStatus{
				HealthState:       to.StringPtr("Healthy"),
				ProvisioningState: to.StringPtr("Provisioned"),
			},
		},
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `COMPONENT       TYPE          PROVISIONING_STATE  HEALTH_STATE
test-component  CoolResource  Provisioned         Healthy
`
	require.Equal(t, expected, buffer.String())
}

func Test_FormatDeploymentTable(t *testing.T) {
	options := GetDeploymentTableFormat()

	// We're just filling in the fields that are read. It's hard to test that something *doesn't* happen.
	components := []*radclient.DeploymentComponent{
		{
			ComponentName: to.StringPtr("frontend"),
		},
		{
			ComponentName: to.StringPtr("backend"),
		},
	}
	obj := radclient.DeploymentResource{
		TrackedResource: radclient.TrackedResource{
			Resource: radclient.Resource{
				Name: to.StringPtr("test-deployment"),
			},
		},
		Properties: &radclient.DeploymentProperties{
			Components: components,
		},
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `DEPLOYMENT       COMPONENTS
test-deployment  frontend backend
`
	require.Equal(t, expected, buffer.String())
}
