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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func Test_SamePolicy(t *testing.T) {
	const base = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::111122223333:root"},"Action":"ses:SendEmail","Resource":"arn:aws:ses:us-west-2:111122223333:identity/a@example.com"}]}`
	cases := []struct {
		name  string
		other string
		same  bool
	}{
		{"the same", base, true},
		{"whitespace and key order", `{ "Statement": [ { "Resource": "arn:aws:ses:us-west-2:111122223333:identity/a@example.com", "Action": "ses:SendEmail",
			"Principal": { "AWS": "arn:aws:iam::111122223333:root" }, "Effect": "Allow" } ], "Version": "2012-10-17" }`, true},
		{"a list of one value", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["arn:aws:iam::111122223333:root"]},"Action":["ses:SendEmail"],"Resource":["arn:aws:ses:us-west-2:111122223333:identity/a@example.com"]}]}`, true},
		{"an account ID for its root ARN", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"111122223333"},"Action":"ses:SendEmail","Resource":"arn:aws:ses:us-west-2:111122223333:identity/a@example.com"}]}`, true},
		{"a duplicate list value", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::111122223333:root"},"Action":["ses:SendEmail","ses:SendEmail"],"Resource":"arn:aws:ses:us-west-2:111122223333:identity/a@example.com"}]}`, true},
		{"another action", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::111122223333:root"},"Action":"ses:SendRawEmail","Resource":"arn:aws:ses:us-west-2:111122223333:identity/a@example.com"}]}`, false},
		{"another account", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"444455556666"},"Action":"ses:SendEmail","Resource":"arn:aws:ses:us-west-2:111122223333:identity/a@example.com"}]}`, false},
		{"a user of the account for its root", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::111122223333:user/u"},"Action":"ses:SendEmail","Resource":"arn:aws:ses:us-west-2:111122223333:identity/a@example.com"}]}`, false},
		{"no Version for 2012-10-17", `{"Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::111122223333:root"},"Action":"ses:SendEmail","Resource":"arn:aws:ses:us-west-2:111122223333:identity/a@example.com"}]}`, false},
		{"another effect", `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":{"AWS":"arn:aws:iam::111122223333:root"},"Action":"ses:SendEmail","Resource":"arn:aws:ses:us-west-2:111122223333:identity/a@example.com"}]}`, false},
		{"not JSON", `not json`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.same, samePolicy(base, c.other))
			require.Equal(t, c.same, samePolicy(c.other, base))
		})
	}

	t.Run("no Version for 2008-10-17", func(t *testing.T) {
		require.True(t, samePolicy(`{"Statement":[]}`, `{"Version":"2008-10-17","Statement":[]}`))
	})
	t.Run("an account ID outside a principal is not its root ARN", func(t *testing.T) {
		require.False(t, samePolicy(
			`{"Statement":[{"Condition":{"StringEquals":{"aws:SourceAccount":"111122223333"}}}]}`,
			`{"Statement":[{"Condition":{"StringEquals":{"aws:SourceAccount":"arn:aws:iam::111122223333:root"}}}]}`,
		))
	})
	t.Run("only the Policy property", func(t *testing.T) {
		equivalent := NewEmailIdentityPolicy(nil).Options().Equivalent
		require.True(t, equivalent("Policy", base, base))
		require.False(t, equivalent("PolicyName", base, base))
	})
}

// Test_EmailIdentityPolicy_CreateWaitsForRead checks that a create returns only once a read shows the policy, so the
// create's progress never reports a policy SES has as lost while reads still miss it.
func Test_EmailIdentityPolicy_CreateWaitsForRead(t *testing.T) {
	ctx := t.Context()
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewEmailIdentityPolicy(api)
	h.wait = noWait
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: identityPolicy("p1", testPolicy(testIdentity))})
	require.NoError(t, err)
	progress, err := h.Progress(ctx, region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_EmailIdentityPolicy_Identifier(t *testing.T) {
	h := NewEmailIdentityPolicy(nil)
	identifier, err := h.Identifier(identityPolicy("p-1_A", testPolicy(testIdentity)))
	require.NoError(t, err)
	require.Equal(t, testIdentity+"|p-1_A", identifier)

	for _, properties := range []map[string]any{
		identityPolicy("bad.name", "{}"),
		identityPolicy("a|b", "{}"),
		with(identityPolicy("p1", "{}"), map[string]any{"EmailIdentity": identityARN(testIdentity)}),
		with(identityPolicy("p1", "{}"), map[string]any{"EmailIdentity": "a|b@example.com"}),
	} {
		_, err := h.Identifier(properties)
		requireRejected(t, err, "")
	}
}

func Test_EmailIdentityPolicy_UpdateRejected(t *testing.T) {
	ctx := t.Context()
	h := NewEmailIdentityPolicy(newFakeAPI(awsfake.NewCloudControl()))
	h.wait = noWait
	current := identityPolicy("p1", testPolicy(testIdentity))
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: current})
	require.NoError(t, err)

	err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: identityPolicy("p1", "not json")})
	requireRejected(t, err, "Unable to parse policy")
	err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: with(current, map[string]any{"Policy": nil})})
	requireRejected(t, err, "set Policy")
	got, err := h.Read(ctx, region, identifier)
	require.NoError(t, err)
	require.Equal(t, current, got)
}

func Test_Handlers_CantList(t *testing.T) {
	for _, h := range []native.Handler{NewEmailIdentityPolicy(nil), NewIdentityNotificationTopic(nil), NewDedicatedIPAssignment(nil)} {
		require.Equal(t, native.ListNone, h.Options().List)
		_, err := h.List(t.Context(), region)
		require.ErrorContains(t, err, h.TypeName())
	}
}
