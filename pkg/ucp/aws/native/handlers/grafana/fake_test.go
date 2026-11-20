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
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/aws/aws-sdk-go-v2/service/grafana/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region = "us-west-2"

	// pageSize is how many service accounts a list page has. AWS doesn't document its page size.
	pageSize = 2

	// settleReads is how many lists of a workspace still show a deleted service account. AWS doesn't say how soon its
	// list shows a change.
	settleReads = 2

	// The error codes of the Amazon Managed Grafana API reference.
	validationCode = "ValidationException"
	conflictCode   = "ConflictException"
	notFoundCode   = "ResourceNotFoundException"
)

// The workspaces the fake has.
const (
	workspace      = "g-0123456789"
	otherWorkspace = "g-abcdef0123"
	oldWorkspace   = "g-0000000008"
	missing        = "g-ffffffffff"
)

// workspaceVersions are the Grafana versions of the fake's workspaces.
var workspaceVersions = map[string]string{
	workspace:      "10.4",
	otherWorkspace: "9.4",
	oldWorkspace:   "8.4",
}

// login returns the login Grafana's GenerateLogin (pkg/services/serviceaccounts/utils.go) makes from a service
// account's name, in the workspace's organization. Two names with the same login can't both be in a workspace.
func login(name string) string {
	generated := "sa-1-" + strings.ToLower(name)
	generated = strings.Replace(generated, "--", "-", 1)
	return strings.ReplaceAll(generated, " ", "-")
}

// fakeAPI is an in-memory Amazon Managed Grafana API for workspace service accounts. It keeps them in an awsfake
// store, keyed by their identifier, in the type's property format, so the suite's Cloud Control sees the same ones. It
// rejects the requests the API reference says AWS rejects (see accountAPI), and where the docs are silent it takes the
// pessimistic side:
//
//   - After a service account is created or deleted, the workspace is busy for the next call to it, which fails with
//     ConflictException, as do calls while busy holds a count for it, such as while the workspace is updating.
//   - A deleted service account stays in the list for settleReads lists of its workspace, and with listLag set, a
//     created one stays out of it for that many. listLag is off by default, because UCP takes a create AWS doesn't
//     list as one that never reached AWS (see Test_WorkspaceServiceAccount_ListLag).
//   - A name that gives the same login as another service account's, such as one that differs only in case or in spaces
//     for dashes, is taken, as in Grafana, and a create with a taken name fails with ConflictException.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu sync.Mutex

	// busy holds how many more calls to each workspace fail with ConflictException.
	busy map[string]int

	// pending holds how many more lists a created service account stays out of, and ghosts how many more lists show a
	// deleted one, by identifier. listLag is how many lists a new service account stays out of.
	pending map[string]int
	ghosts  map[string]*ghost
	listLag int

	nextID int

	// calls counts the calls that reached the fake, by method, and errs maps a method name to an error the call
	// returns instead of acting.
	calls map[string]int
	errs  map[string]error
}

// ghost is a deleted service account the list still shows.
type ghost struct {
	summary types.ServiceAccountSummary
	reads   int
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store:   store,
		busy:    map[string]int{},
		pending: map[string]int{},
		ghosts:  map[string]*ghost{},
		nextID:  1,
		calls:   map[string]int{},
		errs:    map[string]error{},
	}
}

func fault(code string, format string, args ...any) error {
	return &smithy.GenericAPIError{Code: code, Message: fmt.Sprintf(format, args...), Fault: smithy.FaultClient}
}

// begin counts a call and returns the error errs holds for it.
func (f *fakeAPI) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

func (f *fakeAPI) change(apply func()) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	apply()
	return f.failer.After()
}

// checkWorkspace checks a call's workspace: its ID's pattern, that it exists, that it supports service accounts, and
// that it isn't busy. A call to a busy workspace counts down how long it stays busy.
func (f *fakeAPI) checkWorkspace(id *string) error {
	ws := aws.ToString(id)
	if !workspaceIDPattern.MatchString(ws) {
		return fault(validationCode, "workspaceId %q doesn't match the pattern", ws)
	}
	version, ok := workspaceVersions[ws]
	if !ok {
		return fault(notFoundCode, "the workspace %s doesn't exist", ws)
	}
	if major, _ := strconv.Atoi(strings.Split(version, ".")[0]); major < 9 {
		return fault(validationCode, "service accounts need a workspace compatible with Grafana 9 or later")
	}
	if f.busy[ws] > 0 {
		f.busy[ws]--
		return fault(conflictCode, "the workspace %s is updating", ws)
	}
	return nil
}

// summary returns the stored service account as AWS lists it, filling in what the record leaves out, as for a record
// the suite writes.
func summary(p map[string]any) types.ServiceAccountSummary {
	str := func(name string, fallback string) *string {
		if s, _ := p[name].(string); s != "" {
			return aws.String(s)
		}
		return aws.String(fallback)
	}
	return types.ServiceAccountSummary{
		Id:          str("Id", ""),
		Name:        str("Name", ""),
		GrafanaRole: types.Role(aws.ToString(str("GrafanaRole", string(types.RoleViewer)))),
		IsDisabled:  str("IsDisabled", "false"),
	}
}

// accounts returns the identifiers of the workspace's stored service accounts.
func (f *fakeAPI) accounts(ws string) []string {
	out := []string{}
	for _, identifier := range f.store.List(serviceAccountType) {
		if strings.HasPrefix(identifier, ws+"|") {
			out = append(out, identifier)
		}
	}
	return out
}

func (f *fakeAPI) CreateWorkspaceServiceAccount(ctx context.Context, in *grafana.CreateWorkspaceServiceAccountInput, _ ...func(*grafana.Options)) (*grafana.CreateWorkspaceServiceAccountOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateWorkspaceServiceAccount"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxName {
		return nil, fault(validationCode, "name must be 1 to %d characters", maxName)
	}
	if !slices.Contains(roles, in.GrafanaRole) {
		return nil, fault(validationCode, "grafanaRole must be ADMIN, EDITOR, or VIEWER")
	}
	if err := f.checkWorkspace(in.WorkspaceId); err != nil {
		return nil, err
	}
	ws := aws.ToString(in.WorkspaceId)
	for _, identifier := range f.accounts(ws) {
		if login(aws.ToString(summary(f.store.Resource(serviceAccountType, identifier)).Name)) == login(name) {
			return nil, fault(conflictCode, "a service account with the login of %q already exists", name)
		}
	}
	id := strconv.Itoa(f.nextID)
	f.nextID++
	identifier := ws + "|" + id
	err := f.change(func() {
		f.store.Put(serviceAccountType, identifier, map[string]any{
			"WorkspaceId": ws, "Id": id, "Name": name, "GrafanaRole": string(in.GrafanaRole), "IsDisabled": "false",
		})
		if f.listLag > 0 {
			f.pending[identifier] = f.listLag
		}
		f.busy[ws] = 1
	})
	if err != nil {
		return nil, err
	}
	return &grafana.CreateWorkspaceServiceAccountOutput{Id: aws.String(id), Name: aws.String(name), GrafanaRole: in.GrafanaRole, WorkspaceId: aws.String(ws)}, nil
}

func (f *fakeAPI) ListWorkspaceServiceAccounts(ctx context.Context, in *grafana.ListWorkspaceServiceAccountsInput, _ ...func(*grafana.Options)) (*grafana.ListWorkspaceServiceAccountsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListWorkspaceServiceAccounts"); err != nil {
		return nil, err
	}
	if in.MaxResults != nil && (*in.MaxResults < 1 || *in.MaxResults > 100) {
		return nil, fault(validationCode, "maxResults must be 1 to 100")
	}
	if err := f.checkWorkspace(in.WorkspaceId); err != nil {
		return nil, err
	}
	ws := aws.ToString(in.WorkspaceId)
	list := []types.ServiceAccountSummary{}
	for _, identifier := range f.accounts(ws) {
		if f.pending[identifier] == 0 {
			list = append(list, summary(f.store.Resource(serviceAccountType, identifier)))
		}
	}
	for _, identifier := range slices.Sorted(maps.Keys(f.ghosts)) {
		if strings.HasPrefix(identifier, ws+"|") {
			list = append(list, f.ghosts[identifier].summary)
		}
	}
	start := 0
	if in.NextToken != nil {
		var err error
		if start, err = strconv.Atoi(aws.ToString(in.NextToken)); err != nil || start < 0 || start > len(list) {
			return nil, fault(validationCode, "the next token is invalid")
		}
	}
	size := pageSize
	if in.MaxResults != nil && int(*in.MaxResults) < size {
		size = int(*in.MaxResults)
	}
	end := min(start+size, len(list))
	out := &grafana.ListWorkspaceServiceAccountsOutput{ServiceAccounts: list[start:end], WorkspaceId: aws.String(ws)}
	if end < len(list) {
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	if in.NextToken == nil {
		// A list of the first page counts as a list of the workspace, once it returned what AWS had before.
		f.settle(ws)
	}
	return out, nil
}

// settle counts a list of the workspace toward the changes the list doesn't show yet.
func (f *fakeAPI) settle(ws string) {
	for identifier, n := range f.pending {
		if strings.HasPrefix(identifier, ws+"|") {
			if n <= 1 {
				delete(f.pending, identifier)
			} else {
				f.pending[identifier] = n - 1
			}
		}
	}
	for identifier, g := range f.ghosts {
		if strings.HasPrefix(identifier, ws+"|") {
			if g.reads--; g.reads <= 0 {
				delete(f.ghosts, identifier)
			}
		}
	}
}

func (f *fakeAPI) DeleteWorkspaceServiceAccount(ctx context.Context, in *grafana.DeleteWorkspaceServiceAccountInput, _ ...func(*grafana.Options)) (*grafana.DeleteWorkspaceServiceAccountOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteWorkspaceServiceAccount"); err != nil {
		return nil, err
	}
	if aws.ToString(in.ServiceAccountId) == "" {
		return nil, fault(validationCode, "serviceAccountId is required")
	}
	if err := f.checkWorkspace(in.WorkspaceId); err != nil {
		return nil, err
	}
	ws := aws.ToString(in.WorkspaceId)
	identifier := ws + "|" + aws.ToString(in.ServiceAccountId)
	p := f.store.Resource(serviceAccountType, identifier)
	if p == nil {
		return nil, fault(notFoundCode, "the service account %s doesn't exist", aws.ToString(in.ServiceAccountId))
	}
	err := f.change(func() {
		f.ghosts[identifier] = &ghost{summary: summary(p), reads: settleReads}
		delete(f.pending, identifier)
		f.store.Remove(serviceAccountType, identifier)
		f.busy[ws] = 1
	})
	if err != nil {
		return nil, err
	}
	return &grafana.DeleteWorkspaceServiceAccountOutput{ServiceAccountId: in.ServiceAccountId, WorkspaceId: in.WorkspaceId}, nil
}
