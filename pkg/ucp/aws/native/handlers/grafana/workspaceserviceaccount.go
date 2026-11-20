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

// Package grafana has the native handler for Amazon Managed Grafana workspace service accounts, which have no
// CloudFormation type.
package grafana

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/aws/aws-sdk-go-v2/service/grafana/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const serviceAccountType = "AWS::Grafana::WorkspaceServiceAccount"

// maxName is the most characters a service account's name has.
const maxName = 128

// accountTimeout is how long UCP waits for a create or a delete. AWS documents no delay, so this only bounds a wait
// that should be short, such as while the workspace is busy.
const accountTimeout = 15 * time.Minute

// workspaceIDPattern is the documented pattern of a workspace ID.
var workspaceIDPattern = regexp.MustCompile(`^g-[0-9a-f]{10}$`)

// roles are the roles a service account can have.
var roles = []types.Role{types.RoleAdmin, types.RoleEditor, types.RoleViewer}

// API is the part of the Amazon Managed Grafana client the handler uses.
type API interface {
	CreateWorkspaceServiceAccount(ctx context.Context, params *grafana.CreateWorkspaceServiceAccountInput, optFns ...func(*grafana.Options)) (*grafana.CreateWorkspaceServiceAccountOutput, error)
	ListWorkspaceServiceAccounts(ctx context.Context, params *grafana.ListWorkspaceServiceAccountsInput, optFns ...func(*grafana.Options)) (*grafana.ListWorkspaceServiceAccountsOutput, error)
	DeleteWorkspaceServiceAccount(ctx context.Context, params *grafana.DeleteWorkspaceServiceAccountInput, optFns ...func(*grafana.Options)) (*grafana.DeleteWorkspaceServiceAccountOutput, error)
}

var inRegion = awsapi.InRegion[grafana.Options]

var (
	_ native.Handler           = (*WorkspaceServiceAccount)(nil)
	_ native.IdentifierEncoder = (*WorkspaceServiceAccount)(nil)
	_ native.Finder            = (*WorkspaceServiceAccount)(nil)
)

// WorkspaceServiceAccount serves AWS::Grafana::WorkspaceServiceAccount: a service account of an Amazon Managed Grafana
// workspace. Its identifier is WorkspaceId and the Id AWS assigns, joined with "|". AWS has no call that changes a
// service account, so every property is create-only, and UCP rejects a change with 400 before any AWS call. The create
// takes no idempotency token, but AWS keeps a service account's name unique in its workspace, so Find finds a lost
// create by its name. Radius makes no tokens, because AWS returns a token only once.
type WorkspaceServiceAccount struct {
	api API
}

// NewWorkspaceServiceAccount returns the handler for Amazon Managed Grafana workspace service accounts.
func NewWorkspaceServiceAccount(api API) *WorkspaceServiceAccount {
	return &WorkspaceServiceAccount{api: api}
}

// TypeName returns AWS::Grafana::WorkspaceServiceAccount.
func (h *WorkspaceServiceAccount) TypeName() string {
	return serviceAccountType
}

// Options returns how UCP uses the handler. AWS lists service accounts only by workspace, so the type can't be listed.
func (h *WorkspaceServiceAccount) Options() native.Options {
	return native.Options{
		List:          native.ListNone,
		Recovery:      native.RecoverBySearch,
		CreateTimeout: accountTimeout,
		DeleteTimeout: accountTimeout,
	}
}

// invalid returns the client failure of a template AWS would reject, before any AWS call.
func invalid(format string, args ...any) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s: %s", serviceAccountType, fmt.Sprintf(format, args...)),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// account is what a template gives of a service account.
type account struct {
	workspace, name, id string
	role                types.Role
}

// parseAccount checks the properties and returns the service account they give.
func parseAccount(properties map[string]any) (account, error) {
	strs := map[string]string{}
	for _, name := range []string{"WorkspaceId", "Name", "GrafanaRole", "Id"} {
		value, present := properties[name]
		if !present || value == nil {
			continue
		}
		s, isString := value.(string)
		if !isString {
			return account{}, invalid("%s must be a string", name)
		}
		strs[name] = s
	}
	a := account{workspace: strs["WorkspaceId"], name: strs["Name"], role: types.Role(strs["GrafanaRole"]), id: strs["Id"]}
	if _, present := strs["WorkspaceId"]; present && !workspaceIDPattern.MatchString(a.workspace) {
		return account{}, invalid("WorkspaceId must be the ID of a workspace, such as g-0123456789")
	}
	if _, present := strs["Name"]; present && (a.name == "" || utf8.RuneCountInString(a.name) > maxName) {
		return account{}, invalid("Name must be 1 to %d characters", maxName)
	}
	if _, present := strs["GrafanaRole"]; present && !slices.Contains(roles, a.role) {
		return account{}, invalid("GrafanaRole must be one of %v", roles)
	}
	return a, nil
}

// accountID names a service account: its workspace and the ID AWS gave it.
type accountID struct {
	workspace, id string
}

func (id accountID) String() string {
	return id.workspace + "|" + id.id
}

// parseAccountID returns the service account an identifier names. A workspace ID has no "|", so the first one ends it.
func parseAccountID(identifier string) (accountID, error) {
	workspace, id, found := strings.Cut(identifier, "|")
	if !found || id == "" || !workspaceIDPattern.MatchString(workspace) {
		return accountID{}, invalid("%q is not the identifier of a workspace service account: it must be WorkspaceId|Id", identifier)
	}
	return accountID{workspace: workspace, id: id}, nil
}

// Identifier returns WorkspaceId|Id, or "" before AWS assigns the Id. UCP calls it before any AWS call, so a template
// with a value AWS would reject is rejected here.
func (h *WorkspaceServiceAccount) Identifier(properties map[string]any) (string, error) {
	a, err := parseAccount(properties)
	if err != nil || a.workspace == "" || a.id == "" {
		return "", err
	}
	return accountID{workspace: a.workspace, id: a.id}.String(), nil
}

// busy returns the conflict of a change AWS can't make now, because the workspace is changing. UCP answers 409 and
// sends nothing more, so the caller tries again later.
func busy(workspace string, cause error) error {
	return &native.Failure{
		Message: fmt.Sprintf("AWS can't change the service accounts of the Grafana workspace %q now, so nothing was changed. Try again once the workspace is active, "+
			"and give the service account dependsOn on the workspace. If the workspace is active, another service account may have this name, or one that differs "+
			"only in case or punctuation: %v", workspace, cause),
		ClientError: true,
		Conflict:    true,
		Outcome:     native.OutcomeRejected,
	}
}

// callError returns the error of a call: a conflict for AWS's ConflictException, which AWS returns while the workspace
// is changing, ErrNotFound for ResourceNotFoundException, and AWS's error otherwise, as a rejected client failure when
// AWS answered with a 4xx status.
func callError(err error, workspace string) error {
	if awsapi.HasCode(err, "ConflictException") {
		return busy(workspace, err)
	}
	return awsapi.Rejected(awsapi.NotFound(err, "ResourceNotFoundException"))
}

// list calls each with every service account of the workspace, and stops at the first error. It returns ErrNotFound
// when the workspace doesn't exist.
func (h *WorkspaceServiceAccount) list(ctx context.Context, region string, workspace string, each func(s *types.ServiceAccountSummary)) error {
	input := &grafana.ListWorkspaceServiceAccountsInput{WorkspaceId: aws.String(workspace)}
	for {
		page, err := h.api.ListWorkspaceServiceAccounts(ctx, input, inRegion(region))
		if err != nil {
			return callError(err, workspace)
		}
		for i := range page.ServiceAccounts {
			each(&page.ServiceAccounts[i])
		}
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			return nil
		}
	}
}

// find returns the service account with the ID, or ErrNotFound when the workspace doesn't list it.
func (h *WorkspaceServiceAccount) find(ctx context.Context, region string, id accountID) (*types.ServiceAccountSummary, error) {
	var found *types.ServiceAccountSummary
	err := h.list(ctx, region, id.workspace, func(s *types.ServiceAccountSummary) {
		if found == nil && aws.ToString(s.Id) == id.id {
			found = s
		}
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, native.ErrNotFound
	}
	return found, nil
}

// Read returns the service account's properties.
func (h *WorkspaceServiceAccount) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, err := parseAccountID(identifier)
	if err != nil {
		return nil, err
	}
	s, err := h.find(ctx, region, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"WorkspaceId": id.workspace,
		"Id":          id.id,
		"Name":        aws.ToString(s.Name),
		"GrafanaRole": string(s.GrafanaRole),
		"IsDisabled":  aws.ToString(s.IsDisabled),
	}, nil
}

// template returns the service account a create's properties give, all of which it needs.
func template(properties map[string]any) (account, error) {
	a, err := parseAccount(properties)
	if err != nil {
		return account{}, err
	}
	if a.workspace == "" || a.name == "" || a.role == "" {
		return account{}, invalid("set WorkspaceId, Name, and GrafanaRole")
	}
	return a, nil
}

// Create makes the service account.
func (h *WorkspaceServiceAccount) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	a, err := template(request.Properties)
	if err != nil {
		return "", err
	}
	out, err := h.api.CreateWorkspaceServiceAccount(ctx, &grafana.CreateWorkspaceServiceAccountInput{
		WorkspaceId: aws.String(a.workspace),
		Name:        aws.String(a.name),
		GrafanaRole: a.role,
	}, inRegion(request.Region))
	if err != nil {
		err = callError(err, a.workspace)
		if errors.Is(err, native.ErrNotFound) {
			return "", invalid("the Grafana workspace %q doesn't exist", a.workspace)
		}
		return "", err
	}
	if aws.ToString(out.Id) == "" {
		return "", fmt.Errorf("AWS created a service account in the Grafana workspace %q but returned no ID", a.workspace)
	}
	return accountID{workspace: a.workspace, id: aws.ToString(out.Id)}.String(), nil
}

// Find returns the service account of the workspace with the create's name, or "" when there is none. AWS keeps the
// name unique in the workspace, so the one it finds is the lost create's, unless another service account took the
// name since. One with another role can't be the create's, so Find fails instead of taking it over.
func (h *WorkspaceServiceAccount) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	a, err := template(request.Properties)
	if err != nil {
		return "", err
	}
	found := []*types.ServiceAccountSummary{}
	err = h.list(ctx, request.Region, a.workspace, func(s *types.ServiceAccountSummary) {
		if aws.ToString(s.Name) == a.name {
			found = append(found, s)
		}
	})
	switch {
	case errors.Is(err, native.ErrNotFound):
		return "", nil
	case err != nil:
		return "", err
	case len(found) == 0:
		return "", nil
	case len(found) > 1:
		ids := []string{}
		for _, s := range found {
			ids = append(ids, accountID{workspace: a.workspace, id: aws.ToString(s.Id)}.String())
		}
		return "", awsapi.Ambiguous("Grafana workspace service accounts named "+a.name, ids)
	}
	if found[0].GrafanaRole != a.role {
		return "", &native.Failure{
			Message: fmt.Sprintf("The Grafana workspace %q has a service account named %q with the role %s, not the template's %s, so Radius didn't make it. "+
				"Use another name, or delete that service account", a.workspace, a.name, found[0].GrafanaRole, a.role),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return accountID{workspace: a.workspace, id: aws.ToString(found[0].Id)}.String(), nil
}

// Update changes nothing, because every property is create-only.
func (h *WorkspaceServiceAccount) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete deletes the service account, and with it its tokens. It returns ErrNotFound when AWS has no such service
// account or workspace.
func (h *WorkspaceServiceAccount) Delete(ctx context.Context, region string, identifier string) error {
	id, err := parseAccountID(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteWorkspaceServiceAccount(ctx, &grafana.DeleteWorkspaceServiceAccountInput{
		WorkspaceId:      aws.String(id.workspace),
		ServiceAccountId: aws.String(id.id),
	}, inRegion(region))
	return callError(err, id.workspace)
}

// List returns nothing, because AWS lists service accounts only by workspace.
func (h *WorkspaceServiceAccount) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the workspace's list. A create is done once the service account is listed, and fails with
// ErrNotFound when it isn't, which is how UCP learns that a create it lost never reached AWS. A delete is done once
// it isn't listed. An update changes nothing. While the workspace is changing, AWS may not list its service accounts,
// so Progress waits.
func (h *WorkspaceServiceAccount) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	id, err := parseAccountID(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	_, err = h.find(ctx, region, id)
	var failure *native.Failure
	switch {
	case errors.As(err, &failure) && failure.Conflict:
		return native.Progress{State: "workspace busy", Waiting: "workspace active"}, nil
	case err != nil:
		return native.Progress{}, err
	case op == native.OperationCreate:
		return native.Progress{Done: true, State: "listed"}, nil
	}
	return native.Progress{State: "listed", Waiting: "not listed"}, nil
}
