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
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice/types"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	credentialsType = "AWS::Chime::VoiceConnectorTerminationCredentials"

	voiceConnectorIDProperty = "VoiceConnectorId"
	credentialsProperty      = "Credentials"
	usernameField            = "Username"
	passwordField            = "Password"
)

var (
	_ native.Handler           = (*TerminationCredentials)(nil)
	_ native.IdentifierEncoder = (*TerminationCredentials)(nil)
	_ native.ScopedReader      = (*TerminationCredentials)(nil)
	_ native.ScopedDeleter     = (*TerminationCredentials)(nil)
)

// TerminationCredentials serves AWS::Chime::VoiceConnectorTerminationCredentials: the SIP credentials in Credentials,
// as some of the credentials of the Voice Connector VoiceConnectorId. Others can add credentials to the same Voice
// Connector, so the handler reads, changes, and deletes only the usernames the template lists, and leaves the others.
// Its identifier is VoiceConnectorId and a name UCP generates from the record's client token, joined with "|", so
// several templates can add credentials to one Voice Connector.
//
// A Put adds the usernames it doesn't have and sets the password of those it has, so UCP finds a lost create by
// sending it again. AWS lists only usernames, never passwords, and usernames that differ only by case are different
// credentials. Passwords are write-only: UCP sends one only when its fingerprint changed, and the handler keeps them
// out of every error it returns.
type TerminationCredentials struct {
	api  API
	wait pause
}

// NewTerminationCredentials returns the handler for the SIP credentials of a Voice Connector.
func NewTerminationCredentials(api API) *TerminationCredentials {
	return &TerminationCredentials{api: api, wait: sleep}
}

// TypeName returns AWS::Chime::VoiceConnectorTerminationCredentials.
func (h *TerminationCredentials) TypeName() string {
	return credentialsType
}

// Options returns how UCP uses the handler.
func (h *TerminationCredentials) Options() native.Options {
	return native.Options{
		List:     native.ListNone,
		Recovery: native.RecoverByResend,
		Members:  &native.Members{Parent: voiceConnectorIDProperty, List: credentialsProperty, Key: usernameField},
	}
}

// credential is a username and the password a template sets for it.
type credential struct {
	username string
	password string
}

// voiceConnectorOf returns the Voice Connector ID in identifier. An identifier without one, such as one another
// handler assigned, takes VoiceConnectorId from the first of properties that sets it.
func voiceConnectorOf(identifier string, properties ...map[string]any) (string, error) {
	if vc, _, ok := strings.Cut(identifier, "|"); ok && vc != "" {
		return vc, nil
	}
	for _, p := range properties {
		if vc := awsapi.String(p, voiceConnectorIDProperty); vc != "" {
			return vc, nil
		}
	}
	return "", rejected("the identifier %q of the AWS type %s names no Voice Connector", identifier, credentialsType)
}

// usernamesOf returns the usernames the property sets list, once each.
func usernamesOf(sets ...map[string]any) []string {
	out := []string{}
	for _, p := range sets {
		list, _ := p[credentialsProperty].([]any)
		for _, item := range list {
			object, _ := item.(map[string]any)
			if name, _ := object[usernameField].(string); name != "" && !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	return out
}

// template returns the Voice Connector ID and the credentials of properties, with a password only for those that set
// one. It returns a client failure for properties AWS would reject or Radius can't track: no credentials, a
// credential without a username, or a username listed twice, which a Put takes once with either password.
func template(properties map[string]any) (string, []credential, error) {
	vc := awsapi.String(properties, voiceConnectorIDProperty)
	if vc == "" || strings.Contains(vc, "|") {
		return "", nil, rejected("set %s to the ID of a Voice Connector", voiceConnectorIDProperty)
	}
	list, _ := properties[credentialsProperty].([]any)
	if len(list) == 0 {
		return "", nil, rejected("set at least one credential in %s", credentialsProperty)
	}
	out := []credential{}
	for i, item := range list {
		object, _ := item.(map[string]any)
		username, _ := object[usernameField].(string)
		password, _ := object[passwordField].(string)
		if username == "" {
			return "", nil, rejected("set %s of item %d of %s", usernameField, i, credentialsProperty)
		}
		if slices.ContainsFunc(out, func(c credential) bool { return c.username == username }) {
			return "", nil, rejected("item %d of %s has the %s of an earlier item. List each username once", i, credentialsProperty, usernameField)
		}
		out = append(out, credential{username: username, password: password})
	}
	return vc, out, nil
}

// withPasswords returns the Voice Connector ID and the credentials of a template, as template does, and a client
// failure when a credential sets no password.
func withPasswords(properties map[string]any) (string, []credential, error) {
	vc, credentials, err := template(properties)
	if err != nil {
		return "", nil, err
	}
	if i := slices.IndexFunc(credentials, func(c credential) bool { return c.password == "" }); i >= 0 {
		return "", nil, rejected("set %s of item %d of %s", passwordField, i, credentialsProperty)
	}
	return vc, credentials, nil
}

// Identifier returns Id when the properties set it, as for `existing`, and "" otherwise, because the identifier holds
// a name UCP generates from the record's client token. UCP calls it with a template before any AWS call, so properties
// that set credentials AWS would reject or Radius can't track are rejected here, even with Id.
func (h *TerminationCredentials) Identifier(properties map[string]any) (string, error) {
	id := awsapi.String(properties, "Id")
	if _, ok := properties[credentialsProperty]; ok || id == "" {
		if _, _, err := withPasswords(properties); err != nil {
			return "", err
		}
	}
	return id, nil
}

// usernames returns the usernames of the Voice Connector's credentials, or ErrNotFound when AWS has no such Voice
// Connector.
func (h *TerminationCredentials) usernames(ctx context.Context, region string, vc string) ([]string, error) {
	out, err := h.api.ListVoiceConnectorTerminationCredentials(ctx, &chimesdkvoice.ListVoiceConnectorTerminationCredentialsInput{VoiceConnectorId: aws.String(vc)}, inRegion(region))
	if err != nil {
		return nil, notFoundOrRejected(err)
	}
	return out.Usernames, nil
}

// settle lists the Voice Connector's usernames until shown reports that a list shows a change. It stops early when a
// list fails, and after confirmReads lists, because the change succeeded either way: a deploy that still reads the
// earlier usernames sends the same change again, which changes nothing.
func (h *TerminationCredentials) settle(ctx context.Context, region string, vc string, shown func(present []string) bool) {
	for i := range confirmReads {
		if i > 0 {
			h.wait(ctx, confirmPause)
		}
		if ctx.Err() != nil {
			return
		}
		present, err := h.usernames(ctx, region, vc)
		if err != nil || shown(present) {
			return
		}
	}
}

// Read returns every credential of the Voice Connector.
func (h *TerminationCredentials) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	return h.ReadScoped(ctx, region, identifier, nil)
}

// ReadScoped returns the credentials of the Voice Connector whose usernames managed lists, or every credential when
// managed is nil, each with its username only. It returns ErrNotFound when the Voice Connector has none of them, or
// AWS has no such Voice Connector.
func (h *TerminationCredentials) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	vc, err := voiceConnectorOf(identifier, managed...)
	if err != nil {
		return nil, err
	}
	present, err := h.usernames(ctx, region, vc)
	if err != nil {
		return nil, err
	}
	listed := usernamesOf(managed...)
	list := []any{}
	for _, name := range present {
		if managed == nil || slices.Contains(listed, name) {
			list = append(list, map[string]any{usernameField: name})
		}
	}
	if len(list) == 0 {
		return nil, native.ErrNotFound
	}
	return map[string]any{"Id": identifier, voiceConnectorIDProperty: vc, credentialsProperty: list}, nil
}

// put sets the credentials on the Voice Connector, in one call, and waits until a list shows their usernames.
func (h *TerminationCredentials) put(ctx context.Context, region string, vc string, credentials []credential) error {
	input := &chimesdkvoice.PutVoiceConnectorTerminationCredentialsInput{VoiceConnectorId: aws.String(vc)}
	for _, c := range credentials {
		input.Credentials = append(input.Credentials, types.Credential{Username: aws.String(c.username), Password: aws.String(c.password)})
	}
	if _, err := h.api.PutVoiceConnectorTerminationCredentials(ctx, input, inRegion(region)); err != nil {
		return withoutPasswords(err, credentials)
	}
	h.settle(ctx, region, vc, func(present []string) bool {
		return !slices.ContainsFunc(credentials, func(c credential) bool { return !slices.Contains(present, c.username) })
	})
	return nil
}

// withoutPasswords returns err, from a Put of credentials, as a failure whose message has none of their passwords,
// because AWS's message can repeat a value it was sent, and UCP shows and logs the message. A failure AWS answered
// with a 4xx status, or as a client fault, applied none of the Put.
func withoutPasswords(err error, credentials []credential) error {
	failure := &native.Failure{Message: redact(err.Error(), credentials), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeUnknown}
	var answered *native.Failure
	if errors.As(awsapi.Rejected(err), &answered) {
		failure.ClientError = answered.ClientError
	}
	if failure.ClientError {
		failure.Outcome = native.OutcomeRejected
	}
	return failure
}

// redact returns message with "***" in place of each part that is one of the credentials' passwords. It finds every
// part in the original message before it replaces any, so a password that contains or overlaps another is hidden
// whole.
func redact(message string, credentials []credential) string {
	hidden := make([]bool, len(message))
	for _, c := range credentials {
		if c.password == "" {
			continue
		}
		for start := 0; start < len(message); start++ {
			at := strings.Index(message[start:], c.password)
			if at < 0 {
				break
			}
			start += at
			for i := start; i < start+len(c.password); i++ {
				hidden[i] = true
			}
		}
	}
	var out strings.Builder
	for i := 0; i < len(message); {
		if !hidden[i] {
			out.WriteByte(message[i])
			i++
			continue
		}
		out.WriteString("***")
		for i < len(message) && hidden[i] {
			i++
		}
	}
	return out.String()
}

// remove deletes the credentials with the usernames from the Voice Connector, in one call, and waits until a list no
// longer shows them. AWS deletes a username the Voice Connector doesn't have without an error.
func (h *TerminationCredentials) remove(ctx context.Context, region string, vc string, names []string) error {
	_, err := h.api.DeleteVoiceConnectorTerminationCredentials(ctx, &chimesdkvoice.DeleteVoiceConnectorTerminationCredentialsInput{
		VoiceConnectorId: aws.String(vc),
		Usernames:        names,
	}, inRegion(region))
	if err != nil {
		return notFoundOrRejected(err)
	}
	h.settle(ctx, region, vc, func(present []string) bool {
		return !slices.ContainsFunc(names, func(name string) bool { return slices.Contains(present, name) })
	})
	return nil
}

// Create sets the template's credentials on the Voice Connector and returns the identifier. AWS rejects the Put when
// the Voice Connector has no termination settings, or would then have more than 10 credentials.
//
// Before a delete, UCP sends a lost create again without write-only values, so with no passwords. The identifier
// depends only on the client token, so Create then returns it without a call, and the delete removes the usernames the
// Voice Connector has. A template always sets every password, because Identifier rejects one that doesn't.
func (h *TerminationCredentials) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	vc, credentials, err := template(request.Properties)
	if err != nil {
		return "", err
	}
	identifier := vc + "|" + native.GenerateName(request.ClientToken)
	if !slices.ContainsFunc(credentials, func(c credential) bool { return c.password != "" }) {
		return identifier, nil
	}
	if _, credentials, err = withPasswords(request.Properties); err != nil {
		return "", err
	}
	if err := h.put(ctx, request.Region, vc, credentials); err != nil {
		return "", err
	}
	return identifier, nil
}

// Update deletes the usernames the template no longer lists, then sets the credentials AWS doesn't list and those
// whose password UCP sends, because its fingerprint changed. A failure of the delete is rejected, so the record keeps
// every username it may still have to delete. A failure of the Put after the delete is partly applied.
func (h *TerminationCredentials) Update(ctx context.Context, request native.UpdateRequest) error {
	vc, err := voiceConnectorOf(request.Identifier, request.Desired, request.Current)
	if err != nil {
		return err
	}
	_, desired, err := withPasswords(request.Desired)
	if err != nil {
		return err
	}
	target, err := patched(request)
	if err != nil {
		return err
	}
	current := usernamesOf(request.Current)
	sent := sentPasswords(target)
	calls := []func() error{}
	removed := slices.DeleteFunc(slices.Clone(current), func(name string) bool {
		return slices.ContainsFunc(desired, func(c credential) bool { return c.username == name })
	})
	if len(removed) > 0 {
		calls = append(calls, func() error {
			if err := h.remove(ctx, request.Region, vc, removed); err != nil {
				return &native.Failure{Message: err.Error(), ClientError: isClientError(err), Outcome: native.OutcomeRejected}
			}
			return nil
		})
	}
	set := slices.DeleteFunc(slices.Clone(desired), func(c credential) bool {
		return slices.Contains(current, c.username) && !sent[c.username]
	})
	if len(set) > 0 {
		calls = append(calls, func() error {
			err := h.put(ctx, request.Region, vc, set)
			// The Put's failure says AWS applied none of the Put, but the delete before it changed AWS.
			var failure *native.Failure
			if len(removed) > 0 && errors.As(err, &failure) {
				failure.Outcome = native.OutcomePartial
			}
			return err
		})
	}
	return awsapi.Calls(calls...)
}

// isClientError reports whether the request caused err: AWS answered it as a client fault, or it is a native.Failure
// the request caused.
func isClientError(err error) bool {
	var failure *native.Failure
	return ucp_aws.IsClientFault(err) || errors.As(err, &failure) && failure.ClientError
}

// sentPasswords returns the usernames whose password the properties of an update hold, because UCP sends it.
func sentPasswords(properties map[string]any) map[string]bool {
	out := map[string]bool{}
	list, _ := properties[credentialsProperty].([]any)
	for _, item := range list {
		object, _ := item.(map[string]any)
		name, _ := object[usernameField].(string)
		if password, _ := object[passwordField].(string); name != "" && password != "" {
			out[name] = true
		}
	}
	return out
}

// Delete fails, because a delete removes only the credentials the template set. UCP calls DeleteScoped instead.
func (h *TerminationCredentials) Delete(ctx context.Context, region string, identifier string) error {
	return fmt.Errorf("the native handler for %s deletes only the credentials Radius manages, with DeleteScoped", credentialsType)
}

// DeleteScoped deletes the credentials whose usernames managed lists, in one call. It doesn't list them first, because
// a list right after a Put can still miss them, and AWS deletes a username the Voice Connector doesn't have without an
// error. It returns ErrNotFound when managed lists no usernames, or AWS has no such Voice Connector.
func (h *TerminationCredentials) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	vc, err := voiceConnectorOf(identifier, managed...)
	if err != nil {
		return err
	}
	names := usernamesOf(managed...)
	if len(names) == 0 {
		return native.ErrNotFound
	}
	return h.remove(ctx, region, vc, names)
}

// List fails, because credentials can't be listed by the identifiers UCP generates.
func (h *TerminationCredentials) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", credentialsType)
}

// Progress reports every operation as done, because a Put or a delete waited until a list showed it.
func (h *TerminationCredentials) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
