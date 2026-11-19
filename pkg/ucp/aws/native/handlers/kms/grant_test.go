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
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
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
	"Constraints":       map[string]any{"EncryptionContextSubset": map[string]any{"app": "orders", "ResultMetadata": "kept", "empty": ""}},
}

// firstGrant is the ID of the first grant a new fake makes.
var firstGrant = fmt.Sprintf("%064x", 1)

// grantSuite returns the suite for the handler, with a fake that hides a new grant from ListGrants for its first read
// when lagNew is set.
func grantSuite(t *testing.T, lagNew bool) nativetest.Suite {
	return nativetest.Suite{
		TypeName:    grantType,
		Schema:      readFile(t, "aws-kms-grant.json"),
		Metadata:    readFile(t, "aws-kms-grant.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			api.lagNew = lagNew
			return NewGrant(api)
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
			with(template, map[string]any{"Name": "app"}),
			with(template, map[string]any{"Constraints": map[string]any{"EncryptionContextEquals": map[string]any{"tenant/id": "t1"}}}),
			with(template, map[string]any{"Constraints": map[string]any{"EncryptionContextSubset": map[string]any{"tenant/id": "t1"}}}),
			with(template, map[string]any{"Constraints": map[string]any{"EncryptionContextSubset": map[string]any{"app": 1}}}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			key, id, err := parseID(identifier)
			require.NoError(t, err)
			return map[string]any{"KeyId": key, "GrantId": id}
		},
		API: grantAPI,
	}
}

func Test_Suite_Grant(t *testing.T) {
	nativetest.Run(t, grantSuite(t, false))
}

// Test_Suite_GrantListedLate runs the suite with a fake that lists a new grant only from its second read, as AWS's
// eventual consistency allows, so a create sent again, and a delete after a lost create, happen before ListGrants
// lists the grant.
func Test_Suite_GrantListedLate(t *testing.T) {
	nativetest.Run(t, grantSuite(t, true))
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

	_, err = h.Identifier(with(template, map[string]any{"Name": "app"}))
	requireRejected(t, err, "Name", "read-only")
	id, err = h.Identifier(map[string]any{"KeyId": keyARN(keyID), "GrantId": firstGrant, "Name": "app"})
	require.NoError(t, err)
	require.Equal(t, keyARN(keyID)+"|"+firstGrant, id, "a grant AWS reports has a name")

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
	request := native.CreateRequest{Region: region, Properties: with(template, map[string]any{"KeyId": keyID}), ClientToken: uuid.NewString()}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, keyARN(keyID)+"|"+firstGrant, id, "the identifier has the key ARN, as ListGrants reports it")
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, native.GenerateName(request.ClientToken), read["Name"], "the grant is named from the client token")

	again, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, id, again, "a create sent again returns the first grant")
	require.NotEqual(t, id, create(t, h, template), "another create makes another grant")
	require.Len(t, api.grantsOf(keyID), 2)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"KeyId": "ffffffff-ffff-ffff-ffff-ffffffffffff"})})
	require.ErrorContains(t, err, "NotFoundException")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"Operations": []any{"Sign"}})})
	requireRejected(t, err, validationCode)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: without(template, "Operations")})
	requireRejected(t, err, "KeyId and Operations")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"Operations": 1})})
	requireRejected(t, err)

	// The AWS SDK doesn't know ValidationError, so its fault is unknown, but AWS rejected the create.
	api.errs["CreateGrant"] = &smithy.GenericAPIError{Code: validationCode, Message: "1 validation error detected"}
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()})
	requireRejected(t, err, "1 validation error detected")
	api.errs["CreateGrant"] = errors.New("connection reset")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()})
	require.EqualError(t, err, "connection reset", "AWS may have made the grant")
	delete(api.errs, "CreateGrant")

	api.errs["ListGrants"] = errors.New("throttled")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()})
	require.ErrorContains(t, err, "throttled")
	delete(api.errs, "ListGrants")

	h = NewGrant(noARN{api})
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()})
	require.ErrorContains(t, err, "no grant ID")
}

// Test_CreateFailsBeforeSending fails each call Create makes before CreateGrant. AWS got no create, so the failure is
// rejected, keeping a client fault's 400 and anything else's 500, and UCP keeps no record of it.
func Test_CreateFailsBeforeSending(t *testing.T) {
	for _, call := range []string{"DescribeKey", "ListGrants"} {
		for _, failure := range []struct {
			err    error
			client bool
		}{
			{errors.New("connection reset"), false},
			{&smithy.GenericAPIError{Code: "KMSInternalException", Fault: smithy.FaultServer}, false},
			{fault("AccessDeniedException"), true},
		} {
			h, api := newHandler()
			api.errs[call] = failure.err
			_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"KeyId": keyID}), ClientToken: uuid.NewString()})
			var f *native.Failure
			require.ErrorAs(t, err, &f, call)
			require.Equal(t, native.OutcomeRejected, f.Outcome, call)
			require.Equal(t, failure.client, f.ClientError, call)
			require.Contains(t, f.Message, failure.err.Error())
			require.NotContains(t, api.calls, "CreateGrant")
		}
	}
}

// Test_FindRejectedValues finds a lost create whose values AWS rejects, as when the rejection's response was lost.
// No grant can exist, so Find returns "", and the next PUT creates with its own template and a DELETE removes only the
// record. Other errors may follow a create that made the grant, so Find returns them.
func Test_FindRejectedValues(t *testing.T) {
	h, api := newHandler()
	for _, properties := range []map[string]any{
		with(template, map[string]any{"Operations": []any{"Sign"}}),
		with(template, map[string]any{"KeyId": "arn:aws:kms:us-west-2:123:key"}),
	} {
		found, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: properties, ClientToken: uuid.NewString()})
		require.NoError(t, err)
		require.Empty(t, found)
	}
	require.Empty(t, api.grantsOf(keyID))

	_, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"KeyId": keyARN(disabledKeyID)}), ClientToken: uuid.NewString()})
	require.ErrorContains(t, err, "DisabledException", "a key disabled after the create may hold its grant")
	api.errs["CreateGrant"] = errors.New("connection reset")
	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()})
	require.EqualError(t, err, "connection reset")
}

// Test_CreateAfterChange sends a lost create again with a changed template. The key already has the grant named for
// the create, so Create returns it instead of making a second grant, and UCP then compares it with the template.
func Test_CreateAfterChange(t *testing.T) {
	h, api := newHandler()
	request := native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	// Grants on other keys and with other names make ListGrants return more than one page.
	create(t, h, template)
	create(t, h, with(template, map[string]any{"KeyId": keyARN(otherKeyID)}))

	api.calls = nil
	request.Properties = with(template, map[string]any{"Operations": []any{"Decrypt"}})
	again, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.NotContains(t, api.calls, "CreateGrant")

	// Only another way of making grants can give two the name of one create.
	duplicate := keyARN(keyID) + "|" + fmt.Sprintf("%064x", 99)
	api.store.Put(grantType, duplicate, with(api.store.Resource(grantType, id), map[string]any{"GrantId": fmt.Sprintf("%064x", 99)}))
	_, err = h.Create(t.Context(), request)
	require.ErrorContains(t, err, id)
	require.ErrorContains(t, err, duplicate)
	var failure *native.Failure
	require.False(t, errors.As(err, &failure), "the grants may include this create's, so UCP keeps the record")
}

// Test_Find finds a lost create with that create's values. UCP passes them rather than the current template's, so a
// template that has since moved to another key still finds the grant on the first key.
func Test_Find(t *testing.T) {
	h, api := newHandler()
	api.lagNew = true
	request := native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)

	found, err := h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, id, found, "AWS returns the grant before ListGrants lists it")
	require.Len(t, api.grantsOf(keyID), 1)
	require.Empty(t, api.grantsOf(otherKeyID))

	never := native.CreateRequest{Region: region, Properties: with(template, map[string]any{"KeyId": keyARN(otherKeyID)}), ClientToken: uuid.NewString()}
	made, err := h.Find(t.Context(), never)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(made, keyARN(otherKeyID)+"|"), "a create that never reached AWS makes the grant, so the record manages it")

	found, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{"KeyId": "ffffffff-ffff-ffff-ffff-ffffffffffff"}), ClientToken: uuid.NewString()})
	require.NoError(t, err)
	require.Empty(t, found, "a key that no longer exists has no grants")

	api.errs["ListGrants"] = errors.New("throttled")
	_, err = h.Find(t.Context(), request)
	require.ErrorContains(t, err, "throttled")
}

// Test_CreateListedLate sends a create again before ListGrants lists the first grant. The create has the same name and
// values, so AWS returns the first grant.
func Test_CreateListedLate(t *testing.T) {
	h, api := newHandler()
	api.lagNew = true
	request := native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	api.calls = nil
	again, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.Contains(t, api.calls, "CreateGrant", "ListGrants didn't list the grant yet")
	require.Len(t, api.grantsOf(keyID), 1)
}

func Test_CreateServiceGrant(t *testing.T) {
	h, _ := newHandler()
	properties := map[string]any{
		"KeyId":                    keyARN(keyID),
		"GranteeServicePrincipal":  "rds.amazonaws.com",
		"RetiringServicePrincipal": "rds.amazonaws.com",
		"Operations":               []any{"Decrypt"},
		"Constraints":              map[string]any{"SourceArn": "arn:aws:rds:us-west-2:" + account + ":db:app"},
	}
	request := native.CreateRequest{Region: region, Properties: properties, ClientToken: uuid.NewString()}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, with(properties, map[string]any{
		"GrantId": firstGrant, "Name": native.GenerateName(request.ClientToken), "IssuingAccount": "arn:aws:iam::" + account + ":root",
	}), read)
}

// awkwardContext is an encryption context with pairs awsapi.Encode would drop: an empty value and a key named
// ResultMetadata.
var awkwardContext = map[string]any{"app": "orders", "ResultMetadata": "kept", "empty": ""}

// Test_ReadConstraints checks that Read returns every encryption context pair as the create sent it, so a redeploy of
// the same template isn't a change to the create-only Constraints, and that it leaves out constraints with no values.
func Test_ReadConstraints(t *testing.T) {
	for _, c := range []map[string]any{
		{"EncryptionContextEquals": awkwardContext},
		{"EncryptionContextSubset": awkwardContext},
	} {
		h, _ := newHandler()
		properties := with(template, map[string]any{"Constraints": c})
		id := create(t, h, properties)
		read, err := h.Read(t.Context(), region, id)
		require.NoError(t, err)
		require.Equal(t, c, read["Constraints"])
	}

	require.Nil(t, constraintsOf(nil))
	require.Nil(t, constraintsOf(&types.GrantConstraints{EncryptionContextEquals: map[string]string{}, SourceArn: aws.String("")}), "a constraint with no values is no constraint")
}

// Test_CreateAgainWithAwkwardContext sends a lost create again before ListGrants lists the grant. AWS returns the first
// grant for the same name and values, so the fake must keep every encryption context pair to see that they match.
func Test_CreateAgainWithAwkwardContext(t *testing.T) {
	h, api := newHandler()
	api.lagNew = true
	request := native.CreateRequest{Region: region, Properties: with(template, map[string]any{"Constraints": map[string]any{"EncryptionContextSubset": awkwardContext}}), ClientToken: uuid.NewString()}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	again, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.Len(t, api.grantsOf(keyID), 1)
}

func Test_ReadDelete(t *testing.T) {
	h, api := newHandler()
	request := native.CreateRequest{Region: region, Properties: template, ClientToken: uuid.NewString()}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, with(template, map[string]any{
		"Name": native.GenerateName(request.ClientToken), "GrantId": firstGrant, "IssuingAccount": "arn:aws:iam::" + account + ":root",
	}), read)

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

func Test_UpdateList(t *testing.T) {
	h, api := newHandler()
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: keyARN(keyID) + "|g1"}))
	require.Empty(t, api.calls)
	_, err := h.List(t.Context(), region)
	require.ErrorContains(t, err, "can't be listed")
}
