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

package nativetest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/handlers/iam"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

const ruleDoc = "https://docs.aws.amazon.com/test/latest/APIReference/API_CreateThing.html"

func rejectWith(code string, fault smithy.ErrorFault) func(context.Context, *awsfake.CloudControl) error {
	return func(context.Context, *awsfake.CloudControl) error {
		return &smithy.GenericAPIError{Code: code, Message: code, Fault: fault}
	}
}

func Test_RequireRules(t *testing.T) {
	accept := func(context.Context, *awsfake.CloudControl) error { return nil }
	tests := []struct {
		name string
		call Call
		fail string
	}{
		{
			name: "a fake that rejects the call with AWS's error",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size is 1 to 10", Break: rejectWith("ValidationException", smithy.FaultClient), Code: "ValidationException"}}},
		},
		{
			name: "a fake that follows a rule that rejects nothing",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size defaults to 1", Check: accept}}},
		},
		{
			name: "a fake that accepts a call a rule forbids",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size is 1 to 10", Break: accept, Code: "ValidationException"}}},
			fail: "the fake accepts a call AWS rejects",
		},
		{
			name: "a fake that rejects the call for another reason",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size is 1 to 10", Break: rejectWith("ResourceNotFoundException", smithy.FaultClient), Code: "ValidationException"}}},
			fail: "the fake rejects the call for another reason",
		},
		{
			name: "a fake that fails the call as a server fault",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size is 1 to 10", Break: rejectWith("ValidationException", smithy.FaultServer), Code: "ValidationException"}}},
			fail: "client fault",
		},
		{
			name: "a fake that fails the call without an AWS error",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size is 1 to 10", Break: func(context.Context, *awsfake.CloudControl) error { return errors.New("boom") }, Code: "ValidationException"}}},
			fail: "client fault",
		},
		{
			name: "a fake that doesn't follow a rule that rejects nothing",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size defaults to 1", Check: func(context.Context, *awsfake.CloudControl) error { return errors.New("Size is 0") }}}},
			fail: "the fake doesn't follow the rule",
		},
		{
			name: "a rule with no error code",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size is 1 to 10", Break: rejectWith("ValidationException", smithy.FaultClient)}}},
			fail: "Code must be the error code",
		},
		{
			name: "a rule with both Break and Check",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size is 1 to 10", Break: accept, Check: accept, Code: "ValidationException"}}},
			fail: "set either Break or Check",
		},
		{
			name: "a call that doesn't link to AWS's docs",
			call: Call{Doc: "https://example.com/CreateThing"},
			fail: "Doc must link to the call's page",
		},
		{
			name: "a rule that doesn't link to AWS's docs",
			call: Call{Doc: ruleDoc, Rules: []Rule{{Rule: "Size defaults to 1", Doc: "https://example.com", Check: accept}}},
			fail: "Doc must link to AWS's docs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeTB(t)
			f.run(func() { requireRules(f, map[string]Call{"CreateThing": tt.call}) })
			if tt.fail == "" {
				require.False(t, f.failed, f.errors)
				return
			}
			require.True(t, f.failed)
			require.Contains(t, f.errors[0], tt.fail)
		})
	}
}

func Test_RequireRecorded(t *testing.T) {
	calls := []string{"CreateThing", "DeleteThing"}
	tests := []struct {
		name string
		api  map[string]Call
		fail string
	}{
		{name: "every call recorded", api: map[string]Call{"CreateThing": {}, "DeleteThing": {}}},
		{name: "a call with no record", api: map[string]Call{"CreateThing": {}}, fail: "record each AWS call"},
		{name: "a record of a call the handler doesn't make", api: map[string]Call{"CreateThing": {}, "DeleteThing": {}, "TagThing": {}}, fail: "records calls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeTB(t)
			f.run(func() { requireRecorded(f, tt.api, calls, "ThingHandler") })
			if tt.fail == "" {
				require.False(t, f.failed, f.errors)
				return
			}
			require.True(t, f.failed)
			require.Contains(t, f.errors[0], tt.fail)
		})
	}
}

func Test_HandlerCalls(t *testing.T) {
	calls, typeName := handlerCalls(t, iam.NewUserToGroupAddition(nil))
	require.Equal(t, "UserToGroupAddition", typeName)
	require.Equal(t, []string{"AddUserToGroup", "GetGroup", "RemoveUserFromGroup"}, calls)

	// A test's wrapper that embeds the handler, directly or through an interface, stands for it.
	type wrapped struct{ *iam.UserToGroupAddition }
	type viaInterface struct{ native.Handler }
	handlers := []any{
		wrapped{iam.NewUserToGroupAddition(nil)},
		&viaInterface{iam.NewUserToGroupAddition(nil)},
		&struct{ native.Handler }{iam.NewUserToGroupAddition(nil)},
	}
	for _, handler := range handlers {
		calls, typeName = handlerCalls(t, handler)
		require.Equal(t, "UserToGroupAddition", typeName)
		require.Equal(t, []string{"AddUserToGroup", "GetGroup", "RemoveUserFromGroup"}, calls)
	}

	calls, _ = handlerCalls(t, nativefake.New("AWS::Test::Thing", awsfake.NewCloudControl()))
	require.Empty(t, calls)
}

func Test_ParseHandlerSource(t *testing.T) {
	dir := t.TempDir()
	src := `package thing

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	other "example.com/other"
)

type API interface {
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	OneParam(ctx context.Context) error
	NotPointer(ctx context.Context, in s3.ListBucketsInput) error
	WrongName(ctx context.Context, in *s3.GetObjectInput) error
	Local(ctx context.Context, in *LocalInput) error
	Nested(ctx context.Context, in *types.NestedInput) error
	Other(ctx context.Context, in *other.OtherInput) error
}

type Handler struct {
	api    API
	client *s3.Client
}

func (h *Handler) Read(ctx context.Context) error {
	_, err := h.api.GetObject(ctx, nil)
	return err
}

// Delete calls a concrete client, which no interface of the package declares.
func (h *Handler) Delete(ctx context.Context) error {
	_, err := h.client.DeleteObject(ctx, &s3.DeleteObjectInput{})
	return err
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "thing.go"), []byte(src), 0o600))
	parsed := ParseHandlerSource(t, dir)
	require.Equal(t, map[string]string{"GetObject": "s3", "DeleteObject": "s3"}, parsed.API)
	require.Equal(t, []string{"DeleteObject", "GetObject"}, parsed.AllCalls("Handler"))
	require.Nil(t, parsed.Calls("Handler", "Missing"))
}

func Test_HandlerCalls_NoSource(t *testing.T) {
	type unnamedPointer struct{ *iam.UserToGroupAddition }
	for _, handler := range []any{unnamedPointer{}, struct{ native.Handler }{}} {
		f := newFakeTB(t)
		f.run(func() { handlerCalls(f, handler) })
		require.True(t, f.failed)
		require.Contains(t, f.errors[0], "has no methods in its package's source")
	}
}
