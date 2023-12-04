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

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/stretchr/testify/require"
)

func Test_ResourceGroupFormat(t *testing.T) {
	obj := ucpv20231001preview.ResourceGroupResource{
		Name: to.Ptr("test"),
		ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group"),
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, ResourceGroupFormat())
	require.NoError(t, err)

	expected := "GROUP     ID\ntest      /planes/radius/local/resourceGroups/test-group\n"
	require.Equal(t, expected, buffer.String())
}
