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

package status

import (
	"bytes"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/require"
)

func Test_GetApplicationStatusTableFormat(t *testing.T) {
	obj := clients.ApplicationStatus{
		Name:          "test",
		ResourceCount: 3,
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, statusFormat())
	require.NoError(t, err)

	expected := "APPLICATION  RESOURCES\ntest         3\n"
	require.Equal(t, expected, buffer.String())
}

func Test_GetApplicationGatewaysTableFormat(t *testing.T) {
	obj := clients.GatewayStatus{
		Name:     "test",
		Endpoint: "test-endpoint",
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, gatewayFormat())
	require.NoError(t, err)

	expected := "GATEWAY   ENDPOINT\ntest      test-endpoint\n"
	require.Equal(t, expected, buffer.String())
}
