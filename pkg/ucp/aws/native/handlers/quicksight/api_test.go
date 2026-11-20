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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/aws/aws-sdk-go-v2/service/quicksight/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/quicksight/latest/APIReference/"
	commonErrors = apiReference + "CommonErrors.html"
	editionDoc   = "https://docs.aws.amazon.com/quicksight/latest/user/editions.html"

	// addNote and removeNote are what the docs leave unclear about when a read shows a change.
	addNote    = " The docs don't say how soon a read shows a new member or alias: the fake shows it at once."
	removeNote = " The docs don't say how soon a read shows a removal: the fake lists the member for two more reads."

	// folderBusyNote is what the docs leave unclear about a folder that is changing.
	folderBusyNote = " The docs don't say whether AWS takes a change of a folder's members while another one is still " +
		"settling. The fake rejects it then, with ConflictException, which the page doesn't list."

	// templateBusyNote is what the docs leave unclear about a template that is changing.
	templateBusyNote = " The docs don't say when AWS answers ConflictException. The fake answers it while the template is " +
		"making a new version."

	// aliasCodeNote is what the alias calls' pages leave unclear about their constraints.
	aliasCodeNote = " The page names no error for a value outside its length, pattern, or range, or for a reserved alias " +
		"name; the fake returns ValidationError, from the Common Errors page."
)

// apiCall is a call against a fake Quick Sight API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake API backed by store. It stops at
// the first call that fails, and returns its error.
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

// addMember returns a call that adds the dashboard dash-1 to the folder sales, with the changes edits make.
func addMember(edits ...func(*quicksight.CreateFolderMembershipInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &quicksight.CreateFolderMembershipInput{
			AwsAccountId: aws.String(account), FolderId: aws.String("sales"), MemberType: types.MemberTypeDashboard, MemberId: aws.String("dash-1"),
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateFolderMembership(ctx, in)
		return err
	}
}

// removeMember returns a call that removes the dashboard dash-1 from the folder sales, with the changes edits make.
func removeMember(edits ...func(*quicksight.DeleteFolderMembershipInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &quicksight.DeleteFolderMembershipInput{
			AwsAccountId: aws.String(account), FolderId: aws.String("sales"), MemberType: types.MemberTypeDashboard, MemberId: aws.String("dash-1"),
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.DeleteFolderMembership(ctx, in)
		return err
	}
}

// membersOf returns every member of the folder, from every page of ListFolderMembers.
func membersOf(ctx context.Context, f *fakeAPI, folder string) ([]types.MemberIdArnPair, error) {
	members := []types.MemberIdArnPair{}
	in := &quicksight.ListFolderMembersInput{AwsAccountId: aws.String(account), FolderId: aws.String(folder)}
	for {
		page, err := f.ListFolderMembers(ctx, in)
		if err != nil {
			return nil, err
		}
		members = append(members, page.FolderMemberList...)
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return members, nil
		}
	}
}

// createAlias returns a call that creates the alias live of the template report, pointing to version 2, with the
// changes edits make.
func createAlias(edits ...func(*quicksight.CreateTemplateAliasInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &quicksight.CreateTemplateAliasInput{
			AwsAccountId: aws.String(account), TemplateId: aws.String("report"), AliasName: aws.String("live"), TemplateVersionNumber: aws.Int64(2),
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateTemplateAlias(ctx, in)
		return err
	}
}

// updateAlias returns a call that points the alias live of the template report to version, with the changes edits
// make.
func updateAlias(version int64, edits ...func(*quicksight.UpdateTemplateAliasInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &quicksight.UpdateTemplateAliasInput{
			AwsAccountId: aws.String(account), TemplateId: aws.String("report"), AliasName: aws.String("live"), TemplateVersionNumber: aws.Int64(version),
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.UpdateTemplateAlias(ctx, in)
		return err
	}
}

// describeAlias returns a call that reads the alias name of the template report, and fails unless it points to
// version, when version isn't 0.
func describeAlias(name string, version int64) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		out, err := f.DescribeTemplateAlias(ctx, &quicksight.DescribeTemplateAliasInput{
			AwsAccountId: aws.String(account), TemplateId: aws.String("report"), AliasName: aws.String(name),
		})
		if err != nil {
			return err
		}
		if version != 0 && aws.ToInt64(out.TemplateAlias.TemplateVersionNumber) != version {
			return fmt.Errorf("the alias points to version %d, not %d", aws.ToInt64(out.TemplateAlias.TemplateVersionNumber), version)
		}
		return nil
	}
}

func standard(ctx context.Context, f *fakeAPI) error {
	f.standard = true
	return nil
}

// membershipAPI is the record of the AWS calls the folder membership handler makes.
var membershipAPI = map[string]nativetest.Call{
	"CreateFolderMembership": {
		Doc: apiReference + "API_CreateFolderMembership.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "AwsAccountId is 12 digits",
				Break: against(addMember(func(in *quicksight.CreateFolderMembershipInput) { in.AwsAccountId = aws.String("1234") })),
				Code:  invalidCode,
			},
			{
				Rule:  "FolderId and MemberId are 1 to 2048 characters matching [\\w\\-]+",
				Break: against(addMember(func(in *quicksight.CreateFolderMembershipInput) { in.MemberId = aws.String("dash 1") })),
				Code:  invalidCode,
			},
			{
				Rule:  "MemberType is DASHBOARD, ANALYSIS, DATASET, DATASOURCE, or TOPIC",
				Break: against(addMember(func(in *quicksight.CreateFolderMembershipInput) { in.MemberType = "REPORT" })),
				Code:  invalidCode,
			},
			{
				Rule:  "a folder that doesn't exist fails with ResourceNotFoundException",
				Break: against(addMember(func(in *quicksight.CreateFolderMembershipInput) { in.FolderId = aws.String("missing") })),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule:  "an asset the folder already has fails with ResourceExistsException",
				Break: against(addMember(), addMember()),
				Code:  "ResourceExistsException",
			},
			{
				Rule:  "an account whose edition doesn't support folders fails with UnsupportedUserEditionException",
				Doc:   editionDoc,
				Break: against(standard, addMember()),
				Code:  "UnsupportedUserEditionException",
			},
			{
				Rule:  "a caller without access to the account fails with AccessDeniedException",
				Break: against(addMember(func(in *quicksight.CreateFolderMembershipInput) { in.AwsAccountId = aws.String(otherAccount) })),
				Code:  "AccessDeniedException",
			},
		},
		Unclear: "Whether an asset that doesn't exist fails, and with which error: the fake fails with ResourceNotFoundException. " +
			"Which limit LimitExceededException is for: the fake has none." + folderBusyNote + addNote,
	},
	"DeleteFolderMembership": {
		Doc: apiReference + "API_DeleteFolderMembership.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "AwsAccountId is 12 digits",
				Break: against(removeMember(func(in *quicksight.DeleteFolderMembershipInput) { in.AwsAccountId = aws.String("1234") })),
				Code:  invalidCode,
			},
			{
				Rule:  "MemberType is DASHBOARD, ANALYSIS, DATASET, DATASOURCE, or TOPIC",
				Break: against(removeMember(func(in *quicksight.DeleteFolderMembershipInput) { in.MemberType = "REPORT" })),
				Code:  invalidCode,
			},
			{
				Rule:  "a folder that doesn't exist fails with ResourceNotFoundException",
				Break: against(removeMember(func(in *quicksight.DeleteFolderMembershipInput) { in.FolderId = aws.String("missing") })),
				Code:  "ResourceNotFoundException",
			},
		},
		Unclear: "What removing an asset the folder doesn't have does: the fake fails with ResourceNotFoundException." +
			folderBusyNote + removeNote,
	},
	"ListFolderMembers": {
		Doc: apiReference + "API_ListFolderMembers.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "a folder that doesn't exist fails with ResourceNotFoundException",
				Break: against(func(ctx context.Context, f *fakeAPI) error { _, err := membersOf(ctx, f, "missing"); return err }),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule: "a NextToken AWS didn't return fails with InvalidNextTokenException",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListFolderMembers(ctx, &quicksight.ListFolderMembersInput{AwsAccountId: aws.String(account), FolderId: aws.String("sales"), NextToken: aws.String("x")})
					return err
				}),
				Code: "InvalidNextTokenException",
			},
			{
				Rule: "MaxResults is 1 to 100",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListFolderMembers(ctx, &quicksight.ListFolderMembersInput{AwsAccountId: aws.String(account), FolderId: aws.String("sales"), MaxResults: aws.Int32(101)})
					return err
				}),
				Code: invalidCode,
			},
			{
				Rule: "each member has its ID and the ARN of the asset, but not its type, across pages",
				Doc:  apiReference + "API_MemberIdArnPair.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					// In the order AWS lists them, as the fake does.
					added := []types.FolderMember{
						{MemberType: types.MemberTypeDashboard, MemberId: aws.String("dash-1")},
						{MemberType: types.MemberTypeDashboard, MemberId: aws.String("dash-2")},
						{MemberType: types.MemberTypeDataset, MemberId: aws.String("dash-1")},
					}
					for _, m := range added {
						if err := addMember(func(in *quicksight.CreateFolderMembershipInput) {
							in.MemberType, in.MemberId = m.MemberType, m.MemberId
						})(ctx, f); err != nil {
							return err
						}
					}
					members, err := membersOf(ctx, f, "sales")
					if err != nil {
						return err
					}
					if len(members) != len(added) {
						return fmt.Errorf("the folder lists %d members, not %d", len(members), len(added))
					}
					for i, m := range members {
						if aws.ToString(m.MemberId) != aws.ToString(added[i].MemberId) || aws.ToString(m.MemberArn) != memberArn(added[i].MemberType, aws.ToString(added[i].MemberId)) {
							return fmt.Errorf("member %d is %q with the ARN %q", i, aws.ToString(m.MemberId), aws.ToString(m.MemberArn))
						}
					}
					return nil
				},
			},
		},
		Unclear: "Whether the list has DATASOURCE and TOPIC members: the page's summary names only dashboards, analyses, and " +
			"datasets, but MemberIdArnPair has no type, and the fake lists every type. The page size: the fake returns two " +
			"members a page." + addNote + removeNote,
	},
}

// aliasAPI is the record of the AWS calls the template alias handler makes.
var aliasAPI = map[string]nativetest.Call{
	"CreateTemplateAlias": {
		Doc: apiReference + "API_CreateTemplateAlias.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "AliasName is 1 to 2048 characters matching [\\w\\-]+, and a name that starts with $ is reserved",
				Break: against(createAlias(func(in *quicksight.CreateTemplateAliasInput) { in.AliasName = aws.String("$LATEST") })),
				Code:  validationCode,
			},
			{
				Rule:  "TemplateId is 1 to 512 characters matching [\\w\\-]+",
				Break: against(createAlias(func(in *quicksight.CreateTemplateAliasInput) { in.TemplateId = aws.String("my report") })),
				Code:  validationCode,
			},
			{
				Rule:  "TemplateVersionNumber is at least 1",
				Break: against(createAlias(func(in *quicksight.CreateTemplateAliasInput) { in.TemplateVersionNumber = aws.Int64(0) })),
				Code:  validationCode,
			},
			{
				Rule:  "a template that doesn't exist fails with ResourceNotFoundException",
				Break: against(createAlias(func(in *quicksight.CreateTemplateAliasInput) { in.TemplateId = aws.String("missing") })),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule:  "an alias the template already has fails with ResourceExistsException",
				Break: against(createAlias(), createAlias()),
				Code:  "ResourceExistsException",
			},
			{
				Rule:  "an account whose edition doesn't support templates fails with UnsupportedUserEditionException",
				Doc:   editionDoc,
				Break: against(standard, createAlias()),
				Code:  "UnsupportedUserEditionException",
			},
			{
				Rule:  "a caller without access to the account fails with AccessDeniedException",
				Doc:   commonErrors,
				Break: against(createAlias(func(in *quicksight.CreateTemplateAliasInput) { in.AwsAccountId = aws.String(otherAccount) })),
				Code:  "AccessDeniedException",
			},
		},
		Unclear: "What a version the template doesn't have does: the fake fails with ResourceNotFoundException. Which limit " +
			"LimitExceededException is for: the fake has none." + templateBusyNote + aliasCodeNote + addNote,
	},
	"UpdateTemplateAlias": {
		Doc: apiReference + "API_UpdateTemplateAlias.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "an alias the template doesn't have fails with ResourceNotFoundException",
				Break: against(updateAlias(3)),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule:  "TemplateVersionNumber is at least 1",
				Break: against(createAlias(), updateAlias(0)),
				Code:  validationCode,
			},
			{
				Rule:  "the alias points to the version the update sets",
				Check: against(createAlias(), updateAlias(3), describeAlias("live", 3)),
			},
		},
		Unclear: "What a version the template doesn't have does: the fake fails with ResourceNotFoundException, the same " +
			"error as for a missing alias. How soon a read shows the new version: the fake shows it at once." +
			templateBusyNote + aliasCodeNote,
	},
	"DescribeTemplateAlias": {
		Doc: apiReference + "API_DescribeTemplateAlias.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "an alias the template doesn't have fails with ResourceNotFoundException",
				Break: against(describeAlias("live", 0)),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule:  "a new alias reads with the version its create set",
				Check: against(createAlias(), describeAlias("live", 2)),
			},
		},
		Unclear: addNote[1:],
	},
}
