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

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

func Test_HandlerErrModelConversion(t *testing.T) {
	var handlerTest = struct {
		url    string
		method string
	}{
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2022-03-15-privatepreview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	err := &v1.ErrModelConversion{PropertyName: "namespace", ValidValue: "63 characters or less"}
	HandleError(context.Background(), responseWriter, req, err)

	bodyBytes, e := io.ReadAll(responseWriter.Body)
	require.NoError(t, e)
	armerr := v1.ErrorResponse{}
	e = json.Unmarshal(bodyBytes, &armerr)
	require.NoError(t, e)
	require.Equal(t, v1.CodeHTTPRequestPayloadAPISpecValidationFailed, armerr.Error.Code)
	require.Equal(t, armerr.Error.Message, "namespace must be 63 characters or less.")
}

func Test_HandlerErrInvalidModelConversion(t *testing.T) {
	var handlerTest = struct {
		url    string
		method string
	}{
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2022-03-15-privatepreview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	HandleError(context.Background(), responseWriter, req, v1.ErrInvalidModelConversion)

	bodyBytes, e := io.ReadAll(responseWriter.Body)
	require.NoError(t, e)
	armerr := v1.ErrorResponse{}
	e = json.Unmarshal(bodyBytes, &armerr)
	require.NoError(t, e)
	require.Equal(t, v1.CodeHTTPRequestPayloadAPISpecValidationFailed, armerr.Error.Code)
	require.Equal(t, armerr.Error.Message, "invalid model conversion")
}

func Test_HandlerErrInternal(t *testing.T) {
	var handlerTest = struct {
		url    string
		method string
	}{
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2022-03-15-privatepreview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	err := errors.New("Internal error")
	HandleError(context.Background(), responseWriter, req, err)

	bodyBytes, e := io.ReadAll(responseWriter.Body)
	require.NoError(t, e)
	armerr := v1.ErrorResponse{}
	e = json.Unmarshal(bodyBytes, &armerr)
	require.NoError(t, e)
	require.Equal(t, v1.CodeInternal, armerr.Error.Code)
	require.Equal(t, armerr.Error.Message, "Internal error")
}
