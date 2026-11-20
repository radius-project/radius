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

package sns

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/sns/latest/api/"

	// staleNote is what the record of each call that changes an application says about reads right after it.
	staleNote = "The page doesn't say when a change shows. In live calls ListPlatformApplications showed a change after " +
		"up to 9 lists half a second apart, so the fake lists the application as it was for 10 lists, and the handler " +
		"waits, for up to 20 lists, until a list shows a create or a change. "
)

func doc(operation string) string {
	return apiReference + "API_" + operation + ".html"
}

// tokenAttributes are the attributes of an APNs application with token credentials, which the fake takes.
func tokenAttributes(change func(map[string]string)) map[string]string {
	out := map[string]string{
		"PlatformPrincipal":     "ABC123DEFG",
		"PlatformCredential":    "signing-key",
		"ApplePlatformTeamID":   "ABCDE12345",
		"ApplePlatformBundleID": "com.example.app",
	}
	if change != nil {
		change(out)
	}
	return out
}

// create sends a create of an application named testName on platform with attributes to a fake backed by store.
func create(ctx context.Context, store *awsfake.CloudControl, platform string, attributes map[string]string) error {
	_, err := newFakeAPI(store).CreatePlatformApplication(ctx, &sns.CreatePlatformApplicationInput{Name: aws.String(testName), Platform: aws.String(platform), Attributes: attributes})
	return err
}

// set creates testARN with APNs token credentials in store, and sends a change of its attributes.
func set(ctx context.Context, store *awsfake.CloudControl, attributes map[string]string) error {
	if err := create(ctx, store, "APNS", tokenAttributes(nil)); err != nil {
		return err
	}
	_, err := newFakeAPI(store).SetPlatformApplicationAttributes(ctx, &sns.SetPlatformApplicationAttributesInput{PlatformApplicationArn: aws.String(testARN), Attributes: attributes})
	return err
}

// get lists the attributes of testARN from a new fake backed by store, which has made no change to list as it was.
func get(ctx context.Context, store *awsfake.CloudControl) (map[string]string, error) {
	out, err := newFakeAPI(store).ListPlatformApplications(ctx, &sns.ListPlatformApplicationsInput{})
	if err != nil {
		return nil, err
	}
	for _, app := range out.PlatformApplications {
		if aws.ToString(app.PlatformApplicationArn) == testARN {
			return app.Attributes, nil
		}
	}
	return nil, fmt.Errorf("AWS has no %s", testARN)
}

// missingARN is the ARN of an application the fake doesn't have.
const missingARN = "arn:aws:sns:us-west-2:123456789012:app/GCM/missing"

// invalidAttributeRules are the rules for the attributes of a create or a change, made with call.
func invalidAttributeRules(call func(ctx context.Context, store *awsfake.CloudControl, attributes map[string]string) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "SuccessFeedbackSampleRate is an integer from 0 to 100",
			Doc:  doc("SetPlatformApplicationAttributes"),
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, tokenAttributes(func(a map[string]string) { a[sampleRateAttribute] = "101" }))
			},
			Code: invalidParameterCode,
		},
		{
			Rule: "every attribute is one the page lists",
			Doc:  doc("SetPlatformApplicationAttributes"),
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, tokenAttributes(func(a map[string]string) { a["Bogus"] = "1" }))
			},
			Code: invalidParameterCode,
		},
		{
			Rule: "an event attribute is the ARN of a topic",
			Doc:  doc("SetPlatformApplicationAttributes"),
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, tokenAttributes(func(a map[string]string) { a["EventEndpointCreated"] = "topic" }))
			},
			Code: invalidParameterCode,
		},
	}
}

// platformApplicationAPI records the Amazon SNS calls PlatformApplication makes.
var platformApplicationAPI = map[string]nativetest.Call{
	"CreatePlatformApplication": {
		Doc: doc("CreatePlatformApplication"),
		Rules: append([]nativetest.Rule{
			{
				Rule: "the name has only ASCII letters, numbers, underscores, hyphens, and periods",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).CreatePlatformApplication(ctx, &sns.CreatePlatformApplicationInput{Name: aws.String("my app"), Platform: aws.String("GCM"), Attributes: map[string]string{"PlatformCredential": "key"}})
					return err
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "the platform is one AWS supports",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return create(ctx, store, "FOO", map[string]string{"PlatformCredential": "key", "PlatformPrincipal": "id"})
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "PlatformPrincipal and PlatformCredential are set, except that GCM has no PlatformPrincipal",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return create(ctx, store, "ADM", map[string]string{"PlatformCredential": "secret"})
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "GCM takes PlatformCredential without PlatformPrincipal",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					return create(ctx, store, "GCM", map[string]string{"PlatformCredential": "key"})
				},
			},
			{
				Rule: "the push notification service accepts the credentials",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return create(ctx, store, "GCM", map[string]string{"PlatformCredential": invalidCredential})
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "APNs token credentials set ApplePlatformTeamID and ApplePlatformBundleID with the others",
				Doc:  doc("SetPlatformApplicationAttributes"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return create(ctx, store, "APNS", tokenAttributes(func(a map[string]string) { delete(a, "ApplePlatformBundleID") }))
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "a create of an application AWS has, with other attributes, fails",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := create(ctx, store, "APNS", tokenAttributes(nil)); err != nil {
						return err
					}
					return create(ctx, store, "APNS", tokenAttributes(func(a map[string]string) { a["ApplePlatformBundleID"] = "com.example.other" }))
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "a create of an application AWS has, with the same attributes, returns it",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := create(ctx, store, "APNS", tokenAttributes(nil)); err != nil {
						return err
					}
					return create(ctx, store, "APNS", tokenAttributes(nil))
				},
			},
		}, invalidAttributeRules(func(ctx context.Context, store *awsfake.CloudControl, attributes map[string]string) error {
			return create(ctx, store, "APNS", attributes)
		})...),
		Unclear: "The page lists ADM, APNS, APNS_SANDBOX, and GCM as platforms, and its credential list names Baidu, MPNS, " +
			"and WNS too; AWS created a BAIDU application in a live call, and rejected FOO. The fake takes all seven. It " +
			"doesn't say what a create of a name AWS has does: in live calls AWS returned the same application when every " +
			"attribute matched, and rejected the create with InvalidParameter otherwise, as the fake does. The same name " +
			"on another platform is another application. In a live call AWS ignored Enabled in a create, so the type " +
			"makes it read-only. AWS asks for iam:PassRole on a feedback role, which the fake doesn't check. AWS checks " +
			"the credentials with the push notification service: a bogus GCM key failed with InvalidParameter " +
			"(testdata/errors.json), and the fake rejects one credential value. " + staleNote,
	},
	"SetPlatformApplicationAttributes": {
		Doc: doc("SetPlatformApplicationAttributes"),
		Rules: append([]nativetest.Rule{
			{
				Rule: "the application exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).SetPlatformApplicationAttributes(ctx, &sns.SetPlatformApplicationAttributesInput{PlatformApplicationArn: aws.String(missingARN), Attributes: map[string]string{"EventEndpointCreated": ""}})
					return err
				},
				Code: notFoundCode,
			},
			{
				Rule: "PlatformCredential comes with PlatformPrincipal",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return set(ctx, store, map[string]string{"PlatformCredential": "new-key"})
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "an APNs token change sets every token credential",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return set(ctx, store, map[string]string{"ApplePlatformBundleID": "com.example.other"})
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "an attribute sent as \"\" is cleared",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := set(ctx, store, map[string]string{"EventEndpointCreated": testTopic}); err != nil {
						return err
					}
					if _, err := newFakeAPI(store).SetPlatformApplicationAttributes(ctx, &sns.SetPlatformApplicationAttributesInput{PlatformApplicationArn: aws.String(testARN), Attributes: map[string]string{"EventEndpointCreated": ""}}); err != nil {
						return err
					}
					attributes, err := get(ctx, store)
					if err != nil {
						return err
					}
					if value, ok := attributes["EventEndpointCreated"]; ok {
						return fmt.Errorf("EventEndpointCreated is %q", value)
					}
					return nil
				},
			},
			{
				Rule: "certificate credentials replace token credentials, and clear the team and bundle IDs",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := set(ctx, store, map[string]string{"PlatformPrincipal": "certificate", "PlatformCredential": "private-key"}); err != nil {
						return err
					}
					attributes, err := get(ctx, store)
					if err != nil {
						return err
					}
					if attributes["ApplePlatformTeamID"] != "" || attributes["AuthenticationMethod"] != "Certificate" {
						return fmt.Errorf("the attributes are %v", attributes)
					}
					return nil
				},
			},
		}, invalidAttributeRules(func(ctx context.Context, store *awsfake.CloudControl, attributes map[string]string) error {
			return set(ctx, store, attributes)
		})...),
		Unclear: "The page doesn't say how to clear an attribute. In live calls an attribute sent as \"\" was cleared, " +
			"except SuccessFeedbackSampleRate, which AWS rejected with InvalidParameter, so the handler rejects removing " +
			"it before any call. AWS then returned a cleared feedback role as \"\", and left out a cleared event topic; " +
			"the fake does the same. The page doesn't say which attributes go together. In live calls AWS rejected " +
			"PlatformCredential without PlatformPrincipal, and the reverse, and an APNs team or bundle ID change without " +
			"all four token attributes, with InvalidParameter, so the handler sends every credential the template sets " +
			"with any change to one. Certificate credentials sent to a token application cleared the team and bundle IDs. " +
			"AWS took Enabled, which the page doesn't list; the handler never sends it. " + staleNote,
	},
	"DeletePlatformApplication": {
		Doc: doc("DeletePlatformApplication"),
		Rules: []nativetest.Rule{
			{
				Rule: "the ARN names a platform application",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeletePlatformApplication(ctx, &sns.DeletePlatformApplicationInput{PlatformApplicationArn: aws.String("arn:aws:sns:us-west-2:123456789012:topic")})
					return err
				},
				Code: invalidParameterCode,
			},
		},
		Unclear: "The page doesn't say what a delete of an application AWS doesn't have does. In live calls it succeeded, " +
			"so the handler waits until a read no longer finds the application. It doesn't say what happens to the " +
			"application's endpoints. " + staleNote,
	},
	"ListPlatformApplications": {
		Doc: doc("ListPlatformApplications"),
		Rules: []nativetest.Rule{
			{
				Rule: "a page has up to 100 applications, and NextToken gives the next",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					api := newFakeAPI(store)
					for _, name := range []string{"a", "b", "c"} {
						if _, err := api.CreatePlatformApplication(ctx, &sns.CreatePlatformApplicationInput{Name: aws.String(name), Platform: aws.String("GCM"), Attributes: map[string]string{"PlatformCredential": "key"}}); err != nil {
							return err
						}
					}
					out, err := newFakeAPI(store).ListPlatformApplications(ctx, &sns.ListPlatformApplicationsInput{})
					if err != nil {
						return err
					}
					if out.NextToken == nil {
						return fmt.Errorf("the first page has no NextToken")
					}
					return nil
				},
			},
			{
				Rule: "the attributes don't include the credentials",
				Doc:  doc("GetPlatformApplicationAttributes"),
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := create(ctx, store, "APNS", tokenAttributes(nil)); err != nil {
						return err
					}
					attributes, err := get(ctx, store)
					if err != nil {
						return err
					}
					for _, secret := range secretAttributes {
						if _, ok := attributes[secret]; ok {
							return fmt.Errorf("the attributes include %s", secret)
						}
					}
					return nil
				},
			},
		},
		Unclear: "The handler reads an application with this call, because in live calls GetPlatformApplicationAttributes " +
			"returned the attributes from before a change for about 20 seconds, and this call returned the change at once. " +
			"The page doesn't list the attributes it returns. In live calls it returned the same ones as " +
			"GetPlatformApplicationAttributes: never the credentials, and Enabled, which neither page lists. " +
			"GetPlatformApplicationAttributes's page names AppleCertificateExpiryDate, but AWS returned " +
			"AppleCertificateExpirationDate, which the type uses. The fake's pages have two applications, so the " +
			"handler's paging is tested, and it lists an application as it was for 10 lists after a change.",
	},
}
