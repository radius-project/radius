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
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/aws/aws-sdk-go-v2/service/grafana/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/grafana/latest/APIReference/"

	// constraintNote is what every call's page leaves unclear about its constraints.
	constraintNote = " The page names no error for each constraint; the fake returns ValidationException, which the page " +
		"lists for a parameter that caused an error."

	// busyNote is what the docs leave unclear about a workspace that is changing.
	busyNote = " The page lists ConflictException for a resource in an inconsistent state during an update or a deletion, " +
		"but doesn't say when. The fake returns it while the workspace is busy, and for the next call to a workspace " +
		"after a service account was created or deleted."
)

// apiCall is a call against a fake Amazon Managed Grafana API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake backed by store. It stops at the
// first call that fails, and returns its error.
func against(calls ...apiCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// create returns a call that creates a service account named name in workspace, with the changes edits make.
func create(name string, edits ...func(*grafana.CreateWorkspaceServiceAccountInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &grafana.CreateWorkspaceServiceAccountInput{WorkspaceId: aws.String(workspace), Name: aws.String(name), GrafanaRole: types.RoleViewer}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateWorkspaceServiceAccount(ctx, in)
		return err
	}
}

// inWorkspace returns an edit that names the workspace.
func inWorkspace(ws string) func(*grafana.CreateWorkspaceServiceAccountInput) {
	return func(in *grafana.CreateWorkspaceServiceAccountInput) { in.WorkspaceId = aws.String(ws) }
}

// namesOf returns the names of the workspace's service accounts, from every page of the list.
func namesOf(ctx context.Context, f *fakeAPI, ws string) ([]string, error) {
	names := []string{}
	in := &grafana.ListWorkspaceServiceAccountsInput{WorkspaceId: aws.String(ws)}
	for {
		page, err := f.ListWorkspaceServiceAccounts(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, s := range page.ServiceAccounts {
			names = append(names, aws.ToString(s.Name))
		}
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return names, nil
		}
	}
}

// idle waits out the workspace being busy after a change.
func idle(ctx context.Context, f *fakeAPI) error {
	f.busy[workspace] = 0
	return nil
}

// accountAPI is the record of the AWS calls the workspace service account handler makes.
var accountAPI = map[string]nativetest.Call{
	"CreateWorkspaceServiceAccount": {
		Doc: apiReference + "API_CreateWorkspaceServiceAccount.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "workspaceId matches g-[0-9a-f]{10}",
				Break: against(create("ci", inWorkspace("g-123"))),
				Code:  validationCode,
			},
			{
				Rule:  "name is 1 to 128 characters",
				Break: against(create(strings.Repeat("n", 129))),
				Code:  validationCode,
			},
			{
				Rule: "grafanaRole is ADMIN, EDITOR, or VIEWER",
				Break: against(create("ci", func(in *grafana.CreateWorkspaceServiceAccountInput) {
					in.GrafanaRole = "NONE"
				})),
				Code: validationCode,
			},
			{
				Rule:  "a workspace that doesn't exist fails with ResourceNotFoundException",
				Break: against(create("ci", inWorkspace(missing))),
				Code:  notFoundCode,
			},
			{
				Rule:  "service accounts are only for workspaces compatible with Grafana 9 and later",
				Break: against(create("ci", inWorkspace(oldWorkspace))),
				Code:  validationCode,
			},
			{
				Rule:  "the name must be unique within the workspace",
				Break: against(create("ci"), idle, create("ci")),
				Code:  conflictCode,
			},
			{
				Rule: "the same name in another workspace is another service account",
				Check: against(create("ci"), create("ci", inWorkspace(otherWorkspace)), func(ctx context.Context, f *fakeAPI) error {
					if n := len(f.store.List(serviceAccountType)); n != 2 {
						return fmt.Errorf("the workspaces have %d service accounts, not 2", n)
					}
					return nil
				}),
			},
		},
		Unclear: "The error for a name another service account has: the fake returns ConflictException, and also for a name " +
			"that gives the same login, such as one that differs only in case or in spaces for dashes, as Grafana's GenerateLogin makes it." + busyNote +
			" The service quota of service accounts per workspace: the fake has none." + constraintNote,
	},
	"ListWorkspaceServiceAccounts": {
		Doc: apiReference + "API_ListWorkspaceServiceAccounts.html",
		Rules: []nativetest.Rule{
			{
				Rule: "maxResults is 1 to 100",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListWorkspaceServiceAccounts(ctx, &grafana.ListWorkspaceServiceAccountsInput{WorkspaceId: aws.String(workspace), MaxResults: aws.Int32(101)})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "a workspace that doesn't exist fails with ResourceNotFoundException",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListWorkspaceServiceAccounts(ctx, &grafana.ListWorkspaceServiceAccountsInput{WorkspaceId: aws.String(missing)})
					return err
				}),
				Code: notFoundCode,
			},
			{
				Rule: "the list has every service account of the workspace, across pages",
				Check: against(create("a"), idle, create("b"), idle, create("c"), idle, func(ctx context.Context, f *fakeAPI) error {
					names, err := namesOf(ctx, f, workspace)
					if err != nil {
						return err
					}
					if len(names) != 3 {
						return errors.New("the list doesn't have every service account")
					}
					return nil
				}),
			},
		},
		Unclear: "How soon the list shows a created or deleted service account: the fake shows a deleted one for two more " +
			"lists, and a created one at once, or after two lists in Test_WorkspaceServiceAccount_ListLag. The page size." + busyNote,
	},
	"DeleteWorkspaceServiceAccount": {
		Doc: apiReference + "API_DeleteWorkspaceServiceAccount.html",
		Rules: []nativetest.Rule{
			{
				Rule: "a service account that doesn't exist fails with ResourceNotFoundException",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteWorkspaceServiceAccount(ctx, &grafana.DeleteWorkspaceServiceAccountInput{WorkspaceId: aws.String(workspace), ServiceAccountId: aws.String("9")})
					return err
				}),
				Code: notFoundCode,
			},
			{
				Rule: "workspaceId matches g-[0-9a-f]{10}",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteWorkspaceServiceAccount(ctx, &grafana.DeleteWorkspaceServiceAccountInput{WorkspaceId: aws.String("ws"), ServiceAccountId: aws.String("1")})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "the name of a deleted service account can be used again",
				Check: against(create("ci"), idle, func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteWorkspaceServiceAccount(ctx, &grafana.DeleteWorkspaceServiceAccountInput{WorkspaceId: aws.String(workspace), ServiceAccountId: aws.String("1")})
					return err
				}, idle, create("ci")),
			},
		},
		Unclear: "Whether a deleted service account's tokens stop working at once: the page says they fail after the " +
			"delete." + busyNote + constraintNote,
	},
}
