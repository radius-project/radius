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
	obj := OutputEnvObject{
		EnvName:     "test_env_resource",
		ComputeKind: "kubernetes",
		Recipes:     3,
		Providers:   2,
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, GetUpdateEnvironmentTableFormat())
	require.NoError(t, err)

	expected := "NAME               COMPUTE     RECIPES   PROVIDERS\ntest_env_resource  kubernetes  3         2\n"
	require.Equal(t, expected, buffer.String())
}
