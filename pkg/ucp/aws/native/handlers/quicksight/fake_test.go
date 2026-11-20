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
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/aws/aws-sdk-go-v2/service/quicksight/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// account is the fake's caller, and otherAccount another account.
	account      = "123456789012"
	otherAccount = "210987654321"

	region = "us-west-2"

	// pageSize is how many members or aliases a list page has. AWS doesn't document its page size.
	pageSize = 2

	// settleReads is how many reads of a folder still list a member after it is removed. AWS doesn't say how soon a
	// read shows a removal, so the first read after one never does.
	settleReads = 2

	// invalidCode is the error the membership calls list for a value that isn't valid, and validationCode the one the
	// Common Errors page gives, which the fake returns for the alias calls, whose pages list none.
	invalidCode    = "InvalidParameterValueException"
	validationCode = "ValidationError"
)

// folders are the folders of the caller's account in us-west-2.
var folders = []string{"sales", "finance"}

// assets are the assets of the caller's account in us-west-2, by type. dash-1 is also a dataset, because assets of
// different types can have the same ID.
var assets = map[types.MemberType][]string{
	types.MemberTypeDashboard:  {"dash-1", "dash-2"},
	types.MemberTypeAnalysis:   {"analysis-1"},
	types.MemberTypeDataset:    {"dataset-1", "dash-1"},
	types.MemberTypeDatasource: {"source-1"},
	types.MemberTypeTopic:      {"topic-1"},
}

// templates maps each template of the caller's account in us-west-2 to its latest version.
var templates = map[string]int64{"report": 3, "summary": 1}

func memberArn(kind types.MemberType, id string) string {
	return fmt.Sprintf("arn:aws:quicksight:%s:%s:%s/%s", region, account, strings.ToLower(string(kind)), id)
}

func aliasArn(template, alias string) string {
	return fmt.Sprintf("arn:aws:quicksight:%s:%s:template/%s/alias/%s", region, account, template, alias)
}

// pending is a removal of a member that reads don't show yet: reads is how many more still list the member.
type pending struct {
	reads int
}

// fakeAPI is an in-memory Amazon Quick Sight API for folder memberships and template aliases. It keeps them in an
// awsfake store, in their types' property format, so the suite's Cloud Control sees the same resources, and takes
// up any the suite puts in the store directly, even with properties left out. It follows the rules in membershipAPI and
// aliasAPI, and where AWS's docs are silent it takes the pessimistic side:
//
//   - After a member is removed from a folder, the next settleReads reads of the folder still list it. Until then, the
//     folder is still changing, and AWS rejects another change of its members with ConflictException.
//   - A template that is making a new version (building) rejects a change of its aliases with ConflictException.
//   - An asset, a folder, a template, or a template version that doesn't exist fails with ResourceNotFoundException.
//
// A member or an alias reads as soon as the call that adds it returns. UCP takes a create that doesn't read as one AWS
// never got, and sends it again, so a lag there would make every create fail.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu sync.Mutex

	// pending holds the removals reads don't show yet, by identifier.
	pending map[string]*pending

	// building is how many more calls each template stays busy making a new version.
	building map[string]int

	// standard makes every call fail as on Standard edition, which has no folders or templates.
	standard bool

	// noArn makes ListFolderMembers leave out each member's ARN.
	noArn bool

	// calls counts the calls that reached the fake, by method.
	calls map[string]int

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store:    store,
		pending:  map[string]*pending{},
		building: map[string]int{},
		calls:    map[string]int{},
		errs:     map[string]error{},
	}
}

func fault(code string, format string, args ...any) error {
	return &smithy.GenericAPIError{Code: code, Message: fmt.Sprintf(format, args...), Fault: smithy.FaultClient}
}

// begin counts a call, passes time for the templates making a version, and returns the error errs holds for the call,
// or the error every call gets on Standard edition.
func (f *fakeAPI) begin(method string) error {
	f.calls[method]++
	for name, n := range f.building {
		if n <= 1 {
			delete(f.building, name)
		} else {
			f.building[name] = n - 1
		}
	}
	if f.standard {
		return fault("UnsupportedUserEditionException", "this operation isn't supported on Standard edition")
	}
	return f.errs[method]
}

func (f *fakeAPI) change(apply func()) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	apply()
	return f.failer.After()
}

// checkAccount returns ValidationError, or code where the call lists one, for an ID outside the documented pattern,
// and AccessDeniedException for another account than the caller's.
func checkAccount(id *string, code string) error {
	if !accountPattern.MatchString(aws.ToString(id)) {
		return fault(code, "AwsAccountId %q doesn't match the pattern", aws.ToString(id))
	}
	if aws.ToString(id) != account {
		return fault("AccessDeniedException", "the caller can't use the account %s", aws.ToString(id))
	}
	return nil
}

// listed reports whether a read of the folder lists the member with the identifier, and settles its removal.
func (f *fakeAPI) listed(id string) bool {
	if f.store.Resource(folderMembershipType, id) == nil {
		return false
	}
	if p, ok := f.pending[id]; ok {
		if p.reads--; p.reads <= 0 {
			delete(f.pending, id)
			f.store.Remove(folderMembershipType, id)
		}
	}
	return true
}

// changing reports whether a member of the folder prefix names has a removal reads don't show yet.
func (f *fakeAPI) changing(prefix string) bool {
	for id := range f.pending {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

// membership checks a membership call's parameters, the way AWS documents them, and returns its identifier.
func (f *fakeAPI) membership(accountID, folder *string, kind types.MemberType, member *string) (string, error) {
	if err := checkAccount(accountID, invalidCode); err != nil {
		return "", err
	}
	for name, id := range map[string]string{"FolderId": aws.ToString(folder), "MemberId": aws.ToString(member)} {
		if !validID(id, maxIDLength) {
			return "", fault(invalidCode, "%s %q doesn't match the pattern", name, id)
		}
	}
	if _, known := memberKind(string(kind)); !known {
		return "", fault(invalidCode, "MemberType %q isn't a valid value", kind)
	}
	if !slices.Contains(folders, aws.ToString(folder)) {
		return "", fault("ResourceNotFoundException", "the folder %q doesn't exist", aws.ToString(folder))
	}
	if !slices.Contains(assets[kind], aws.ToString(member)) {
		return "", fault("ResourceNotFoundException", "the %s %q doesn't exist", kind, aws.ToString(member))
	}
	return membershipID{account: account, folder: aws.ToString(folder), kind: kind, member: aws.ToString(member)}.String(), nil
}

func (f *fakeAPI) CreateFolderMembership(ctx context.Context, in *quicksight.CreateFolderMembershipInput, _ ...func(*quicksight.Options)) (*quicksight.CreateFolderMembershipOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateFolderMembership"); err != nil {
		return nil, err
	}
	id, err := f.membership(in.AwsAccountId, in.FolderId, in.MemberType, in.MemberId)
	if err != nil {
		return nil, err
	}
	if f.changing(account + "|" + aws.ToString(in.FolderId) + "|") {
		return nil, fault("ConflictException", "the folder %q is still changing", aws.ToString(in.FolderId))
	}
	if f.store.Resource(folderMembershipType, id) != nil {
		return nil, fault("ResourceExistsException", "the folder already has the %s %q", in.MemberType, aws.ToString(in.MemberId))
	}
	err = f.change(func() {
		f.store.Put(folderMembershipType, id, map[string]any{
			"AwsAccountId": account, "FolderId": aws.ToString(in.FolderId), "MemberType": string(in.MemberType), "MemberId": aws.ToString(in.MemberId),
		})
	})
	if err != nil {
		return nil, err
	}
	return &quicksight.CreateFolderMembershipOutput{FolderMember: &types.FolderMember{MemberId: in.MemberId, MemberType: in.MemberType}, Status: 200}, nil
}

func (f *fakeAPI) DeleteFolderMembership(ctx context.Context, in *quicksight.DeleteFolderMembershipInput, _ ...func(*quicksight.Options)) (*quicksight.DeleteFolderMembershipOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteFolderMembership"); err != nil {
		return nil, err
	}
	id, err := f.membership(in.AwsAccountId, in.FolderId, in.MemberType, in.MemberId)
	if err != nil {
		return nil, err
	}
	if f.changing(account + "|" + aws.ToString(in.FolderId) + "|") {
		return nil, fault("ConflictException", "the folder %q is still changing", aws.ToString(in.FolderId))
	}
	if f.store.Resource(folderMembershipType, id) == nil {
		return nil, fault("ResourceNotFoundException", "the folder doesn't have the %s %q", in.MemberType, aws.ToString(in.MemberId))
	}
	if err := f.change(func() { f.pending[id] = &pending{reads: settleReads} }); err != nil {
		return nil, err
	}
	return &quicksight.DeleteFolderMembershipOutput{Status: 200}, nil
}

func (f *fakeAPI) ListFolderMembers(ctx context.Context, in *quicksight.ListFolderMembersInput, _ ...func(*quicksight.Options)) (*quicksight.ListFolderMembersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListFolderMembers"); err != nil {
		return nil, err
	}
	if err := checkAccount(in.AwsAccountId, invalidCode); err != nil {
		return nil, err
	}
	folder := aws.ToString(in.FolderId)
	if !validID(folder, maxIDLength) {
		return nil, fault(invalidCode, "FolderId %q doesn't match the pattern", folder)
	}
	if in.MaxResults != nil && (*in.MaxResults < 1 || *in.MaxResults > 100) {
		return nil, fault(invalidCode, "MaxResults must be 1 to 100")
	}
	if !slices.Contains(folders, folder) {
		return nil, fault("ResourceNotFoundException", "the folder %q doesn't exist", folder)
	}
	members := []types.MemberIdArnPair{}
	for _, id := range f.store.List(folderMembershipType) {
		parts := strings.Split(id, "|")
		if len(parts) != 4 || parts[0] != account || parts[1] != folder || !f.listed(id) {
			continue
		}
		m := types.MemberIdArnPair{MemberId: aws.String(parts[3])}
		if !f.noArn {
			m.MemberArn = aws.String(memberArn(types.MemberType(parts[2]), parts[3]))
		}
		members = append(members, m)
	}
	items, next, err := page(members, in.NextToken)
	if err != nil {
		return nil, err
	}
	return &quicksight.ListFolderMembersOutput{FolderMemberList: items, NextToken: next}, nil
}

// page returns the items of the page that token starts, and the next page's token, or nil on the last page. A token
// the fake didn't return fails with InvalidNextTokenException.
func page[T any](items []T, token *string) ([]T, *string, error) {
	start := 0
	if token != nil {
		var err error
		if start, err = strconv.Atoi(*token); err != nil || start < 0 || start > len(items) {
			return nil, nil, fault("InvalidNextTokenException", "the NextToken value isn't valid")
		}
	}
	end := min(start+pageSize, len(items))
	var next *string
	if end < len(items) {
		next = aws.String(strconv.Itoa(end))
	}
	return items[start:end], next, nil
}

// alias checks an alias call's parameters, the way AWS documents them, and returns its identifier. A name that starts
// with $ is one of AWS's own.
func (f *fakeAPI) alias(accountID, template, name *string) (string, error) {
	if err := checkAccount(accountID, validationCode); err != nil {
		return "", err
	}
	if !validID(aws.ToString(template), maxTemplateIDLength) {
		return "", fault(validationCode, "TemplateId %q doesn't match the pattern", aws.ToString(template))
	}
	if n := aws.ToString(name); !validID(n, maxAliasLength) {
		if n == "$LATEST" || n == "$PUBLISHED" {
			return "", fault(validationCode, "the alias name %q is reserved", n)
		}
		return "", fault(validationCode, "AliasName %q doesn't match the pattern", n)
	}
	if _, ok := templates[aws.ToString(template)]; !ok {
		return "", fault("ResourceNotFoundException", "the template %q doesn't exist", aws.ToString(template))
	}
	return aliasID{account: account, template: aws.ToString(template), alias: aws.ToString(name)}.String(), nil
}

// pointable returns an error unless the template has the version and isn't making a new one, so an alias can point
// to it.
func (f *fakeAPI) pointable(template string, version *int64) error {
	if version == nil || *version < 1 {
		return fault(validationCode, "TemplateVersionNumber must be at least 1")
	}
	if f.building[template] > 0 {
		return fault("ConflictException", "the template %q is making a new version", template)
	}
	if *version > templates[template] {
		return fault("ResourceNotFoundException", "the template %q has no version %d", template, *version)
	}
	return nil
}

// templateAlias returns the alias as AWS returns it. A record the suite put in the store may have no version.
func (f *fakeAPI) templateAlias(id string) *types.TemplateAlias {
	parts := strings.Split(id, "|")
	out := &types.TemplateAlias{AliasName: aws.String(parts[2]), Arn: aws.String(aliasArn(parts[1], parts[2]))}
	if v, ok := f.store.Resource(templateAliasType, id)["TemplateVersionNumber"].(float64); ok {
		out.TemplateVersionNumber = aws.Int64(int64(v))
	}
	return out
}

func (f *fakeAPI) CreateTemplateAlias(ctx context.Context, in *quicksight.CreateTemplateAliasInput, _ ...func(*quicksight.Options)) (*quicksight.CreateTemplateAliasOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateTemplateAlias"); err != nil {
		return nil, err
	}
	id, err := f.alias(in.AwsAccountId, in.TemplateId, in.AliasName)
	if err != nil {
		return nil, err
	}
	if err := f.pointable(aws.ToString(in.TemplateId), in.TemplateVersionNumber); err != nil {
		return nil, err
	}
	if f.store.Resource(templateAliasType, id) != nil {
		return nil, fault("ResourceExistsException", "the template already has the alias %q", aws.ToString(in.AliasName))
	}
	err = f.change(func() {
		f.store.Put(templateAliasType, id, map[string]any{
			"AwsAccountId": account, "TemplateId": aws.ToString(in.TemplateId), "AliasName": aws.ToString(in.AliasName),
			"TemplateVersionNumber": float64(*in.TemplateVersionNumber),
		})
	})
	if err != nil {
		return nil, err
	}
	return &quicksight.CreateTemplateAliasOutput{TemplateAlias: f.templateAlias(id), Status: 200}, nil
}

func (f *fakeAPI) UpdateTemplateAlias(ctx context.Context, in *quicksight.UpdateTemplateAliasInput, _ ...func(*quicksight.Options)) (*quicksight.UpdateTemplateAliasOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("UpdateTemplateAlias"); err != nil {
		return nil, err
	}
	id, err := f.alias(in.AwsAccountId, in.TemplateId, in.AliasName)
	if err != nil {
		return nil, err
	}
	if err := f.pointable(aws.ToString(in.TemplateId), in.TemplateVersionNumber); err != nil {
		return nil, err
	}
	if f.store.Resource(templateAliasType, id) == nil {
		return nil, fault("ResourceNotFoundException", "the template has no alias %q", aws.ToString(in.AliasName))
	}
	err = f.change(func() {
		properties := f.store.Resource(templateAliasType, id)
		properties["TemplateVersionNumber"] = float64(*in.TemplateVersionNumber)
		f.store.Put(templateAliasType, id, properties)
	})
	if err != nil {
		return nil, err
	}
	return &quicksight.UpdateTemplateAliasOutput{TemplateAlias: f.templateAlias(id), Status: 200}, nil
}

func (f *fakeAPI) DescribeTemplateAlias(ctx context.Context, in *quicksight.DescribeTemplateAliasInput, _ ...func(*quicksight.Options)) (*quicksight.DescribeTemplateAliasOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeTemplateAlias"); err != nil {
		return nil, err
	}
	id, err := f.alias(in.AwsAccountId, in.TemplateId, in.AliasName)
	if err != nil {
		return nil, err
	}
	if f.store.Resource(templateAliasType, id) == nil {
		return nil, fault("ResourceNotFoundException", "the template has no alias %q", aws.ToString(in.AliasName))
	}
	return &quicksight.DescribeTemplateAliasOutput{TemplateAlias: f.templateAlias(id), Status: 200}, nil
}
