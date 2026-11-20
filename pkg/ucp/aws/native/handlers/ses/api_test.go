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

package ses

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	v2Reference = "https://docs.aws.amazon.com/ses/latest/APIReference-V2/"
	v1Reference = "https://docs.aws.amazon.com/ses/latest/APIReference/"
	guide       = "https://docs.aws.amazon.com/ses/latest/dg/"

	// policyGuide is the page of SES's developer guide that states the limits of a sending authorization policy.
	policyGuide = guide + "policy-anatomy.html"

	// rateNote is what the record of each call that SES takes at most once a second says.
	rateNote = "The page says SES takes the call at most once a second; the SDK's retryer sends a throttled call " +
		"again, and the fake doesn't throttle. "

	// staleNote is what the record of each read says about reads right after a change.
	staleNote = "The page doesn't say when a change shows in a read. Live reads showed it at once, but SES's guide says " +
		"notification changes can take minutes, so the fake returns the earlier value for two reads after a change, " +
		"and the handlers read until a read shows the change. "

	// verifiedNote is what the record of each call on an identity says about verification.
	verifiedNote = "The page says the call is only for a verified identity, but live calls on an unverified email " +
		"identity succeeded; the fake takes any identity SES has. "

	// v1ErrorNote is what the record of each SES v1 call says about its errors.
	v1ErrorNote = "The page names no errors. Live calls got InvalidParameterValue for an identity SES doesn't have and " +
		"for a topic SNS doesn't have, and ValidationError for a notification type SES doesn't take; the SDK doesn't " +
		"model them, so their fault is unknown, and testdata/errors.json has AWS's recorded answers. The fake returns " +
		"the same codes. "
)

func v2Doc(page string) string {
	return v2Reference + "API_" + page + ".html"
}

func v1Doc(page string) string {
	return v1Reference + "API_" + page + ".html"
}

// testPolicy returns a policy that lets the account send email from identity, with actions or ses:SendEmail, in the
// form the fake stores it.
func testPolicy(identity string, actions ...string) string {
	var action any = "ses:SendEmail"
	if len(actions) > 0 {
		action = actions
	}
	b, _ := json.Marshal(map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{map[string]any{
			"Effect":    "Allow",
			"Principal": map[string]any{"AWS": "arn:aws:iam::" + account + ":root"},
			"Action":    action,
			"Resource":  identityARN(identity),
		}},
	})
	return string(b)
}

// createPolicy creates a policy named name for identity in a fake backed by store.
func createPolicy(ctx context.Context, store *awsfake.CloudControl, identity string, name string, policy string) error {
	_, err := newFakeAPI(store).CreateEmailIdentityPolicy(ctx, &sesv2.CreateEmailIdentityPolicyInput{EmailIdentity: aws.String(identity), PolicyName: aws.String(name), Policy: aws.String(policy)})
	return err
}

// updatePolicy updates the policy named name of identity in a fake backed by store.
func updatePolicy(ctx context.Context, store *awsfake.CloudControl, identity string, name string, policy string) error {
	_, err := newFakeAPI(store).UpdateEmailIdentityPolicy(ctx, &sesv2.UpdateEmailIdentityPolicyInput{EmailIdentity: aws.String(identity), PolicyName: aws.String(name), Policy: aws.String(policy)})
	return err
}

// policyRules are the rules the create and update calls share. call sends the policy for testIdentity.
func policyRules(call func(ctx context.Context, store *awsfake.CloudControl, name string, policy string) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "PolicyName has at most 64 characters",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, strings.Repeat("a", 65), testPolicy(testIdentity))
			},
			Code: badRequestCode,
		},
		{
			Rule: "PolicyName has only letters, digits, dashes, and underscores",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "bad.name", testPolicy(testIdentity))
			},
			Code: badRequestCode,
		},
		{
			Rule: "Policy is JSON",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "p1", "not json")
			},
			Code: badRequestCode,
		},
		{
			Rule: "Policy is at most 4 KB",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "p1", `{"Id":"`+strings.Repeat("x", maxPolicyBytes)+`","Statement":[]}`)
			},
			Code: badRequestCode,
		},
		{
			Rule: "every statement has an Effect of Allow or Deny, at least one principal, an action, and a resource",
			Doc:  policyGuide,
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				for _, field := range []string{"Effect", "Principal", "Action", "Resource"} {
					var doc map[string]any
					_ = json.Unmarshal([]byte(testPolicy(testIdentity)), &doc)
					delete(doc["Statement"].([]any)[0].(map[string]any), field)
					b, _ := json.Marshal(doc)
					if err := call(ctx, store, "p1", string(b)); err == nil {
						return fmt.Errorf("the fake took a statement without %s", field)
					} else if !strings.Contains(err.Error(), badRequestCode) {
						return err
					}
				}
				for _, principal := range []string{`{}`, `{"AWS":[]}`} {
					policy := strings.Replace(testPolicy(testIdentity), `{"AWS":"arn:aws:iam::`+account+`:root"}`, principal, 1)
					if err := call(ctx, store, "p1", policy); err == nil {
						return fmt.Errorf("the fake took the principal %s", principal)
					}
				}
				return call(ctx, store, "p1", `{"Statement":[{}]}`)
			},
			Code: badRequestCode,
		},
		{
			Rule: "the policy's resource is the ARN of the identity the policy is for",
			Doc:  policyGuide,
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "p1", testPolicy("other@example.com"))
			},
			Code: badRequestCode,
		},
	}
}

// identityPolicyAPI records the SES v2 calls EmailIdentityPolicy makes.
var identityPolicyAPI = map[string]nativetest.Call{
	"CreateEmailIdentityPolicy": {
		Doc: v2Doc("CreateEmailIdentityPolicy"),
		Rules: append(policyRules(func(ctx context.Context, store *awsfake.CloudControl, name string, policy string) error {
			return createPolicy(ctx, store, testIdentity, name, policy)
		}),
			nativetest.Rule{
				Rule: "the identity exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return createPolicy(ctx, store, missingIdentity, "p1", testPolicy(missingIdentity))
				},
				Code: notFoundCode,
			},
			nativetest.Rule{
				Rule: "the identity has no policy with the name",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createPolicy(ctx, store, testIdentity, "p1", testPolicy(testIdentity)); err != nil {
						return err
					}
					return createPolicy(ctx, store, testIdentity, "p1", testPolicy(testIdentity))
				},
				Code: alreadyExistsCode,
			},
			nativetest.Rule{
				Rule: "an identity has at most 20 policies",
				Doc:  policyGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					for i := range maxPolicies + 1 {
						if err := createPolicy(ctx, store, testIdentity, fmt.Sprintf("p%d", i), testPolicy(testIdentity)); err != nil {
							return err
						}
					}
					return nil
				},
				Code: limitCode,
			},
		),
		Unclear: verifiedNote + rateNote + "SES stores a policy in its own form, which the page doesn't describe: in " +
			"live calls it removed whitespace, wrote a list of one value as the value, reordered lists, wrote an account " +
			"ID in a principal as the account's root ARN, and added Version 2008-10-17 to a policy without one. The fake " +
			"stores the same form, except for the order of keys and lists, and the handler counts a policy in either form " +
			"as the same. " +
			"Live calls also got BadRequestException for an action of another service, and for a statement whose principal " +
			"lists none, which the fake returns too.",
	},
	"UpdateEmailIdentityPolicy": {
		Doc: v2Doc("UpdateEmailIdentityPolicy"),
		Rules: append(policyRules(func(ctx context.Context, store *awsfake.CloudControl, name string, policy string) error {
			_ = createPolicy(ctx, store, testIdentity, name, testPolicy(testIdentity))
			return updatePolicy(ctx, store, testIdentity, name, policy)
		}),
			nativetest.Rule{
				Rule: "the identity exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return updatePolicy(ctx, store, missingIdentity, "p1", testPolicy(missingIdentity))
				},
				Code: notFoundCode,
			},
		),
		Unclear: verifiedNote + rateNote + "The page says the call succeeds for a policy the identity doesn't have, but " +
			"live calls got NotFoundException; the fake does the same, and the handler reports it as a request AWS " +
			"rejected.",
	},
	"GetEmailIdentityPolicies": {
		Doc: v2Doc("GetEmailIdentityPolicies"),
		Rules: []nativetest.Rule{
			{
				Rule: "the identity exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetEmailIdentityPolicies(ctx, &sesv2.GetEmailIdentityPoliciesInput{EmailIdentity: aws.String(missingIdentity)})
					return err
				},
				Code: notFoundCode,
			},
			{
				Rule: "the call returns every policy of the identity, by name",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					api := newFakeAPI(store)
					for _, name := range []string{"p1", "p2"} {
						if _, err := api.CreateEmailIdentityPolicy(ctx, &sesv2.CreateEmailIdentityPolicyInput{EmailIdentity: aws.String(testIdentity), PolicyName: aws.String(name), Policy: aws.String(testPolicy(testIdentity))}); err != nil {
							return err
						}
					}
					var out *sesv2.GetEmailIdentityPoliciesOutput
					var err error
					for range staleReads + 1 {
						if out, err = api.GetEmailIdentityPolicies(ctx, &sesv2.GetEmailIdentityPoliciesInput{EmailIdentity: aws.String(testIdentity)}); err != nil {
							return err
						}
					}
					if len(out.Policies) != 2 || out.Policies["p1"] != testPolicy(testIdentity) {
						return fmt.Errorf("got %v", out.Policies)
					}
					return nil
				},
			},
		},
		Unclear: verifiedNote + rateNote + staleNote + "The identity is case-sensitive: in live calls another case got " +
			"NotFoundException, and an identity's ARN got BadRequestException, so the schema takes no ARN.",
	},
	"DeleteEmailIdentityPolicy": {
		Doc: v2Doc("DeleteEmailIdentityPolicy"),
		Rules: []nativetest.Rule{
			{
				Rule: "the identity exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeleteEmailIdentityPolicy(ctx, &sesv2.DeleteEmailIdentityPolicyInput{EmailIdentity: aws.String(missingIdentity), PolicyName: aws.String("p1")})
					return err
				},
				Code: notFoundCode,
			},
		},
		Unclear: verifiedNote + rateNote + "The page says the call succeeds for a policy the identity doesn't have, but " +
			"live calls got NotFoundException; the fake does the same, and the handler takes it as already deleted.",
	},
}

// setTopic sets the topic of kind for identity in a fake backed by store, or clears it when topic is "".
func setTopic(ctx context.Context, store *awsfake.CloudControl, identity string, kind sestypes.NotificationType, topic string) error {
	in := &ses.SetIdentityNotificationTopicInput{Identity: aws.String(identity), NotificationType: kind}
	if topic != "" {
		in.SnsTopic = aws.String(topic)
	}
	_, err := newFakeAPI(store).SetIdentityNotificationTopic(ctx, in)
	return err
}

// attributesOf returns the notification attributes of identity in a fake backed by store.
func attributesOf(ctx context.Context, store *awsfake.CloudControl, identity string) (sestypes.IdentityNotificationAttributes, error) {
	out, err := newFakeAPI(store).GetIdentityNotificationAttributes(ctx, &ses.GetIdentityNotificationAttributesInput{Identities: []string{identity}})
	if err != nil {
		return sestypes.IdentityNotificationAttributes{}, err
	}
	return out.NotificationAttributes[identity], nil
}

// notificationTopicAPI records the SES v1 calls IdentityNotificationTopic makes.
var notificationTopicAPI = map[string]nativetest.Call{
	"SetIdentityNotificationTopic": {
		Doc: v1Doc("SetIdentityNotificationTopic"),
		Rules: []nativetest.Rule{
			{
				Rule: "NotificationType is Bounce, Complaint, or Delivery",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return setTopic(ctx, store, testIdentity, "Open", topicA)
				},
				Code: validationErrorCode,
			},
			{
				Rule: "the identity is one SES has",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return setTopic(ctx, store, missingIdentity, sestypes.NotificationTypeBounce, topicA)
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "SnsTopic is the ARN of an SNS topic",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return setTopic(ctx, store, testIdentity, sestypes.NotificationTypeBounce, snsTopic(missingPrefix+"-topic"))
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "a call without SnsTopic clears the topic, which stops the notifications",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := setTopic(ctx, store, testIdentity, sestypes.NotificationTypeDelivery, topicA); err != nil {
						return err
					}
					if err := setTopic(ctx, store, testIdentity, sestypes.NotificationTypeDelivery, ""); err != nil {
						return err
					}
					if _, ok := store.Read(notificationTopicType, testIdentity+"|Delivery"); ok {
						return fmt.Errorf("the topic is still set")
					}
					return nil
				},
			},
			{
				Rule: "an identity needs a Bounce and a Complaint topic while email feedback forwarding is off",
				Doc:  v1Doc("SetIdentityFeedbackForwardingEnabled"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := setTopic(ctx, store, noForwardingIdentity, sestypes.NotificationTypeComplaint, topicA); err != nil {
						return err
					}
					return setTopic(ctx, store, noForwardingIdentity, sestypes.NotificationTypeComplaint, "")
				},
				Code: invalidParameterCode,
			},
		},
		Unclear: verifiedNote + rateNote + v1ErrorNote + "The page doesn't say what happens to a topic SES needs while " +
			"email feedback forwarding is off; SetIdentityFeedbackForwardingEnabled and SES's guide say forwarding can " +
			"only be off while the identity has a Bounce and a Complaint topic. In live calls on an unverified email " +
			"identity, SES turned forwarding off with only a Bounce topic and cleared the Bounce topic while forwarding " +
			"was off. The fake rejects clearing either topic while forwarding is off, and the handler's delete then says " +
			"to turn forwarding on. The page doesn't say what clearing the topic does to the headers setting; in live " +
			"calls SES kept it, and so does the fake.",
	},
	"SetIdentityHeadersInNotificationsEnabled": {
		Doc: v1Doc("SetIdentityHeadersInNotificationsEnabled"),
		Rules: []nativetest.Rule{
			{
				Rule: "the notification type has a topic",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).SetIdentityHeadersInNotificationsEnabled(ctx, &ses.SetIdentityHeadersInNotificationsEnabledInput{Identity: aws.String(testIdentity), NotificationType: sestypes.NotificationTypeBounce, Enabled: true})
					return err
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "NotificationType is Bounce, Complaint, or Delivery",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).SetIdentityHeadersInNotificationsEnabled(ctx, &ses.SetIdentityHeadersInNotificationsEnabledInput{Identity: aws.String(testIdentity), NotificationType: "Open", Enabled: true})
					return err
				},
				Code: validationErrorCode,
			},
		},
		Unclear: rateNote + v1ErrorNote + "The page says the setting can only be set while the type has a topic, but live " +
			"calls set it without one; the fake rejects it, so the handler sets the topic first and turns headers off " +
			"before it clears the topic. The page names no error for it; the fake returns InvalidParameterValue.",
	},
	"GetIdentityNotificationAttributes": {
		Doc: v1Doc("GetIdentityNotificationAttributes"),
		Rules: []nativetest.Rule{
			{
				Rule: "the call reads at most 100 identities",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					identities := make([]string, maxIdentities+1)
					for i := range identities {
						identities[i] = fmt.Sprintf("user%d@example.com", i)
					}
					_, err := newFakeAPI(store).GetIdentityNotificationAttributes(ctx, &ses.GetIdentityNotificationAttributesInput{Identities: identities})
					return err
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "email feedback forwarding is on, and headers are off, until a call changes them",
				Doc:  guide + "monitor-sending-activity-using-notifications-email.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					attributes, err := attributesOf(ctx, store, testIdentity)
					if err != nil {
						return err
					}
					if !attributes.ForwardingEnabled || attributes.HeadersInBounceNotificationsEnabled || attributes.BounceTopic != nil {
						return fmt.Errorf("got %+v", attributes)
					}
					return nil
				},
			},
		},
		Unclear: rateNote + staleNote + "The page doesn't say what the call returns for an identity SES doesn't have. In " +
			"live calls it left the identity out, without an error, and so does the fake. The identity is " +
			"case-sensitive: in live calls another case was left out too. An identity's ARN read the same identity, so " +
			"the schema takes only the name, so two forms can't name one topic.",
	},
}

// putIP moves ip to pool in a fake backed by store.
func putIP(ctx context.Context, store *awsfake.CloudControl, ip string, pool string) error {
	_, err := newFakeAPI(store).PutDedicatedIpInPool(ctx, &sesv2.PutDedicatedIpInPoolInput{Ip: aws.String(ip), DestinationPoolName: aws.String(pool)})
	return err
}

// dedicatedIPAPI records the SES v2 calls DedicatedIPAssignment makes.
var dedicatedIPAPI = map[string]nativetest.Call{
	"PutDedicatedIpInPool": {
		Doc: v2Doc("PutDedicatedIpInPool"),
		Rules: []nativetest.Rule{
			{
				Rule: "the address exists and belongs to the account",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putIP(ctx, store, unleasedIP, "pool-b")
				},
				Code: notFoundCode,
			},
			{
				Rule: "the destination pool exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putIP(ctx, store, testIP, "no-such-pool")
				},
				Code: notFoundCode,
			},
			{
				Rule: "SES manages ses-default-dedicated-pool, which can't be edited",
				Doc:  guide + "managing-ip-pools.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putIP(ctx, store, testIP, defaultPool)
				},
				Code: badRequestCode,
			},
		},
		Unclear: "The pages don't give the error for a move into ses-default-dedicated-pool or ses-shared-pool, into or " +
			"out of a managed pool, or of an address that isn't an IP address; the fake returns BadRequestException for " +
			"each. The page doesn't say what a move to the pool the address is in does; the fake moves it. The " +
			"recording account leases no dedicated IP address, so testdata/dedicatedip.json is written from the API " +
			"reference, and only its errors in testdata/errors.json are recorded.",
	},
	"GetDedicatedIp": {
		Doc: v2Doc("GetDedicatedIp"),
		Rules: []nativetest.Rule{
			{
				Rule: "the address belongs to the account",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetDedicatedIp(ctx, &sesv2.GetDedicatedIpInput{Ip: aws.String(unleasedIP)})
					return err
				},
				Code: notFoundCode,
			},
			{
				Rule: "the address is an IP address",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetDedicatedIp(ctx, &sesv2.GetDedicatedIpInput{Ip: aws.String("not-an-ip")})
					return err
				},
				Code: badRequestCode,
			},
		},
		Unclear: staleNote + "The page doesn't say which pool it returns for an address in no pool of the account's; " +
			"SES's guide says ses-default-dedicated-pool holds them, so the fake returns that name.",
	},
}
