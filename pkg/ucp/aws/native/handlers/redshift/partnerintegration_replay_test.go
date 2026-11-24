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

package redshift

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// The recordings need a provisioned cluster, RADIUS_REC_CLUSTER, of the account RADIUS_REC_ACCOUNT, with a
// database dev.

// Test_Replay_PartnerIntegration adds Fivetran to a database of a cluster and deletes it, against AWS's responses.
// AWS returns no status until the partner reports one.
func Test_Replay_PartnerIntegration(t *testing.T) {
	s := awsreplay.New(t, "testdata/partnerintegration.json")
	api := redshift.NewFromConfig(s.Config(t.Context()))
	nativetest.Lifecycle{
		Handler: NewPartnerIntegration(api),
		Session: s,
		Properties: map[string]any{
			"AccountId":         s.Input("RADIUS_REC_ACCOUNT"),
			"ClusterIdentifier": s.Input("RADIUS_REC_CLUSTER"),
			"DatabaseName":      "dev",
			"PartnerName":       "Fivetran",
		},
		Poll: 5 * time.Second,
	}.Run(t)
}

// Test_Replay_PartnerIntegrationRejected adds a partner AWS doesn't support. AWS answers 404 with PartnerNotFound,
// which the handler reports as a rejected client failure, so UCP answers 400 and drops the record.
func Test_Replay_PartnerIntegrationRejected(t *testing.T) {
	s := awsreplay.New(t, "testdata/partnerintegrationrejected.json")
	api := redshift.NewFromConfig(s.Config(t.Context()))
	_, err := NewPartnerIntegration(api).Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{
		"AccountId":         s.Input("RADIUS_REC_ACCOUNT"),
		"ClusterIdentifier": s.Input("RADIUS_REC_CLUSTER"),
		"DatabaseName":      "dev",
		"PartnerName":       "Sisu",
	}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, partnerNotFound)
}
