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
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/aws/aws-sdk-go-v2/service/quicksight/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func newMembershipHandler() (*FolderMembership, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewFolderMembership(api), api
}

// requireRejected checks that err is a failure AWS applied none of: a rejected client failure, or a client fault from
// the fake, which UCP takes as rejected. The real SDK's 4xx errors become rejected failures (see the replay tests).
func requireRejected(t *testing.T, err error) {
	t.Helper()
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		require.Equal(t, smithy.FaultClient, apiErr.ErrorFault(), "%v", err)
		return
	}
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError, "%v", err)
	require.False(t, failure.Conflict, "%v", err)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
}

// requireConflict checks that err is a conflict UCP answers with 409, which names dependsOn.
func requireConflict(t *testing.T, err error) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict, "%v", err)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "dependsOn")
}

// wait reads the operation's progress until it finishes, as UCP's worker does.
func wait(t *testing.T, h native.Handler, identifier string, op native.Operation) {
	t.Helper()
	for range 10 {
		p, err := h.Progress(t.Context(), region, identifier, op)
		if op == native.OperationDelete && errors.Is(err, native.ErrNotFound) {
			return
		}
		require.NoError(t, err)
		if p.Done {
			return
		}
	}
	require.FailNow(t, "the operation never finished")
}

func addTo(t *testing.T, h *FolderMembership, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	wait(t, h, id, native.OperationCreate)
	return id
}

func Test_FolderMembership_Identifier(t *testing.T) {
	h, api := newMembershipHandler()
	id, err := h.Identifier(membershipTemplate())
	require.NoError(t, err)
	require.Equal(t, account+"|sales|DASHBOARD|dash-1", id)
	for _, partial := range []string{"AwsAccountId", "FolderId", "MemberType", "MemberId"} {
		properties := membershipTemplate()
		delete(properties, partial)
		id, err := h.Identifier(properties)
		require.NoError(t, err)
		require.Empty(t, id, "without %s", partial)
		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
		requireRejected(t, err)
	}
	for _, change := range []map[string]any{
		{"AwsAccountId": "12345678901a"},
		{"FolderId": "a|b"},
		{"MemberType": "dashboard"},
		{"MemberId": ""},
	} {
		_, err := h.Identifier(with(membershipTemplate(), change))
		requireRejected(t, err)
		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(membershipTemplate(), change)})
		requireRejected(t, err)
	}
	for _, identifier := range []string{
		"", account + "|sales|DASHBOARD", account + "|sales|REPORT|dash-1", "1234|sales|DASHBOARD|dash-1",
		account + "|sales|DASHBOARD|dash 1", account + "|sales|DASHBOARD|dash-1|x",
	} {
		_, err := h.Read(t.Context(), region, identifier)
		requireRejected(t, err)
		requireRejected(t, h.Delete(t.Context(), region, identifier))
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		requireRejected(t, err)
	}
	require.Empty(t, api.calls, "a bad identifier never reaches AWS")
}

// Test_FolderMembership_CreateFindsExisting checks that a create sent again, after AWS took the first one, succeeds
// without a second membership.
func Test_FolderMembership_CreateFindsExisting(t *testing.T) {
	h, api := newMembershipHandler()
	id := addTo(t, h, membershipTemplate())
	again, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: membershipTemplate()})
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.Equal(t, 2, api.calls["CreateFolderMembership"])
	require.Len(t, api.store.List(folderMembershipType), 1)
}

// Test_FolderMembership_SameIDOtherType checks that a dataset with the ID of a dashboard in the folder isn't taken for
// the dashboard: AWS lists members without their type.
func Test_FolderMembership_SameIDOtherType(t *testing.T) {
	h, _ := newMembershipHandler()
	dashboard := addTo(t, h, membershipTemplate())
	dataset := with(membershipTemplate(), map[string]any{"MemberType": "DATASET"})
	id, err := h.Identifier(dataset)
	require.NoError(t, err)
	_, err = h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "UCP sends a create again for a member the folder doesn't list")

	addTo(t, h, dataset)
	read, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, memberArn(types.MemberTypeDataset, "dash-1"), read["MemberArn"])
	read, err = h.Read(t.Context(), region, dashboard)
	require.NoError(t, err)
	require.Equal(t, with(membershipTemplate(), map[string]any{"MemberArn": memberArn(types.MemberTypeDashboard, "dash-1")}), read)
}

// Test_FolderMembership_BadArn checks that a member AWS lists with the ID but no ARN the handler can read fails the
// read instead of being taken as missing or as present.
func Test_FolderMembership_BadArn(t *testing.T) {
	h, api := newMembershipHandler()
	id := addTo(t, h, membershipTemplate())
	api.noArn = true
	_, err := h.Read(t.Context(), region, id)
	require.ErrorContains(t, err, "isn't the ARN")
	require.NotErrorIs(t, err, native.ErrNotFound)

	for _, m := range []types.MemberIdArnPair{
		{MemberId: aws.String("dash-1"), MemberArn: aws.String("arn:aws:s3:::dashboard/dash-1")},
		{MemberId: aws.String("dash-1"), MemberArn: aws.String(memberArn(types.MemberTypeDashboard, "dash-2"))},
		{MemberId: aws.String("dash-1"), MemberArn: aws.String("arn:aws:quicksight:us-west-2:" + account + ":dashboard")},
	} {
		_, err := membershipID{account: account, folder: "sales", kind: types.MemberTypeDashboard, member: "dash-1"}.isMember(m)
		require.Error(t, err, aws.ToString(m.MemberArn))
	}
}

// Test_FolderMembership_Pages checks that a member on a later page of the folder's list is found, and that a folder
// that doesn't exist reads as not found, while a create in it is AWS's rejection.
func Test_FolderMembership_Pages(t *testing.T) {
	h, _ := newMembershipHandler()
	for _, member := range []map[string]any{
		{"MemberType": "ANALYSIS", "MemberId": "analysis-1"},
		{"MemberType": "DASHBOARD", "MemberId": "dash-2"},
		{"MemberType": "DATASET", "MemberId": "dataset-1"},
		{"MemberType": "TOPIC", "MemberId": "topic-1"},
	} {
		addTo(t, h, with(membershipTemplate(), member))
	}
	_, err := h.Read(t.Context(), region, account+"|sales|TOPIC|topic-1")
	require.NoError(t, err)

	missing := with(membershipTemplate(), map[string]any{"FolderId": "missing"})
	id, err := h.Identifier(missing)
	require.NoError(t, err)
	_, err = h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), region, id), native.ErrNotFound)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: missing})
	requireRejected(t, err)
	require.NotErrorIs(t, err, native.ErrNotFound, "a create in a missing folder is AWS's rejection, not a missing resource")

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources)
}

// Test_FolderMembership_Delete checks that a delete waits until the folder no longer lists the member, that the
// folder rejects another change of its members until then with a conflict, and that the next deploy succeeds.
func Test_FolderMembership_Delete(t *testing.T) {
	h, api := newMembershipHandler()
	id := addTo(t, h, membershipTemplate())
	p, err := h.Progress(t.Context(), region, id, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, p.Done)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{}))

	require.NoError(t, h.Delete(t.Context(), region, id))
	other := with(membershipTemplate(), map[string]any{"MemberId": "dash-2"})
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: other})
	requireConflict(t, err)
	requireConflict(t, h.Delete(t.Context(), region, id))

	p, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.False(t, p.Done, "the first read after the delete still lists the member")
	wait(t, h, id, native.OperationDelete)
	require.Empty(t, api.store.List(folderMembershipType))
	addTo(t, h, other)
}

// Test_FolderMembership_Errors checks that AWS's errors are returned as they are: a failed list, an edition without
// folders, and another account than the caller's.
func Test_FolderMembership_Errors(t *testing.T) {
	h, api := newMembershipHandler()
	id := addTo(t, h, membershipTemplate())
	boom := errors.New("boom")
	api.errs["ListFolderMembers"] = boom
	_, err := h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, boom)
	_, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.ErrorIs(t, err, boom)

	api.errs = map[string]error{}
	api.standard = true
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(membershipTemplate(), map[string]any{"MemberId": "dash-2"})})
	requireRejected(t, err)
	requireRejected(t, h.Delete(t.Context(), region, id))

	api.standard = false
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(membershipTemplate(), map[string]any{"AwsAccountId": otherAccount})})
	requireRejected(t, err)
}

// Test_FolderMembership_Request checks the input of each call.
func Test_FolderMembership_Request(t *testing.T) {
	api := &membershipInputs{fakeAPI: newFakeAPI(awsfake.NewCloudControl())}
	h := NewFolderMembership(api)
	id := addTo(t, h, with(membershipTemplate(), map[string]any{"MemberType": "TOPIC", "MemberId": "topic-1"}))
	require.NoError(t, h.Delete(t.Context(), region, id))
	want := struct{ account, folder, kind, member string }{account, "sales", "TOPIC", "topic-1"}
	require.Equal(t, want, api.created)
	require.Equal(t, want, api.deleted)
	require.NotEmpty(t, api.pageSizes)
	for _, size := range api.pageSizes {
		require.Equal(t, int32(100), size, "the fewest pages AWS allows")
	}
}

// membershipInputs records the inputs of CreateFolderMembership and DeleteFolderMembership, and the page size of
// ListFolderMembers.
type membershipInputs struct {
	*fakeAPI
	created, deleted struct{ account, folder, kind, member string }
	pageSizes        []int32
}

func (m *membershipInputs) ListFolderMembers(ctx context.Context, in *quicksight.ListFolderMembersInput, opts ...func(*quicksight.Options)) (*quicksight.ListFolderMembersOutput, error) {
	m.pageSizes = append(m.pageSizes, aws.ToInt32(in.MaxResults))
	return m.fakeAPI.ListFolderMembers(ctx, in, opts...)
}

func (m *membershipInputs) CreateFolderMembership(ctx context.Context, in *quicksight.CreateFolderMembershipInput, opts ...func(*quicksight.Options)) (*quicksight.CreateFolderMembershipOutput, error) {
	m.created = struct{ account, folder, kind, member string }{aws.ToString(in.AwsAccountId), aws.ToString(in.FolderId), string(in.MemberType), aws.ToString(in.MemberId)}
	return m.fakeAPI.CreateFolderMembership(ctx, in, opts...)
}

func (m *membershipInputs) DeleteFolderMembership(ctx context.Context, in *quicksight.DeleteFolderMembershipInput, opts ...func(*quicksight.Options)) (*quicksight.DeleteFolderMembershipOutput, error) {
	m.deleted = struct{ account, folder, kind, member string }{aws.ToString(in.AwsAccountId), aws.ToString(in.FolderId), string(in.MemberType), aws.ToString(in.MemberId)}
	return m.fakeAPI.DeleteFolderMembership(ctx, in, opts...)
}
