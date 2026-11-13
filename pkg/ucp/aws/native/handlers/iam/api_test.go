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

package iam

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/IAM/latest/APIReference/"
	commonErrors = apiReference + "CommonErrors.html"
	nameRules    = "https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html#reference_iam-quotas-names"
)

// addUser adds user to group with a fake IAM API backed by store.
func addUser(ctx context.Context, store *awsfake.CloudControl, group string, user string) error {
	_, err := newFakeAPI(store).AddUserToGroup(ctx, &iam.AddUserToGroupInput{GroupName: aws.String(group), UserName: aws.String(user)})
	return err
}

func removeUser(ctx context.Context, store *awsfake.CloudControl, group string, user string) error {
	_, err := newFakeAPI(store).RemoveUserFromGroup(ctx, &iam.RemoveUserFromGroupInput{GroupName: aws.String(group), UserName: aws.String(user)})
	return err
}

// nameRulesOf returns the rules IAM's API reference gives for the group and user names of a call that takes both.
func nameRulesOf(call func(ctx context.Context, store *awsfake.CloudControl, group string, user string) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "GroupName and UserName are 1 to 128 characters of [\\w+=,.@-]",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "admins", strings.Repeat("a", 129))
			},
			Code: "ValidationError",
			Doc:  commonErrors,
		},
		{
			Rule: "GroupName and UserName have no spaces",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "app admins", "ana")
			},
			Code: "ValidationError",
			Doc:  commonErrors,
		},
		{
			Rule: "the group and the user exist",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "admins", missingPrefix+"-user")
			},
			Code: "NoSuchEntity",
		},
	}
}

// userToGroupAdditionAPI records the IAM calls UserToGroupAddition makes.
var userToGroupAdditionAPI = map[string]nativetest.Call{
	"AddUserToGroup": {
		Doc: apiReference + "API_AddUserToGroup.html",
		Rules: append(nameRulesOf(addUser), nativetest.Rule{
			Rule: "user names aren't distinguished by case, so adding ANA to a group that has ana adds no user",
			Doc:  nameRules,
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				f := newFakeAPI(store)
				if err := addUser(ctx, store, "admins", "ana"); err != nil {
					return err
				}
				if err := addUser(ctx, store, "admins", "ANA"); err != nil {
					return err
				}
				if users := f.users("admins"); len(users) != 1 {
					return fmt.Errorf("the group has users %v", users)
				}
				return nil
			},
		}),
		Unclear: "The page doesn't say what AWS does when the user is already in the group; the fake accepts it and changes " +
			"nothing, as the handler's recovery by resend expects. It lists LimitExceeded without the limit it applies.",
	},
	"GetGroup": {
		Doc: apiReference + "API_GetGroup.html",
		Rules: []nativetest.Rule{
			{
				Rule: "MaxItems is at least 1",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetGroup(ctx, &iam.GetGroupInput{GroupName: aws.String("admins"), MaxItems: aws.Int32(0)})
					return err
				},
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "MaxItems is at most 1000",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetGroup(ctx, &iam.GetGroupInput{GroupName: aws.String("admins"), MaxItems: aws.Int32(1001)})
					return err
				},
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "GroupName is 1 to 128 characters of [\\w+=,.@-]",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetGroup(ctx, &iam.GetGroupInput{GroupName: aws.String("")})
					return err
				},
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "the group exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetGroup(ctx, &iam.GetGroupInput{GroupName: aws.String(missingPrefix + "-group")})
					return err
				},
				Code: "NoSuchEntity",
			},
			{
				Rule: "IAM may return fewer users than MaxItems while there are more, with IsTruncated true and a Marker",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					for _, u := range []string{"ana", "bo", "cy"} {
						if err := addUser(ctx, store, "admins", u); err != nil {
							return err
						}
					}
					out, err := newFakeAPI(store).GetGroup(ctx, &iam.GetGroupInput{GroupName: aws.String("admins"), MaxItems: aws.Int32(100)})
					if err != nil {
						return err
					}
					if !out.IsTruncated || out.Marker == nil || len(out.Users) == 3 {
						return fmt.Errorf("the fake returned every user on one page")
					}
					return nil
				},
			},
		},
	},
	"RemoveUserFromGroup": {
		Doc:   apiReference + "API_RemoveUserFromGroup.html",
		Rules: nameRulesOf(removeUser),
		Unclear: "The page doesn't say which error AWS returns for a user who isn't in the group. The fake returns " +
			"NoSuchEntity, and the handler treats it as already removed.",
	},
}
