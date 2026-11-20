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

package ses

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// identityInput names an email identity with no policies and no notification topics, and accountInput the
	// recording account's ID.
	identityInput = "RADIUS_REC_SES_IDENTITY"
	accountInput  = "RADIUS_REC_AWS_ACCOUNT"
)

// replay returns the session for the cassette file, its AWS config's SES v2 and v1 clients, and a pause that waits
// only when recording.
func replay(t *testing.T, file string) (*awsreplay.Session, *sesv2.Client, *ses.Client, pause) {
	s := awsreplay.New(t, file)
	cfg := s.Config(t.Context())
	return s, sesv2.NewFromConfig(cfg), ses.NewFromConfig(cfg), func(_ context.Context, d time.Duration) { s.Wait(d) }
}

// requireRejected requires err to be a client failure AWS applied none of, whose message contains text.
func requireRejected(t *testing.T, err error, text string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError, failure.Message)
	require.Equal(t, native.OutcomeRejected, failure.Outcome, failure.Message)
	require.Contains(t, failure.Message, text)
}

// Test_Replay_EmailIdentityPolicy creates a sending authorization policy written as a template would write it, adds
// an action, and deletes it, against AWS's responses. SES returns the policy in its own form, which the handler counts
// as the template's.
func Test_Replay_EmailIdentityPolicy(t *testing.T) {
	s, api, _, wait := replay(t, "testdata/emailidentitypolicy.json")
	identity, acct := s.Input(identityInput), s.Input(accountInput)
	resource := "arn:aws:ses:" + awsreplay.Region + ":" + acct + ":identity/" + identity
	h := NewEmailIdentityPolicy(api)
	h.wait = wait

	template := `{
  "Version": "2012-10-17",
  "Statement": [{"Effect": "Allow", "Principal": {"AWS": "` + acct + `"}, "Action": ["ses:SendEmail"], "Resource": "` + resource + `"}]
}`
	stored := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::` + acct + `:root"},` +
		`"Action":"ses:SendEmail","Resource":"` + resource + `"}]}`
	updated := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::` + acct + `:root"},` +
		`"Action":["ses:SendEmail","ses:SendRawEmail"],"Resource":"` + resource + `"}]}`
	policy := func(p string) map[string]any {
		return map[string]any{"EmailIdentity": identity, "PolicyName": "radius-rec", "Policy": p}
	}
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: policy(template),
		Want:       policy(stored),
		Update:     policy(updated),
		Poll:       time.Second,
	}.Run(t)
	require.True(t, h.Options().Equivalent("Policy", template, stored), "SES's form of the policy is the template's")
}

// Test_Replay_IdentityNotificationTopic publishes an identity's bounce notifications to a topic, moves them to
// another topic with the original headers, and deletes the setting, which turns headers off and clears the topic,
// against AWS's responses.
func Test_Replay_IdentityNotificationTopic(t *testing.T) {
	s, _, api, wait := replay(t, "testdata/identitynotificationtopic.json")
	identity := s.Input(identityInput)
	topic, other := s.Input("RADIUS_REC_SES_TOPIC"), s.Input("RADIUS_REC_SES_OTHER_TOPIC")
	h := NewIdentityNotificationTopic(api)
	h.wait = wait
	setting := func(topic string, headers bool) map[string]any {
		return map[string]any{"Identity": identity, "NotificationType": "Bounce", "SnsTopic": topic, headersProperty: headers}
	}
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: map[string]any{"Identity": identity, "NotificationType": "Bounce", "SnsTopic": topic},
		Want:       setting(topic, false),
		Update:     setting(other, true),
		Poll:       time.Second,
	}.Run(t)
}

// Test_Replay_DedicatedIPAssignment moves a dedicated IP address to a pool, then to another, against responses
// written from the API reference, because the recording account leases no dedicated IP address. A delete calls
// nothing in AWS.
func Test_Replay_DedicatedIPAssignment(t *testing.T) {
	if awsreplay.Recording() {
		t.Skip("the recording account leases no dedicated IP address, so the cassette is written from the API reference")
	}
	s, api, _, wait := replay(t, "testdata/dedicatedipassignment.json")
	ip := s.Input("RADIUS_REC_SES_DEDICATED_IP")
	h := NewDedicatedIPAssignment(api)
	h.wait = wait
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: map[string]any{"Ip": ip, poolProperty: "radius-rec-pool-a"},
		Update:     map[string]any{"Ip": ip, poolProperty: "radius-rec-pool-b"},
		Poll:       time.Second,
		SkipDelete: true,
	}.Run(t)
}

// Test_Replay_Errors checks how the handlers report AWS's errors, recorded from real calls: an identity SES doesn't
// have, a policy SES doesn't take, a policy or topic SNS doesn't have, and a dedicated IP address the account doesn't
// lease. The SES v1 errors have no modeled type, so the SDK reads them without a fault. Each is a request AWS
// rejected and applied none of, and a read or delete of something SES doesn't have means AWS has no such resource.
func Test_Replay_Errors(t *testing.T) {
	s, v2, v1, wait := replay(t, "testdata/errors.json")
	identity := s.Input(identityInput)
	missingID := s.Input("RADIUS_REC_SES_MISSING_IDENTITY")
	topic, missingTopic := s.Input("RADIUS_REC_SES_TOPIC"), s.Input("RADIUS_REC_SES_MISSING_TOPIC")
	ip := s.Input("RADIUS_REC_SES_UNLEASED_IP")
	ctx := t.Context()
	policies, topics, ips := NewEmailIdentityPolicy(v2), NewIdentityNotificationTopic(v1), NewDedicatedIPAssignment(v2)
	policies.wait, topics.wait, ips.wait = wait, wait, wait

	_, err := policies.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"EmailIdentity": missingID, "PolicyName": "p1", "Policy": "{}"}})
	requireRejected(t, err, notFoundCode)
	_, err = policies.Read(ctx, awsreplay.Region, missingID+"|p1")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = policies.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"EmailIdentity": identity, "PolicyName": "p1", "Policy": "not json"}})
	requireRejected(t, err, "Unable to parse policy")
	require.ErrorIs(t, policies.Delete(ctx, awsreplay.Region, identity+"|p1"), native.ErrNotFound, "SES has no such policy")

	_, err = topics.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"Identity": missingID, "NotificationType": "Bounce", "SnsTopic": topic}})
	requireRejected(t, err, "Must be a verified email address or domain")
	_, err = topics.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"Identity": identity, "NotificationType": "Bounce", "SnsTopic": missingTopic}})
	requireRejected(t, err, "SNS topic")
	_, err = topics.Read(ctx, awsreplay.Region, missingID+"|Bounce")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = topics.Read(ctx, awsreplay.Region, identity+"|Bounce")
	require.ErrorIs(t, err, native.ErrNotFound, "the identity has no bounce topic")

	_, err = ips.Read(ctx, awsreplay.Region, ip)
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = ips.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"Ip": ip, poolProperty: "radius-rec-pool-a"}})
	requireRejected(t, err, notFoundCode)
}
