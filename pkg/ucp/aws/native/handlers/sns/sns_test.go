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

package sns

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region = "us-west-2"

	// testName is the name of the application the tests create, testIdentifier and testARN name it on APNs, and
	// testTopic and otherTopic are topics its events go to.
	testName       = "my-app"
	testIdentifier = "APNS|" + testName
	testARN        = "arn:aws:sns:us-west-2:123456789012:app/APNS/" + testName
	testTopic      = "arn:aws:sns:us-west-2:123456789012:events"
	otherTopic     = "arn:aws:sns:us-west-2:123456789012:other-events"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// newHandler returns a handler that doesn't wait, with a fake backed by store.
func newHandler(store *awsfake.CloudControl) (*PlatformApplication, *fakeAPI) {
	api := newFakeAPI(store)
	h := NewPlatformApplication(api)
	h.wait = noWait
	return h, api
}

// application returns the template of an APNs application with token credentials, changed by change.
func application(change func(map[string]any)) map[string]any {
	out := map[string]any{
		"Platform":              "APNS",
		"Name":                  testName,
		"PlatformPrincipal":     "ABC123DEFG",
		"PlatformCredential":    "signing-key",
		"ApplePlatformTeamID":   "ABCDE12345",
		"ApplePlatformBundleID": "com.example.app",
		"EventEndpointCreated":  testTopic,
	}
	if change != nil {
		change(out)
	}
	return out
}

func Test_Suite_PlatformApplication(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    platformApplicationType,
		Schema:      readFile(t, "aws-sns-platformapplication.json"),
		Metadata:    readFile(t, "aws-sns-platformapplication.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h, _ := newHandler(store)
			return h
		},
		WriteOnly:  []string{"PlatformCredential", "PlatformPrincipal"},
		Properties: application(nil),
		Identifier: testIdentifier,
		Changed: application(func(p map[string]any) {
			p["EventEndpointCreated"] = otherTopic
			p["EventEndpointDeleted"] = testTopic
		}),
		ChangedWriteOnly: application(func(p map[string]any) { p["PlatformCredential"] = "new-signing-key" }),
		Removable:        "EventEndpointCreated",
		CreateOnlyChange: application(func(p map[string]any) { p["Platform"] = "APNS_SANDBOX" }),
		IdentifierProperties: func(identifier string) map[string]any {
			platform, name, _ := strings.Cut(identifier, "|")
			return map[string]any{"Platform": platform, "Name": name}
		},
		// A name the identifier can't hold, such as one with "/", would name another ARN.
		Invalid:      []map[string]any{application(func(p map[string]any) { p["Name"] = "a/b" })},
		SameResource: []map[string]any{application(func(p map[string]any) { p["EventEndpointCreated"] = otherTopic })},
		API:          platformApplicationAPI,
	})
}

// update returns the update from current, a read, to desired, with the patch UCP computes: it leaves out read-only
// values, and write-only values whose fingerprints are unchanged, unless secrets changed.
func update(t *testing.T, current map[string]any, desired map[string]any, secretsChanged bool) native.UpdateRequest {
	t.Helper()
	from := maps.Clone(current)
	for _, readOnly := range []string{arnProperty, "Enabled", "AuthenticationMethod", "AppleCertificateExpirationDate"} {
		delete(from, readOnly)
	}
	to := desired
	if !secretsChanged {
		to = withoutSecrets(desired)
	}
	patch, err := jsondiff.Compare(from, to)
	require.NoError(t, err)
	return native.UpdateRequest{Region: region, Identifier: testIdentifier, Current: current, Desired: desired, Patch: patch}
}

// requireRejected requires err to be a client failure AWS applied none of, whose message contains text.
func requireRejected(t *testing.T, err error, text string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, text)
}

// withoutSecrets returns properties without the write-only values, as UCP gives an update's Current.
func withoutSecrets(properties map[string]any) map[string]any {
	out := maps.Clone(properties)
	for _, secret := range secretAttributes {
		delete(out, secret)
	}
	return out
}

func Test_PlatformApplication_Read(t *testing.T) {
	h, api := newHandler(awsfake.NewCloudControl())
	ctx := t.Context()
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: application(nil)})
	require.NoError(t, err)

	got, err := h.Read(ctx, region, testIdentifier)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"Platform":               "APNS",
		"Name":                   testName,
		"PlatformApplicationArn": testARN,
		"ApplePlatformTeamID":    "ABCDE12345",
		"ApplePlatformBundleID":  "com.example.app",
		"EventEndpointCreated":   testTopic,
		"Enabled":                "true",
		"AuthenticationMethod":   "Token",
	}, got, "a read has no credentials")

	t.Run("an attribute the type doesn't have is left out", func(t *testing.T) {
		props, _ := api.store.Read(platformApplicationType, testIdentifier)
		props["SomeNewSecret"] = "secret"
		api.store.Put(platformApplicationType, testIdentifier, props)
		got, err := h.Read(ctx, region, testIdentifier)
		require.NoError(t, err)
		require.NotContains(t, got, "SomeNewSecret")
	})

	t.Run("a cleared feedback role AWS returns as empty is left out", func(t *testing.T) {
		props, _ := api.store.Read(platformApplicationType, testIdentifier)
		props["SuccessFeedbackRoleArn"] = ""
		api.store.Put(platformApplicationType, testIdentifier, props)
		got, err := h.Read(ctx, region, testIdentifier)
		require.NoError(t, err)
		require.NotContains(t, got, "SuccessFeedbackRoleArn")
	})

	t.Run("an identifier that isn't a platform and a name is rejected before any call", func(t *testing.T) {
		api.calls = nil
		for _, identifier := range []string{testName, "APNS|a/b", "apns|" + testName, "APNS|"} {
			_, err := h.Read(ctx, region, identifier)
			requireRejected(t, err, "Platform and Name")
		}
		require.Empty(t, api.calls)
	})

	t.Run("an error is returned as it is, so waiting for an operation AWS applied doesn't fail it", func(t *testing.T) {
		api.errs["ListPlatformApplications"] = apiError(http.StatusBadRequest, "Throttling", "Rate exceeded")
		defer delete(api.errs, "ListPlatformApplications")
		_, err := h.Progress(ctx, region, testIdentifier, native.OperationCreate)
		require.ErrorContains(t, err, "Rate exceeded")
		var failure *native.Failure
		require.False(t, errors.As(err, &failure))
	})

	t.Run("a missing application is not found", func(t *testing.T) {
		_, err := h.Read(ctx, region, "GCM|missing")
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_PlatformApplication_CreateWaitsForRead(t *testing.T) {
	h, api := newHandler(awsfake.NewCloudControl())
	ctx := t.Context()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: application(nil)})
	require.NoError(t, err)
	require.Equal(t, testIdentifier, identifier)
	require.Equal(t, staleReads+1, count(api.calls, "ListPlatformApplications"), "the create reads until a read finds the application")

	progress, err := h.Progress(ctx, region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_PlatformApplication_Update(t *testing.T) {
	ctx := t.Context()
	// withRate is a template that keeps the sample rate the application is created with, changed by change.
	withRate := func(change func(map[string]any)) map[string]any {
		return application(func(p map[string]any) {
			p[sampleRateAttribute] = "50"
			if change != nil {
				change(p)
			}
		})
	}
	setup := func(t *testing.T) (*PlatformApplication, *fakeAPI, map[string]any) {
		h, api := newHandler(awsfake.NewCloudControl())
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: withRate(nil)})
		require.NoError(t, err)
		current, err := h.Read(ctx, region, testIdentifier)
		require.NoError(t, err)
		api.calls = nil
		return h, api, current
	}

	t.Run("a removed topic is cleared", func(t *testing.T) {
		h, _, current := setup(t)
		require.NoError(t, h.Update(ctx, update(t, current, withRate(func(p map[string]any) { delete(p, "EventEndpointCreated") }), false)))
		got, err := h.Read(ctx, region, testIdentifier)
		require.NoError(t, err)
		require.NotContains(t, got, "EventEndpointCreated")
	})

	t.Run("a removed sample rate is rejected before any call", func(t *testing.T) {
		h, api, current := setup(t)
		err := h.Update(ctx, update(t, current, application(nil), false))
		requireRejected(t, err, sampleRateAttribute)
		require.Empty(t, api.calls)
	})

	t.Run("a changed bundle ID sends the unchanged secrets with it", func(t *testing.T) {
		h, api, current := setup(t)
		desired := withRate(func(p map[string]any) { p["ApplePlatformBundleID"] = "com.example.other" })
		require.NoError(t, h.Update(ctx, update(t, current, desired, false)), "AWS takes the bundle ID only with the other credentials")
		props, _ := api.store.Read(platformApplicationType, testIdentifier)
		require.Equal(t, "com.example.other", props["ApplePlatformBundleID"])
		require.Contains(t, api.calls, "SetPlatformApplicationAttributes")
	})

	t.Run("a changed topic sends no credentials", func(t *testing.T) {
		h, api, current := setup(t)
		props, _ := api.store.Read(platformApplicationType, testIdentifier)
		props["PlatformCredential"] = "rotated-outside-the-template"
		api.store.Put(platformApplicationType, testIdentifier, props)
		require.NoError(t, h.Update(ctx, update(t, current, withRate(func(p map[string]any) { p["EventEndpointCreated"] = otherTopic }), false)))
		props, _ = api.store.Read(platformApplicationType, testIdentifier)
		require.Equal(t, "rotated-outside-the-template", props["PlatformCredential"], "only the topic is sent")
		require.Equal(t, otherTopic, props["EventEndpointCreated"])
	})

	t.Run("certificate credentials replace token credentials", func(t *testing.T) {
		h, _, current := setup(t)
		desired := withRate(func(p map[string]any) {
			delete(p, "ApplePlatformTeamID")
			delete(p, "ApplePlatformBundleID")
			p["PlatformPrincipal"] = "certificate"
			p["PlatformCredential"] = "private-key"
		})
		require.NoError(t, h.Update(ctx, update(t, current, desired, true)))
		got, err := h.Read(ctx, region, testIdentifier)
		require.NoError(t, err)
		require.Equal(t, "Certificate", got["AuthenticationMethod"], "the update waits until a read shows the new credentials")
		require.NotContains(t, got, "ApplePlatformTeamID")
	})

	t.Run("an update that changes nothing AWS stores makes no call", func(t *testing.T) {
		h, api, current := setup(t)
		require.NoError(t, h.Update(ctx, update(t, current, withRate(nil), false)))
		require.Empty(t, api.calls)
	})

	t.Run("an empty value AWS doesn't have is no change", func(t *testing.T) {
		h, api, current := setup(t)
		desired := withRate(func(p map[string]any) {
			p["EventEndpointDeleted"] = ""
			p["SuccessFeedbackRoleArn"] = ""
		})
		require.NoError(t, h.Update(ctx, update(t, current, desired, false)))
		require.Empty(t, api.calls, "Read leaves out empty values, so every deploy has them in its patch")
	})

	t.Run("an empty team ID AWS doesn't have doesn't resend the credentials", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		gcm := map[string]any{"Platform": "GCM", "Name": testName, "PlatformCredential": "key", "ApplePlatformTeamID": ""}
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: gcm})
		require.NoError(t, err)
		current, err := h.Read(ctx, region, "GCM|"+testName)
		require.NoError(t, err)
		api.calls = nil
		request := update(t, current, gcm, false)
		request.Identifier = "GCM|" + testName
		require.NoError(t, h.Update(ctx, request))
		require.Empty(t, api.calls)
	})

	t.Run("an empty value clears what AWS has", func(t *testing.T) {
		h, api, current := setup(t)
		require.NoError(t, h.Update(ctx, update(t, current, withRate(func(p map[string]any) { p["EventEndpointCreated"] = "" }), false)))
		props, _ := api.store.Read(platformApplicationType, testIdentifier)
		require.NotContains(t, props, "EventEndpointCreated")
	})

	t.Run("an update of a missing application is not found", func(t *testing.T) {
		h, api, current := setup(t)
		api.store.Remove(platformApplicationType, testIdentifier)
		api.stale = map[string]*stale{}
		err := h.Update(ctx, update(t, current, withRate(func(p map[string]any) { p["EventEndpointCreated"] = otherTopic }), false))
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("an update of an application deleted after the list is not found", func(t *testing.T) {
		h, api, current := setup(t)
		api.errs["SetPlatformApplicationAttributes"] = notFound()
		err := h.Update(ctx, update(t, current, withRate(func(p map[string]any) { p["EventEndpointCreated"] = otherTopic }), false))
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_PlatformApplication_Delete(t *testing.T) {
	h, api := newHandler(awsfake.NewCloudControl())
	ctx := t.Context()
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: application(nil)})
	require.NoError(t, err)

	require.NoError(t, h.Delete(ctx, region, testIdentifier))
	progress, err := h.Progress(ctx, region, testIdentifier, native.OperationDelete)
	require.NoError(t, err)
	require.False(t, progress.Done, "a read right after the delete still finds the application")
	require.Equal(t, "deleted", progress.Waiting)
	for range staleReads {
		_, err = h.Progress(ctx, region, testIdentifier, native.OperationDelete)
	}
	require.ErrorIs(t, err, native.ErrNotFound)

	require.ErrorIs(t, h.Delete(ctx, region, testIdentifier), native.ErrNotFound, "a delete of a deleted application finishes")
	_, ok := api.store.Read(platformApplicationType, testIdentifier)
	require.False(t, ok)
}

func Test_PlatformApplication_List(t *testing.T) {
	h, api := newHandler(awsfake.NewCloudControl())
	ctx := t.Context()
	for _, name := range []string{"a", "b", "c"} {
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"Platform": "GCM", "Name": name, "PlatformCredential": "key"}})
		require.NoError(t, err)
	}
	resources, err := h.List(ctx, region)
	require.NoError(t, err)
	identifiers := []string{}
	for _, r := range resources {
		identifiers = append(identifiers, r.Identifier)
		require.NotContains(t, r.Properties, "PlatformCredential")
	}
	require.Equal(t, []string{"GCM|a", "GCM|b", "GCM|c"}, identifiers, "every page")

	api.errs["ListPlatformApplications"] = apiError(http.StatusForbidden, "AuthorizationError", "denied")
	_, err = h.List(ctx, region)
	requireRejected(t, err, "denied")
}

func Test_PlatformAndName(t *testing.T) {
	for arn, want := range map[string][]string{
		"arn:aws:sns:us-west-2:123456789012:app/GCM/my-app":             {"GCM", "my-app"},
		"arn:aws-cn:sns:cn-north-1:123456789012:app/APNS_SANDBOX/a.b_c": {"APNS_SANDBOX", "a.b_c"},
		"arn:aws:sns:us-west-2:123456789012:topic":                      nil,
		"arn:aws:sns:us-west-2:123456789012:app/GCM":                    nil,
		"arn:aws:sns:us-west-2:123456789012:endpoint/GCM/my-app/1":      nil,
	} {
		platform, name, ok := platformAndName(arn)
		require.Equal(t, want != nil, ok, arn)
		if want != nil {
			require.Equal(t, want, []string{platform, name}, arn)
		}
	}
}

func count(calls []string, method string) int {
	n := 0
	for _, call := range calls {
		if call == method {
			n++
		}
	}
	return n
}

// requireUnknown requires err to be a failure whose outcome is unknown, which asks to deploy again and contains text.
func requireUnknown(t *testing.T, err error, text string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.False(t, failure.ClientError)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome)
	require.Contains(t, failure.Message, "Deploy again")
	require.Contains(t, failure.Message, text)
}

func Test_PlatformApplication_UnconfirmedChange(t *testing.T) {
	ctx := t.Context()

	t.Run("a create that reads keep failing to show is unknown", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		h.wait = func(context.Context, time.Duration) {
			api.errs["ListPlatformApplications"] = apiError(http.StatusBadRequest, "Throttling", "Rate exceeded")
		}
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: application(nil)})
		requireUnknown(t, err, "Rate exceeded")
		require.Equal(t, testIdentifier, identifier, "UCP keeps the identifier of a create AWS accepted")
		require.Equal(t, confirmReads, count(api.calls, "ListPlatformApplications"))
	})

	t.Run("a create no read shows in time is unknown", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		reads := 0
		h.wait = func(context.Context, time.Duration) {
			// AWS keeps showing no application for longer than the handler waits.
			reads++
			api.stale[testIdentifier].reads = confirmReads
		}
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: application(nil)})
		requireUnknown(t, err, "no read showed it")
		require.Equal(t, confirmReads-1, reads)
	})

	t.Run("a create whose context ends is unknown", func(t *testing.T) {
		h, _ := newHandler(awsfake.NewCloudControl())
		ctx, cancel := context.WithCancel(ctx)
		h.wait = func(context.Context, time.Duration) { cancel() }
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: application(nil)})
		requireUnknown(t, err, "context canceled")
	})

	t.Run("an update no read shows in time is unknown", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: application(nil)})
		require.NoError(t, err)
		current, err := h.Read(ctx, region, testIdentifier)
		require.NoError(t, err)
		h.wait = func(context.Context, time.Duration) { api.stale[testIdentifier].reads = confirmReads }
		err = h.Update(ctx, update(t, current, application(func(p map[string]any) { p["EventEndpointCreated"] = otherTopic }), false))
		requireUnknown(t, err, "no read showed it")
	})

	t.Run("sleep returns when the context ends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		sleep(ctx, time.Hour)
	})
}

func Test_PlatformApplication_CreateProgress(t *testing.T) {
	ctx := t.Context()

	t.Run("a lost create waits for a read to show the application", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		_, err := api.CreatePlatformApplication(ctx, &sns.CreatePlatformApplicationInput{Name: aws.String(testName), Platform: aws.String("APNS"), Attributes: attributes(application(nil), credentialAttributes...)})
		require.NoError(t, err)
		api.calls = nil
		progress, err := h.Progress(ctx, region, testIdentifier, native.OperationCreate)
		require.NoError(t, err)
		require.True(t, progress.Done)
		require.Equal(t, staleReads+1, count(api.calls, "ListPlatformApplications"))
	})

	t.Run("a create AWS never got is not found after the reads", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		_, err := h.Progress(ctx, region, testIdentifier, native.OperationCreate)
		require.ErrorIs(t, err, native.ErrNotFound)
		require.Equal(t, confirmReads, count(api.calls, "ListPlatformApplications"))
	})

	t.Run("a context that ends stops the reads", func(t *testing.T) {
		h, _ := newHandler(awsfake.NewCloudControl())
		ctx, cancel := context.WithCancel(ctx)
		h.wait = func(context.Context, time.Duration) { cancel() }
		_, err := h.Progress(ctx, region, testIdentifier, native.OperationCreate)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func Test_PlatformApplication_InvalidIdentifier(t *testing.T) {
	h, api := newHandler(awsfake.NewCloudControl())
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: application(func(p map[string]any) { p["Name"] = "a/b" })})
	requireRejected(t, err, "Platform and Name")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: application(func(p map[string]any) { delete(p, "Name") })})
	requireRejected(t, err, "Platform and Name")
	requireRejected(t, h.Delete(t.Context(), region, testName), "Platform and Name")
	require.Empty(t, api.calls)
}
