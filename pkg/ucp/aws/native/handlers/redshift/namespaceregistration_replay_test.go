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
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// The recordings need a Redshift Serverless namespace, RADIUS_REC_NAMESPACE, and its workgroup,
// RADIUS_REC_WORKGROUP, and a provisioned cluster, RADIUS_REC_CLUSTER. The consumer is the example account
// 111122223333: AWS takes any 12-digit account.
const registrationRecordedConsumer = "111122223333"

// newRecordedRegistration returns the handler, using the session's AWS config.
func newRecordedRegistration(t *testing.T, s *awsreplay.Session) *NamespaceRegistration {
	cfg := s.Config(t.Context())
	return NewNamespaceRegistration(redshift.NewFromConfig(cfg), redshiftserverless.NewFromConfig(cfg))
}

// Test_Replay_NamespaceRegistrationServerless registers a serverless namespace and deregisters it, against AWS's
// responses. Each call leaves the namespace and workgroup MODIFYING for a while, and AWS lists the registration only
// once they are free, which the handler waits for.
func Test_Replay_NamespaceRegistrationServerless(t *testing.T) {
	s := awsreplay.New(t, "testdata/namespaceregistrationserverless.json")
	nativetest.Lifecycle{
		Handler:    newRecordedRegistration(t, s),
		Session:    s,
		Properties: serverlessRegistration(s.Input("RADIUS_REC_NAMESPACE"), s.Input("RADIUS_REC_WORKGROUP"), registrationRecordedConsumer),
		Poll:       5 * time.Second,
	}.Run(t)
}

// Test_Replay_NamespaceRegistrationProvisioned registers a provisioned cluster and deregisters it, against AWS's
// responses.
func Test_Replay_NamespaceRegistrationProvisioned(t *testing.T) {
	s := awsreplay.New(t, "testdata/namespaceregistrationprovisioned.json")
	nativetest.Lifecycle{
		Handler:    newRecordedRegistration(t, s),
		Session:    s,
		Properties: provisionedRegistration(s.Input("RADIUS_REC_CLUSTER"), registrationRecordedConsumer),
		Poll:       5 * time.Second,
	}.Run(t)
}

// Test_Replay_NamespaceRegistrationBusy registers a second consumer right after the first, while the first leaves
// the serverless namespace MODIFYING. AWS answers InvalidClusterState, which the handler reports as a conflict AWS
// applied none of, so UCP answers 409 and the caller deploys again.
func Test_Replay_NamespaceRegistrationBusy(t *testing.T) {
	s := awsreplay.New(t, "testdata/namespaceregistrationbusy.json")
	h := newRecordedRegistration(t, s)
	namespace, workgroup := s.Input("RADIUS_REC_NAMESPACE"), s.Input("RADIUS_REC_WORKGROUP")
	id, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: serverlessRegistration(namespace, workgroup, registrationRecordedConsumer)})
	require.NoError(t, err)
	// The test's context has ended by the time cleanup runs.
	ctx := context.WithoutCancel(t.Context())
	t.Cleanup(func() {
		// The namespace may still be busy with the registration.
		for range 30 {
			if err := h.Delete(ctx, awsreplay.Region, id); err == nil {
				break
			}
			s.Wait(5 * time.Second)
		}
		for range 30 {
			if _, err := h.Progress(ctx, awsreplay.Region, id, native.OperationDelete); err != nil {
				require.ErrorIs(t, err, native.ErrNotFound)
				return
			}
			s.Wait(5 * time.Second)
		}
		t.Errorf("deregister %s of %s by hand", registrationRecordedConsumer, namespace)
	})

	_, err = h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: serverlessRegistration(namespace, workgroup, "222233334444")})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "The serverless namespace "+namespace+" is busy")
	require.Contains(t, failure.Message, registrationBusy)
}

// Test_Replay_NamespaceRegistrationRejected registers a cluster AWS doesn't have. AWS answers 404 with
// ClusterNotFound, which the handler reports as a rejected client failure, so UCP answers 400 and drops the record.
func Test_Replay_NamespaceRegistrationRejected(t *testing.T) {
	s := awsreplay.New(t, "testdata/namespaceregistrationrejected.json")
	_, err := newRecordedRegistration(t, s).Create(t.Context(), native.CreateRequest{
		Region:     awsreplay.Region,
		Properties: provisionedRegistration("radius-rec-"+s.Input("RADIUS_REC_RUN"), registrationRecordedConsumer),
	})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.False(t, failure.Conflict)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, registrationClusterNotFound)
}
