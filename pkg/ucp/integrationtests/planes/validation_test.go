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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/integrationtests/testserver"
	"github.com/stretchr/testify/require"
)

const (
	invalidAPIVersionErrorMessage = "API version 'unsupported-version' for type 'ucp/openapi' is not supported. The supported api-versions are '2022-09-01-privatepreview'."
)

func Test_Planes_GET_BadAPIVersion(t *testing.T) {
	ucp, _, _ := testserver.StartWithMocks(t, api.DefaultModules)

	response := ucp.MakeRequest(http.MethodGet, "/planes?api-version=unsupported-version", nil)
	response.EqualsErrorCode(http.StatusBadRequest, v1.CodeInvalidApiVersionParameter)
	require.Equal(t, invalidAPIVersionErrorMessage, response.Error.Error.Message)
}

func Test_Planes_PUT_BadAPIVersion(t *testing.T) {
	ucp, _, _ := testserver.StartWithMocks(t, api.DefaultModules)

	requestBody := v20220901privatepreview.PlaneResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20220901privatepreview.PlaneResourceProperties{},
	}

	response := ucp.MakeTypedRequest(http.MethodPut, "/planes/radius/local?api-version=unsupported-version", requestBody)
	response.EqualsErrorCode(http.StatusBadRequest, v1.CodeInvalidApiVersionParameter)
	require.Equal(t, invalidAPIVersionErrorMessage, response.Error.Error.Message)
}
