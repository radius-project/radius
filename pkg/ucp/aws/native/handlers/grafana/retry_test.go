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

package grafana

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// Test_WorkspaceServiceAccount_CreateNotRetried checks that the SDK never sends a create again after an error, through
// the real SDK client. AWS may have made the service account before the error, and the create takes no token, so a
// retry would fail because the name is taken, and UCP would drop the record of a service account AWS has. Without the
// retry, the error leaves the outcome unknown, so the next deploy finds the service account by its name.
func Test_WorkspaceServiceAccount_CreateNotRetried(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			w.Header().Set("X-Amzn-Errortype", "InternalServerException")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message": "internal error"}`))
			return
		}
		w.Header().Set("X-Amzn-Errortype", "ConflictException")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message": "A service account with this name already exists."}`))
	}))
	t.Cleanup(server.Close)

	cfg := aws.Config{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		BaseEndpoint: aws.String(server.URL),
		Retryer: func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.Backoff = retry.BackoffDelayerFunc(func(int, error) (time.Duration, error) { return 0, nil })
			})
		},
	}
	h := NewWorkspaceServiceAccount(grafana.NewFromConfig(cfg))
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("ci")})
	require.Error(t, err)
	require.Equal(t, int32(1), requests.Load(), "the SDK sent the create again")
	var failure *native.Failure
	require.False(t, errors.As(err, &failure) && failure.Outcome == native.OutcomeRejected, "the outcome stays unknown: %v", err)

	// Other calls keep the SDK's retries.
	requests.Store(0)
	_, err = h.Read(t.Context(), region, workspace+"|1")
	require.Error(t, err)
	require.Equal(t, int32(2), requests.Load())
}
