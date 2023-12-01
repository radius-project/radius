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

package update

import (
	"bytes"
	"testing"

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/require"
)

func Test_environmentFormat(t *testing.T) {
	obj := environmentForDisplay{
		Name:        "test_env_resource",
		ComputeKind: "kubernetes",
		Recipes:     3,
		Providers:   2,
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, environmentFormat())
	require.NoError(t, err)

	expected := "NAME               COMPUTE     RECIPES   PROVIDERS\ntest_env_resource  kubernetes  3         2\n"
	require.Equal(t, expected, buffer.String())
}
