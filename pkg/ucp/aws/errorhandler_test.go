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

package aws

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"

	armrpc_v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

func Test_ConflictingRequestToken(t *testing.T) {
	const (
		clientToken  = "6c80ae98-2567-4d00-8451-b438022bb712"
		requestToken = "5cebd516-0f6b-44ca-aa7d-3b687f2bf7a3"
		// The message live Cloud Control returns.
		live = "ClientToken " + clientToken + " is already associated with an existing operation with RequestToken " + requestToken
	)
	conflict := func(message string) error {
		return &types.ClientTokenConflictException{Message: aws.String(message)}
	}

	tests := []struct {
		name        string
		err         error
		clientToken string
		want        string
	}{
		{
			name:        "the error the SDK returns",
			err:         &smithy.OperationError{ServiceID: "CloudControl", OperationName: "CreateResource", Err: &smithyhttp.ResponseError{Err: conflict(live)}},
			clientToken: clientToken,
			want:        requestToken,
		},
		{name: "the exception alone", err: conflict(live), clientToken: clientToken, want: requestToken},
		{name: "an upper-case request token", err: conflict("ClientToken " + clientToken + " ... RequestToken 5CEBD516-0F6B-44CA-AA7D-3B687F2BF7A3"), clientToken: clientToken, want: "5CEBD516-0F6B-44CA-AA7D-3B687F2BF7A3"},
		{name: "no error", err: nil, clientToken: clientToken},
		{name: "another error with the same message", err: &types.ThrottlingException{Message: aws.String(live)}, clientToken: clientToken},
		{name: "a plain error with the same message", err: errors.New(live), clientToken: clientToken},
		{name: "no message", err: &types.ClientTokenConflictException{}, clientToken: clientToken},
		{name: "no client token", err: conflict(live), clientToken: ""},
		{name: "a message for another client token", err: conflict(live), clientToken: "0d6f2a1e-9a43-4f6e-8a7e-6d7b3c2a1f00"},
		{name: "a message with no request token", err: conflict("ClientToken " + clientToken + " is already associated with an existing operation"), clientToken: clientToken},
		{name: "a request token that is not a UUID", err: conflict("ClientToken " + clientToken + " ... RequestToken not-a-uuid"), clientToken: clientToken},
		{name: "a request token longer than a UUID", err: conflict("ClientToken " + clientToken + " ... RequestToken " + requestToken + "0"), clientToken: clientToken},
		{name: "two request tokens", err: conflict(live + " and RequestToken " + clientToken), clientToken: clientToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ConflictingRequestToken(tt.err, tt.clientToken)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.want != "", ok)
		})
	}
}

func Test_HandleAWSError_Throttled(t *testing.T) {
	answer := func(status int, err error) error {
		return &smithy.OperationError{ServiceID: "CloudControl", OperationName: "CreateResource", Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}}, Err: err,
		}}
	}
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"throttling modeled as a client fault", answer(http.StatusBadRequest, &types.ThrottlingException{Message: aws.String("Rate exceeded")}), http.StatusTooManyRequests, armrpc_v1.CodeThrottled},
		{"throttling with a server fault", answer(http.StatusBadRequest, &smithy.GenericAPIError{Code: "Throttling", Message: "Rate exceeded", Fault: smithy.FaultServer}), http.StatusTooManyRequests, armrpc_v1.CodeThrottled},
		{"429 with another code", answer(http.StatusTooManyRequests, &smithy.GenericAPIError{Code: "SlowDown", Message: "Rate exceeded"}), http.StatusTooManyRequests, armrpc_v1.CodeThrottled},
		{"429 the SDK couldn't decode", answer(http.StatusTooManyRequests, &smithy.DeserializationError{Err: errors.New("Rate exceeded")}), http.StatusTooManyRequests, armrpc_v1.CodeThrottled},
		{"a quota the user must raise", answer(http.StatusBadRequest, &smithy.GenericAPIError{Code: "LimitExceededException", Message: "Rate exceeded", Fault: smithy.FaultClient}), http.StatusBadRequest, "LimitExceededException"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := HandleAWSError(t.Context(), tt.err)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			require.NoError(t, resp.Apply(t.Context(), w, httptest.NewRequest(http.MethodPut, "/", nil)))
			require.Equal(t, tt.status, w.Code)
			body := armrpc_v1.ErrorResponse{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tt.code, body.Error.Code)
			require.Contains(t, body.Error.Message, "Rate exceeded")
			if tt.code == armrpc_v1.CodeThrottled {
				require.Contains(t, body.Error.Message, "AWS limited the rate of calls. Deploy again later")
			}
		})
	}
}
