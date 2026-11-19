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

package kms

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func without(properties map[string]any, name string) map[string]any {
	out := maps.Clone(properties)
	delete(out, name)
	return out
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	maps.Copy(out, changes)
	return out
}

// template is a grant that lets the app role decrypt and make data keys with keyID, for requests whose encryption
// context has app=orders, and lets the admin role retire it.
var template = map[string]any{
	"KeyId":             keyARN(keyID),
	"GranteePrincipal":  grantee,
	"RetiringPrincipal": "arn:aws:iam::" + account + ":role/admin",
	"Operations":        []any{"Decrypt", "GenerateDataKey"},
	"Constraints":       map[string]any{"EncryptionContextSubset": map[string]any{"app": "orders"}},
}

// firstGrant is the ID of the first grant a new fake makes.
var firstGrant = fmt.Sprintf("%064x", 1)

func Test_Suite_Grant(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    grantType,
		Schema:      readFile(t, "aws-kms-grant.json"),
		Metadata:    readFile(t, "aws-kms-grant.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewGrant(newFakeAPI(store))
		},
		IdentifierProperty: "GrantId",
		Properties:         template,
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          template,
		CreateOnlyChange: with(template, map[string]any{"Operations": []any{"Decrypt"}}),
		OtherForm:        map[string]any{"Operations": []any{"GenerateDataKey", "Decrypt"}},
		// The key ID form of the identifier of the first grant the template makes.
		SameResourceNames: []string{keyID + "|" + firstGrant},
		Invalid: []map[string]any{
			with(template, map[string]any{"Operations": "Decrypt"}),
			with(template, map[string]any{"Operations": []any{"Decrypt", "Decrypt"}}),
			with(template, map[string]any{"KeyId": 5}),
			with(template, map[string]any{"KeyId": "alias/app"}),
			without(template, "Operations"),
			with(template, map[string]any{"Constraints": map[string]any{"EncryptionContextSubset": map[string]any{"app": 1}}}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			key, id, err := parseID(identifier)
			require.NoError(t, err)
			return map[string]any{"KeyId": key, "GrantId": id}
		},
		API: grantAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := kms.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func newHandler() (*Grant, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewGrant(api), api
}

// create creates the grant the properties give, with a new client token, and returns its identifier.
func create(t *testing.T, h *Grant, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties, ClientToken: uuid.NewString()})
	require.NoError(t, err)
	return id
}

// requireRejected requires err to be a client failure AWS never saw, with a message that has each of words.
func requireRejected(t *testing.T, err error, words ...string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	for _, w := range words {
		require.Contains(t, failure.Message, w)
	}
}

func Test_Identifier(t *testing.T) {
	h, _ := newHandler()
	id, err := h.Identifier(template)
	require.NoError(t, err)
	require.Empty(t, id, "AWS assigns the grant ID")

	id, err = h.Identifier(map[string]any{"KeyId": keyID, "GrantId": firstGrant})
	require.NoError(t, err)
	require.Equal(t, keyID+"|"+firstGrant, id)

	_, err = h.Identifier(without(template, "Operations"))
	requireRejected(t, err, "KeyId and Operations")
	_, err = h.Identifier(without(template, "KeyId"))
	requireRejected(t, err, "KeyId and Operations")

	_, err = h.Identifier(with(template, map[string]any{"Operations": []any{"Decrypt", 2}}))
	requireRejected(t, err, "types")
	_, err = h.Identifier(with(template, map[string]any{"KeyId": "arn:aws:kms:us-west-2:" + account + ":alias/app"}))
	requireRejected(t, err, "alias")
	_, err = h.Identifier(with(template, map[string]any{"Operations": []any{"Decrypt", "Encrypt", "Decrypt"}}))
	requireRejected(t, err, "Decrypt more than once")
}

func Test_ResolveIdentifier(t *testing.T) {
	h, api := newHandler()
	id, err := h.ResolveIdentifier(t.Context(), region, keyID+"|g1")
	require.NoError(t, err)
	require.Equal(t, keyARN(keyID)+"|g1", id, "a key ID resolves to the key ARN AWS reports")

	api.calls = nil
	id, err = h.ResolveIdentifier(t.Context(), region, keyARN(keyID)+"|g1")
	require.NoError(t, err)
	require.Equal(t, keyARN(keyID)+"|g1", id)
	require.Empty(t, api.calls, "a key ARN needs no call")

	for _, bad := range []string{keyID, "|g1", keyID + "|", keyID + "|g1|g2", "alias/app|g1"} {
		_, err = h.ResolveIdentifier(t.Context(), region, bad)
		requireRejected(t, err, "key ID or key ARN", "grant ID")
	}

	_, err = h.ResolveIdentifier(t.Context(), region, "ffffffff-ffff-ffff-ffff-ffffffffffff|g1")
	require.ErrorContains(t, err, "NotFoundException")

	h = NewGrant(noARN{api})
	_, err = h.ResolveIdentifier(t.Context(), region, keyID+"|g1")
	require.ErrorContains(t, err, "without its ARN")
}

// noARN describes a key without its ARN, and makes grants without IDs.
type noARN struct {
	*fakeAPI
}

func (n noARN) DescribeKey(ctx context.Context, in *kms.DescribeKeyInput, _ ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	return &kms.DescribeKeyOutput{}, nil
}

func (n noARN) CreateGrant(ctx context.Context, in *kms.CreateGrantInput, _ ...func(*kms.Options)) (*kms.CreateGrantOutput, error) {
	return &kms.CreateGrantOutput{}, nil
}

func Test_sameKey(t *testing.T) {
	arn := keyARN(keyID)
	require.True(t, sameKey("KeyId", keyID, arn))
	require.False(t, sameKey("KeyId", otherKeyID, arn))
	require.False(t, sameKey("KeyId", keyARN(otherKeyID), arn))
	require.False(t, sameKey("GranteePrincipal", keyID, arn))
	require.False(t, sameKey("KeyId", 5, arn))
	require.False(t, sameKey("KeyId", keyID, nil))
}

func Test_Create(t *testing.T) {
	h, api := newHandler()
	id := create(t, h, with(template, map[string]any{"KeyId": keyID, "Name": "app"}))
	require.Equal(t, keyARN(keyID)+"|"+firstGrant, id, "the identifier has the key ARN, as ListGrants reports it")

	api.calls = nil
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"Name": "app", "Operations": []any{"GenerateDataKey", "Decrypt"}})})
	requireRejected(t, err, `named "app"`, id)
	require.NotContains(t, api.calls, "CreateGrant", "AWS would return the grant that is there")

	other := create(t, h, with(template, map[string]any{"Name": "app", "Operations": []any{"Decrypt"}}))
	require.NotEqual(t, id, other, "a grant with the same name and other values is another grant")

	request := native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()}
	unnamed, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	read, err := h.Read(t.Context(), region, unnamed)
	require.NoError(t, err)
	require.Equal(t, native.GenerateName(request.ClientToken), read["Name"], "a grant the template doesn't name gets a name from the client token")
	require.NotEqual(t, unnamed, create(t, h, template), "another create gets another name")
	_, err = h.Create(t.Context(), request)
	requireRejected(t, err, "earlier attempt", unnamed)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"KeyId": "ffffffff-ffff-ffff-ffff-ffffffffffff"})})
	require.ErrorContains(t, err, "NotFoundException")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"Operations": []any{"Sign"}})})
	requireRejected(t, err, validationCode)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: without(template, "Operations")})
	requireRejected(t, err, "KeyId and Operations")

	// The AWS SDK doesn't know ValidationError, so its fault is unknown, but AWS rejected the create.
	api.errs["CreateGrant"] = &smithy.GenericAPIError{Code: validationCode, Message: "1 validation error detected"}
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()})
	requireRejected(t, err, "1 validation error detected")
	api.errs["CreateGrant"] = errors.New("connection reset")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()})
	require.EqualError(t, err, "connection reset", "AWS may have made the grant")
	delete(api.errs, "CreateGrant")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"Operations": 1})})
	requireRejected(t, err)

	api.errs["ListGrants"] = errors.New("throttled")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"Name": "new"})})
	require.ErrorContains(t, err, "throttled")
	delete(api.errs, "ListGrants")

	h = NewGrant(noARN{api})
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template})
	require.ErrorContains(t, err, "no grant ID")
}

func Test_CreateServiceGrant(t *testing.T) {
	h, _ := newHandler()
	properties := map[string]any{
		"KeyId":                    keyARN(keyID),
		"GranteeServicePrincipal":  "rds.amazonaws.com",
		"RetiringServicePrincipal": "rds.amazonaws.com",
		"Operations":               []any{"Decrypt"},
		"Constraints":              map[string]any{"SourceArn": "arn:aws:rds:us-west-2:" + account + ":db:app"},
		"Name":                     "rds",
	}
	id := create(t, h, properties)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, with(properties, map[string]any{"GrantId": firstGrant, "IssuingAccount": "arn:aws:iam::" + account + ":root"}), read)

	found, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, id, found, "a grant to a service principal is found without a grantee filter")
}

func Test_Find(t *testing.T) {
	h, api := newHandler()
	named := with(template, map[string]any{"Name": "app"})
	found, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: named})
	require.NoError(t, err)
	require.Empty(t, found)

	// Grants on other keys and with other values make ListGrants return more than one page.
	create(t, h, with(template, map[string]any{"Operations": []any{"Decrypt"}}))
	create(t, h, with(template, map[string]any{"KeyId": keyARN(otherKeyID)}))
	create(t, h, with(named, map[string]any{"Constraints": map[string]any{"EncryptionContextEquals": map[string]any{"app": "orders"}}}))
	id := create(t, h, named)
	found, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: with(named, map[string]any{"Operations": []any{"GenerateDataKey", "Decrypt"}})})
	require.NoError(t, err)
	require.Equal(t, id, found, "operations in another order are the same grant")

	found, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: with(named, map[string]any{"KeyId": "ffffffff-ffff-ffff-ffff-ffffffffffff"})})
	require.NoError(t, err)
	require.Empty(t, found, "a key that is gone has no grants")

	// AWS returns an existing grant for a create with the same name and values, so only a grant made some other way
	// is a second match.
	duplicate := keyARN(keyID) + "|" + fmt.Sprintf("%064x", 99)
	api.store.Put(grantType, duplicate, with(api.store.Resource(grantType, id), map[string]any{"GrantId": fmt.Sprintf("%064x", 99)}))
	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: named})
	require.ErrorContains(t, err, id)
	require.ErrorContains(t, err, duplicate)

	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"KeyId": "alias/app"})})
	requireRejected(t, err, "alias")

	api.errs["ListGrants"] = errors.New("throttled")
	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: named})
	require.ErrorContains(t, err, "throttled")
}

func Test_ReadDelete(t *testing.T) {
	h, api := newHandler()
	id := create(t, h, with(template, map[string]any{"Name": "app"}))
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, with(template, map[string]any{"Name": "app", "GrantId": firstGrant, "IssuingAccount": "arn:aws:iam::" + account + ":root"}), read)

	_, err = h.Read(t.Context(), region, keyARN(keyID)+"|missing")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), region, "ffffffff-ffff-ffff-ffff-ffffffffffff|"+firstGrant)
	require.ErrorIs(t, err, native.ErrNotFound, "a key that is gone has no grants")
	_, err = h.Read(t.Context(), region, keyID)
	requireRejected(t, err, "grant ID")
	require.ErrorIs(t, h.Delete(t.Context(), region, keyARN(keyID)+"|missing"), native.ErrNotFound)

	api.errs["RetireGrant"] = fault("NotFoundException")
	require.ErrorIs(t, h.Delete(t.Context(), region, id), native.ErrNotFound, "a grant AWS already retired is gone")
	api.errs["RetireGrant"] = errors.New("throttled")
	require.ErrorContains(t, h.Delete(t.Context(), region, id), "throttled")
	delete(api.errs, "RetireGrant")

	require.NoError(t, h.Delete(t.Context(), region, id))
	progress, err := h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "listed", Waiting: "retired"}, progress, "a retired grant can still be listed for a while")
	_, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_Progress(t *testing.T) {
	h, api := newHandler()
	api.lagNew = true
	id := create(t, h, template)
	progress, err := h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "not listed", Waiting: "listed"}, progress, "a new grant can take a while to be listed")
	progress, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)

	progress, err = h.Progress(t.Context(), region, "not an identifier", native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done, "an update changes nothing")

	api.errs["ListGrants"] = errors.New("throttled")
	_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.ErrorContains(t, err, "throttled")
}

// Test_LostCreateListedLate loses a create whose grant ListGrants doesn't list yet, as when a deploy looks for the
// lost create right after it. The create sent again finds the grant and sends nothing, and the next search finds it.
func Test_LostCreateListedLate(t *testing.T) {
	h, api := newHandler()
	api.lagNew = true
	request := native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	found, err := h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, found, "ListGrants doesn't list the grant yet")
	_, err = h.Create(t.Context(), request)
	requireRejected(t, err, "deploy again", id)
	found, err = h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, id, found)
	require.Len(t, api.grantsOf(keyID), 1)
}

func Test_UpdateList(t *testing.T) {
	h, api := newHandler()
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: keyARN(keyID) + "|g1"}))
	require.Empty(t, api.calls)
	_, err := h.List(t.Context(), region)
	require.ErrorContains(t, err, "can't be listed")
}
