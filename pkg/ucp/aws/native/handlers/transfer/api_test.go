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

package transfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/transfer"
	"github.com/aws/aws-sdk-go-v2/service/transfer/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/transfer/latest/APIReference/"
	commonErrors = apiReference + "CommonErrors.html"
	mapEntryDoc  = apiReference + "API_HomeDirectoryMapEntry.html"
	posixDoc     = apiReference + "API_PosixProfile.html"
	targetsGuide = "https://docs.aws.amazon.com/transfer/latest/userguide/logical-dir-mappings.html"

	// externalID is the SID of the directory group the rules use.
	externalID = "S-1-1-12-1234567890-123456789-1234567890-1234"
	role       = "arn:aws:iam::123456789012:role/transfer-access"

	// constraintNote is what every call's page leaves unclear about its constraints.
	constraintNote = " The page names no error for a value outside its length, range, or pattern; the fake returns " +
		"ValidationError, from the Common Errors page."
)

// apiCall is a call against a fake Transfer Family API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake Transfer Family API backed by
// store. It stops at the first call that fails, and returns its error.
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

// createAccess creates the access of externalID to server with role, with the changes edits make to the input.
func createAccess(edits ...func(*transfer.CreateAccessInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &transfer.CreateAccessInput{ServerId: aws.String(server), ExternalId: aws.String(externalID), Role: aws.String(role)}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateAccess(ctx, in)
		return err
	}
}

// updateAccess updates the access of externalID to server with the changes edit makes to the input.
func updateAccess(edit func(*transfer.UpdateAccessInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &transfer.UpdateAccessInput{ServerId: aws.String(server), ExternalId: aws.String(externalID)}
		edit(in)
		_, err := f.UpdateAccess(ctx, in)
		return err
	}
}

// logical sets LOGICAL with one mapping.
func logical(in *transfer.CreateAccessInput) {
	in.HomeDirectoryType = types.HomeDirectoryTypeLogical
	in.HomeDirectoryMappings = []types.HomeDirectoryMapEntry{{Entry: aws.String("/"), Target: aws.String("/bucket/home")}}
}

// describe returns a call that passes the access of externalID to server to check.
func describe(check func(*types.DescribedAccess) error) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		out, err := f.DescribeAccess(ctx, &transfer.DescribeAccessInput{ServerId: aws.String(server), ExternalId: aws.String(externalID)})
		if err != nil {
			return err
		}
		return check(out.Access)
	}
}

// expect returns an error unless got is want.
func expect[T comparable](what string, got T, want T) error {
	if got != want {
		return fmt.Errorf("%s is %v, not %v", what, got, want)
	}
	return nil
}

// accessAPI records the Transfer Family calls Access makes.
var accessAPI = map[string]nativetest.Call{
	"CreateAccess": {
		Doc: apiReference + "API_CreateAccess.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "ServerId is s- and 17 lowercase hex characters",
				Doc:   commonErrors,
				Break: against(createAccess(func(in *transfer.CreateAccessInput) { in.ServerId = aws.String("s-0123") })),
				Code:  validationCode,
			},
			{
				Rule:  "ExternalId matches S-1-[\\d-]+",
				Doc:   commonErrors,
				Break: against(createAccess(func(in *transfer.CreateAccessInput) { in.ExternalId = aws.String("transfer-users") })),
				Code:  validationCode,
			},
			{
				Rule:  "Role is required",
				Doc:   commonErrors,
				Break: against(createAccess(func(in *transfer.CreateAccessInput) { in.Role = nil })),
				Code:  validationCode,
			},
			{
				Rule:  "Role is 20 to 2048 characters matching arn:.*role/\\S+",
				Doc:   commonErrors,
				Break: against(createAccess(func(in *transfer.CreateAccessInput) { in.Role = aws.String("arn:aws:iam::123456789012:user/me") })),
				Code:  validationCode,
			},
			{
				Rule:  "HomeDirectory is empty or a path that starts with /",
				Doc:   commonErrors,
				Break: against(createAccess(func(in *transfer.CreateAccessInput) { in.HomeDirectory = aws.String("bucket/home") })),
				Code:  validationCode,
			},
			{
				Rule:  "Policy is at most 2048 characters",
				Doc:   commonErrors,
				Break: against(createAccess(func(in *transfer.CreateAccessInput) { in.Policy = aws.String(strings.Repeat("a", 2049)) })),
				Code:  validationCode,
			},
			{
				Rule: "HomeDirectoryMappings has at least 1 item",
				Doc:  commonErrors,
				Break: against(createAccess(logical, func(in *transfer.CreateAccessInput) {
					in.HomeDirectoryMappings = []types.HomeDirectoryMapEntry{}
				})),
				Code: validationCode,
			},
			{
				Rule: "a mapping's Entry and Target are paths that start with /",
				Doc:  mapEntryDoc,
				Break: against(createAccess(logical, func(in *transfer.CreateAccessInput) {
					in.HomeDirectoryMappings[0].Target = aws.String("bucket/home")
				})),
				Code: validationCode,
			},
			{
				Rule: "a mapping's Target can't end with /",
				Doc:  targetsGuide,
				Break: against(createAccess(logical, func(in *transfer.CreateAccessInput) {
					in.HomeDirectoryMappings[0].Target = aws.String("/bucket/home/")
				})),
				Code: invalidRequest,
			},
			{
				Rule: "a mapping's Type is FILE or DIRECTORY",
				Doc:  mapEntryDoc,
				Break: against(createAccess(logical, func(in *transfer.CreateAccessInput) {
					in.HomeDirectoryMappings[0].Type = "LINK"
				})),
				Code: validationCode,
			},
			{
				Rule: "a mapping's Type is DIRECTORY when the call leaves it out",
				Doc:  mapEntryDoc,
				Check: against(createAccess(logical), describe(func(a *types.DescribedAccess) error {
					return expect("the mapping's type", a.HomeDirectoryMappings[0].Type, types.MapTypeDirectory)
				})),
			},
			{
				Rule: "HomeDirectoryMappings can be set only when HomeDirectoryType is LOGICAL",
				Break: against(createAccess(logical, func(in *transfer.CreateAccessInput) {
					in.HomeDirectoryType = types.HomeDirectoryTypePath
				})),
				Code: invalidRequest,
			},
			{
				Rule: "HomeDirectoryType LOGICAL needs HomeDirectoryMappings",
				Break: against(createAccess(func(in *transfer.CreateAccessInput) {
					in.HomeDirectoryType = types.HomeDirectoryTypeLogical
				})),
				Code: invalidRequest,
			},
			{
				Rule: "HomeDirectory can be set with HomeDirectoryType LOGICAL",
				Check: against(createAccess(logical, func(in *transfer.CreateAccessInput) {
					in.HomeDirectory = aws.String("/bucket/home")
				})),
			},
			{
				Rule: "HomeDirectoryType is PATH when the call leaves it out",
				Check: against(createAccess(), describe(func(a *types.DescribedAccess) error {
					return expect("HomeDirectoryType", a.HomeDirectoryType, types.HomeDirectoryTypePath)
				})),
			},
			{
				Rule: "a POSIX Uid, Gid, and secondary group ID are 0 to 4294967295",
				Doc:  posixDoc,
				Break: against(createAccess(func(in *transfer.CreateAccessInput) {
					in.PosixProfile = &types.PosixProfile{Uid: aws.Int64(1000), Gid: aws.Int64(4294967296)}
				})),
				Code: validationCode,
			},
			{
				Rule: "a POSIX profile has at most 16 secondary group IDs",
				Doc:  posixDoc,
				Break: against(createAccess(func(in *transfer.CreateAccessInput) {
					in.PosixProfile = &types.PosixProfile{Uid: aws.Int64(1000), Gid: aws.Int64(1000), SecondaryGids: make([]int64, 17)}
				})),
				Code: validationCode,
			},
			{
				Rule:  "the server exists",
				Break: against(createAccess(func(in *transfer.CreateAccessInput) { in.ServerId = aws.String("s-fffffffffffffffff") })),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule:  "the server has no access for the ExternalId",
				Break: against(createAccess(), createAccess()),
				Code:  "ResourceExistsException",
			},
		},
		Unclear: "The page says both that HomeDirectory can be used with LOGICAL and that a template can't have both " +
			"HomeDirectory and HomeDirectoryMappings; the fake takes both, as the first sentence says. It doesn't say " +
			"what AWS does for a server whose identity provider isn't a directory; the fake has only a directory server. " +
			"It doesn't say whether AWS checks the role or the policy beyond their length and pattern; the fake doesn't. " +
			"The user guide says a mapping's target can't end with / without naming the error; the fake returns " +
			"InvalidRequestException. The SDK rejects a call without Role before it reaches AWS, so the handler fails " +
			"a template without Role before it calls. " +
			"The call has no idempotency token, so a create sent again gets ResourceExistsException. The handler reads " +
			"the access before it creates one, and reports that error as an unknown outcome, because an SDK retry of a " +
			"create whose response was lost gets it." + constraintNote,
	},
	"DescribeAccess": {
		Doc: apiReference + "API_DescribeAccess.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the access exists",
				Break: against(describe(func(*types.DescribedAccess) error { return nil })),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule: "the access has the values the create set, as it set them",
				Doc:  apiReference + "API_DescribedAccess.html",
				Check: against(createAccess(logical, func(in *transfer.CreateAccessInput) {
					in.Policy = aws.String(`{"Version":"2012-10-17"}`)
					in.PosixProfile = &types.PosixProfile{Uid: aws.Int64(1000), Gid: aws.Int64(100), SecondaryGids: []int64{4, 5}}
				}), describe(func(a *types.DescribedAccess) error {
					if aws.ToString(a.Policy) != `{"Version":"2012-10-17"}` || aws.ToString(a.HomeDirectoryMappings[0].Target) != "/bucket/home" {
						return fmt.Errorf("the access is %+v", a)
					}
					return expect("the second secondary group ID", a.PosixProfile.SecondaryGids[1], 5)
				})),
			},
		},
		Unclear: "The page lists Role in the response, but another AWS client's code says DescribeAccess doesn't return " +
			"it; the fake leaves it out, and the type makes Role write-only, so the handler never reads it. The page " +
			"doesn't say whether AWS returns an unset HomeDirectory, Policy, HomeDirectoryType, or mapping Type as empty " +
			"or leaves it out, or reformats the policy; the fake leaves them out and returns the policy as the call sent " +
			"it. The handler reads them as empty, PATH, and DIRECTORY, and compares Policy as JSON." + constraintNote,
	},
	"UpdateAccess": {
		Doc: apiReference + "API_UpdateAccess.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the access exists",
				Break: against(updateAccess(func(in *transfer.UpdateAccessInput) { in.Role = aws.String(role) })),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule:  "Role is 20 to 2048 characters matching arn:.*role/\\S+",
				Doc:   commonErrors,
				Break: against(createAccess(), updateAccess(func(in *transfer.UpdateAccessInput) { in.Role = aws.String("role/x") })),
				Code:  validationCode,
			},
			{
				Rule: "HomeDirectoryMappings can be set only when HomeDirectoryType is LOGICAL",
				Break: against(createAccess(), updateAccess(func(in *transfer.UpdateAccessInput) {
					in.HomeDirectoryMappings = []types.HomeDirectoryMapEntry{{Entry: aws.String("/"), Target: aws.String("/bucket")}}
				})),
				Code: invalidRequest,
			},
			{
				Rule: "HomeDirectoryType LOGICAL needs HomeDirectoryMappings",
				Break: against(createAccess(), updateAccess(func(in *transfer.UpdateAccessInput) {
					in.HomeDirectoryType = types.HomeDirectoryTypeLogical
				})),
				Code: invalidRequest,
			},
			{
				Rule: "a mapping's Target can't end with /",
				Doc:  targetsGuide,
				Break: against(createAccess(), updateAccess(func(in *transfer.UpdateAccessInput) {
					in.HomeDirectoryType = types.HomeDirectoryTypeLogical
					in.HomeDirectoryMappings = []types.HomeDirectoryMapEntry{{Entry: aws.String("/"), Target: aws.String("/")}}
				})),
				Code: invalidRequest,
			},
			{
				Rule: "a change to LOGICAL with HomeDirectoryMappings in the same call takes effect",
				Check: against(createAccess(), updateAccess(func(in *transfer.UpdateAccessInput) {
					in.HomeDirectoryType = types.HomeDirectoryTypeLogical
					in.HomeDirectoryMappings = []types.HomeDirectoryMapEntry{{Entry: aws.String("/"), Target: aws.String("/bucket"), Type: types.MapTypeFile}}
				}), describe(func(a *types.DescribedAccess) error {
					return expect("the mapping's type", a.HomeDirectoryMappings[0].Type, types.MapTypeFile)
				})),
			},
			{
				Rule: "HomeDirectory and Policy can be empty",
				Check: against(createAccess(func(in *transfer.CreateAccessInput) {
					in.HomeDirectory, in.Policy = aws.String("/bucket/home"), aws.String("{}")
				}), updateAccess(func(in *transfer.UpdateAccessInput) {
					in.HomeDirectory, in.Policy = aws.String(""), aws.String("")
				}), describe(func(a *types.DescribedAccess) error {
					return expect("HomeDirectory and Policy", aws.ToString(a.HomeDirectory)+aws.ToString(a.Policy), "")
				})),
			},
			{
				Rule: "a POSIX profile has at most 16 secondary group IDs",
				Doc:  posixDoc,
				Break: against(createAccess(), updateAccess(func(in *transfer.UpdateAccessInput) {
					in.PosixProfile = &types.PosixProfile{Uid: aws.Int64(1), Gid: aws.Int64(1), SecondaryGids: make([]int64, 17)}
				})),
				Code: validationCode,
			},
		},
		Unclear: "The page doesn't say what AWS does with a parameter the call leaves out; the fake keeps the access's " +
			"value. It gives HomeDirectory and Policy a minimum length of 0 without saying that an empty value clears " +
			"them; the fake clears them. HomeDirectoryMappings has at least 1 item and PosixProfile needs Uid and Gid, " +
			"so the call has no value that removes either, and the handler fails a deploy that removes one before any " +
			"call. The page doesn't say whether a change to PATH drops the mappings; the fake keeps them." + constraintNote,
	},
	"DeleteAccess": {
		Doc: apiReference + "API_DeleteAccess.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the access exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteAccess(ctx, &transfer.DeleteAccessInput{ServerId: aws.String(server), ExternalId: aws.String(externalID)})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "a deleted access is gone",
				Check: against(createAccess(), func(ctx context.Context, f *fakeAPI) error {
					if _, err := f.DeleteAccess(ctx, &transfer.DeleteAccessInput{ServerId: aws.String(server), ExternalId: aws.String(externalID)}); err != nil {
						return err
					}
					if err := describe(func(*types.DescribedAccess) error { return nil })(ctx, f); err == nil {
						return fmt.Errorf("the access is still there")
					}
					return nil
				}),
			},
		},
		Unclear: "The page doesn't say whether the access is gone when the call returns; the fake removes it in the call." + constraintNote,
	},
}
