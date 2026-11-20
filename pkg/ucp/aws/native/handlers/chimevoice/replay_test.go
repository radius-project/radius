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

package chimevoice

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// voiceConnectorInput names the ID of a Voice Connector with termination settings and no credentials, which the
	// recording adds credentials to and deletes them from.
	voiceConnectorInput = "RADIUS_REC_CHIME_VOICE_CONNECTOR_ID"

	// noTerminationInput names the ID of a Voice Connector without termination settings, and missingInput the ID of
	// a Voice Connector AWS doesn't have.
	noTerminationInput = "RADIUS_REC_CHIME_NO_TERMINATION_VOICE_CONNECTOR_ID"
	missingInput       = "RADIUS_REC_CHIME_MISSING_VOICE_CONNECTOR_ID"
)

// replay returns the session for the cassette file and the handler, using an Amazon Chime SDK Voice client that uses
// it, which waits only when recording.
func replay(t *testing.T, file string) (*awsreplay.Session, *TerminationCredentials) {
	s := awsreplay.New(t, file)
	h := NewTerminationCredentials(chimesdkvoice.NewFromConfig(s.Config(t.Context())))
	h.wait = func(_ context.Context, d time.Duration) { s.Wait(d) }
	return s, h
}

// passwords returns n random passwords, which the cassette never contains.
func passwords(s *awsreplay.Session, n int) []string {
	out := []string{}
	for range n {
		out = append(out, rand.Text())
	}
	s.Scrub(out...)
	return out
}

// Test_Replay_TerminationCredentials adds a credential to a Voice Connector, changes its password and adds a second,
// and deletes both, against AWS's responses.
func Test_Replay_TerminationCredentials(t *testing.T) {
	s, h := replay(t, "testdata/terminationcredentials.json")
	vc := s.Input(voiceConnectorInput)
	pw := passwords(s, 3)
	nativetest.Lifecycle{
		Handler:     h,
		Session:     s,
		Properties:  creds(vc, "radius-rec-1", pw[0]),
		Want:        withoutPassword(creds(vc, "radius-rec-1", pw[0]), "radius-rec-1"),
		Update:      creds(vc, "radius-rec-1", pw[1], "radius-rec-2", pw[2]),
		WantUpdated: withoutPassword(creds(vc, "radius-rec-1", pw[1], "radius-rec-2", pw[2]), "radius-rec-1", "radius-rec-2"),
		Poll:        time.Second,
	}.Run(t)
}

// Test_Replay_Errors checks how the handler reports AWS's errors, recorded from real calls: a Voice Connector AWS
// doesn't have, an ID outside the pattern of AWS's API model, which the SDK reads without a fault, a Voice Connector
// without termination settings, and more than 10 credentials. Each is a request AWS rejected and applied none of, and
// a read or delete on a Voice Connector AWS doesn't have means AWS has no such resource.
func Test_Replay_Errors(t *testing.T) {
	s, h := replay(t, "testdata/errors.json")
	vc := s.Input(voiceConnectorInput)
	noTermination := s.Input(noTerminationInput)
	missing := s.Input(missingInput)
	pw := passwords(s, maxCredentials+1)
	ctx := t.Context()
	create := func(properties map[string]any) error {
		_, err := h.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: properties, ClientToken: token})
		return err
	}
	requireRejected := func(err error, code string) {
		t.Helper()
		message := requireFailure(t, err, native.OutcomeRejected, true)
		require.Contains(t, message, code)
		for _, p := range pw {
			require.NotContains(t, message, p)
		}
	}

	requireRejected(create(creds(missing, "radius-rec-1", pw[0])), notFoundCode)
	_, err := h.Read(ctx, awsreplay.Region, missing+"|x")
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.DeleteScoped(ctx, awsreplay.Region, missing+"|x", []map[string]any{creds(missing, "radius-rec-1", "")}), native.ErrNotFound)

	requireRejected(create(creds("BAD_ID", "radius-rec-1", pw[0])), validationCode)
	_, err = h.Read(ctx, awsreplay.Region, "BAD_ID|x")
	requireReadError(t, err, validationCode)

	requireRejected(create(creds(noTermination, "radius-rec-1", pw[0])), badRequestCode)

	pairs := []string{}
	for i, p := range pw {
		pairs = append(pairs, fmt.Sprintf("radius-rec-%d", i), p)
	}
	requireRejected(create(creds(vc, pairs...)), badRequestCode)
	_, err = h.Read(ctx, awsreplay.Region, vc+"|x")
	require.ErrorIs(t, err, native.ErrNotFound, "AWS applied none of the rejected Put")
}
