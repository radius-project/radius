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

package opensearch

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_VpcEndpoint creates an endpoint to a VPC domain with one security group, updates it to leave the
// security groups out, which puts back the VPC's default group, and deletes it, against AWS's responses.
func Test_Replay_VpcEndpoint(t *testing.T) {
	s := awsreplay.New(t, "testdata/vpcendpoint.json")
	cfg := s.Config(t.Context())
	subnet := s.Input("RADIUS_REC_SUBNET_ID")
	properties := map[string]any{
		"DomainArn":  s.Input("RADIUS_REC_OPENSEARCH_DOMAIN_ARN"),
		"VpcOptions": map[string]any{"SubnetIds": []any{subnet}, "SecurityGroupIds": []any{s.Input("RADIUS_REC_SECURITY_GROUP_ID")}},
	}
	update := map[string]any{"DomainArn": properties["DomainArn"], "VpcOptions": map[string]any{"SubnetIds": []any{subnet}}}
	nativetest.Lifecycle{
		Handler:     NewVpcEndpoint(opensearch.NewFromConfig(cfg), ec2.NewFromConfig(cfg)),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"Status": "ACTIVE"}),
		Update:      update,
		WantUpdated: with(update, map[string]any{"Status": "ACTIVE"}),
		Poll:        20 * time.Second,
	}.Run(t)
}

// Test_Replay_VpcEndpointErrors runs AWS's errors through the SDK: a create while AWS is changing the domain, which
// AWS answers with 409 ConflictException, must be a conflict, and an update AWS answers with 400 ValidationException,
// an error the SDK's model doesn't list for the call, must be rejected, so UCP answers 400, not 500.
func Test_Replay_VpcEndpointErrors(t *testing.T) {
	s := awsreplay.New(t, "testdata/vpcendpointerrors.json")
	cfg := s.Config(t.Context())
	h := NewVpcEndpoint(opensearch.NewFromConfig(cfg), ec2.NewFromConfig(cfg))
	subnet := s.Input("RADIUS_REC_SUBNET_ID")

	_, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, ClientToken: "token-1", Properties: map[string]any{
		"DomainArn":  s.Input("RADIUS_REC_OPENSEARCH_DOMAIN_ARN"),
		"VpcOptions": map[string]any{"SubnetIds": []any{subnet}},
	}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)

	err = h.Update(t.Context(), native.UpdateRequest{
		Region:     awsreplay.Region,
		Identifier: "aos-0a1b2c3d4e5f6a7b8",
		Desired:    map[string]any{"VpcOptions": map[string]any{"SubnetIds": []any{}}},
		Patch:      jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/VpcOptions/SubnetIds", Value: []any{}}},
	})
	require.ErrorAs(t, err, &failure)
	require.False(t, failure.Conflict)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "ValidationException")
}

// Test_Replay_VpcEndpointAccess authorizes another account on a VPC domain and revokes it, against AWS's responses.
// Every property is create-only, so there is no update step.
func Test_Replay_VpcEndpointAccess(t *testing.T) {
	s := awsreplay.New(t, "testdata/vpcendpointaccess.json")
	properties := map[string]any{
		"DomainName": s.Input("RADIUS_REC_OPENSEARCH_DOMAIN"),
		"Account":    s.Input("RADIUS_REC_OTHER_ACCOUNT_ID"),
	}
	nativetest.Lifecycle{
		Handler:    NewVpcEndpointAccess(opensearch.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties,
		Want:       with(properties, map[string]any{"PrincipalType": "AWS_ACCOUNT"}),
		Poll:       5 * time.Second,
	}.Run(t)
}

// Test_Replay_VpcEndpointAccessErrors runs AWS's answer for a domain that doesn't exist, 409
// ResourceNotFoundException, through the SDK: a create is rejected, so UCP answers 400, and a read is not found.
func Test_Replay_VpcEndpointAccessErrors(t *testing.T) {
	s := awsreplay.New(t, "testdata/vpcendpointaccesserrors.json")
	h := NewVpcEndpointAccess(opensearch.NewFromConfig(s.Config(t.Context())))
	account := s.Input("RADIUS_REC_OTHER_ACCOUNT_ID")

	_, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"DomainName": "radius-rec-missing", "Account": account}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.False(t, errors.Is(err, native.ErrNotFound))

	_, err = h.Read(t.Context(), awsreplay.Region, "radius-rec-missing|"+account)
	require.ErrorIs(t, err, native.ErrNotFound)
}
