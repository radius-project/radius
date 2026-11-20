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
	"maps"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// connReplayTemplate returns a template for a connection named archive between the session's domains.
func connReplayTemplate(s *awsreplay.Session) map[string]any {
	return map[string]any{
		"ConnectionAlias":      "archive",
		"LocalDomainInfo":      connDomainProperty(s.Input("RADIUS_REC_OPENSEARCH_DOMAIN"), "", ""),
		"RemoteDomainInfo":     connDomainProperty(s.Input("RADIUS_REC_OPENSEARCH_REMOTE_DOMAIN"), "", ""),
		"ConnectionMode":       "DIRECT",
		"ConnectionProperties": map[string]any{"CrossClusterSearch": map[string]any{"SkipUnavailable": "ENABLED"}},
	}
}

// Test_Replay_OutboundConnection creates a connection between two domains of the account, waits until AWS validated
// it, and deletes it, against AWS's responses. Every property is create-only, so there is no update step.
func Test_Replay_OutboundConnection(t *testing.T) {
	s := awsreplay.New(t, "testdata/outboundconnection.json")
	properties := connReplayTemplate(s)
	want := maps.Clone(properties)
	want["ConnectionStatus"] = map[string]any{"StatusCode": "PENDING_ACCEPTANCE"}
	nativetest.Lifecycle{
		Handler:    NewOutboundConnection(opensearch.NewFromConfig(s.Config(t.Context())), sts.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties,
		Want:       want,
		Poll:       10 * time.Second,
	}.Run(t)
}

// Test_Replay_OutboundConnectionErrors runs AWS's errors through the SDK. ResourceAlreadyExistsException for a
// connection AWS doesn't list yet is a conflict. LimitExceededException, which AWS answers with 409, and a 400
// ValidationException, which the SDK's model doesn't list for the call, are rejected, so UCP answers 400. A delete AWS
// answers with ResourceNotFoundException, also 409, is not found.
func Test_Replay_OutboundConnectionErrors(t *testing.T) {
	s := awsreplay.New(t, "testdata/outboundconnectionerrors.json")
	h := NewOutboundConnection(opensearch.NewFromConfig(s.Config(t.Context())), sts.NewFromConfig(s.Config(t.Context())))
	request := native.CreateRequest{Region: awsreplay.Region, Properties: connReplayTemplate(s)}

	_, err := h.Create(t.Context(), request)
	requireConnConflict(t, err, "doesn't list yet")
	require.Contains(t, err.Error(), "ResourceAlreadyExistsException")

	_, err = h.Create(t.Context(), request)
	requireConnRejected(t, err, "LimitExceededException")

	request.Properties = maps.Clone(request.Properties)
	request.Properties["RemoteDomainInfo"] = connDomainProperty(s.Input("RADIUS_REC_OPENSEARCH_REMOTE_DOMAIN"), "", "us-wst-2")
	_, err = h.Create(t.Context(), request)
	requireConnRejected(t, err, "ValidationException")

	require.ErrorIs(t, h.Delete(t.Context(), awsreplay.Region, "cc-0a1b2c3d4e5f"), native.ErrNotFound)
}
