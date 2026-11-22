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

package neptune

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/neptune"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

const (
	retryEndpoint = `<DescribeDBClusterEndpointsResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/"><DescribeDBClusterEndpointsResult><DBClusterEndpoints><DBClusterEndpointList><DBClusterEndpointIdentifier>reads</DBClusterEndpointIdentifier><DBClusterIdentifier>graph</DBClusterIdentifier><Status>available</Status><EndpointType>CUSTOM</EndpointType><CustomEndpointType>READER</CustomEndpointType><StaticMembers><member>graph-1</member></StaticMembers><ExcludedMembers/><DBClusterEndpointArn>arn:aws:rds:us-west-2:123456789012:cluster-endpoint:reads</DBClusterEndpointArn></DBClusterEndpointList></DBClusterEndpoints></DescribeDBClusterEndpointsResult></DescribeDBClusterEndpointsResponse>`
	retryCluster  = `<DescribeDBClustersResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/"><DescribeDBClustersResult><DBClusters><DBCluster><DBClusterIdentifier>graph</DBClusterIdentifier><Engine>neptune</Engine><Status>available</Status></DBCluster></DBClusters></DescribeDBClustersResult></DescribeDBClustersResponse>`
	retryError    = `<ErrorResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/"><Error><Type>%s</Type><Code>%s</Code><Message>%s</Message></Error></ErrorResponse>`
)

// Test_ModifyNotRetried checks that the SDK never sends ModifyDBClusterEndpoint again after an error, through the real
// SDK client. AWS may have taken the change before the error, which leaves the endpoint modifying, so a retry gets
// InvalidDBClusterEndpointStateFault, which would read as a change AWS rejected. Without the retry, the error leaves
// the outcome unknown.
func Test_ModifyNotRetried(t *testing.T) {
	var modifies atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "text/xml")
		switch r.Form.Get("Action") {
		case "DescribeDBClusterEndpoints":
			_, _ = w.Write([]byte(retryEndpoint))
		case "DescribeDBClusters":
			_, _ = w.Write([]byte(retryCluster))
		case "ModifyDBClusterEndpoint":
			if modifies.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write(fmt.Appendf(nil, retryError, "Receiver", "InternalFailure", "internal error"))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(fmt.Appendf(nil, retryError, "Sender", "InvalidDBClusterEndpointStateFault", "the endpoint is modifying"))
		default:
			t.Errorf("unexpected call %s", r.Form.Get("Action"))
		}
	}))
	t.Cleanup(server.Close)

	h := NewClusterEndpoint(neptune.NewFromConfig(retryConfig(server.URL)))
	current := map[string]any{"DBClusterEndpointIdentifier": "reads", "DBClusterIdentifier": "graph", "EndpointType": "READER", "StaticMembers": []any{"graph-1"}}
	desired := with(current, map[string]any{"EndpointType": "ANY"})
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Current: current, Desired: desired, Patch: patch})
	require.Error(t, err)
	require.Equal(t, int32(1), modifies.Load(), "the SDK sent the modify again")
	var failure *native.Failure
	require.False(t, errors.As(err, &failure) && failure.Outcome == native.OutcomeRejected, "the outcome stays unknown: %v", err)
}

// retryConfig returns a config for an AWS API at url, whose SDK retries a failed call at once.
func retryConfig(url string) aws.Config {
	return aws.Config{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		BaseEndpoint: aws.String(url),
		Retryer: func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.Backoff = retry.BackoffDelayerFunc(func(int, error) (time.Duration, error) { return 0, nil })
			})
		},
	}
}
