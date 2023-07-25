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

package integrationtests

import (
	"net/http"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/integrationtests/testserver"
	"github.com/stretchr/testify/require"
)

func Test_Handler_MethodNotAllowed(t *testing.T) {
	ucp, _, _ := testserver.StartWithMocks(t, testserver.NoModules)

	response := ucp.MakeRequest(http.MethodDelete, "/planes?api-version=2022-09-01-privatepreview", nil)
	require.Equal(t, "failed to parse route: undefined route path", response.Error.Error.Details[0].Message)
}

func Test_Handler_NotFound(t *testing.T) {
	ucp, _, _ := testserver.StartWithMocks(t, testserver.NoModules)

	response := ucp.MakeRequest(http.MethodGet, "/abc", nil)
	response.EqualsErrorCode(http.StatusNotFound, v1.CodeNotFound)
	require.Regexp(t, "The request 'GET /.*/abc' is invalid.", response.Error.Error.Message)
}
