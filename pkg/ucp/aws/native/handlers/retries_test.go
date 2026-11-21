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

package handlers

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
)

// Test_Replay_Retries checks how a handler from All reports an error the SDK got after retrying a call, against RDS's
// error responses: a 4xx after an attempt AWS didn't answer with a 4xx, such as a 5xx whose create AWS may have
// applied, leaves the outcome unknown, and a 4xx on every attempt is a call AWS rejected, which is not the request's
// fault when AWS throttled it. A failed read before the create sends nothing, so it applied none of it.
func Test_Replay_Retries(t *testing.T) {
	if awsreplay.Recording() {
		t.Skip("the cassette is written by hand from RDS's error responses; recording it needs AWS to fail on purpose")
	}
	s := awsreplay.New(t, "testdata/retries.json")
	cfg := s.Config(t.Context())
	cfg.RetryMaxAttempts = 3
	cfg.Retryer = func() aws.Retryer {
		return retry.NewStandard(func(o *retry.StandardOptions) {
			o.Backoff = retry.BackoffDelayerFunc(func(int, error) (time.Duration, error) { return 0, nil })
		})
	}
	var tenants native.Handler
	for _, h := range All(cfg) {
		if h.TypeName() == "AWS::RDS::TenantDatabase" {
			tenants = h
		}
	}
	require.NotNil(t, tenants)
	create := func() error {
		_, err := tenants.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{
			"DBInstanceIdentifier": s.Input("RADIUS_REC_ORACLE_CDB_INSTANCE"),
			"TenantDBName":         "pdbrec",
			"MasterUsername":       "admin",
			"MasterUserPassword":   "Abc@12345678",
		}})
		return err
	}

	requireUnknown(t, create(), "TenantDatabaseAlreadyExists", "an earlier attempt may have reached AWS")
	var failure *native.Failure
	require.ErrorAs(t, create(), &failure)
	require.False(t, failure.ClientError, "throttling isn't the request's fault")
	require.True(t, failure.Throttled)
	require.Equal(t, native.OutcomeRejected, failure.Outcome, "AWS throttled every attempt, so none applied")
	require.Contains(t, failure.Message, "AWS limited the rate of calls, so it changed nothing. Deploy again later")

	requireUnknown(t, create(), "InvalidDBInstanceState", "an earlier attempt may have reached AWS")

	require.ErrorAs(t, create(), &failure)
	require.True(t, failure.ClientError, "AWS answered every attempt with a 4xx, so none applied")
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "InvalidParameterValue")

	require.ErrorAs(t, create(), &failure)
	require.False(t, failure.ClientError, "throttling isn't the request's fault")
	require.True(t, failure.Throttled)
	require.Equal(t, native.OutcomeRejected, failure.Outcome, "a failed read of the instance sends no create, however the SDK retried it")
	require.Contains(t, failure.Message, "Throttling")
	require.NotContains(t, failure.Message, "earlier attempt", "the read before the create changed nothing")
}

// requireUnknown requires err to be a failure whose outcome is unknown, which UCP answers with 500 and whose values it
// keeps as possibly applied, naming AWS's code and saying why.
func requireUnknown(t *testing.T, err error, code string, why string) {
	t.Helper()
	var failure *native.Failure
	require.True(t, errors.As(err, &failure), "%v", err)
	require.False(t, failure.ClientError)
	require.False(t, failure.Conflict)
	require.False(t, failure.Throttled)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome)
	require.Contains(t, failure.Message, code)
	require.Contains(t, failure.Message, why)
}
