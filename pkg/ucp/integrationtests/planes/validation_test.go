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

package planes

import (
	"net/http"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/stretchr/testify/require"
)

const (
	invalidAPIVersionErrorMessage = "API version 'unsupported-version' for type 'ucp/openapi' is not supported. The supported api-versions are '2023-10-01-preview'."
)

func Test_Planes_GET_BadAPIVersion(t *testing.T) {
	ucp := testserver.StartWithMocks(t, api.DefaultModules)

	response := ucp.MakeRequest(http.MethodGet, "/planes?api-version=unsupported-version", nil)
	response.EqualsErrorCode(http.StatusBadRequest, v1.CodeInvalidApiVersionParameter)
	require.Equal(t, invalidAPIVersionErrorMessage, response.Error.Error.Message)
}

func Test_Planes_PUT_BadAPIVersion(t *testing.T) {
	ucp := testserver.StartWithMocks(t, api.DefaultModules)

	requestBody := v20231001preview.RadiusPlaneResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.RadiusPlaneResourceProperties{},
	}

	response := ucp.MakeTypedRequest(http.MethodPut, "/planes/radius/local?api-version=unsupported-version", requestBody)
	response.EqualsErrorCode(http.StatusBadRequest, v1.CodeInvalidApiVersionParameter)
	require.Equal(t, invalidAPIVersionErrorMessage, response.Error.Error.Message)
}
