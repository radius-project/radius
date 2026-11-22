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

package redshiftserverless

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// The recordings need a Redshift Serverless workgroup, RADIUS_REC_WORKGROUP, whose ARN is RADIUS_REC_WORKGROUP_ARN,
// a subnet of its VPC, RADIUS_REC_SUBNET, and two security groups of that VPC, RADIUS_REC_SG and RADIUS_REC_SG2.

// Test_Replay_UsageLimit creates a usage limit of a workgroup, changes its amount and breach action, and deletes it,
// against AWS's responses. Each change leaves the workgroup MODIFYING for a while, which the handler waits for. The
// last read of the deleted limit gets ValidationException, which the handler takes as not found.
func Test_Replay_UsageLimit(t *testing.T) {
	s := awsreplay.New(t, "testdata/usagelimit.json")
	api := redshiftserverless.NewFromConfig(s.Config(t.Context()))
	properties := map[string]any{
		"ResourceArn":  s.Input("RADIUS_REC_WORKGROUP_ARN"),
		"UsageType":    "serverless-compute",
		"Amount":       float64(10),
		"Period":       "daily",
		"BreachAction": "log",
	}
	updated := with(properties, map[string]any{"Amount": float64(20), "BreachAction": "emit-metric"})
	nativetest.Lifecycle{
		Handler:    NewUsageLimit(api),
		Session:    s,
		Properties: properties,
		Update:     updated,
		Poll:       5 * time.Second,
	}.Run(t)
}

// Test_Replay_EndpointAccess creates a managed VPC endpoint of a workgroup, changes its security group, and deletes
// it, against AWS's responses.
func Test_Replay_EndpointAccess(t *testing.T) {
	s := awsreplay.New(t, "testdata/endpointaccess.json")
	api := redshiftserverless.NewFromConfig(s.Config(t.Context()))
	properties := map[string]any{
		"EndpointName":        "radius-rec-" + s.Input("RADIUS_REC_RUN"),
		"WorkgroupName":       s.Input("RADIUS_REC_WORKGROUP"),
		"SubnetIds":           []any{s.Input("RADIUS_REC_SUBNET")},
		"VpcSecurityGroupIds": []any{s.Input("RADIUS_REC_SG")},
	}
	updated := with(properties, map[string]any{"VpcSecurityGroupIds": []any{s.Input("RADIUS_REC_SG2")}})
	nativetest.Lifecycle{
		Handler:     NewEndpointAccess(api),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"EndpointStatus": endpointActive}),
		Update:      updated,
		WantUpdated: with(updated, map[string]any{"EndpointStatus": endpointActive}),
		Poll:        15 * time.Second,
	}.Run(t)
}

// Test_Replay_EndpointAccessWorkgroupBusy creates an endpoint right after a usage limit, while the usage limit leaves
// the workgroup MODIFYING. AWS answers ConflictException, which the handler reports as a conflict AWS applied none of,
// so UCP answers 409 and the caller deploys again.
func Test_Replay_EndpointAccessWorkgroupBusy(t *testing.T) {
	s := awsreplay.New(t, "testdata/endpointaccessbusy.json")
	api := redshiftserverless.NewFromConfig(s.Config(t.Context()))
	limits := NewUsageLimit(api)
	id, err := limits.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{
		"ResourceArn": s.Input("RADIUS_REC_WORKGROUP_ARN"),
		"UsageType":   "serverless-compute",
		"Amount":      float64(10),
	}})
	require.NoError(t, err)
	t.Cleanup(func() {
		// The test's context has ended by now, so the delete uses one that hasn't.
		require.NoError(t, limits.Delete(context.WithoutCancel(t.Context()), awsreplay.Region, id))
	})

	workgroup := s.Input("RADIUS_REC_WORKGROUP")
	identifier, err := NewEndpointAccess(api).Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{
		"EndpointName":        "radius-rec-" + s.Input("RADIUS_REC_RUN"),
		"WorkgroupName":       workgroup,
		"SubnetIds":           []any{s.Input("RADIUS_REC_SUBNET")},
		"VpcSecurityGroupIds": []any{s.Input("RADIUS_REC_SG")},
	}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "The workgroup "+workgroup+" is busy")
	require.Contains(t, failure.Message, conflict)
	require.Empty(t, identifier, "AWS made nothing, so a recording has nothing to clean up")
}

// Test_Replay_CustomDomainAssociation associates a domain with a workgroup, changes its certificate, and removes it,
// against responses written from the API reference: the recording account has no certificate Amazon issued, which the
// call requires.
func Test_Replay_CustomDomainAssociation(t *testing.T) {
	s := awsreplay.New(t, "testdata/customdomainassociation.json")
	api := redshiftserverless.NewFromConfig(s.Config(t.Context()))
	properties := map[string]any{
		"WorkgroupName":              s.Input("RADIUS_REC_WORKGROUP"),
		"CustomDomainName":           s.Input("RADIUS_REC_DOMAIN"),
		"CustomDomainCertificateArn": s.Input("RADIUS_REC_CERTIFICATE"),
	}
	updated := with(properties, map[string]any{"CustomDomainCertificateArn": s.Input("RADIUS_REC_CERTIFICATE2")})
	nativetest.Lifecycle{
		Handler:    NewCustomDomainAssociation(api),
		Session:    s,
		Properties: properties,
		Update:     updated,
		Poll:       5 * time.Second,
	}.Run(t)
}

// Test_Replay_CustomDomainRejected associates a domain with a certificate ACM doesn't have. AWS answers 400 with
// ValidationException, which the handler reports as a rejected client failure, so UCP answers 400 and drops the
// record.
func Test_Replay_CustomDomainRejected(t *testing.T) {
	s := awsreplay.New(t, "testdata/customdomainrejected.json")
	api := redshiftserverless.NewFromConfig(s.Config(t.Context()))
	identifier, err := NewCustomDomainAssociation(api).Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{
		"WorkgroupName":              s.Input("RADIUS_REC_WORKGROUP"),
		"CustomDomainName":           "radius-rec.example.com",
		"CustomDomainCertificateArn": "arn:aws:acm:us-west-2:123456789012:certificate/00000000-0000-0000-0000-000000000000",
	}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.False(t, failure.Conflict)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, validation)
	require.Empty(t, identifier, "AWS made nothing, so a recording has nothing to clean up")
}
