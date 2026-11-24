/*
Copyright 2026 The Radius Authors.

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

package rds

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// retryRegion is the region of the retry tests' client.
const retryRegion = "us-west-2"

// retryAnswers are what the retry tests' server answers each action with after its first request, which always fails
// with InternalFailure. A read then succeeds. A change answers as AWS would answer the same call sent again after the
// first one reached AWS: the tenant database already exists, the instance is busy with the change, or the final
// snapshot or the stream is already being made.
var retryAnswers = map[string]string{
	"DescribeDBInstances":     `<DescribeDBInstancesResponse><DescribeDBInstancesResult><DBInstances><DBInstance><DBInstanceIdentifier>app-cdb</DBInstanceIdentifier><DBInstanceStatus>available</DBInstanceStatus></DBInstance></DBInstances></DescribeDBInstancesResult></DescribeDBInstancesResponse>`,
	"DescribeTenantDatabases": `<DescribeTenantDatabasesResponse><DescribeTenantDatabasesResult><TenantDatabases><TenantDatabase><DBInstanceIdentifier>app-cdb</DBInstanceIdentifier><TenantDBName>pdb1</TenantDBName><Status>available</Status><TenantDatabaseARN>arn:aws:rds:us-west-2:123456789012:db:app-cdb:pdb1</TenantDatabaseARN></TenantDatabase></TenantDatabases></DescribeTenantDatabasesResult></DescribeTenantDatabasesResponse>`,
	"CreateTenantDatabase":    "TenantDatabaseAlreadyExists",
	"ModifyTenantDatabase":    "InvalidDBInstanceState",
	"DeleteTenantDatabase":    "DBSnapshotAlreadyExists",
	"StartActivityStream":     "InvalidDBClusterStateFault",
}

// retryServer is an RDS endpoint that fails the first request of each action with firstCode, and answers later ones
// from retryAnswers, or with success for a change when firstCode is a throttle. It counts the requests of each action.
type retryServer struct {
	firstCode string

	mu       sync.Mutex
	requests map[string]int
}

func (s *retryServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	action := r.Form.Get("Action")
	s.mu.Lock()
	s.requests[action]++
	first := s.requests[action] == 1
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/xml")
	answer := retryAnswers[action]
	switch {
	case first && s.firstCode == throttled:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>Rate exceeded</Message></Error></ErrorResponse>`, throttled)
	case first:
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `<ErrorResponse><Error><Type>Receiver</Type><Code>InternalFailure</Code><Message>internal error</Message></Error></ErrorResponse>`)
	case answer != "" && answer[0] == '<':
		_, _ = fmt.Fprint(w, answer)
	case s.firstCode == throttled:
		_, _ = fmt.Fprintf(w, `<%[1]sResponse><%[1]sResult></%[1]sResult></%[1]sResponse>`, action)
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>sent again</Message></Error></ErrorResponse>`, answer)
	}
}

// throttled is RDS's error code for a throttled call.
const throttled = "Throttling"

// newRetryClient returns the real RDS client, with the SDK's standard retries and no backoff, sending to a new
// retryServer that fails each action's first request with firstCode.
func newRetryClient(t *testing.T, firstCode string) (*rds.Client, *retryServer) {
	s := &retryServer{firstCode: firstCode, requests: map[string]int{}}
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	return rds.NewFromConfig(aws.Config{
		Region:       retryRegion,
		Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		BaseEndpoint: aws.String(server.URL),
		Retryer: func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.Backoff = retry.BackoffDelayerFunc(func(int, error) (time.Duration, error) { return 0, nil })
				o.RateLimiter = ratelimit.None
			})
		},
	}), s
}

// Test_ChangesNotRetried checks that the SDK sends each change once, unless AWS throttled it. None takes an idempotency
// token, so the SDK's retry of one whose first attempt reached AWS gets an error, such as TenantDatabaseAlreadyExists,
// that UCP would read as rejected, and it would drop the record of a change AWS has. Sent once, the InternalFailure
// leaves the outcome unknown, and the next deploy reads what AWS has. Reads keep the SDK's retries.
func Test_ChangesNotRetried(t *testing.T) {
	cases := []struct {
		action string
		call   func(t *testing.T, api *rds.Client) error
	}{
		{"CreateTenantDatabase", func(t *testing.T, api *rds.Client) error {
			_, err := NewTenantDatabase(api).Create(t.Context(), native.CreateRequest{Region: retryRegion, Properties: tenant})
			return err
		}},
		{"ModifyTenantDatabase", func(t *testing.T, api *rds.Client) error {
			return NewTenantDatabase(api).Update(t.Context(), native.UpdateRequest{
				Region: retryRegion, Identifier: "app-cdb|pdb1", Current: tenant,
				Desired: with(tenant, map[string]any{"MasterUserPassword": "pass-two-5678"}),
				Patch:   jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/MasterUserPassword", Value: "pass-two-5678"}},
			})
		}},
		{"DeleteTenantDatabase", func(t *testing.T, api *rds.Client) error {
			return NewTenantDatabase(api).Delete(t.Context(), retryRegion, "app-cdb|pdb1")
		}},
		{"StartActivityStream", func(t *testing.T, api *rds.Client) error {
			_, err := NewActivityStream(api).Create(t.Context(), native.CreateRequest{Region: retryRegion, Properties: stream})
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.action, func(t *testing.T) {
			api, server := newRetryClient(t, "InternalFailure")
			err := c.call(t, api)
			require.Error(t, err)
			require.Equal(t, 1, server.requests[c.action], "the SDK sent %s again", c.action)
			var failure *native.Failure
			require.False(t, errors.As(err, &failure) && failure.Outcome == native.OutcomeRejected, "the outcome stays unknown: %v", err)
			for action, n := range server.requests {
				if action != c.action {
					require.Equal(t, 2, n, "the SDK retries %s", action)
				}
			}
		})

		// AWS refuses a throttled call before acting on it, so the SDK sends it again.
		t.Run(c.action+" throttled", func(t *testing.T) {
			api, server := newRetryClient(t, throttled)
			require.NoError(t, c.call(t, api))
			require.Equal(t, 2, server.requests[c.action])
		})
	}
}
