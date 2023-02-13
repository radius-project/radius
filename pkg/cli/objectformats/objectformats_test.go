// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"bytes"
	"testing"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_GenericEnvTableFormat(t *testing.T) {
	obj := v20220315privatepreview.EnvironmentResource{
		Name: to.Ptr("test_env_resource"),
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, GetGenericEnvironmentTableFormat())
	require.NoError(t, err)

	expected := "NAME\ntest_env_resource\n"
	require.Equal(t, expected, buffer.String())
}

func Test_EnvTableFormat(t *testing.T) {
	obj := v20220315privatepreview.EnvironmentResource{
		Name: to.Ptr("test_env_resource"),
		Properties: &v20220315privatepreview.EnvironmentProperties{
			Providers: &v20220315privatepreview.Providers{
				Azure: &v20220315privatepreview.ProvidersAzure{
					Scope: to.Ptr("/subscriptions/testSubId/resourceGroups/testResourceGroup"),
				},
				Aws: &v20220315privatepreview.ProvidersAws{
					Scope: to.Ptr("/planes/aws/aws/accounts/testAccountId/regions/us-west-2"),
				},
			},
			UseDevRecipes: to.Ptr(false),
		},
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, GetEnvironmentTableFormat())
	require.NoError(t, err)

	expected := "ENV_NAME           AZURE                                                      AWS                                                       DEV_RECIPES\ntest_env_resource  /subscriptions/testSubId/resourceGroups/testResourceGroup  /planes/aws/aws/accounts/testAccountId/regions/us-west-2  false\n"
	require.Equal(t, expected, buffer.String())
}
