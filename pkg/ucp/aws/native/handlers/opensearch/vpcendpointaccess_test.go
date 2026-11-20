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

package opensearch

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

const service = "application.opensearchservice.amazonaws.com"

func newAccessHandler() (*VpcEndpointAccess, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewVpcEndpointAccess(api), api
}

func createAccess(t *testing.T, h *VpcEndpointAccess, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.True(t, wait(t, h, id, native.OperationCreate).Done)
	return id
}

func Test_VpcEndpointAccess_Identifier(t *testing.T) {
	h, _ := newAccessHandler()
	for _, tc := range []struct {
		properties map[string]any
		want       string
	}{
		{map[string]any{"DomainName": "logs", "Account": otherAccount}, "logs|" + otherAccount},
		{map[string]any{"DomainName": "logs", "Service": service}, "logs|" + service},
		{map[string]any{"Account": otherAccount}, ""},
		{map[string]any{"DomainName": "logs"}, ""},
	} {
		id, err := h.Identifier(tc.properties)
		require.NoError(t, err)
		require.Equal(t, tc.want, id)
	}
	for _, properties := range []map[string]any{
		{"DomainName": 5, "Account": otherAccount},
		{"DomainName": "lo", "Account": otherAccount},
		{"DomainName": "logs", "Account": otherAccount, "Service": service},
		{"DomainName": "logs", "Account": "12a"},
		{"DomainName": "logs", "Service": "es.amazonaws.com"},
		{"DomainName": "logs", "Account": otherAccount, "ServiceOptions": map[string]any{}},
	} {
		_, err := h.Identifier(properties)
		requireRejected(t, err)
	}
	for _, identifier := range []string{"logs", "logs|", "Logs|" + otherAccount, "logs|es.amazonaws.com", "logs|arn:aws:iam::1:root"} {
		_, err := h.Read(t.Context(), region, identifier)
		requireRejected(t, err)
		requireRejected(t, h.Delete(t.Context(), region, identifier))
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		requireRejected(t, err)
	}
}

// Test_VpcEndpointAccess_CreateFindsExisting checks that a create sent again, after AWS took the first one, sends
// nothing, because AWS rejects authorizing a principal the domain already lists.
func Test_VpcEndpointAccess_CreateFindsExisting(t *testing.T) {
	h, api := newAccessHandler()
	properties := map[string]any{"DomainName": "logs", "Account": otherAccount}
	id := createAccess(t, h, properties)
	again, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.Equal(t, 1, api.calls["AuthorizeVpcEndpointAccess"])
}

// Test_VpcEndpointAccess_RootARN checks that an account AWS lists as its root ARN, which the docs allow, is read,
// found again by a create, and revoked.
func Test_VpcEndpointAccess_RootARN(t *testing.T) {
	h, api := newAccessHandler()
	api.rootARN = true
	properties := map[string]any{"DomainName": "logs", "Account": otherAccount}
	id := createAccess(t, h, properties)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"DomainName": "logs", "Account": otherAccount, "Principal": otherAccount, "PrincipalType": "AWS_ACCOUNT"}, read)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, 1, api.calls["AuthorizeVpcEndpointAccess"])

	require.NoError(t, h.Delete(t.Context(), region, id))
	wait(t, h, id, native.OperationDelete)
	require.Empty(t, api.store.List(vpcEndpointAccessType))

	// Another account's root ARN, or a service with the same value, doesn't match.
	id2 := accessID{domain: "logs", principal: otherAccount, kind: types.PrincipalTypeAwsAccount}
	require.True(t, id2.matches(types.AuthorizedPrincipal{Principal: aws.String("arn:aws-cn:iam::" + otherAccount + ":root"), PrincipalType: types.PrincipalTypeAwsAccount}))
	require.False(t, id2.matches(types.AuthorizedPrincipal{Principal: aws.String("arn:aws:iam::" + account + ":root"), PrincipalType: types.PrincipalTypeAwsAccount}))
	require.False(t, id2.matches(types.AuthorizedPrincipal{Principal: aws.String("arn:aws:iam::" + otherAccount + ":user/x"), PrincipalType: types.PrincipalTypeAwsAccount}))
	require.False(t, id2.matches(types.AuthorizedPrincipal{Principal: aws.String(otherAccount), PrincipalType: types.PrincipalTypeAwsService}))
}

// Test_VpcEndpointAccess_Service checks that a service principal keeps its Regions, and that a delete revokes it with
// the Regions AWS lists.
func Test_VpcEndpointAccess_Service(t *testing.T) {
	h, api := newAccessHandler()
	properties := map[string]any{"DomainName": "logs", "Service": service, "ServiceOptions": map[string]any{"SupportedRegions": list("us-east-1", "eu-west-1")}}
	id := createAccess(t, h, properties)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, with(properties, map[string]any{"Principal": service, "PrincipalType": "AWS_SERVICE"}), read)

	revoked := &revokeInputs{fakeAPI: api}
	h = NewVpcEndpointAccess(revoked)
	require.NoError(t, h.Delete(t.Context(), region, id))
	require.Len(t, revoked.inputs, 1)
	require.Equal(t, []string{"us-east-1", "eu-west-1"}, revoked.inputs[0].ServiceOptions.SupportedRegions)
	require.Nil(t, revoked.inputs[0].Account)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(properties, map[string]any{"ServiceOptions": "us-east-1"})})
	requireRejected(t, err)
}

// revokeInputs records the input of each RevokeVpcEndpointAccess.
type revokeInputs struct {
	*fakeAPI
	inputs []*opensearch.RevokeVpcEndpointAccessInput
}

func (r *revokeInputs) RevokeVpcEndpointAccess(ctx context.Context, in *opensearch.RevokeVpcEndpointAccessInput, opts ...func(*opensearch.Options)) (*opensearch.RevokeVpcEndpointAccessOutput, error) {
	r.inputs = append(r.inputs, in)
	return r.fakeAPI.RevokeVpcEndpointAccess(ctx, in, opts...)
}

// Test_VpcEndpointAccess_DomainBusy checks that an authorization AWS rejects while the domain is changing is returned
// as AWS's error and changes nothing, and that the next deploy succeeds.
func Test_VpcEndpointAccess_DomainBusy(t *testing.T) {
	h, api := newAccessHandler()
	api.processing["logs"] = 2
	properties := map[string]any{"DomainName": "logs", "Account": otherAccount}
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	requireRejected(t, err)
	require.Empty(t, api.store.List(vpcEndpointAccessType))
	createAccess(t, h, properties)
}

// Test_VpcEndpointAccess_Pages checks that a principal on a later page of the domain's list is found, and that a
// domain that doesn't exist reads as not found.
func Test_VpcEndpointAccess_Pages(t *testing.T) {
	h, _ := newAccessHandler()
	ids := []string{}
	for i := range 3 {
		ids = append(ids, createAccess(t, h, map[string]any{"DomainName": "logs", "Account": fmt.Sprintf("10000000000%d", i)}))
	}
	_, err := h.Read(t.Context(), region, ids[2])
	require.NoError(t, err)
	_, err = h.Read(t.Context(), region, "logs|999999999999")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), region, "gone|"+otherAccount)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), region, "gone|"+otherAccount), native.ErrNotFound)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"DomainName": "gone", "Account": otherAccount}})
	requireRejected(t, err)
	require.NotErrorIs(t, err, native.ErrNotFound, "a create of a missing domain is AWS's rejection, not a missing resource")

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources)
}

// Test_VpcEndpointAccess_Progress checks the progress of each operation: a create is done once AWS lists the
// principal, a delete waits until it doesn't, and an update changes nothing.
func Test_VpcEndpointAccess_Progress(t *testing.T) {
	h, _ := newAccessHandler()
	id := createAccess(t, h, map[string]any{"DomainName": "logs", "Account": otherAccount})
	p, err := h.Progress(t.Context(), region, id, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, p.Done)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{}))

	p, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.False(t, p.Done, "the principal is still listed")
	require.NoError(t, h.Delete(t.Context(), region, id))
	_, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "UCP sends a create again for an identifier AWS doesn't list")
}

// Test_VpcEndpointAccess_CreateInput checks that a create without all of its identifier, or with a bad value, and a
// create whose list of the domain fails, send nothing.
func Test_VpcEndpointAccess_CreateInput(t *testing.T) {
	h, api := newAccessHandler()
	for _, properties := range []map[string]any{
		{"DomainName": "logs"},
		{"Account": otherAccount},
		{"DomainName": "logs", "Account": 7},
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
		requireRejected(t, err)
	}
	boom := errors.New("boom")
	api.errs["ListVpcEndpointAccess"] = boom
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"DomainName": "logs", "Account": otherAccount}})
	requireRejectedBefore(t, err, boom.Error())
	require.Zero(t, api.calls["AuthorizeVpcEndpointAccess"])
}

// Test_VpcEndpointAccess_OtherServiceOptions checks that a create that finds the service already authorized with other
// Regions fails instead of taking it as the template's.
func Test_VpcEndpointAccess_OtherServiceOptions(t *testing.T) {
	h, api := newAccessHandler()
	properties := map[string]any{"DomainName": "logs", "Service": service, "ServiceOptions": map[string]any{"SupportedRegions": list("us-east-1")}}
	createAccess(t, h, with(properties, map[string]any{"ServiceOptions": map[string]any{"SupportedRegions": list("eu-west-1")}}))
	for _, template := range []map[string]any{properties, {"DomainName": "logs", "Service": service}} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template})
		requireRejectedBefore(t, err, "SupportedRegions")
	}

	// The same Regions in another order are the same options.
	createAccess(t, h, with(properties, map[string]any{"ServiceOptions": map[string]any{"SupportedRegions": list("eu-west-1")}}))
	require.Equal(t, 1, api.calls["AuthorizeVpcEndpointAccess"])
}

// Test_VpcEndpointAccess_FailsBeforeChange checks that a list that fails before a create or delete sends anything is
// reported as rejected.
func Test_VpcEndpointAccess_FailsBeforeChange(t *testing.T) {
	h, api := newAccessHandler()
	id := createAccess(t, h, map[string]any{"DomainName": "logs", "Account": otherAccount})
	api.errs["ListVpcEndpointAccess"] = errors.New("list down")
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"DomainName": "logs", "Account": account}})
	requireRejectedBefore(t, err, "list down")
	requireRejectedBefore(t, h.Delete(t.Context(), region, id), "list down")
}
