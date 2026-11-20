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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// requireRejected requires err to be a client failure AWS applied none of, whose message contains text.
func requireRejected(t *testing.T, err error, text string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, text)
}

// update returns the update from current to desired, with the patch UCP computes for the values desired sets.
func update(t *testing.T, current map[string]any, desired map[string]any) native.UpdateRequest {
	t.Helper()
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	return native.UpdateRequest{Region: region, Identifier: testVC, Current: current, Desired: desired, Patch: patch}
}

func Test_LoggingConfiguration_MissingVoiceConnector(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := withoutWait(NewLoggingConfiguration(api))
	ctx := t.Context()

	// AWS returns both logs off for a Voice Connector it doesn't have, so only the Voice Connector check finds it gone.
	_, err := h.Read(ctx, region, missingVC)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Reset(ctx, region, missingVC), native.ErrNotFound, "a delete of a deleted Voice Connector's setting finishes")
	_, err = h.Progress(ctx, region, missingVC, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = h.Create(ctx, native.CreateRequest{Region: region, Properties: logging(missingVC, true, false)})
	requireRejected(t, err, notFoundCode)
}

func Test_LoggingConfiguration_Update(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := withoutWait(NewLoggingConfiguration(api))
	ctx := t.Context()

	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: logging(testVC, true, true)})
	require.NoError(t, err)
	current, err := h.Read(ctx, region, testVC)
	require.NoError(t, err)

	t.Run("a log the template no longer sets is turned off", func(t *testing.T) {
		desired := map[string]any{"VoiceConnectorId": testVC, "LoggingConfiguration": map[string]any{"EnableSIPLogs": true}}
		request := update(t, current, desired)
		request.Patch = jsondiff.Patch{{Type: jsondiff.OperationRemove, Path: "/LoggingConfiguration/EnableMediaMetricLogs"}}
		require.NoError(t, h.Update(ctx, request))
		got, err := h.Read(ctx, region, testVC)
		require.NoError(t, err)
		require.Equal(t, logging(testVC, true, false), got)
	})

	t.Run("a log only AWS set stays as AWS has it", func(t *testing.T) {
		api.store.Put(loggingType, testVC, logging(testVC, false, true))
		current, err := h.Read(ctx, region, testVC)
		require.NoError(t, err)
		desired := map[string]any{"VoiceConnectorId": testVC, "LoggingConfiguration": map[string]any{"EnableSIPLogs": true}}
		request := update(t, current, desired)
		request.Patch = jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/LoggingConfiguration/EnableSIPLogs", Value: true}}
		require.NoError(t, h.Update(ctx, request))
		got, err := h.Read(ctx, region, testVC)
		require.NoError(t, err)
		require.Equal(t, logging(testVC, true, true), got)
	})

	t.Run("a template without settings is rejected before any call", func(t *testing.T) {
		api.calls = nil
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"VoiceConnectorId": testVC}})
		requireRejected(t, err, "set LoggingConfiguration")
		err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: testVC, Desired: map[string]any{"VoiceConnectorId": testVC}})
		requireRejected(t, err, "set LoggingConfiguration")
		require.Empty(t, api.calls)
	})
}

func Test_LoggingConfiguration_ResetWaitsForOff(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := withoutWait(NewLoggingConfiguration(api))
	ctx := t.Context()
	api.store.Put(loggingType, testVC, logging(testVC, true, true))

	progress, err := h.Progress(ctx, region, testVC, native.OperationDelete)
	require.NoError(t, err)
	require.False(t, progress.Done, "a log is still on")
	require.Equal(t, "off", progress.Waiting)

	require.NoError(t, h.Reset(ctx, region, testVC))
	progress, err = h.Progress(ctx, region, testVC, native.OperationDelete)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.Equal(t, logging(testVC, false, false), api.store.Resource(loggingType, testVC))
}

func Test_Termination_UpdateKeepsWhatAWSFilledIn(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := withoutWait(NewTermination(api))
	ctx := t.Context()
	settings := map[string]any{"CallingRegions": []any{"US"}, "CidrAllowedList": []any{"203.0.113.0/27"}, "DefaultPhoneNumber": "+12065550100"}
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: termination(testVC, settings)})
	require.NoError(t, err)
	current, err := h.Read(ctx, region, testVC)
	require.NoError(t, err)
	require.EqualValues(t, 1, current["Termination"].(map[string]any)["CpsLimit"], "AWS fills in CpsLimit")

	// The template drops DefaultPhoneNumber and adds CA. The Put sends the CpsLimit and Disabled AWS has, and clears
	// the phone number.
	request := update(t, current, nil)
	request.Desired = termination(testVC, map[string]any{"CallingRegions": []any{"US", "CA"}, "CidrAllowedList": []any{"203.0.113.0/27"}})
	request.Patch = jsondiff.Patch{
		{Type: jsondiff.OperationRemove, Path: "/Termination/DefaultPhoneNumber"},
		{Type: jsondiff.OperationReplace, Path: "/Termination/CallingRegions", Value: []any{"US", "CA"}},
	}
	require.NoError(t, h.Update(ctx, request))
	got, err := h.Read(ctx, region, testVC)
	require.NoError(t, err)
	require.Equal(t, termination(testVC, map[string]any{
		"CallingRegions": []any{"US", "CA"}, "CidrAllowedList": []any{"203.0.113.0/27"}, "CpsLimit": float64(1), "Disabled": false,
	}), got)

	t.Run("a patch that doesn't apply to what AWS has is rejected", func(t *testing.T) {
		request := update(t, got, nil)
		request.Desired = got
		request.Patch = jsondiff.Patch{{Type: jsondiff.OperationRemove, Path: "/Termination/Missing/Value"}}
		var failure *native.Failure
		require.ErrorAs(t, h.Update(ctx, request), &failure)
		require.Equal(t, native.OutcomeRejected, failure.Outcome, "nothing reached AWS")
		require.Contains(t, failure.Message, "Deploy again")
	})
}

func Test_Settings_Delete(t *testing.T) {
	for _, tt := range []struct {
		name     string
		handler  func(api API) native.Handler
		typeName string
		props    map[string]any
	}{
		{"streaming", func(api API) native.Handler { return withoutWait(NewStreamingConfiguration(api)) }, streamingType, streaming(testVC, map[string]any{"DataRetentionInHours": 1, "Disabled": false})},
		{"origination", func(api API) native.Handler { return withoutWait(NewOrigination(api)) }, originationType, origination(testVC, route("sip.example.com", 1, 1))},
		{"termination", func(api API) native.Handler { return withoutWait(NewTermination(api)) }, terminationType, termination(testVC, map[string]any{"CallingRegions": []any{"US"}, "CidrAllowedList": []any{"203.0.113.0/27"}})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			h := tt.handler(api)
			ctx := t.Context()
			_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: tt.props})
			require.NoError(t, err)

			require.NoError(t, h.Delete(ctx, region, testVC))
			progress, err := h.Progress(ctx, region, testVC, native.OperationDelete)
			require.NoError(t, err)
			require.False(t, progress.Done, "AWS still shows the settings right after the delete")
			require.Equal(t, "absent", progress.Waiting)
			for range staleReads {
				_, err = h.Progress(ctx, region, testVC, native.OperationDelete)
			}
			require.ErrorIs(t, err, native.ErrNotFound)
			require.NoError(t, h.Delete(ctx, region, testVC), "AWS deletes settings a Voice Connector doesn't have")

			_, err = h.Progress(ctx, region, testVC, native.OperationCreate)
			require.ErrorIs(t, err, native.ErrNotFound, "a lost create that never reached AWS is sent again")

			require.ErrorIs(t, h.Delete(ctx, region, missingVC), native.ErrNotFound)
			_, err = h.List(ctx, region)
			require.ErrorContains(t, err, "can't be listed")
			progress, err = h.Progress(ctx, region, testVC, native.OperationUpdate)
			require.NoError(t, err)
			require.True(t, progress.Done)

			_, err = h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"VoiceConnectorId": testVC}})
			var failure *native.Failure
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)
		})
	}
}

func Test_Settings_EmergencyCallingBlocksDelete(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	for _, h := range []native.Handler{NewOrigination(api), NewTermination(api)} {
		err := h.Delete(t.Context(), region, emergencyVC)
		require.Error(t, err)
		require.NotErrorIs(t, err, native.ErrNotFound, "a delete AWS rejected keeps the record")
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
	}
}

func Test_Confirm(t *testing.T) {
	want := map[string]any{"VoiceConnectorId": testVC}
	reads := 0
	waits := []time.Duration{}
	wait := func(_ context.Context, d time.Duration) { waits = append(waits, d) }

	t.Run("stops once a read shows the Put", func(t *testing.T) {
		reads, waits = 0, nil
		confirm(t.Context(), wait, func(context.Context, string, string) (map[string]any, error) {
			reads++
			if reads < 3 {
				return nil, native.ErrNotFound
			}
			return want, nil
		}, region, testVC, want)
		require.Equal(t, 3, reads)
		require.Equal(t, []time.Duration{confirmPause, confirmPause}, waits)
	})

	t.Run("gives up after confirmReads reads", func(t *testing.T) {
		reads = 0
		confirm(t.Context(), noWait, func(context.Context, string, string) (map[string]any, error) {
			reads++
			return map[string]any{"VoiceConnectorId": "other"}, nil
		}, region, testVC, want)
		require.Equal(t, confirmReads, reads)
	})

	t.Run("stops at a read that fails", func(t *testing.T) {
		reads = 0
		confirm(t.Context(), noWait, func(context.Context, string, string) (map[string]any, error) {
			reads++
			return nil, errors.New("throttled")
		}, region, testVC, want)
		require.Equal(t, 1, reads)
	})

	t.Run("stops when the request ends", func(t *testing.T) {
		reads = 0
		ctx, cancel := context.WithCancel(t.Context())
		confirm(ctx, func(context.Context, time.Duration) { cancel() }, func(context.Context, string, string) (map[string]any, error) {
			reads++
			return nil, native.ErrNotFound
		}, region, testVC, want)
		require.Equal(t, 1, reads)
	})

	t.Run("sleep returns when the request ends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		start := time.Now()
		sleep(ctx, time.Hour)
		require.Less(t, time.Since(start), time.Minute)
	})
}

func Test_Fake_ObservedBehavior(t *testing.T) {
	ctx := t.Context()
	store := awsfake.NewCloudControl()
	h := withoutWait(NewStreamingConfiguration(newFakeAPI(store)))
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: streaming(testVC, map[string]any{"DataRetentionInHours": 1, "Disabled": false})})
	require.NoError(t, err)
	got, err := h.Read(ctx, region, testVC)
	require.NoError(t, err)
	b, _ := json.Marshal(got["StreamingConfiguration"].(map[string]any)["StreamingNotificationTargets"])
	require.JSONEq(t, `[{"NotificationTarget":"EventBridge"}]`, string(b), "AWS sends notifications to EventBridge when the Put leaves the targets out")

	o := withoutWait(NewOrigination(newFakeAPI(store)))
	props := origination(testVC, map[string]any{"Host": "sip.example.com", "Priority": 1, "Weight": 1})
	_, err = o.Create(ctx, native.CreateRequest{Region: region, Properties: props})
	require.NoError(t, err)
	got, err = o.Read(ctx, region, testVC)
	require.NoError(t, err)
	require.EqualValues(t, 0, got["Origination"].(map[string]any)["Routes"].([]any)[0].(map[string]any)["Port"], "AWS reports a port the Put left out as 0")

	_, err = o.Read(ctx, region, "not-an-id")
	require.ErrorContains(t, err, validationCode)
}

func Test_Settings_Errors(t *testing.T) {
	ctx := t.Context()
	throttled := apiError(429, "ThrottledClientException", "slow down")

	t.Run("a failed Voice Connector check fails the logging read", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := withoutWait(NewLoggingConfiguration(api))
		api.errs["GetVoiceConnector"] = throttled
		_, err := h.Read(ctx, region, testVC)
		require.ErrorContains(t, err, "ThrottledClientException")
		require.NotErrorIs(t, err, native.ErrNotFound)
		_, err = h.Progress(ctx, region, testVC, native.OperationDelete)
		require.ErrorContains(t, err, "ThrottledClientException")

		delete(api.errs, "GetVoiceConnector")
		api.errs["GetVoiceConnectorLoggingConfiguration"] = throttled
		_, err = h.Read(ctx, region, testVC)
		require.ErrorContains(t, err, "ThrottledClientException")
		_, err = h.List(ctx, region)
		require.ErrorContains(t, err, "can't be listed")
	})

	t.Run("a value of the wrong type is rejected before any call", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := withoutWait(NewStreamingConfiguration(api))
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: streaming(testVC, map[string]any{"DataRetentionInHours": "a day", "Disabled": false})})
		requireRejected(t, err, "StreamingConfiguration")
		require.Empty(t, api.calls)
	})

	for _, h := range []native.Handler{
		withoutWait(NewLoggingConfiguration(newFakeAPI(awsfake.NewCloudControl()))),
		withoutWait(NewStreamingConfiguration(newFakeAPI(awsfake.NewCloudControl()))),
		withoutWait(NewOrigination(newFakeAPI(awsfake.NewCloudControl()))),
		withoutWait(NewTermination(newFakeAPI(awsfake.NewCloudControl()))),
	} {
		t.Run(h.TypeName()+": an update without settings, or one AWS rejects", func(t *testing.T) {
			err := h.Update(ctx, native.UpdateRequest{Region: region, Identifier: testVC, Desired: map[string]any{"VoiceConnectorId": testVC}})
			var failure *native.Failure
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)

			request := update(t, map[string]any{"VoiceConnectorId": testVC}, nil)
			request.Desired = map[string]any{"VoiceConnectorId": testVC, "LoggingConfiguration": map[string]any{}, "StreamingConfiguration": map[string]any{}, "Origination": map[string]any{}, "Termination": map[string]any{}}
			request.Patch = jsondiff.Patch{{Type: jsondiff.OperationRemove, Path: "/Missing"}}
			require.ErrorAs(t, h.Update(ctx, request), &failure)
			require.Equal(t, native.OutcomeRejected, failure.Outcome)
		})
	}

	t.Run("a Put AWS rejects", func(t *testing.T) {
		_, err := withoutWait(NewOrigination(newFakeAPI(awsfake.NewCloudControl()))).Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"VoiceConnectorId": testVC, "Origination": map[string]any{"Disabled": true}}})
		requireRejected(t, err, badRequestCode)
		_, err = withoutWait(NewStreamingConfiguration(newFakeAPI(awsfake.NewCloudControl()))).Create(ctx, native.CreateRequest{Region: region, Properties: streaming(testVC, map[string]any{"DataRetentionInHours": -1, "Disabled": false})})
		requireRejected(t, err, validationCode)
	})

	t.Run("a logging template that sets LoggingConfiguration to {} turns every log off", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := withoutWait(NewLoggingConfiguration(api))
		lastApplied := logging(testVC, true, true)
		api.store.Put(loggingType, testVC, lastApplied)
		current, err := h.Read(ctx, region, testVC)
		require.NoError(t, err)
		desired := map[string]any{"VoiceConnectorId": testVC, "LoggingConfiguration": map[string]any{}}

		// The patch UCP computes, as the engine does.
		currentJSON, _ := json.Marshal(current)
		desiredJSON, _ := json.Marshal(desired)
		patch, err := awsoperations.GeneratePatchWithOptions(currentJSON, desiredJSON, []byte(readFile(t, "aws-chime-voiceconnectorloggingconfiguration.json")), awsoperations.PatchOptions{
			Removed:     awsoperations.RemovedPaths(lastApplied, desired),
			LastApplied: lastApplied,
		})
		require.NoError(t, err)
		require.NoError(t, h.Update(ctx, native.UpdateRequest{Region: region, Identifier: testVC, Current: current, Desired: desired, Patch: patch}))
		require.Equal(t, logging(testVC, false, false), api.store.Resource(loggingType, testVC))
	})
}

func Test_LoggingConfiguration_UpdateWithBadPatchedValue(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := withoutWait(NewLoggingConfiguration(api))
	current := logging(testVC, true, false)
	request := update(t, current, nil)
	request.Desired = map[string]any{"VoiceConnectorId": testVC, "LoggingConfiguration": map[string]any{}}
	request.Patch = jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/LoggingConfiguration/EnableSIPLogs", Value: "true"}}
	requireRejected(t, h.Update(t.Context(), request), "LoggingConfiguration")
	require.Empty(t, api.calls, "no Put turns the logs off")
}

func Test_Fake_RejectsHostsThatArentRoutable(t *testing.T) {
	for _, host := range []string{"10.0.0.10", "127.0.0.1", "203.0.113.10", "2001:db8::10", "fe80::1"} {
		store := awsfake.NewCloudControl()
		err := putOrigination(t.Context(), store, &types.Origination{Routes: []types.OriginationRoute{sdkRoute(func(r *types.OriginationRoute) { r.Host = aws.String(host) })}})
		require.ErrorContains(t, err, "not permitted", host)
		require.Zero(t, store.Count(originationType), host)
	}
	require.NoError(t, putOrigination(t.Context(), awsfake.NewCloudControl(), &types.Origination{Routes: []types.OriginationRoute{sdkRoute(func(r *types.OriginationRoute) { r.Host = aws.String("54.240.196.186") })}}))
}

func Test_Settings_LostCreateResponse(t *testing.T) {
	ctx := t.Context()
	api := newFakeAPI(awsfake.NewCloudControl())
	h := withoutWait(NewOrigination(api))

	// The Put reached AWS, but UCP lost its response before it waited for a read to show it. AWS still reads as having
	// no settings for a while.
	_, err := api.PutVoiceConnectorOrigination(ctx, &chimesdkvoice.PutVoiceConnectorOriginationInput{VoiceConnectorId: aws.String(testVC), Origination: &types.Origination{Routes: []types.OriginationRoute{sdkRoute(nil)}}})
	require.NoError(t, err)
	progress, err := h.Progress(ctx, region, testVC, native.OperationCreate)
	require.NoError(t, err, "the lost create is found, not sent again or dropped from the record")
	require.True(t, progress.Done)
}

func Test_Settings_MissingRequiredParameter(t *testing.T) {
	h := withoutWait(NewStreamingConfiguration(newFakeAPI(awsfake.NewCloudControl())))
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: streaming(testVC, map[string]any{"DataRetentionInHours": 1})})
	requireRejected(t, err, "Disabled")
}

func Test_CreateProgress_StopsWhenTheRequestEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	reads := 0
	_, err := createProgress(ctx, func(context.Context, time.Duration) { cancel() }, func(context.Context, string, string) (map[string]any, error) {
		reads++
		return nil, native.ErrNotFound
	}, region, testVC)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, reads)
}
