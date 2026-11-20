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
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// requireFailure requires err to be a native.Failure with the outcome, caused by the request when client is true, and
// returns its message.
func requireFailure(t *testing.T, err error, outcome native.Outcome, client bool) string {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, outcome, failure.Outcome)
	require.Equal(t, client, failure.ClientError)
	return failure.Message
}

// withoutPassword returns the credentials of a template with the password of each username in names left out, as UCP
// leaves out a password whose fingerprint didn't change.
func withoutPassword(properties map[string]any, names ...string) map[string]any {
	list := []any{}
	for _, item := range properties[credentialsProperty].([]any) {
		object := item.(map[string]any)
		name := object[usernameField].(string)
		if contains(names, name) {
			object = map[string]any{usernameField: name}
		}
		list = append(list, object)
	}
	return map[string]any{voiceConnectorIDProperty: properties[voiceConnectorIDProperty], credentialsProperty: list}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// update returns the update UCP sends to go from current to desired, sending only the passwords of the usernames in
// sent.
func update(t *testing.T, identifier string, current map[string]any, desired map[string]any, sent ...string) native.UpdateRequest {
	t.Helper()
	unsent := []string{}
	for _, item := range desired[credentialsProperty].([]any) {
		if name := item.(map[string]any)[usernameField].(string); !contains(sent, name) {
			unsent = append(unsent, name)
		}
	}
	patch, err := jsondiff.Compare(current, withoutPassword(desired, unsent...))
	require.NoError(t, err)
	return native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: desired, Patch: patch}
}

func Test_Identifier(t *testing.T) {
	h := newTracked(awsfake.NewCloudControl())
	id, err := h.Identifier(map[string]any{"Id": testVC + "|x"})
	require.NoError(t, err)
	require.Equal(t, testVC+"|x", id, "existing reads by Id")

	_, err = h.Identifier(map[string]any{"Id": testVC + "|x", voiceConnectorIDProperty: testVC, credentialsProperty: []any{map[string]any{usernameField: "ana"}}})
	requireFailure(t, err, native.OutcomeRejected, true)

	id, err = h.Identifier(creds(testVC, "ana", "secret-ana"))
	require.NoError(t, err)
	require.Empty(t, id, "the identifier holds a name UCP generates from the client token")

	for name, properties := range map[string]map[string]any{
		"no Voice Connector":        creds("", "ana", "secret-ana"),
		"a Voice Connector with |":  creds("abc|def", "ana", "secret-ana"),
		"no credentials":            creds(testVC),
		"no username":               creds(testVC, "", "secret-ana"),
		"no password":               creds(testVC, "ana", ""),
		"a username listed twice":   creds(testVC, "ana", "secret-ana", "ana", "secret-other"),
		"a credential not a object": {voiceConnectorIDProperty: testVC, credentialsProperty: []any{"ana"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.Identifier(properties)
			message := requireFailure(t, err, native.OutcomeRejected, true)
			require.NotContains(t, message, "secret")
		})
	}
	require.Empty(t, h.api.calls)
}

func Test_Create(t *testing.T) {
	ctx := t.Context()
	identifier := testVC + "|" + native.GenerateName(token)

	t.Run("one Put, which a list shows before it returns", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		got, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "secret-ana", "bo", "secret-bo"), ClientToken: token})
		require.NoError(t, err)
		require.Equal(t, identifier, got)
		require.Equal(t, []string{"PutVoiceConnectorTerminationCredentials", "ListVoiceConnectorTerminationCredentials", "ListVoiceConnectorTerminationCredentials", "ListVoiceConnectorTerminationCredentials"}, h.api.calls, "the first lists return the earlier usernames")
		props, err := h.Read(ctx, region, got)
		require.NoError(t, err)
		require.Equal(t, read(identifier, testVC, "ana", "bo"), props)
		require.Equal(t, "secret-bo", h.api.password(testVC, "bo"))

		again, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "secret-ana", "bo", "secret-bo"), ClientToken: token})
		require.NoError(t, err)
		require.Equal(t, identifier, again, "a create sent again finds the same resource")
	})

	t.Run("sent again before a delete, without passwords", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		got, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: withoutPassword(creds(testVC, "ana", "secret-ana"), "ana"), ClientToken: token})
		require.NoError(t, err)
		require.Equal(t, identifier, got)
		require.Empty(t, h.api.calls, "nothing can be sent without the passwords")
	})

	t.Run("no credentials", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC), ClientToken: token})
		requireFailure(t, err, native.OutcomeRejected, true)
		require.Empty(t, h.api.calls)
	})

	t.Run("some passwords missing", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: withoutPassword(creds(testVC, "ana", "secret-ana", "bo", "secret-bo"), "bo"), ClientToken: token})
		requireFailure(t, err, native.OutcomeRejected, true)
		require.Empty(t, h.api.calls)
	})

	for name, vc := range map[string]string{"no termination settings": noTerminationVC, "no such Voice Connector": missingVC} {
		t.Run(name, func(t *testing.T) {
			h := newTracked(awsfake.NewCloudControl())
			got, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(vc, "ana", "secret-ana"), ClientToken: token})
			requireFailure(t, err, native.OutcomeRejected, true)
			require.Empty(t, got)
		})
	}

	t.Run("more than 10 credentials", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		pairs := []string{}
		for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"} {
			pairs = append(pairs, name, "secret-"+name)
		}
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, pairs...), ClientToken: token})
		message := requireFailure(t, err, native.OutcomeRejected, true)
		require.Contains(t, message, "limits: 10")
	})

	t.Run("an error that repeats a password", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		h.api.errs["PutVoiceConnectorTerminationCredentials"] = invalid("Value 'secret-ana' at 'credentials.1.member.password' failed to satisfy constraint")
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "secret-ana"), ClientToken: token})
		message := requireFailure(t, err, native.OutcomeRejected, true)
		require.NotContains(t, message, "secret-ana")
		require.Contains(t, message, "credentials.1.member.password")
	})

	t.Run("an error that repeats passwords that contain or overlap others", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		h.api.errs["PutVoiceConnectorTerminationCredentials"] = invalid("Values 'prefix-RemainingSIPSecret42', 'abcXYZ', and 'prefix' were rejected")
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "prefix", "bo", "prefix-RemainingSIPSecret42", "cy", "abc", "di", "cXYZ"), ClientToken: token})
		message := requireFailure(t, err, native.OutcomeRejected, true)
		require.Equal(t, "Values '***', '***', and '***' were rejected", message[len(message)-len("Values '***', '***', and '***' were rejected"):])
		for _, part := range []string{"prefix", "Remaining", "Secret42", "abc", "XYZ"} {
			require.NotContains(t, message, part)
		}
	})

	t.Run("a server error that repeats a password", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		h.api.errs["PutVoiceConnectorTerminationCredentials"] = apiError(http.StatusInternalServerError, "ServiceFailureException", "failed for secret-ana")
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "secret-ana"), ClientToken: token})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, native.OutcomeUnknown, failure.Outcome, "AWS may have applied it")
		require.NotContains(t, failure.Message, "secret-ana")
	})
}

func Test_ReadScoped(t *testing.T) {
	store := awsfake.NewCloudControl()
	h := newTracked(store)
	ctx := t.Context()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "secret-ana", "bo", "secret-bo"), ClientToken: token})
	require.NoError(t, err)
	store.Put(outsideType, testVC, creds(testVC, outsider, "secret-outsider"))

	props, err := h.Read(ctx, region, identifier)
	require.NoError(t, err)
	require.Equal(t, read(identifier, testVC, "ana", "bo", outsider), props, "a read without a record returns every username")

	props, err = h.ReadScoped(ctx, region, identifier, []map[string]any{creds(testVC, "Ana", "x", "bo", "x"), creds(testVC, "zed", "x")})
	require.NoError(t, err)
	require.Equal(t, read(identifier, testVC, "bo"), props, "usernames are case sensitive, so Ana isn't ana")

	_, err = h.ReadScoped(ctx, region, identifier, []map[string]any{creds(testVC, "zed", "x")})
	require.ErrorIs(t, err, native.ErrNotFound, "none of the managed usernames")
	_, err = h.Read(ctx, region, missingVC+"|x")
	require.ErrorIs(t, err, native.ErrNotFound, "no such Voice Connector")

	props, err = h.ReadScoped(ctx, region, "id-1", []map[string]any{{credentialsProperty: []any{}}, creds(testVC, "ana", "x")})
	require.NoError(t, err)
	require.Equal(t, testVC, props[voiceConnectorIDProperty], "an identifier without a Voice Connector takes it from the managed properties")

	_, err = h.Read(ctx, region, "id-1")
	requireFailure(t, err, native.OutcomeRejected, true)

	_, err = h.Read(ctx, region, "BAD|x")
	requireFailure(t, err, native.OutcomeRejected, true)
}

func Test_Update(t *testing.T) {
	ctx := t.Context()
	setup := func(t *testing.T) (*tracked, string) {
		h := newTracked(awsfake.NewCloudControl())
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "secret-ana", "bo", "secret-bo"), ClientToken: token})
		require.NoError(t, err)
		h.api.calls = nil
		return h, identifier
	}
	puts := func(h *tracked) []string {
		out := []string{}
		for _, call := range h.api.calls {
			if call != "ListVoiceConnectorTerminationCredentials" {
				out = append(out, call)
			}
		}
		return out
	}

	t.Run("deletes come first, then one Put of new usernames and sent passwords", func(t *testing.T) {
		h, identifier := setup(t)
		desired := creds(testVC, "bo", "secret-bo-2", "cy", "secret-cy")
		require.NoError(t, h.Update(ctx, update(t, identifier, read(identifier, testVC, "ana", "bo"), desired, "bo", "cy")))
		require.Equal(t, []string{"DeleteVoiceConnectorTerminationCredentials", "PutVoiceConnectorTerminationCredentials"}, puts(h))
		require.Equal(t, []string{"bo", "cy"}, h.api.usernames(testVC))
		require.Equal(t, "secret-bo-2", h.api.password(testVC, "bo"))
	})

	t.Run("an unsent password isn't sent", func(t *testing.T) {
		h, identifier := setup(t)
		desired := creds(testVC, "ana", "secret-ana", "bo", "secret-bo", "cy", "secret-cy")
		require.NoError(t, h.Update(ctx, update(t, identifier, read(identifier, testVC, "ana", "bo"), desired, "cy")))
		require.Equal(t, []string{"PutVoiceConnectorTerminationCredentials"}, puts(h))
		require.Equal(t, []string{"ana", "bo", "cy"}, h.api.usernames(testVC))
	})

	t.Run("a username AWS doesn't list is sent, even when the patch lines it up with another", func(t *testing.T) {
		h, identifier := setup(t)
		require.NoError(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{creds(testVC, "ana", "x")}))
		h.api.calls = nil
		// AWS lists bo only, so the patch turns bo's item into ana's and adds bo's item, without unsent passwords.
		desired := creds(testVC, "ana", "secret-ana", "bo", "secret-bo")
		require.NoError(t, h.Update(ctx, update(t, identifier, read(identifier, testVC, "bo"), desired)))
		require.Equal(t, []string{"PutVoiceConnectorTerminationCredentials"}, puts(h))
		require.Equal(t, []string{"ana", "bo"}, h.api.usernames(testVC))
	})

	t.Run("a failed delete is rejected", func(t *testing.T) {
		h, identifier := setup(t)
		h.api.errs["DeleteVoiceConnectorTerminationCredentials"] = apiError(http.StatusTooManyRequests, "ThrottledClientException", "slow down")
		err := h.Update(ctx, update(t, identifier, read(identifier, testVC, "ana", "bo"), creds(testVC, "bo", "secret-bo", "cy", "secret-cy"), "bo", "cy"))
		requireFailure(t, err, native.OutcomeRejected, true)
		require.Equal(t, []string{"DeleteVoiceConnectorTerminationCredentials"}, puts(h))
	})

	t.Run("a Put AWS rejects after a delete is partly applied", func(t *testing.T) {
		h, identifier := setup(t)
		err := h.Update(ctx, update(t, identifier, read(identifier, testVC, "ana", "bo"), creds(testVC, "bo", "secret-bo", "cy", "secret-cy"), "bo", "cy"))
		require.NoError(t, err)
		h.api.calls = nil
		h.api.errs["PutVoiceConnectorTerminationCredentials"] = badRequest("no")
		err = h.Update(ctx, update(t, identifier, read(identifier, testVC, "bo", "cy"), creds(testVC, "di", "secret-di"), "di"))
		requireFailure(t, err, native.OutcomePartial, true)
	})

	t.Run("a Put AWS rejects alone is rejected", func(t *testing.T) {
		h, identifier := setup(t)
		h.api.errs["PutVoiceConnectorTerminationCredentials"] = badRequest("no")
		err := h.Update(ctx, update(t, identifier, read(identifier, testVC, "ana", "bo"), creds(testVC, "ana", "secret-ana-2", "bo", "secret-bo"), "ana", "bo"))
		requireFailure(t, err, native.OutcomeRejected, true)
	})

	t.Run("a template without a password", func(t *testing.T) {
		h, identifier := setup(t)
		err := h.Update(ctx, native.UpdateRequest{Region: region, Identifier: identifier, Current: read(identifier, testVC, "ana"), Desired: withoutPassword(creds(testVC, "ana", "x"), "ana")})
		requireFailure(t, err, native.OutcomeRejected, true)
		require.Empty(t, h.api.calls)
	})

	t.Run("an identifier with no Voice Connector", func(t *testing.T) {
		h, _ := setup(t)
		err := h.Update(ctx, native.UpdateRequest{Region: region, Identifier: "id-1", Current: map[string]any{}, Desired: map[string]any{}})
		requireFailure(t, err, native.OutcomeRejected, true)
		require.Empty(t, h.api.calls)
	})

	t.Run("a patch that doesn't apply", func(t *testing.T) {
		h, identifier := setup(t)
		err := h.Update(ctx, native.UpdateRequest{Region: region, Identifier: identifier, Current: read(identifier, testVC, "ana"), Desired: creds(testVC, "ana", "x"), Patch: jsondiff.Patch{{Type: jsondiff.OperationRemove, Path: "/Missing/0"}}})
		requireFailure(t, err, native.OutcomeRejected, false)
		require.Empty(t, h.api.calls)
	})
}

func Test_Delete(t *testing.T) {
	store := awsfake.NewCloudControl()
	h := newTracked(store)
	ctx := t.Context()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "secret-ana", "bo", "secret-bo"), ClientToken: token})
	require.NoError(t, err)
	store.Put(outsideType, testVC, creds(testVC, outsider, "secret-outsider"))

	require.ErrorContains(t, h.Delete(ctx, region, identifier), "DeleteScoped")
	require.ErrorIs(t, h.DeleteScoped(ctx, region, identifier, nil), native.ErrNotFound, "no managed properties lists no usernames")

	h.api.errs["DeleteVoiceConnectorTerminationCredentials"] = apiError(http.StatusServiceUnavailable, "ServiceUnavailableException", "down")
	err = h.DeleteScoped(ctx, region, identifier, []map[string]any{creds(testVC, "ana", "x", "bo", "x")})
	require.ErrorContains(t, err, "ServiceUnavailableException")
	require.False(t, errors.Is(err, native.ErrNotFound))
	delete(h.api.errs, "DeleteVoiceConnectorTerminationCredentials")

	h.api.calls = nil
	require.NoError(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{creds(testVC, "ana", "x", "bo", "x")}))
	require.Equal(t, []string{"DeleteVoiceConnectorTerminationCredentials", "ListVoiceConnectorTerminationCredentials", "ListVoiceConnectorTerminationCredentials", "ListVoiceConnectorTerminationCredentials"}, h.api.calls, "the delete waits until a list no longer shows the usernames")
	require.Equal(t, []string{outsider}, h.api.usernames(testVC), "a credential added outside Radius stays")

	require.NoError(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{creds(testVC, "ana", "x")}), "AWS deletes a username it doesn't have")
	require.ErrorIs(t, h.DeleteScoped(ctx, region, missingVC+"|x", []map[string]any{creds(missingVC, "ana", "x")}), native.ErrNotFound)
	requireFailure(t, h.DeleteScoped(ctx, region, "id-1", nil), native.OutcomeRejected, true)

	t.Run("right after a Put whose response was lost", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		h.api.failer.FailOn(0, apiError(http.StatusInternalServerError, "ServiceFailureException", "lost"), true)
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: creds(testVC, "ana", "secret-ana"), ClientToken: token})
		require.Error(t, err)
		// UCP sends the lost create again without passwords, then deletes. A list still misses ana.
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: withoutPassword(creds(testVC, "ana", "x"), "ana"), ClientToken: token})
		require.NoError(t, err)
		require.NoError(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{withoutPassword(creds(testVC, "ana", "x"), "ana")}))
		require.Empty(t, h.api.usernames(testVC), "the delete doesn't trust a list that misses the Put")
	})
}

func Test_Redact(t *testing.T) {
	for _, tc := range []struct {
		name      string
		message   string
		passwords []string
		want      string
	}{
		{"a password inside another, shorter first", "got prefix-rest", []string{"prefix", "prefix-rest"}, "got ***"},
		{"a password inside another, longer first", "got prefix-rest", []string{"prefix-rest", "prefix"}, "got ***"},
		{"passwords that overlap", "got abcdef", []string{"abcd", "cdef"}, "got ***"},
		{"a password repeated", "aa1 and aa1", []string{"aa1"}, "*** and ***"},
		{"a password that overlaps itself", "xaaax", []string{"aa"}, "x***x"},
		{"passwords side by side", "onetwo!", []string{"one", "two"}, "***!"},
		{"no password, or an empty one", "nothing here", []string{""}, "nothing here"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			credentials := []credential{}
			for _, p := range tc.passwords {
				credentials = append(credentials, credential{username: "u", password: p})
			}
			require.Equal(t, tc.want, redact(tc.message, credentials))
		})
	}
}

func Test_Settle(t *testing.T) {
	t.Run("stops after confirmReads lists", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		h.settle(t.Context(), region, testVC, func([]string) bool { return false })
		require.Len(t, h.api.calls, confirmReads)
	})

	t.Run("stops when a list fails", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		h.settle(t.Context(), region, missingVC, func([]string) bool { return false })
		require.Len(t, h.api.calls, 1)
	})

	t.Run("stops when the context ends", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		ctx, cancel := context.WithCancel(t.Context())
		h.wait = func(context.Context, time.Duration) { cancel() }
		h.settle(ctx, region, testVC, func([]string) bool { return false })
		require.Len(t, h.api.calls, 1)
	})

	t.Run("sleep waits, or returns when the context ends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		sleep(ctx, time.Hour)
		sleep(t.Context(), time.Millisecond)
	})
}
