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

type updateEnvObject struct {
	EnvName       string
	AzureSubId    string
	AzureRgId     string
	AWSAccountId  string
	AWSRegion     string
	UseDevRecipes bool
}

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
	obj := updateEnvObject{
		EnvName:       "test_env_resource",
		AzureSubId:    "testSubId",
		AzureRgId:     "testResourceGroup",
		AWSAccountId:  "testAccountId",
		AWSRegion:     "us-west-2",
		UseDevRecipes: true,
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, GetUpdateEnvironmentTableFormat())
	require.NoError(t, err)

	expected := "NAME               AZURE_SUBSCRIPTION  AZURE_RESOURCE_GROUP  AWS_ACCOUNT    AWS_REGION  DEV_RECIPES\ntest_env_resource  testSubId           testResourceGroup     testAccountId  us-west-2   true\n"
	require.Equal(t, expected, buffer.String())
}
