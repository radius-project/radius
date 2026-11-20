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

package chime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// voiceConnectorInput names the ID of a Voice Connector with no settings, which the recording changes and sets back.
const voiceConnectorInput = "RADIUS_REC_CHIME_VOICE_CONNECTOR_ID"

// replay returns the session for the cassette file, an Amazon Chime SDK Voice client that uses it, and a pause that
// waits only when recording.
func replay(t *testing.T, file string) (*awsreplay.Session, API, pause) {
	s := awsreplay.New(t, file)
	return s, chimesdkvoice.NewFromConfig(s.Config(t.Context())), func(_ context.Context, d time.Duration) { s.Wait(d) }
}

// Test_Replay_LoggingConfiguration turns SIP logs on for a Voice Connector, then media metric logs too, and deletes
// the setting, which turns both off again, against AWS's responses.
func Test_Replay_LoggingConfiguration(t *testing.T) {
	s, api, wait := replay(t, "testdata/loggingconfiguration.json")
	vc := s.Input(voiceConnectorInput)
	h := NewLoggingConfiguration(api)
	h.wait = wait
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: logging(vc, true, false),
		Update:     logging(vc, true, true),
		Poll:       time.Second,
	}.Run(t)
}

// Test_Replay_StreamingConfiguration sets a Voice Connector's streaming settings, changes them, and deletes them,
// against AWS's responses.
func Test_Replay_StreamingConfiguration(t *testing.T) {
	s, api, wait := replay(t, "testdata/streamingconfiguration.json")
	vc := s.Input(voiceConnectorInput)
	h := NewStreamingConfiguration(api)
	h.wait = wait
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: streaming(vc, map[string]any{"DataRetentionInHours": 24, "Disabled": false}),
		// AWS sends streaming notifications to EventBridge when the template leaves the targets out.
		Want: streaming(vc, map[string]any{"DataRetentionInHours": 24, "Disabled": false, "StreamingNotificationTargets": []any{
			map[string]any{"NotificationTarget": "EventBridge"},
		}}),
		Update: streaming(vc, map[string]any{"DataRetentionInHours": 48, "Disabled": true, "StreamingNotificationTargets": []any{
			map[string]any{"NotificationTarget": "SNS"},
		}}),
		Poll: time.Second,
	}.Run(t)
}

// Test_Replay_Origination routes a Voice Connector's inbound calls to one SIP host, adds a second, and deletes the
// settings, against AWS's responses.
func Test_Replay_Origination(t *testing.T) {
	s, api, wait := replay(t, "testdata/origination.json")
	vc := s.Input(voiceConnectorInput)
	h := NewOrigination(api)
	h.wait = wait
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: origination(vc, route("sip1.example.com", 1, 1)),
		Update:     origination(vc, route("sip1.example.com", 1, 1), route("sip2.example.com", 2, 1)),
		Poll:       time.Second,
	}.Run(t)
}

// Test_Replay_Termination lets a Voice Connector call the US, adds Canada, and deletes the settings, against AWS's
// responses.
func Test_Replay_Termination(t *testing.T) {
	s, api, wait := replay(t, "testdata/termination.json")
	vc := s.Input(voiceConnectorInput)
	h := NewTermination(api)
	h.wait = wait
	settings := func(regions ...any) map[string]any {
		return map[string]any{"CallingRegions": regions, "CidrAllowedList": []any{"203.0.113.0/27"}}
	}
	withDefaults := func(regions ...any) map[string]any {
		out := settings(regions...)
		out["CpsLimit"], out["Disabled"] = 1, false
		return out
	}
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: termination(vc, settings("US")),
		// AWS fills in CpsLimit and Disabled.
		Want:        termination(vc, withDefaults("US")),
		Update:      termination(vc, settings("US", "CA")),
		WantUpdated: termination(vc, withDefaults("US", "CA")),
		Poll:        time.Second,
	}.Run(t)
}

// Test_Replay_Errors checks how the handlers report AWS's errors, recorded from real calls: a Voice Connector AWS
// doesn't have, a value outside a limit of AWS's API model, which the SDK reads without a fault, and settings AWS
// doesn't take. Each is a request AWS rejected and applied none of, and a read or delete of settings or a Voice
// Connector AWS doesn't have means AWS has no such resource.
func Test_Replay_Errors(t *testing.T) {
	s, api, wait := replay(t, "testdata/errors.json")
	vc := s.Input(voiceConnectorInput)
	missing := s.Input("RADIUS_REC_CHIME_MISSING_VOICE_CONNECTOR_ID")
	ctx := t.Context()
	logs, streams, routes, terms := NewLoggingConfiguration(api), NewStreamingConfiguration(api), NewOrigination(api), NewTermination(api)
	for _, h := range []interface{ setWait(pause) }{logs, streams, routes, terms} {
		h.setWait(wait)
	}

	_, err := logs.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: logging(missing, true, false)})
	requireRejected(t, err, notFoundCode)
	_, err = logs.Read(ctx, awsreplay.Region, missing)
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = terms.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: termination(vc, map[string]any{
		"CallingRegions": []any{"US"}, "CidrAllowedList": []any{"203.0.113.0/27"}, "CpsLimit": 0,
	})})
	requireRejected(t, err, validationCode)
	_, err = terms.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: termination(vc, map[string]any{
		"CallingRegions": []any{"US"}, "CidrAllowedList": []any{"203.0.113.0/24"},
	})})
	requireRejected(t, err, badRequestCode)

	routeList := []map[string]any{}
	for range maxRoutes + 1 {
		routeList = append(routeList, route("sip.example.com", 1, 1))
	}
	_, err = routes.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: origination(vc, routeList...)})
	requireRejected(t, err, "more than 10")

	_, err = streams.Read(ctx, awsreplay.Region, vc)
	require.ErrorIs(t, err, native.ErrNotFound)
	err = terms.Delete(ctx, awsreplay.Region, missing)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.False(t, errors.Is(routes.Delete(ctx, awsreplay.Region, vc), native.ErrNotFound), "AWS deletes settings a Voice Connector doesn't have")
}
