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

package awsresources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func Test_TimedOut_Error(t *testing.T) {
	require.EqualError(t, &timedOut{timeout: time.Minute}, "the operation did not finish within 1m0s")
}

func Test_failureOf(t *testing.T) {
	request := &ctrl.Request{ResourceID: "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.RDS/TenantDatabase/app"}
	for _, tt := range []struct {
		failure *native.Failure
		code    string
	}{
		{&native.Failure{Message: "AWS limited the rate of calls", Throttled: true, Outcome: native.OutcomeRejected}, v1.CodeThrottled},
		{&native.Failure{Message: "invalid engine", ClientError: true}, v1.CodeInvalid},
		{&native.Failure{Message: "AWS failed"}, v1.CodeInternal},
	} {
		got := failureOf(tt.failure, request)
		require.Equal(t, v1.ErrorDetails{Code: tt.code, Message: tt.failure.Message, Target: request.ResourceID}, got)
	}
}
