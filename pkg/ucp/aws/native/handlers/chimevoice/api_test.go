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

package chimevoice

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/chime-sdk/latest/APIReference/"

	// liveNote starts each rule the API reference doesn't state, which AWS applied in live calls. testdata/errors.json
	// has AWS's recorded answers.
	liveNote = "Not on the page; AWS's live answer, recorded in testdata/errors.json: "

	// idNote is what the record of each call says about the Voice Connector ID.
	idNote = "The page gives the ID's pattern as .*\\S.*, but AWS answers an ID that isn't 21 or 22 lowercase letters and " +
		"digits, or a UUID, with ValidationException, which the page doesn't list and the SDK doesn't model, so its " +
		"fault is unknown; testdata/errors.json has AWS's answer, and the fake returns the same code. "
)

func doc(operation string) string {
	return apiReference + "API_voice-chime_" + operation + ".html"
}

// credentialDoc is the page of the Credential data type.
var credentialDoc = doc("Credential")

// sdkCredential returns a credential of the SDK.
func sdkCredential(username string, password string) types.Credential {
	return types.Credential{Username: aws.String(username), Password: aws.String(password)}
}

// putCredentials sends a Put of credentials for the Voice Connector vc to api.
func putCredentials(ctx context.Context, api *fakeAPI, vc string, credentials ...types.Credential) error {
	_, err := api.PutVoiceConnectorTerminationCredentials(ctx, &chimesdkvoice.PutVoiceConnectorTerminationCredentialsInput{VoiceConnectorId: aws.String(vc), Credentials: credentials})
	return err
}

// deleteCredentials sends a delete of the usernames for the Voice Connector vc to api.
func deleteCredentials(ctx context.Context, api *fakeAPI, vc string, usernames ...string) error {
	_, err := api.DeleteVoiceConnectorTerminationCredentials(ctx, &chimesdkvoice.DeleteVoiceConnectorTerminationCredentialsInput{VoiceConnectorId: aws.String(vc), Usernames: usernames})
	return err
}

// listed returns the usernames api lists for the Voice Connector vc.
func listed(ctx context.Context, api *fakeAPI, vc string) ([]string, error) {
	out, err := api.ListVoiceConnectorTerminationCredentials(ctx, &chimesdkvoice.ListVoiceConnectorTerminationCredentialsInput{VoiceConnectorId: aws.String(vc)})
	if err != nil {
		return nil, err
	}
	return out.Usernames, nil
}

// requireListed returns an error unless api lists want, sorted, for the Voice Connector vc, once the lists that
// return the earlier usernames are over. It returns an error when the first list already shows want, because a list
// right after a change returns the earlier usernames.
func requireListed(ctx context.Context, api *fakeAPI, vc string, want ...string) error {
	for i := range staleReads + 1 {
		got, err := listed(ctx, api, vc)
		if err != nil {
			return err
		}
		if i < staleReads && slices.Equal(got, want) {
			return fmt.Errorf("list %d right after the change already shows %v", i, want)
		}
		if i == staleReads && !slices.Equal(got, want) {
			return fmt.Errorf("AWS lists %v, not %v", got, want)
		}
	}
	return nil
}

// voiceConnectorRule is the rule of every call: AWS has the Voice Connector.
func voiceConnectorRule(call func(ctx context.Context, api *fakeAPI) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "the Voice Connector exists",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return call(ctx, newFakeAPI(store))
		},
		Code: notFoundCode,
	}
}

// credentialsAPI records the Amazon Chime SDK Voice calls TerminationCredentials makes.
var credentialsAPI = map[string]nativetest.Call{
	"ListVoiceConnectorTerminationCredentials": {
		Doc: doc("ListVoiceConnectorTerminationCredentials"),
		Rules: []nativetest.Rule{
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				_, err := listed(ctx, api, missingVC)
				return err
			}),
			{
				Rule: "it returns the usernames only, never a password, and none for a Voice Connector without credentials",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					api := newFakeAPI(store)
					if got, err := listed(ctx, api, testVC); err != nil || len(got) > 0 {
						return fmt.Errorf("a Voice Connector without credentials lists %v, %v", got, err)
					}
					if err := putCredentials(ctx, api, testVC, sdkCredential("ana", "secret-1")); err != nil {
						return err
					}
					return requireListed(ctx, api, testVC, "ana")
				},
			},
		},
		Unclear: idNote + "The page doesn't say when a change shows. In live calls, a list right after a Put or a delete " +
			"returned the earlier usernames for about a second; the fake returns them for two lists. The page doesn't say " +
			"in what order it lists usernames; AWS listed them sorted, and so does the fake.",
	},
	"PutVoiceConnectorTerminationCredentials": {
		Doc: doc("PutVoiceConnectorTerminationCredentials"),
		Rules: []nativetest.Rule{
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				return putCredentials(ctx, api, missingVC, sdkCredential("ana", "secret-1"))
			}),
			{
				Rule: liveNote + "the Voice Connector has termination settings",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putCredentials(ctx, newFakeAPI(store), noTerminationVC, sdkCredential("ana", "secret-1"))
				},
				Code: badRequestCode,
			},
			{
				Rule: liveNote + "the call sets at least one credential",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putCredentials(ctx, newFakeAPI(store), testVC, []types.Credential{}...)
				},
				Code: badRequestCode,
			},
			{
				Rule: "each credential has a username and a password. The Credential page marks neither required, but AWS's " +
					"live answer, recorded in testdata/errors.json, rejects a credential without either",
				Doc: credentialDoc,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putCredentials(ctx, newFakeAPI(store), testVC, types.Credential{Username: aws.String("ana")})
				},
				Code: badRequestCode,
			},
			{
				Rule: liveNote + "a Voice Connector has at most 10 credentials, counting each new username of the call",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					list := []types.Credential{}
					for i := range maxCredentials + 1 {
						list = append(list, sdkCredential(fmt.Sprintf("user%d", i), "secret-1"))
					}
					return putCredentials(ctx, newFakeAPI(store), testVC, list...)
				},
				Code: badRequestCode,
			},
			{
				Rule: "the page says it updates the credentials. In live calls, a Put added the usernames the Voice Connector " +
					"didn't have, set the password of those it had, and kept the others",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					api := newFakeAPI(store)
					if err := putCredentials(ctx, api, testVC, sdkCredential("ana", "secret-1")); err != nil {
						return err
					}
					if err := putCredentials(ctx, api, testVC, sdkCredential("bo", "secret-2"), sdkCredential("ana", "secret-3")); err != nil {
						return err
					}
					if got := api.password(testVC, "ana"); got != "secret-3" {
						return fmt.Errorf("ana's password is %q, not the second Put's", got)
					}
					return requireListed(ctx, api, testVC, "ana", "bo")
				},
			},
			{
				Rule: "usernames are RFC 2617 user names, which are case sensitive. In live calls, User1 and user1 were two " +
					"credentials",
				Doc: credentialDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					api := newFakeAPI(store)
					if err := putCredentials(ctx, api, testVC, sdkCredential("User1", "secret-1"), sdkCredential("user1", "secret-2")); err != nil {
						return err
					}
					return requireListed(ctx, api, testVC, "User1", "user1")
				},
			},
		},
		Unclear: idNote + "The page lists BadRequestException without saying which values it rejects; the rules above are " +
			"AWS's live answers. The Credential page asks for US-ASCII, but AWS took a username with é and 300-character " +
			"usernames and passwords; the fake checks no characters or lengths. A Put that listed one username twice " +
			"succeeded and kept it once, without saying which password it kept, and counted both against the limit of " +
			"10; the handler rejects a template that lists a username twice, before any call, and the fake counts both. " +
			"The page doesn't say whether a Put AWS rejects applies any of its credentials; the fake applies none. The " +
			"page doesn't say in what order AWS checks the call; the fake checks the ID, the Voice Connector, the " +
			"credentials, the termination settings, then the limit. AWS never returns a password, so a password changed " +
			"outside Radius can't be seen.",
	},
	"DeleteVoiceConnectorTerminationCredentials": {
		Doc: doc("DeleteVoiceConnectorTerminationCredentials"),
		Rules: []nativetest.Rule{
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				return deleteCredentials(ctx, api, missingVC, "ana")
			}),
			{
				Rule: liveNote + "the call names at least one username",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return deleteCredentials(ctx, newFakeAPI(store), testVC, []string{}...)
				},
				Code: badRequestCode,
			},
			{
				Rule: "the page doesn't say what it does with a username the Voice Connector doesn't have. In live calls, AWS " +
					"deleted the others without an error",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					api := newFakeAPI(store)
					if err := putCredentials(ctx, api, testVC, sdkCredential("ana", "secret-1"), sdkCredential("bo", "secret-2")); err != nil {
						return err
					}
					if err := deleteCredentials(ctx, api, testVC, "ana", "nobody"); err != nil {
						return err
					}
					return requireListed(ctx, api, testVC, "bo")
				},
			},
		},
		Unclear: idNote + "The SDK rejects a call without Usernames before it sends it, and so does the fake. In live calls, a " +
			"Voice Connector kept its credentials after its termination settings were deleted, and a delete of them then " +
			"succeeded; the fake doesn't model termination settings beyond the Put's rule.",
	},
}
