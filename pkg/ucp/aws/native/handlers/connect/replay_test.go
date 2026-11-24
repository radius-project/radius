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

package connect

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/connect"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// instanceInput and instanceARNInput name an Amazon Connect instance, which costs nothing without phone numbers,
	// and flowInput the ID of one of its flows. phoneInput is the ARN of a phone number claimed to the instance, and
	// otherFlowInput another flow, which only the hand-written cassette has: claiming a number costs money.
	instanceInput    = "RADIUS_REC_CONNECT_INSTANCE_ID"
	instanceARNInput = "RADIUS_REC_CONNECT_INSTANCE_ARN"
	flowInput        = "RADIUS_REC_CONNECT_FLOW_ID"
	otherFlowInput   = "RADIUS_REC_CONNECT_OTHER_FLOW_ID"
	phoneInput       = "RADIUS_REC_CONNECT_PHONE_NUMBER_ARN"

	// unclaimedPhoneID is a phone number ID no instance has.
	unclaimedPhoneID = "00000000-0000-4000-8000-000000000000"
)

// replay returns the session for the cassette file and a handler that uses it, which waits only when recording.
func replay(t *testing.T, file string) (*awsreplay.Session, *PhoneNumberContactFlowAssociation) {
	s := awsreplay.New(t, file)
	h := NewPhoneNumberContactFlowAssociation(connect.NewFromConfig(s.Config(t.Context())))
	h.wait = func(_ context.Context, d time.Duration) { s.Wait(d) }
	return s, h
}

// Test_Replay_PhoneNumberContactFlowAssociation sends a phone number's calls to one flow, then another, and removes
// the flow, against responses written from the AWS API reference.
func Test_Replay_PhoneNumberContactFlowAssociation(t *testing.T) {
	s, h := replay(t, "testdata/phonenumbercontactflowassociation.json")
	phone, instance := s.Input(phoneInput), s.Input(instanceInput)
	flow, other := s.Input(flowInput), s.Input(otherFlowInput)
	flowARN := func(id string) string { return s.Input(instanceARNInput) + "/contact-flow/" + id }
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: association(phone, instance, flow),
		// AWS returns the flow's ARN.
		Want:        association(phone, instance, flowARN(flow)),
		Update:      association(phone, instance, other),
		WantUpdated: association(phone, instance, flowARN(other)),
		Poll:        time.Second,
	}.Run(t)
}

// Test_Replay_Errors checks how the handler reports AWS's errors, recorded from real calls on an instance with no
// phone numbers: a phone number AWS doesn't have, and an instance AWS doesn't have. A rejected create applied nothing,
// and a read or delete of a number with no flow, or in an instance AWS doesn't have, means AWS has no such association.
func Test_Replay_Errors(t *testing.T) {
	s, h := replay(t, "testdata/errors.json")
	ctx := t.Context()
	instance, flow := s.Input(instanceInput), s.Input(flowInput)
	// The instance's ARN gives the account and Region of a phone number ARN.
	unclaimed := strings.Replace(s.Input(instanceARNInput), "instance/"+instance, "phone-number/"+unclaimedPhoneID, 1)

	_, err := h.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: association(unclaimed, instance, flow)})
	requireRejected(t, err, "Phone number not found")
	_, err = h.Read(ctx, awsreplay.Region, unclaimed+"|"+instance)
	require.ErrorIs(t, err, native.ErrNotFound, "an instance with no flow for the number")
	_, err = h.Read(ctx, awsreplay.Region, unclaimed+"|"+missingInstance)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(ctx, awsreplay.Region, unclaimed+"|"+instance), native.ErrNotFound)
}
