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

package redshift

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	profileReference = "https://docs.aws.amazon.com/redshift/latest/APIReference/"
	profileGuide     = "https://docs.aws.amazon.com/redshift/latest/mgmt/connecting-with-authentication-profiles-using.html"

	// profileLimitsUnclear is what the pages leave unclear about the limits of a name.
	profileLimitsUnclear = "The page gives the name's length and pattern without the error code of a name outside them; AWS " +
		"answered one with ValidationError in a run against real AWS."
)

// profileCall is a call against a fake Redshift API for authentication profiles, after any calls it needs to set up.
type profileCall func(ctx context.Context, f *profileFake) error

// profileAgainst returns calls as a rule's Break or Check, made in order against one fake backed by store. It stops
// at the first call that fails, and returns its error.
func profileAgainst(calls ...profileCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newProfileFake(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

func createProfile(name string, content string) profileCall {
	return func(ctx context.Context, f *profileFake) error {
		_, err := f.CreateAuthenticationProfile(ctx, &redshift.CreateAuthenticationProfileInput{
			AuthenticationProfileName:    aws.String(name),
			AuthenticationProfileContent: aws.String(content),
		})
		return err
	}
}

func modifyProfile(name string, content string) profileCall {
	return func(ctx context.Context, f *profileFake) error {
		_, err := f.ModifyAuthenticationProfile(ctx, &redshift.ModifyAuthenticationProfileInput{
			AuthenticationProfileName:    aws.String(name),
			AuthenticationProfileContent: aws.String(content),
		})
		return err
	}
}

func deleteProfile(name string) profileCall {
	return func(ctx context.Context, f *profileFake) error {
		_, err := f.DeleteAuthenticationProfile(ctx, &redshift.DeleteAuthenticationProfileInput{AuthenticationProfileName: aws.String(name)})
		return err
	}
}

// describeProfile returns a call that reads the profile with the name and fails unless its content is want, or
// unless AWS doesn't list it when want is "".
func describeProfile(name string, want string) profileCall {
	return func(ctx context.Context, f *profileFake) error {
		out, err := f.DescribeAuthenticationProfiles(ctx, &redshift.DescribeAuthenticationProfilesInput{AuthenticationProfileName: aws.String(name)})
		if want == "" {
			if err == nil {
				return fmt.Errorf("AWS still lists the profile %s", name)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if got := aws.ToString(out.AuthenticationProfiles[0].AuthenticationProfileContent); got != want {
			return fmt.Errorf("the profile has content %s, not %s", got, want)
		}
		return nil
	}
}

// profileContentRules are the rules of the content CreateAuthenticationProfile and ModifyAuthenticationProfile take.
// call makes the call with content.
func profileContentRules(call func(content string) []profileCall) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "the content is in JSON format",
			Break: profileAgainst(call("not json")...),
			Code:  profileInvalid,
		},
		{
			Rule:  "the profile store rejects content that holds secrets, such as SecretAccessKey",
			Doc:   profileGuide,
			Break: profileAgainst(call(`{"App_ID":"x","SecretAccessKey":"y"}`)...),
			Code:  profileInvalid,
		},
	}
}

// profileAPI records the Redshift calls AuthenticationProfile makes.
var profileAPI = map[string]nativetest.Call{
	"CreateAuthenticationProfile": {
		Doc: profileReference + "API_CreateAuthenticationProfile.html",
		Rules: append(profileContentRules(func(content string) []profileCall {
			return []profileCall{createProfile("odbc", content)}
		}), []nativetest.Rule{
			{
				Rule:  "the name is at most 63 letters, digits, and hyphens",
				Break: profileAgainst(createProfile("odbc_profile", `{"App_ID":"x"}`)),
				Code:  profileValidation,
			},
			{
				Rule:  "a profile with the name already exists",
				Break: profileAgainst(createProfile("odbc", `{"App_ID":"x"}`), createProfile("odbc", `{"App_ID":"y"}`)),
				Code:  profileExists,
			},
		}...),
		Unclear: profileLimitsUnclear + " The page doesn't say what form AWS stores the content in. In a run against real " +
			"AWS it removed the spaces, turned numbers and booleans into strings with the number's own text, such as " +
			"\"1.50\", dropped null values, kept the order of the keys in one run and changed it in another, and " +
			"rejected an empty object, a nested object or " +
			"list, and content with Password; the fake does the same, and keeps the order. The handler compares the content " +
			"as JSON, in any order. The page names AuthenticationProfileQuotaExceededFault " +
			"without the quota, which the quotas page doesn't list, so the fake has none. The page doesn't say how soon a " +
			"new profile is listed. AWS listed it at once in that run, and so does the fake: the handler takes a profile " +
			"AWS doesn't list after a create as gone, so the deploy fails and the next one creates it again.",
	},
	"ModifyAuthenticationProfile": {
		Doc: profileReference + "API_ModifyAuthenticationProfile.html",
		Rules: append(profileContentRules(func(content string) []profileCall {
			return []profileCall{createProfile("odbc", `{"App_ID":"x"}`), modifyProfile("odbc", content)}
		}), []nativetest.Rule{
			{
				Rule:  "the profile exists",
				Break: profileAgainst(modifyProfile("odbc", `{"App_ID":"x"}`)),
				Code:  profileNotFound,
			},
			{
				Rule: "the new content replaces the whole content",
				Check: profileAgainst(createProfile("odbc", `{"App_ID":"x","Client_ID":"y"}`), modifyProfile("odbc", `{"App_ID":"z"}`),
					describeProfile("odbc", `{"App_ID":"z"}`)),
			},
		}...),
		Unclear: "AWS listed the new content at once in a run against real AWS, and so does the fake, because the handler " +
			"can't tell the old content from the new while it waits.",
	},
	"DeleteAuthenticationProfile": {
		Doc: profileReference + "API_DeleteAuthenticationProfile.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the profile exists",
				Break: profileAgainst(deleteProfile("odbc")),
				Code:  profileNotFound,
			},
		},
		Unclear: "The page doesn't say how soon a deleted profile is gone. AWS no longer listed it in a run against real " +
			"AWS; the fake still lists it for one read.",
	},
	"DescribeAuthenticationProfiles": {
		Doc: profileReference + "API_DescribeAuthenticationProfiles.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "a name AWS has no profile for is not found",
				Break: profileAgainst(describeProfile("odbc", "{}")),
				Code:  profileNotFound,
			},
			{
				Rule: "a call without a name lists every profile of the account",
				Check: profileAgainst(createProfile("odbc", `{"App_ID":"x"}`), createProfile("jdbc", `{"App_ID":"y"}`),
					func(ctx context.Context, f *profileFake) error {
						out, err := f.DescribeAuthenticationProfiles(ctx, &redshift.DescribeAuthenticationProfilesInput{})
						if err != nil {
							return err
						}
						if len(out.AuthenticationProfiles) != 2 {
							return fmt.Errorf("the list has %d profiles", len(out.AuthenticationProfiles))
						}
						return nil
					}),
			},
			{
				Rule: "names are case sensitive",
				Check: profileAgainst(createProfile("Odbc", `{"App_ID":"x"}`), createProfile("odbc", `{"App_ID":"y"}`),
					describeProfile("odbc", `{"App_ID":"y"}`), describeProfile("Odbc", `{"App_ID":"x"}`)),
			},
			{
				Rule:  "AWS stores the content in its own form",
				Check: profileAgainst(createProfile("odbc", `{ "b": 1.50, "a": true, "c": null }`), describeProfile("odbc", `{"b":"1.50","a":"true"}`)),
			},
		},
		Unclear: "The page doesn't say whether names are case sensitive. AWS kept two profiles whose names differed only " +
			"in case in a run against real AWS. The page names InvalidAuthenticationProfileRequestFault without saying when " +
			"AWS returns it for a read. " +
			"AWS answered a run of quick reads with Throttling in a run against real AWS, which the SDK retries.",
	},
}
