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
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// errFailNow stops a fakeTB's test body.
var errFailNow = errors.New("FailNow")

// fakeTB is a testing.TB whose failure doesn't fail the real test, so a test can check what a failed recording does.
type fakeTB struct {
	testing.TB
	ctx      context.Context
	cancel   context.CancelFunc
	failed   bool
	cleanups []func()
}

func newFakeTB(t *testing.T) *fakeTB {
	ctx, cancel := context.WithCancel(t.Context())
	return &fakeTB{TB: t, ctx: ctx, cancel: cancel}
}

func (f *fakeTB) Helper()                           {}
func (f *fakeTB) Context() context.Context          { return f.ctx }
func (f *fakeTB) Failed() bool                      { return f.failed }
func (f *fakeTB) Cleanup(fn func())                 { f.cleanups = append(f.cleanups, fn) }
func (f *fakeTB) Logf(format string, args ...any)   {}
func (f *fakeTB) Errorf(format string, args ...any) { f.failed = true }
func (f *fakeTB) Fatalf(format string, args ...any) { f.Errorf(format, args...); f.FailNow() }
func (f *fakeTB) FailNow()                          { f.failed = true; panic(errFailNow) }

// run runs fn as a test body, then its cleanups, last first, as the testing package does.
func (f *fakeTB) run(fn func()) {
	func() {
		defer func() {
			if r := recover(); r != nil && r != errFailNow {
				panic(r)
			}
		}()
		fn()
	}()
	f.cancel()
	for _, cleanup := range slices.Backward(f.cleanups) {
		cleanup()
	}
}

// Test_Settings_CreateKeepsTheIdentifierOfAnAcceptedPut checks that a Create whose Put AWS accepted, but no read showed,
// returns the Voice Connector's ID with its failure, so a failed recording cleans up the settings AWS has, and that a
// Create AWS rejected returns none, so a recording never touches settings it didn't make.
func Test_Settings_CreateKeepsTheIdentifierOfAnAcceptedPut(t *testing.T) {
	throttled := apiError(429, "ThrottledClientException", "slow down")
	settings := []struct {
		name     string
		handler  func(api API) native.Handler
		get      string
		props    map[string]any
		rejected map[string]any
		gone     func(store *awsfake.CloudControl) bool
	}{
		{
			"logging", func(api API) native.Handler { return withoutWait(NewLoggingConfiguration(api)) }, "GetVoiceConnectorLoggingConfiguration",
			logging(testVC, true, true), logging(missingVC, true, true),
			func(store *awsfake.CloudControl) bool {
				return slices.Equal([]any{false, false}, []any{
					store.Resource(loggingType, testVC)["LoggingConfiguration"].(map[string]any)["EnableSIPLogs"],
					store.Resource(loggingType, testVC)["LoggingConfiguration"].(map[string]any)["EnableMediaMetricLogs"],
				})
			},
		},
		{
			"streaming", func(api API) native.Handler { return withoutWait(NewStreamingConfiguration(api)) }, "GetVoiceConnectorStreamingConfiguration",
			streaming(testVC, map[string]any{"DataRetentionInHours": 1, "Disabled": false}), streaming(testVC, map[string]any{"DataRetentionInHours": -1, "Disabled": false}),
			func(store *awsfake.CloudControl) bool { return store.Resource(streamingType, testVC) == nil },
		},
		{
			"origination", func(api API) native.Handler { return withoutWait(NewOrigination(api)) }, "GetVoiceConnectorOrigination",
			origination(testVC, route("sip.example.com", 1, 1)), origination(testVC, route("10.0.0.10", 1, 1)),
			func(store *awsfake.CloudControl) bool { return store.Resource(originationType, testVC) == nil },
		},
		{
			"termination", func(api API) native.Handler { return withoutWait(NewTermination(api)) }, "GetVoiceConnectorTermination",
			termination(testVC, map[string]any{"CallingRegions": []any{"US"}, "CidrAllowedList": []any{"203.0.113.0/27"}}),
			termination(testVC, map[string]any{"CallingRegions": []any{"US"}, "CidrAllowedList": []any{"203.0.113.0/24"}}),
			func(store *awsfake.CloudControl) bool { return store.Resource(terminationType, testVC) == nil },
		},
	}
	for _, tt := range settings {
		t.Run(tt.name+": no read shows the Put", func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			h := tt.handler(api)
			api.errs[tt.get] = throttled
			identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: tt.props})
			requireUnconfirmed(t, err, "ThrottledClientException")
			require.Equal(t, testVC, identifier)
		})

		t.Run(tt.name+": AWS rejects the Put", func(t *testing.T) {
			identifier, err := tt.handler(newFakeAPI(awsfake.NewCloudControl())).Create(t.Context(), native.CreateRequest{Region: region, Properties: tt.rejected})
			var failure *native.Failure
			require.ErrorAs(t, err, &failure)
			require.Equal(t, native.OutcomeRejected, failure.Outcome)
			require.Empty(t, identifier)
		})

		t.Run(tt.name+": a failed recording cleans up an accepted Put", func(t *testing.T) {
			t.Setenv(awsreplay.RecordEnv, "1")
			store := awsfake.NewCloudControl()
			api := newFakeAPI(store)
			h := tt.handler(api)
			api.errs[tt.get] = throttled
			f := newFakeTB(t)
			l := nativetest.Lifecycle{Handler: h, Session: awsreplay.New(t, filepath.Join(t.TempDir(), "cassette.json")), Properties: tt.props, Poll: time.Millisecond}
			f.run(func() {
				defer delete(api.errs, tt.get)
				l.Run(f)
			})
			require.True(t, f.failed, "the create fails")
			require.NotEmpty(t, f.cleanups, "the recording registers cleanup for the settings AWS accepted")
			require.True(t, tt.gone(store), "cleanup deleted or reset the settings")
		})

		t.Run(tt.name+": a failed recording leaves alone what a rejected Put didn't make", func(t *testing.T) {
			t.Setenv(awsreplay.RecordEnv, "1")
			f := newFakeTB(t)
			l := nativetest.Lifecycle{Handler: tt.handler(newFakeAPI(awsfake.NewCloudControl())), Session: awsreplay.New(t, filepath.Join(t.TempDir(), "cassette.json")), Properties: tt.rejected, Poll: time.Millisecond}
			f.run(func() { l.Run(f) })
			require.True(t, f.failed)
			require.Empty(t, f.cleanups)
		})
	}
}
