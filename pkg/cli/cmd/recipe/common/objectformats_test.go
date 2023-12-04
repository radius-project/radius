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

package common

import (
	"bytes"
	"testing"

	types "github.com/radius-project/radius/pkg/cli/cmd/recipe"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/require"
)

func Test_RecipeFormat(t *testing.T) {
	obj := types.EnvironmentRecipe{
		Name:            "test",
		ResourceType:    "test-type",
		TemplateKind:    "test-kind",
		TemplatePath:    "test-path",
		TemplateVersion: "test-version",
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, RecipeFormat())
	require.NoError(t, err)

	expected := "RECIPE    TYPE       TEMPLATE KIND  TEMPLATE VERSION  TEMPLATE\ntest      test-type  test-kind      test-version      test-path\n"
	require.Equal(t, expected, buffer.String())
}

func Test_RecipeParametersFormat(t *testing.T) {
	obj := types.RecipeParameter{
		Name:         "test",
		DefaultValue: 1,
		Type:         "test-type",
		MaxValue:     "3",
		MinValue:     "4",
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, RecipeParametersFormat())
	require.NoError(t, err)

	expected := "PARAMETER  TYPE       DEFAULT VALUE  MIN       MAX\ntest       test-type  1              4         3\n"
	require.Equal(t, expected, buffer.String())
}
