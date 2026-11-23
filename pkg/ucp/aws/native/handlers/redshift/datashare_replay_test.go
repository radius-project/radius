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

// The recordings need two accounts. The producer account has a data share, RADIUS_REC_DATASHARE_ARN, made with
// CREATE DATASHARE and granted in SQL to the consumer account, RADIUS_REC_CONSUMER_ACCOUNT, with GRANT USAGE ON
// DATASHARE ... TO ACCOUNT. The authorization tests record as the producer account, with the data share not yet
// authorized. The consumer association tests record as the consumer account, after the producer authorized the data
// share, with a Redshift Serverless namespace of the consumer account, RADIUS_REC_CONSUMER_NAMESPACE_ARN, and the
// producer's account ID, RADIUS_REC_PRODUCER_ACCOUNT. Each test scrubs the other account's ID.

// Test_Replay_DataShareAuthorization authorizes a data share for a consumer account, allows writes, and removes the
// authorization, against AWS's responses. The create is done once AWS reports the data share AUTHORIZED, without the
// consumer. The cassette has the consumer's account ID as 444455556666 in place of the x characters the recording
// scrubbed it to, because ConsumerIdentifier must be 12 digits.
func Test_Replay_DataShareAuthorization(t *testing.T) {
	s := awsreplay.New(t, "testdata/datashareauthorization.json")
	consumer := s.Input("RADIUS_REC_CONSUMER_ACCOUNT")
	s.Scrub(consumer)
	api := redshift.NewFromConfig(s.Config(t.Context()))
	properties := map[string]any{
		"DataShareArn":       s.Input("RADIUS_REC_DATASHARE_ARN"),
		"ConsumerIdentifier": consumer,
	}
	nativetest.Lifecycle{
		Handler:     NewDataShareAuthorization(api),
		Session:     s,
		Properties:  properties,
		Want:        dataShareWith(properties, map[string]any{"AllowWrites": false, "Status": "AUTHORIZED"}),
		Update:      dataShareWith(properties, map[string]any{"AllowWrites": true}),
		WantUpdated: dataShareWith(properties, map[string]any{"AllowWrites": true, "Status": "AUTHORIZED"}),
		Poll:        5 * time.Second,
	}.Run(t)
}

// Test_Replay_DataShareAuthorizationRejected authorizes a data share for an account it isn't granted to. AWS answers
// 400 with InvalidDataShareFault, which the handler reports, with how to grant the data share, as a rejected client
// failure, so UCP answers 400 and drops the record.
func Test_Replay_DataShareAuthorizationRejected(t *testing.T) {
	s := awsreplay.New(t, "testdata/datashareauthorizationrejected.json")
	api := redshift.NewFromConfig(s.Config(t.Context()))
	h := NewDataShareAuthorization(api)
	arn := s.Input("RADIUS_REC_DATASHARE_ARN")
	_, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"DataShareArn": arn, "ConsumerIdentifier": "111122223333"}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, dataShareFault)
	require.Contains(t, failure.Message, "GRANT USAGE ON DATASHARE")
}

// Test_Replay_DataShareConsumerAssociation associates a data share another account authorized with a namespace of
// the consumer account, accepts writes, and removes the association, against AWS's responses.
func Test_Replay_DataShareConsumerAssociation(t *testing.T) {
	s := awsreplay.New(t, "testdata/datashareconsumerassociation.json")
	s.Scrub(s.Input("RADIUS_REC_PRODUCER_ACCOUNT"))
	api := redshift.NewFromConfig(s.Config(t.Context()))
	properties := map[string]any{
		"DataShareArn": s.Input("RADIUS_REC_DATASHARE_ARN"),
		"ConsumerArn":  s.Input("RADIUS_REC_CONSUMER_NAMESPACE_ARN"),
	}
	nativetest.Lifecycle{
		Handler:     NewDataShareConsumerAssociation(api),
		Session:     s,
		Properties:  properties,
		Want:        dataShareWith(properties, map[string]any{"AllowWrites": false, "AssociateEntireAccount": false}),
		Update:      dataShareWith(properties, map[string]any{"AllowWrites": true}),
		WantUpdated: dataShareWith(properties, map[string]any{"AllowWrites": true}),
		Poll:        5 * time.Second,
	}.Run(t)
}

// Test_Replay_DataShareConsumerAssociationRejected associates an authorized data share with a namespace the consumer
// account doesn't have, RADIUS_REC_MISSING_NAMESPACE_ARN, and with a namespace of another account. AWS answers 400 with
// InvalidNamespaceFault, and with InvalidParameterValue, which the SDK doesn't model. The handler reports both as
// rejected client failures, so UCP answers 400 and drops the record.
func Test_Replay_DataShareConsumerAssociationRejected(t *testing.T) {
	s := awsreplay.New(t, "testdata/datashareconsumerassociationrejected.json")
	s.Scrub(s.Input("RADIUS_REC_PRODUCER_ACCOUNT"))
	api := redshift.NewFromConfig(s.Config(t.Context()))
	h := NewDataShareConsumerAssociation(api)
	arn := s.Input("RADIUS_REC_DATASHARE_ARN")
	for _, tt := range []struct{ namespace, code string }{
		{s.Input("RADIUS_REC_MISSING_NAMESPACE_ARN"), dataShareNamespaceFault},
		{"arn:aws:redshift-serverless:us-west-2:111122223333:namespace/00000000-0000-0000-0000-000000000000", dataShareParameterFault},
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"DataShareArn": arn, "ConsumerArn": tt.namespace}})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure, tt.namespace)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
		require.Contains(t, failure.Message, tt.code)
	}
}
