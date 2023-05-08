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

package clientv2

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/stretchr/testify/require"
)

var (
	roleAssignmentBaseURL = "https://management.azure.com/subscriptions/00000000-0000-0000-0000-000000000000/providers/Microsoft.Authorization/roleAssignments"
	roleAssigmentName     = "test-role-assignment"
	apiVersion            = "2022-04-01"
)

func parseURL(t *testing.T, path string) *url.URL {
	u, err := url.Parse(path)
	if err != nil {
		t.Fatal(err)
	}

	return u
}

func TestExtractResponseError_WithResponseError(t *testing.T) {
	tests := []struct {
		name string

		// opResp is the response of the client pipeline.
		opResp *http.Response

		errorCode string
		ok        bool
		err       error
	}{
		{
			name: "create-role-assignment-bad-request",
			opResp: &http.Response{
				Status:     "400 Bad Request",
				StatusCode: http.StatusBadRequest,
				Body: io.NopCloser(strings.NewReader(`
				<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN""http://www.w3.org/TR/html4/strict.dtd">
				<HTML><HEAD><TITLE>Bad Request</TITLE>
				<META HTTP-EQUIV="Content-Type" Content="text/html; charset=us-ascii"></HEAD>
				<BODY><h2>Bad Request</h2>
				<hr><p>HTTP Error 400. The request is badly formed.</p>
				</BODY></HTML>
				`)),
				Request: &http.Request{
					Method: http.MethodPost,
					URL:    parseURL(t, roleAssignmentBaseURL),
				},
			},
			errorCode: "",
			ok:        true,
			err:       nil,
		},
		{
			name: "delete-role-assignment-by-id",
			opResp: &http.Response{
				Status:     "500 Internal Server Error",
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{ "code": "Database is down", "message": "Database is down" }`)),
				Request: &http.Request{
					Method: http.MethodDelete,
					URL:    parseURL(t, roleAssignmentBaseURL+"/"+roleAssigmentName+"?api-version="+apiVersion),
				},
			},
			errorCode: "Database is down",
			ok:        true,
			err:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// get the response error
			err := runtime.NewResponseError(tt.opResp)

			// send the response error to ExtractResponseError
			respErr, ok := ExtractResponseError(err)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.opResp.StatusCode, respErr.StatusCode)
			require.Equal(t, tt.errorCode, respErr.ErrorCode)
			require.Equal(t, tt.opResp, respErr.RawResponse)
		})
	}
}

func TestExtractResponseError_WithError(t *testing.T) {
	tests := []struct {
		name      string
		clientErr error
		errorCode string
		respErr   *azcore.ResponseError
		ok        bool
	}{
		{
			name:      "create-role-assignment-internal-error",
			clientErr: errors.New("internal error"),
			ok:        false,
			respErr:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// send the response error to ExtractResponseError
			respErr, ok := ExtractResponseError(tt.clientErr)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.respErr, respErr)
		})
	}
}

func TestIs404Error(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "404-error",
			args: args{
				err: runtime.NewResponseError(
					&http.Response{
						Status:     "404 Not Found",
						StatusCode: http.StatusNotFound,
						Body:       http.NoBody,
						Request: &http.Request{
							Method: http.MethodGet,
							URL:    parseURL(t, roleAssignmentBaseURL+"/"+roleAssigmentName+"?api-version="+apiVersion),
						},
					},
				),
			},
			want: true,
		},
		{
			name: "not-404-error",
			args: args{
				err: runtime.NewResponseError(
					&http.Response{
						Status:     "500 Internal Server Error",
						StatusCode: http.StatusInternalServerError,
						Body:       http.NoBody,
						Request: &http.Request{
							Method: http.MethodDelete,
							URL:    parseURL(t, roleAssignmentBaseURL+"/"+roleAssigmentName+"?api-version="+apiVersion),
						},
					},
				),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Is404Error(tt.args.err); got != tt.want {
				t.Errorf("Is404Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
