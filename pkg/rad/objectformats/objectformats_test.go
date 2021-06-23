// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"bytes"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/rad/output"
	"github.com/Azure/radius/pkg/radclient"
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
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `APPLICATION
test-app  
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
	}

	buffer := bytes.Buffer{}
	err := output.Write(output.FormatTable, &obj, &buffer, options)
	require.NoError(t, err)

	expected := `COMPONENT       KIND
test-component  radius.dev/TestComponent@v1alpha1  
`
	require.Equal(t, expected, buffer.String())
}

func Test_FormatDeploymentTable(t *testing.T) {
	options := GetDeploymentTableFormat()

	// We're just filling in the fields that are read. It's hard to test that something *doesn't* happen.
	components := []radclient.DeploymentPropertiesComponentsItem{
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
			Components: &components,
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
