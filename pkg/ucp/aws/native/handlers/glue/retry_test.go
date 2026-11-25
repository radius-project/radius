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

package glue

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
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// Test_CreateNotRetried checks that the SDK never sends CreatePartitionIndex again after an error, through the real
// SDK client. AWS may have made the index before the error, which leaves it CREATING, so a retry could get Glue's
// error for a table busy with another index, which reads as a create AWS rejected, and UCP would drop the record of
// an index AWS has. Without the retry, the error leaves the outcome unknown, and the create returns the identifier.
func Test_CreateNotRetried(t *testing.T) {
	var creates, reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		switch r.Header.Get("X-Amz-Target") {
		case "AWSGlue.GetPartitionIndexes":
			if reads.Add(1) > 2 {
				_, _ = w.Write([]byte(`{"PartitionIndexDescriptorList":[]}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"__type":"InternalServiceException","Message":"internal error"}`))
		case "AWSGlue.CreatePartitionIndex":
			if creates.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"__type":"InternalServiceException","Message":"internal error"}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"__type":"ResourceNumberLimitExceededException","Message":"Index by_date is in CREATING state. Only 1 index can be created or deleted simultaneously per table."}`))
		default:
			t.Errorf("unexpected call %s", r.Header.Get("X-Amz-Target"))
		}
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
	id, err := NewPartitionIndex(glue.NewFromConfig(cfg)).Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
	require.Error(t, err)
	require.Equal(t, int32(1), creates.Load(), "the SDK sent the create again")
	require.Equal(t, int32(3), reads.Load(), "other calls keep the SDK's retries")
	var failure *native.Failure
	require.False(t, errors.As(err, &failure) && failure.Outcome == native.OutcomeRejected, "the outcome stays unknown: %v", err)
	require.Equal(t, account+"|sales|orders|by_date", id, "AWS may have made the index")
}
