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

package connect

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/connect"
	"github.com/aws/aws-sdk-go-v2/service/connect/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/connect/latest/APIReference/"

	// tdgNote is what the record of each call that takes a phone number says about traffic distribution groups.
	tdgNote = "The page says a number claimed to a traffic distribution group must be given as its ARN in the group's " +
		"other Region, and gets ResourceNotFoundException by ID there; the fake does the same. "

	// staleNote is what the record of each call that changes an association says about lists right after it.
	staleNote = "The page doesn't say when a change shows in ListFlowAssociations, and the recording account claims no " +
		"phone number, so no live call showed it. The fake lists the association as it was for 10 lists, as Amazon SNS " +
		"did for up to 9 in live calls, and the handler waits, for up to 20 lists, until a list shows a change. "
)

func doc(operation string) string {
	return apiReference + "API_" + operation + ".html"
}

// associate sends an association of flow with phone in instance to a fake backed by store.
func associate(ctx context.Context, store *awsfake.CloudControl, phone string, instance string, flow string) error {
	_, err := newFakeAPI(store).AssociatePhoneNumberContactFlow(ctx, &connect.AssociatePhoneNumberContactFlowInput{PhoneNumberId: aws.String(phone), InstanceId: aws.String(instance), ContactFlowId: aws.String(flow)})
	return err
}

// list returns the flow of each phone number with one in instance, from a new fake backed by store, which has made no
// change to list as it was.
func list(ctx context.Context, store *awsfake.CloudControl, instance string) (map[string]string, error) {
	out, err := newFakeAPI(store).ListFlowAssociations(ctx, &connect.ListFlowAssociationsInput{InstanceId: aws.String(instance), ResourceType: types.ListFlowAssociationResourceTypeVoicePhoneNumber})
	if err != nil {
		return nil, err
	}
	flows := map[string]string{}
	for _, association := range out.FlowAssociationSummaryList {
		flows[aws.ToString(association.ResourceId)] = aws.ToString(association.FlowId)
	}
	return flows, nil
}

// phoneNumberRules are the rules for the phone number of a call, made with call.
func phoneNumberRules(call func(ctx context.Context, store *awsfake.CloudControl, phone string, instance string) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "the instance exists",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, testPhone, missingInstance)
			},
			Code: notFoundCode,
		},
		{
			Rule: "the phone number is claimed to the instance",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, missingPhone, testInstance)
			},
			Code: notFoundCode,
		},
		{
			Rule: "a number claimed to a traffic distribution group is given as its ARN in the group's other Region",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, tdgPhone, testInstance)
			},
			Code: notFoundCode,
		},
		{
			Rule: "a number claimed to a traffic distribution group is found by its ARN in the group's other Region",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, phoneARN(tdgPhone), testInstance)
			},
		},
	}
}

// associationAPI records the Amazon Connect calls PhoneNumberContactFlowAssociation makes.
var associationAPI = map[string]nativetest.Call{
	"AssociatePhoneNumberContactFlow": {
		Doc: doc("AssociatePhoneNumberContactFlow"),
		Rules: append(phoneNumberRules(func(ctx context.Context, store *awsfake.CloudControl, phone string, instance string) error {
			return associate(ctx, store, phone, instance, testFlow)
		}),
			nativetest.Rule{
				Rule: "ContactFlowId is at most 500 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return associate(ctx, store, testPhone, testInstance, fmt.Sprintf("%0501d", 0))
				},
				Code: invalidParameterCode,
			},
			nativetest.Rule{
				Rule: "InstanceId is 1 to 100 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return associate(ctx, store, testPhone, fmt.Sprintf("%0101d", 0), testFlow)
				},
				Code: invalidParameterCode,
			},
			nativetest.Rule{
				Rule: "the phone number is a UUID or an ARN",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return associate(ctx, store, "not-a-uuid", testInstance, testFlow)
				},
				Code: invalidParameterCode,
			},
			nativetest.Rule{
				Rule: "the flow is in the instance",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return associate(ctx, store, testPhone, testInstance, missingFlow)
				},
				Code: notFoundCode,
			},
			nativetest.Rule{
				Rule: "an association replaces the number's flow",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := associate(ctx, store, testPhone, testInstance, testFlow); err != nil {
						return err
					}
					if err := associate(ctx, store, testPhone, testInstance, otherFlow); err != nil {
						return err
					}
					flows, err := list(ctx, store, testInstance)
					if err != nil {
						return err
					}
					if flows[phoneARN(testPhone)] != otherFlow {
						return fmt.Errorf("the flows are %v", flows)
					}
					return nil
				},
			},
		),
		Unclear: tdgNote + "The page doesn't say what an association of a number that has a flow does. The handler sends " +
			"it to change the flow, so if AWS rejected it, the update would fail and change nothing; the fake replaces " +
			"the flow, as the console does. In live calls AWS answered a phone number it doesn't have with " +
			"ResourceNotFoundException before it checked the instance or the flow (testdata/errors.json), and a phone number " +
			"that isn't a UUID or an ARN with InvalidParameterException. The handler sends a number by ARN, which AWS took. It doesn't name the error for a flow of " +
			"another instance, or of another type; the fake returns ResourceNotFoundException for one it doesn't have, " +
			"and doesn't check types. " + staleNote,
	},
	"DisassociatePhoneNumberContactFlow": {
		Doc: doc("DisassociatePhoneNumberContactFlow"),
		Rules: append(phoneNumberRules(func(ctx context.Context, store *awsfake.CloudControl, phone string, instance string) error {
			_, err := newFakeAPI(store).DisassociatePhoneNumberContactFlow(ctx, &connect.DisassociatePhoneNumberContactFlowInput{PhoneNumberId: aws.String(phone), InstanceId: aws.String(instance)})
			return err
		}),
			nativetest.Rule{
				Rule: "the number has no flow afterwards",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := associate(ctx, store, testPhone, testInstance, testFlow); err != nil {
						return err
					}
					if _, err := newFakeAPI(store).DisassociatePhoneNumberContactFlow(ctx, &connect.DisassociatePhoneNumberContactFlowInput{PhoneNumberId: aws.String(testPhone), InstanceId: aws.String(testInstance)}); err != nil {
						return err
					}
					flows, err := list(ctx, store, testInstance)
					if err != nil {
						return err
					}
					if len(flows) != 0 {
						return fmt.Errorf("the flows are %v", flows)
					}
					return nil
				},
			},
		),
		Unclear: tdgNote + "The page doesn't say what a disassociation of a number without a flow does; the fake " +
			"succeeds, and the handler finishes a delete either way, once a list no longer shows the number. In a live " +
			"call AWS answered a phone number it doesn't have with ResourceNotFoundException (testdata/errors.json). " +
			staleNote,
	},
	"ListFlowAssociations": {
		Doc: doc("ListFlowAssociations"),
		Rules: []nativetest.Rule{
			{
				Rule: "the instance exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := list(ctx, store, missingInstance)
					return err
				},
				Code: notFoundCode,
			},
			{
				Rule: "MaxResults is from 1 to 1000",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListFlowAssociations(ctx, &connect.ListFlowAssociationsInput{InstanceId: aws.String(testInstance), MaxResults: aws.Int32(1001)})
					return err
				},
				Code: invalidParameterCode,
			},
			{
				Rule: "ResourceType lists only associations of that type",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := associate(ctx, store, testPhone, testInstance, testFlow); err != nil {
						return err
					}
					out, err := newFakeAPI(store).ListFlowAssociations(ctx, &connect.ListFlowAssociationsInput{InstanceId: aws.String(testInstance), ResourceType: types.ListFlowAssociationResourceTypeInboundEmail})
					if err != nil {
						return err
					}
					if len(out.FlowAssociationSummaryList) != 0 {
						return fmt.Errorf("an email list has %v", out.FlowAssociationSummaryList)
					}
					return nil
				},
			},
			{
				Rule: "NextToken gives the next page",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					for _, phone := range []string{testPhone, otherPhone, phoneARN(tdgPhone)} {
						if err := associate(ctx, store, phone, testInstance, testFlow); err != nil {
							return err
						}
					}
					out, err := newFakeAPI(store).ListFlowAssociations(ctx, &connect.ListFlowAssociationsInput{InstanceId: aws.String(testInstance), ResourceType: types.ListFlowAssociationResourceTypeVoicePhoneNumber})
					if err != nil {
						return err
					}
					if out.NextToken == nil {
						return fmt.Errorf("the first page has no NextToken")
					}
					return nil
				},
			},
		},
		Unclear: "The page doesn't say what ResourceId and FlowId hold for VOICE_PHONE_NUMBER. In a live call " +
			"BatchGetFlowAssociation gave a phone number's ResourceId as its ARN, as the fake does. The handler takes a " +
			"flow by ID or ARN, so it doesn't depend on FlowId's form. " +
			"In a live call AWS answered an instance it doesn't have with ResourceNotFoundException (testdata/errors.json), " +
			"and took the instance's ID or ARN; the handler sends its ID. The fake's pages have two associations, so the handler's paging " +
			"is tested.",
	},
}
