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

package quicksight

import (
	"context"
	"maps"
	"math"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func newAliasHandler() (*TemplateAlias, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTemplateAlias(api)
	h.wait = noWait
	return h, api
}

func makeAlias(t *testing.T, h *TemplateAlias, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	wait(t, h, id, native.OperationCreate)
	return id
}

func Test_TemplateAlias_Identifier(t *testing.T) {
	h, api := newAliasHandler()
	id, err := h.Identifier(aliasTemplate())
	require.NoError(t, err)
	require.Equal(t, account+"|report|live", id)
	for _, partial := range []string{"AwsAccountId", "TemplateId", "AliasName", "TemplateVersionNumber"} {
		properties := aliasTemplate()
		delete(properties, partial)
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
		requireRejected(t, err)
	}
	for _, change := range []map[string]any{
		{"AwsAccountId": "12345678901"},
		{"AliasName": "$PUBLISHED"},
		{"AliasName": strings.Repeat("a", maxAliasLength+1)},
		{"TemplateId": "report|x"},
		{"TemplateVersionNumber": float64(-1)},
		{"TemplateVersionNumber": nil},
		{"TemplateVersionNumber": 1e300},
		{"TemplateVersionNumber": float64(1 << 63)},
	} {
		_, err := h.Identifier(with(aliasTemplate(), change))
		requireRejected(t, err)
		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(aliasTemplate(), change)})
		requireRejected(t, err)
		if _, ok := change["TemplateVersionNumber"]; ok {
			requireRejected(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: account + "|report|live", Desired: with(aliasTemplate(), change)}))
		}
	}
	for _, identifier := range []string{"", account + "|report", account + "|report|$LATEST", "1234|report|live", account + "|report|live|x"} {
		_, err := h.Read(t.Context(), region, identifier)
		requireRejected(t, err)
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		requireRejected(t, err)
		requireRejected(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Desired: aliasTemplate()}))
	}
	require.Empty(t, api.calls, "a bad template or identifier never reaches AWS")

	for n, want := range map[any]int64{2: 2, int64(9007199254740993): 9007199254740993, int64(math.MaxInt64): math.MaxInt64} {
		version, err := templateVersion(map[string]any{"TemplateVersionNumber": n})
		require.NoError(t, err)
		require.Equal(t, want, version, "%v", n)
	}
	for _, n := range []any{0, int64(-1)} {
		_, err := templateVersion(map[string]any{"TemplateVersionNumber": n})
		requireRejected(t, err)
	}
}

// Test_TemplateAlias_CreateFindsExisting checks that a create that finds the alias already there, such as after a
// create whose response was lost, points it to the template's version.
func Test_TemplateAlias_CreateFindsExisting(t *testing.T) {
	h, api := newAliasHandler()
	id := makeAlias(t, h, with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(1)}))
	again, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: aliasTemplate()})
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.Equal(t, 1, api.calls["UpdateTemplateAlias"])
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, with(aliasTemplate(), map[string]any{"Arn": aliasArn("report", "live")}), read)

	// The same version is sent again, so the alias ends as the template sets it.
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: aliasTemplate()})
	require.NoError(t, err)
	require.Equal(t, 2, api.calls["UpdateTemplateAlias"])
}

// Test_TemplateAlias_Versions checks that an alias can't point to a version the template doesn't have, as AWS's
// rejection and not as a missing alias, and that an update points it to another version.
func Test_TemplateAlias_Versions(t *testing.T) {
	h, _ := newAliasHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(4)})})
	requireRejected(t, err)
	require.NotErrorIs(t, err, native.ErrNotFound)

	id := makeAlias(t, h, aliasTemplate())
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(9)})})
	requireRejected(t, err)
	require.NotErrorIs(t, err, native.ErrNotFound)

	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(3)})}))
	wait(t, h, id, native.OperationUpdate)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, float64(3), read["TemplateVersionNumber"])

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(aliasTemplate(), map[string]any{"TemplateId": "missing"})})
	requireRejected(t, err)
	require.NotErrorIs(t, err, native.ErrNotFound)
}

// Test_TemplateAlias_TemplateBusy checks that a create or update AWS rejects while the template is making a new version
// is a conflict, and that the next deploy succeeds.
func Test_TemplateAlias_TemplateBusy(t *testing.T) {
	h, api := newAliasHandler()
	api.building["report"] = 2
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: aliasTemplate()})
	requireConflict(t, err)
	require.Empty(t, api.store.List(templateAliasType))
	id := makeAlias(t, h, aliasTemplate())

	api.building["report"] = 2
	requireConflict(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: aliasTemplate()}))
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: aliasTemplate()}))
}

// Test_TemplateAlias_Progress checks that every operation is done once the alias reads, and that an alias AWS doesn't
// have is not found, so UCP sends a lost create again.
func Test_TemplateAlias_Progress(t *testing.T) {
	h, api := newAliasHandler()
	for _, op := range []native.Operation{native.OperationCreate, native.OperationUpdate} {
		_, err := h.Progress(t.Context(), region, account+"|report|live", op)
		require.ErrorIs(t, err, native.ErrNotFound)
	}
	id := makeAlias(t, h, aliasTemplate())
	p, err := h.Progress(t.Context(), region, id, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, p.Done)

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources)

	api.standard = true
	_, err = h.Read(t.Context(), region, id)
	requireRejected(t, err)
}

// Test_TemplateAlias_DeleteLeavesAlias checks that a delete makes no AWS call: AWS's docs say DeleteTemplateAlias also
// deletes the template version the alias points to.
func Test_TemplateAlias_DeleteLeavesAlias(t *testing.T) {
	h, api := newAliasHandler()
	id := makeAlias(t, h, aliasTemplate())
	calls := maps.Clone(api.calls)
	require.NoError(t, h.Delete(t.Context(), region, id))
	require.Equal(t, calls, api.calls)
	require.Len(t, api.store.List(templateAliasType), 1)
}

// Test_TemplateAlias_EmptyAnswer checks that an answer without the alias reads as not found instead of failing.
func Test_TemplateAlias_EmptyAnswer(t *testing.T) {
	h := NewTemplateAlias(emptyDescribe{newFakeAPI(awsfake.NewCloudControl())})
	_, err := h.Read(t.Context(), region, account+"|report|live")
	require.ErrorIs(t, err, native.ErrNotFound)
}

// emptyDescribe answers DescribeTemplateAlias without an alias.
type emptyDescribe struct {
	*fakeAPI
}

func (emptyDescribe) DescribeTemplateAlias(context.Context, *quicksight.DescribeTemplateAliasInput, ...func(*quicksight.Options)) (*quicksight.DescribeTemplateAliasOutput, error) {
	return &quicksight.DescribeTemplateAliasOutput{}, nil
}

// Test_TemplateAlias_SparseAnswer checks a read of an alias AWS returns without its optional TemplateVersionNumber and
// Arn: the version is the recorded one, or unknown without a record, and Arn is left out.
func Test_TemplateAlias_SparseAnswer(t *testing.T) {
	h, api := newAliasHandler()
	id := makeAlias(t, h, aliasTemplate())
	api.noVersion, api.noAliasArn = true, true

	read, unknown, err := h.ReadRecorded(t.Context(), region, id, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"TemplateVersionNumber"}, unknown)
	require.Equal(t, map[string]any{"AwsAccountId": account, "TemplateId": "report", "AliasName": "live"}, read)

	read, unknown, err = h.ReadRecorded(t.Context(), region, id, aliasTemplate())
	require.NoError(t, err)
	require.Empty(t, unknown)
	require.Equal(t, float64(2), read["TemplateVersionNumber"])

	// AWS's version wins over the record's.
	api.noVersion = false
	read, _, err = h.ReadRecorded(t.Context(), region, id, with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(3)}))
	require.NoError(t, err)
	require.Equal(t, float64(2), read["TemplateVersionNumber"])
}

// Test_TemplateAlias_UpdateShown checks that an update finishes only once a read shows the alias's new version, so the
// next deploy reads what the update set.
func Test_TemplateAlias_UpdateShown(t *testing.T) {
	h, _ := newAliasHandler()
	id := makeAlias(t, h, aliasTemplate())
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(3)})}))
	wait(t, h, id, native.OperationUpdate)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, float64(3), read["TemplateVersionNumber"])
}

// Test_TemplateAlias_CreateExistingShown checks that a create that points an existing alias to the template's version
// returns only once a read shows that version.
func Test_TemplateAlias_CreateExistingShown(t *testing.T) {
	h, _ := newAliasHandler()
	makeAlias(t, h, with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(1)}))
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: aliasTemplate()})
	require.NoError(t, err)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, float64(2), read["TemplateVersionNumber"])
}

// Test_TemplateAlias_CreateKeepsIdentifier checks that a create AWS may have applied returns the alias's identifier
// with its error, and one AWS rejected returns none.
func Test_TemplateAlias_CreateKeepsIdentifier(t *testing.T) {
	h, api := newAliasHandler()
	api.errs["CreateTemplateAlias"] = &smithy.GenericAPIError{Code: "InternalFailureException", Message: "lost", Fault: smithy.FaultServer}
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: aliasTemplate()})
	require.Error(t, err)
	require.Equal(t, account+"|report|live", id)

	api.errs["CreateTemplateAlias"] = fault("ThrottlingException", "slow down")
	id, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: aliasTemplate()})
	requireRejected(t, err)
	require.Empty(t, id)
}

// Test_TemplateAlias_UpdateNotShown checks that an update no read confirms fails with an unknown outcome, which asks
// the user to deploy again, instead of reporting success.
func Test_TemplateAlias_UpdateNotShown(t *testing.T) {
	h, api := newAliasHandler()
	id := makeAlias(t, h, aliasTemplate())
	api.holdVersion = true
	err := h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(3)})})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome)
	require.False(t, failure.ClientError)
	require.Contains(t, failure.Message, "Deploy again")
	require.Equal(t, 1+confirmReads, api.calls["DescribeTemplateAlias"], "one read for the create, then confirmReads")

	// A read that fails while confirming is named in the failure, and stays unknown.
	api.holdVersion = false
	api.errs["DescribeTemplateAlias"] = fault("ThrottlingException", "slow down")
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: with(aliasTemplate(), map[string]any{"TemplateVersionNumber": float64(1)})})
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome)
	require.Contains(t, failure.Message, "ThrottlingException")

	// A context that ends stops the wait.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	h.wait = sleep
	err = h.confirm(ctx, region, aliasID{account: account, template: "report", alias: "live"}, 3)
	require.ErrorAs(t, err, &failure)
	require.Contains(t, failure.Message, context.Canceled.Error())
}
