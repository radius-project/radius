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
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	baseURI            = "https://127.0.0.1:49176/apis/api.ucp.dev/v1alpha3"
	resourceID         = "/planes/radius/local/resourceGroups/kind-radius-wi/providers/Microsoft.Resources/deployments/rad-deploy-test"
	operationsEndpoint = "/operations"
)

func TestTryUnfoldResponseError(t *testing.T) {
	type args struct {
		resp *http.Response
	}
	tests := []struct {
		name string
		args args
		want *v1.ErrorDetails
	}{
		{
			name: "nil",
			args: args{
				resp: &http.Response{
					Status:     "404 Not Found",
					StatusCode: http.StatusNotFound,
					Body:       http.NoBody,
					Request: &http.Request{
						Method: http.MethodGet,
						URL:    parseURL(t, roleAssignmentBaseURL+"/"+roleAssigmentName+"?api-version="+apiVersion),
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respErr := runtime.NewResponseError(tt.args.resp)
			_ = TryUnfoldResponseError(respErr)
		})
	}
}

func TestUnfoldResponseError(t *testing.T) {
	type args struct {
		resp *http.Response
	}
	tests := []struct {
		name string
		args args
		code string
	}{
		{
			name: "deployment-failed",
			args: args{
				resp: &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{ "id": null, "error": { "code": "DeploymentFailed", "target": null, "message": "At least one resource deployment operation failed." } }`)),
					Request: &http.Request{
						Method: http.MethodGet,
						URL:    parseURL(t, baseURI+resourceID+operationsEndpoint+"?api-version="+apiVersion),
					},
				},
			},
			code: "DeploymentFailed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respErr := runtime.NewResponseError(tt.args.resp)
			errorDetails := TryUnfoldResponseError(respErr)
			require.Equal(t, tt.code, errorDetails.Code)
		})
	}
}

func Test_readResponseBody(t *testing.T) {
	type args struct {
		resp *http.Response
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "nil-body",
			args: args{
				resp: &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Body:       nil,
					Request: &http.Request{
						Method: http.MethodGet,
						URL:    parseURL(t, baseURI+resourceID+operationsEndpoint+"?api-version="+apiVersion),
					},
				},
			},
			want:    []byte{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readResponseBody(tt.args.resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("readResponseBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readResponseBody() = %v, want %v", got, tt.want)
			}
		})
	}
}
