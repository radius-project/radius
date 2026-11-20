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

package grafana

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/aws/smithy-go"
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

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	maps.Copy(out, changes)
	return out
}

// serviceAccount returns a template for a service account named name in workspace.
func serviceAccount(name string) map[string]any {
	return map[string]any{"WorkspaceId": workspace, "Name": name, "GrafanaRole": "VIEWER"}
}

func newHandler() (*WorkspaceServiceAccount, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewWorkspaceServiceAccount(api), api
}

func Test_Suite_WorkspaceServiceAccount(t *testing.T) {
	properties := serviceAccount("ci-reader")
	nativetest.Run(t, nativetest.Suite{
		TypeName:    serviceAccountType,
		Schema:      readFile(t, "aws-grafana-workspaceserviceaccount.json"),
		Metadata:    readFile(t, "aws-grafana-workspaceserviceaccount.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewWorkspaceServiceAccount(newFakeAPI(store))
		},
		Properties: properties,
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: with(properties, map[string]any{"GrafanaRole": "EDITOR"}),
		// Grafana keeps a role as Viewer, and the API reference doesn't say AWS always returns it in uppercase.
		OtherForm: map[string]any{"GrafanaRole": "Viewer"},
		Invalid: []map[string]any{
			with(properties, map[string]any{"WorkspaceId": "g-XYZ"}),
			with(properties, map[string]any{"GrafanaRole": "NONE"}),
			with(properties, map[string]any{"Name": ""}),
			with(properties, map[string]any{"Name": strings.Repeat("n", maxName+1)}),
			with(properties, map[string]any{"Name": 7}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseAccountID(identifier)
			require.NoError(t, err)
			return map[string]any{"WorkspaceId": id.workspace, "Id": id.id}
		},
		API: accountAPI,
	})
}

// wait calls Progress until op finishes, and fails the test when it doesn't within 10 reads.
func wait(t *testing.T, h *WorkspaceServiceAccount, identifier string, op native.Operation) {
	t.Helper()
	for range 10 {
		p, err := h.Progress(t.Context(), region, identifier, op)
		if op == native.OperationDelete && errors.Is(err, native.ErrNotFound) {
			return
		}
		require.NoError(t, err)
		if p.Done {
			require.Nil(t, p.Failure)
			return
		}
	}
	require.FailNow(t, "the operation never finished")
}

func createAccount(t *testing.T, h *WorkspaceServiceAccount, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	wait(t, h, id, native.OperationCreate)
	return id
}

// requireFailure checks that err is a native.Failure that is a conflict or not, rejected, with a message that has
// want.
func requireFailure(t *testing.T, err error, conflict bool, want string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError, "%v", err)
	require.Equal(t, conflict, failure.Conflict, "%v", err)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, want)
}

func Test_WorkspaceServiceAccount_Identifier(t *testing.T) {
	h, api := newHandler()
	id, err := h.Identifier(serviceAccount("ci"))
	require.NoError(t, err)
	require.Empty(t, id, "AWS assigns the Id")
	id, err = h.Identifier(with(serviceAccount("ci"), map[string]any{"Id": "7"}))
	require.NoError(t, err)
	require.Equal(t, workspace+"|7", id)

	for _, identifier := range []string{workspace, workspace + "|", "ws|7", "G-0123456789|7"} {
		_, err := h.Read(t.Context(), region, identifier)
		requireFailure(t, err, false, "is not the identifier")
		requireFailure(t, h.Delete(t.Context(), region, identifier), false, "is not the identifier")
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		requireFailure(t, err, false, "is not the identifier")
	}
	for _, properties := range []map[string]any{{"WorkspaceId": workspace}, {"Name": "ci", "GrafanaRole": "VIEWER"}, with(serviceAccount("ci"), map[string]any{"WorkspaceId": "ws"})} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
		requireFailure(t, err, false, "WorkspaceId")
		_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: properties})
		requireFailure(t, err, false, "WorkspaceId")
	}
	require.Zero(t, api.calls["CreateWorkspaceServiceAccount"]+api.calls["ListWorkspaceServiceAccounts"]+api.calls["DeleteWorkspaceServiceAccount"])
}

// Test_WorkspaceServiceAccount_FirstLook checks that the workspace is busy right after a create or delete, so Progress
// waits for it the first time it looks, and that a deleted service account AWS still lists keeps the delete waiting.
func Test_WorkspaceServiceAccount_FirstLook(t *testing.T) {
	h, _ := newHandler()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("ci")})
	require.NoError(t, err)
	p, err := h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, p.Done, "the workspace is busy")
	wait(t, h, id, native.OperationCreate)
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"WorkspaceId": workspace, "Id": "1", "Name": "ci", "GrafanaRole": "VIEWER", "IsDisabled": "false"}, got)

	require.NoError(t, h.Delete(t.Context(), region, id))
	for range 2 {
		p, err = h.Progress(t.Context(), region, id, native.OperationDelete)
		require.NoError(t, err)
		require.False(t, p.Done, "the workspace is busy, then AWS still lists the service account")
	}
	wait(t, h, id, native.OperationDelete)
	require.ErrorIs(t, h.Delete(t.Context(), region, id), native.ErrNotFound)
}

// Test_WorkspaceServiceAccount_Busy checks that a create or delete AWS rejects while the workspace is changing is a
// conflict, so UCP answers 409, and that it changes nothing.
func Test_WorkspaceServiceAccount_Busy(t *testing.T) {
	h, api := newHandler()
	id := createAccount(t, h, serviceAccount("ci"))
	api.busy[workspace] = 3
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("other")})
	requireFailure(t, err, true, "another service account may have this name")
	err = h.Delete(t.Context(), region, id)
	requireFailure(t, err, true, "dependsOn")
	require.NotContains(t, err.Error(), "this name", "only a create can clash on the name")
	_, err = h.Read(t.Context(), region, id)
	requireFailure(t, err, true, "dependsOn")
	require.Len(t, api.store.List(serviceAccountType), 1)
	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("ci")})
	require.NoError(t, err)
}

// Test_WorkspaceServiceAccount_TakenName checks that a create with a name another service account has, or one that
// Grafana takes as the same, fails as a conflict whose message names the cause, and changes nothing. Names Grafana
// takes as different are made.
func Test_WorkspaceServiceAccount_TakenName(t *testing.T) {
	h, api := newHandler()
	createAccount(t, h, serviceAccount("CI Reader"))
	for _, name := range []string{"CI Reader", "ci-reader", "ci reader"} {
		api.busy[workspace] = 0
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount(name)})
		requireFailure(t, err, true, "another service account may have this name")
	}
	require.Len(t, api.store.List(serviceAccountType), 1)
	for _, name := range []string{"ci.reader", "ci_reader", "çi-reader"} {
		api.busy[workspace] = 0
		createAccount(t, h, serviceAccount(name))
	}
	require.Len(t, api.store.List(serviceAccountType), 4)
}

// Test_WorkspaceServiceAccount_Find checks that Find finds a lost create by its name, and only by its exact name, and
// never takes over one with another role or picks between two.
func Test_WorkspaceServiceAccount_Find(t *testing.T) {
	h, api := newHandler()
	find := func(properties map[string]any) (string, error) {
		return h.Find(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	}
	id := createAccount(t, h, serviceAccount("ci"))
	// More service accounts than a page holds, so the name is found across pages.
	for i := range 3 {
		createAccount(t, h, serviceAccount(fmt.Sprintf("other-%d", i)))
	}
	found, err := find(serviceAccount("ci"))
	require.NoError(t, err)
	require.Equal(t, id, found)

	for _, properties := range []map[string]any{serviceAccount("CI"), serviceAccount("c"), with(serviceAccount("ci"), map[string]any{"WorkspaceId": otherWorkspace}), with(serviceAccount("ci"), map[string]any{"WorkspaceId": missing})} {
		found, err := find(properties)
		require.NoError(t, err)
		require.Empty(t, found)
	}

	// A role AWS returns in another case is the same role.
	stored := api.store.Resource(serviceAccountType, id)
	stored["GrafanaRole"] = "Viewer"
	api.store.Put(serviceAccountType, id, stored)
	found, err = find(serviceAccount("ci"))
	require.NoError(t, err)
	require.Equal(t, id, found)

	_, err = find(with(serviceAccount("ci"), map[string]any{"GrafanaRole": "ADMIN"}))
	requireFailure(t, err, false, "with the role Viewer, not the template's ADMIN")

	// AWS keeps names unique, but if it ever lists two, Find takes neither.
	api.store.Put(serviceAccountType, workspace+"|99", map[string]any{"WorkspaceId": workspace, "Id": "99", "Name": "ci", "GrafanaRole": "VIEWER"})
	_, err = find(serviceAccount("ci"))
	require.ErrorContains(t, err, workspace+"|99")
	require.ErrorContains(t, err, id)

	api.errs["ListWorkspaceServiceAccounts"] = errors.New("list down")
	_, err = find(serviceAccount("ci"))
	require.ErrorContains(t, err, "list down")
}

// Test_WorkspaceServiceAccount_ListLag checks what happens if AWS lists a new service account only after a while,
// which its docs don't rule out. UCP takes a create AWS doesn't list as one that never reached AWS, and Find doesn't
// see it either, so the create sent again fails because the name is taken. A later deploy finds it, and AWS never has
// two.
func Test_WorkspaceServiceAccount_ListLag(t *testing.T) {
	h, api := newHandler()
	api.listLag = 2
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("ci")})
	require.NoError(t, err)
	api.busy[workspace] = 0
	_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)

	found, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("ci")})
	require.NoError(t, err)
	require.Empty(t, found)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("ci")})
	requireFailure(t, err, true, "another service account may have this name")

	found, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("ci")})
	require.NoError(t, err)
	require.Equal(t, id, found)
	require.Len(t, api.store.List(serviceAccountType), 1)
}

// Test_WorkspaceServiceAccount_Errors checks the errors AWS returns for a workspace that doesn't exist or doesn't
// support service accounts, and for a delete AWS rejects.
func Test_WorkspaceServiceAccount_Errors(t *testing.T) {
	h, api := newHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(serviceAccount("ci"), map[string]any{"WorkspaceId": missing})})
	requireFailure(t, err, false, "doesn't exist")
	require.NotErrorIs(t, err, native.ErrNotFound, "a create in a missing workspace is AWS's rejection, not a missing resource")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(serviceAccount("ci"), map[string]any{"WorkspaceId": oldWorkspace})})
	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, validationCode, apiErr.ErrorCode())

	_, err = h.Read(t.Context(), region, missing+"|1")
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), region, missing+"|1"), native.ErrNotFound)
	_, err = h.Progress(t.Context(), region, workspace+"|1", native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "UCP learns a lost create never reached AWS")

	id := createAccount(t, h, serviceAccount("ci"))
	api.busy[workspace] = 0
	api.errs["DeleteWorkspaceServiceAccount"] = errors.New("delete down")
	require.ErrorContains(t, h.Delete(t.Context(), region, id), "delete down")
	api.errs["CreateWorkspaceServiceAccount"] = errors.New("create down")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("b")})
	require.ErrorContains(t, err, "create down")

	// A create AWS answers without an ID.
	h = NewWorkspaceServiceAccount(noID{api})
	delete(api.errs, "CreateWorkspaceServiceAccount")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: serviceAccount("c")})
	require.ErrorContains(t, err, "returned no ID")
}

// noID is an API whose creates return no ID.
type noID struct {
	*fakeAPI
}

func (n noID) CreateWorkspaceServiceAccount(ctx context.Context, in *grafana.CreateWorkspaceServiceAccountInput, opts ...func(*grafana.Options)) (*grafana.CreateWorkspaceServiceAccountOutput, error) {
	out, err := n.fakeAPI.CreateWorkspaceServiceAccount(ctx, in, opts...)
	if out != nil {
		out.Id = nil
	}
	return out, err
}

// Test_WorkspaceServiceAccount_UpdateAndList checks that an update changes nothing, as every property is create-only,
// and that List returns nothing, because AWS lists service accounts only by workspace.
func Test_WorkspaceServiceAccount_UpdateAndList(t *testing.T) {
	h, api := newHandler()
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{}))
	p, err := h.Progress(t.Context(), region, workspace+"|1", native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, p.Done)
	list, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, list)
	require.Zero(t, api.calls["ListWorkspaceServiceAccounts"])
}

func Test_InRegion(t *testing.T) {
	opts := grafana.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}
