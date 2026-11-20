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

// Package connect has the native handler for AWS::Connect::PhoneNumberContactFlowAssociation, the flow an Amazon
// Connect phone number's calls go to, which has no CloudFormation type.
package connect

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/connect"
	"github.com/aws/aws-sdk-go-v2/service/connect/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	associationType = "AWS::Connect::PhoneNumberContactFlowAssociation"

	phoneNumberIDProperty = "PhoneNumberId"
	instanceIDProperty    = "InstanceId"
	contactFlowIDProperty = "ContactFlowId"

	// notFoundCode is the error Amazon Connect returns for an instance or a phone number it doesn't have.
	notFoundCode = "ResourceNotFoundException"

	// confirmReads is how many times a create or update reads the association before it stops waiting for a read to
	// show the change, and confirmPause is how long it waits between reads.
	confirmReads = 20
	confirmPause = 500 * time.Millisecond

	// timeout is how long UCP waits for any operation.
	timeout = 10 * time.Minute
)

// API is the part of the Amazon Connect client the handler uses.
type API interface {
	AssociatePhoneNumberContactFlow(ctx context.Context, params *connect.AssociatePhoneNumberContactFlowInput, optFns ...func(*connect.Options)) (*connect.AssociatePhoneNumberContactFlowOutput, error)
	DisassociatePhoneNumberContactFlow(ctx context.Context, params *connect.DisassociatePhoneNumberContactFlowInput, optFns ...func(*connect.Options)) (*connect.DisassociatePhoneNumberContactFlowOutput, error)
	ListFlowAssociations(ctx context.Context, params *connect.ListFlowAssociationsInput, optFns ...func(*connect.Options)) (*connect.ListFlowAssociationsOutput, error)
}

var inRegion = awsapi.InRegion[connect.Options]

var (
	// phoneNumberPattern is the form of a phone number's ARN, and instancePattern of an instance's ID. The identifier
	// takes one form of each, so two aliases can't hold one association by naming it two ways.
	phoneNumberPattern = regexp.MustCompile(`^arn:aws[a-z-]*:connect:[a-z0-9-]+:[0-9]{12}:phone-number/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	instancePattern    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// pause waits for d, or until ctx ends. Tests replace it, so they don't wait.
type pause func(ctx context.Context, d time.Duration)

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

var (
	_ native.Handler           = (*PhoneNumberContactFlowAssociation)(nil)
	_ native.IdentifierEncoder = (*PhoneNumberContactFlowAssociation)(nil)
)

// PhoneNumberContactFlowAssociation serves AWS::Connect::PhoneNumberContactFlowAssociation, the flow that answers the
// calls to a phone number claimed to an Amazon Connect instance. A phone number has at most one flow, so the
// association's identifier is the phone number's ARN and the instance's ID, joined with "|". AWS takes either form of
// each, but the identifier takes one, so two aliases can't hold one association. A number is given by ARN, because a
// number claimed to a traffic distribution group is found only by ARN in the group's other Region.
//
// Amazon Connect has no call that reads one phone number's flow, so Read lists the instance's phone number flow
// associations with ListFlowAssociations and picks the number's, which AWS gives by ARN, with the flow's ARN. A
// template may give the flow by ID or by ARN, and both mean the same flow (see equivalent).
//
// A create or update associates the flow with AssociatePhoneNumberContactFlow, which a deploy sends again to change
// the flow, and waits, for up to confirmReads reads, until a read shows it. A delete removes the association, which
// leaves the phone number without a flow.
type PhoneNumberContactFlowAssociation struct {
	api  API
	wait pause
}

// NewPhoneNumberContactFlowAssociation returns the handler for Amazon Connect phone number flow associations.
func NewPhoneNumberContactFlowAssociation(api API) *PhoneNumberContactFlowAssociation {
	return &PhoneNumberContactFlowAssociation{api: api, wait: sleep}
}

// TypeName returns AWS::Connect::PhoneNumberContactFlowAssociation.
func (h *PhoneNumberContactFlowAssociation) TypeName() string {
	return associationType
}

// Options returns how UCP uses the handler. An association can't be listed except under its instance, and its
// identifier is known before the create.
func (h *PhoneNumberContactFlowAssociation) Options() native.Options {
	return native.Options{
		List:          native.ListNone,
		Recovery:      native.RecoverByIdentifier,
		Equivalent:    equivalent,
		CreateTimeout: timeout,
		UpdateTimeout: timeout,
		DeleteTimeout: timeout,
	}
}

// equivalent counts a flow given by ID and the same flow's ARN, which AWS returns, as the same value.
func equivalent(path string, template any, fromAWS any) bool {
	t, ok1 := template.(string)
	a, ok2 := fromAWS.(string)
	return path == contactFlowIDProperty && ok1 && ok2 && flowID(t) == flowID(a)
}

// flowID returns the ID of a flow given by ID or by ARN, such as
// arn:aws:connect:us-west-2:123456789012:instance/<instance ID>/contact-flow/<flow ID>.
func flowID(flow string) string {
	if _, id, ok := strings.Cut(flow, "/contact-flow/"); ok && strings.HasPrefix(flow, "arn:") {
		return id
	}
	return flow
}

// rejected returns a client failure AWS applied none of, with message.
func rejected(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// notFound returns err wrapped with ErrNotFound when AWS has no such instance or phone number.
func notFound(err error) error {
	return awsapi.NotFound(err, notFoundCode)
}

// parseIdentifier returns the phone number and the instance an identifier gives.
func parseIdentifier(identifier string) (phoneNumber string, instance string, err error) {
	phoneNumber, instance, _ = strings.Cut(identifier, "|")
	if !phoneNumberPattern.MatchString(phoneNumber) || !instancePattern.MatchString(instance) {
		return "", "", rejected("the identifier of an %s is its PhoneNumberId, the phone number's ARN, and its InstanceId, the instance's ID, joined with |", associationType)
	}
	return phoneNumber, instance, nil
}

// Identifier returns the identifier the properties give, their PhoneNumberId and InstanceId joined with "|", or ""
// when they don't give both. A phone number that isn't an ARN, or an instance that isn't an ID, is a client failure.
func (h *PhoneNumberContactFlowAssociation) Identifier(properties map[string]any) (string, error) {
	phoneNumber, instance := awsapi.String(properties, phoneNumberIDProperty), awsapi.String(properties, instanceIDProperty)
	if phoneNumber == "" || instance == "" {
		return "", nil
	}
	identifier := phoneNumber + "|" + instance
	if _, _, err := parseIdentifier(identifier); err != nil {
		return "", err
	}
	return identifier, nil
}

// Read returns the association of the phone number with the identifier, or ErrNotFound when the number has no flow,
// or AWS has no such instance or number. It returns any other error as it is, never as a rejected failure, because
// Progress calls it after AWS applied an operation.
func (h *PhoneNumberContactFlowAssociation) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	phoneNumber, instance, err := parseIdentifier(identifier)
	if err != nil {
		return nil, err
	}
	pages := connect.NewListFlowAssociationsPaginator(h.api, &connect.ListFlowAssociationsInput{
		InstanceId:   aws.String(instance),
		ResourceType: types.ListFlowAssociationResourceTypeVoicePhoneNumber,
	}, func(o *connect.ListFlowAssociationsPaginatorOptions) {
		o.StopOnDuplicateToken = true
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, notFound(err)
		}
		for _, association := range page.FlowAssociationSummaryList {
			if aws.ToString(association.ResourceId) == phoneNumber && aws.ToString(association.FlowId) != "" {
				return map[string]any{
					phoneNumberIDProperty: phoneNumber,
					instanceIDProperty:    instance,
					contactFlowIDProperty: aws.ToString(association.FlowId),
				}, nil
			}
		}
	}
	return nil, native.ErrNotFound
}

// associate sets the phone number's flow to flow, and waits until a read shows it.
func (h *PhoneNumberContactFlowAssociation) associate(ctx context.Context, region string, identifier string, flow string) error {
	phoneNumber, instance, err := parseIdentifier(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.AssociatePhoneNumberContactFlow(ctx, &connect.AssociatePhoneNumberContactFlowInput{
		PhoneNumberId: aws.String(phoneNumber),
		InstanceId:    aws.String(instance),
		ContactFlowId: aws.String(flow),
	}, inRegion(region))
	if err != nil {
		return err
	}
	h.confirm(ctx, region, identifier, flow)
	return nil
}

// confirm reads the association until a read shows flow, so the next read shows the change. It stops early when a read
// fails with anything but ErrNotFound, and after confirmReads reads, because the change succeeded either way: a deploy
// that still reads the earlier flow then sends the same change again, which changes nothing.
func (h *PhoneNumberContactFlowAssociation) confirm(ctx context.Context, region string, identifier string, flow string) {
	for i := range confirmReads {
		if i > 0 {
			h.wait(ctx, confirmPause)
		}
		if ctx.Err() != nil {
			return
		}
		got, err := h.Read(ctx, region, identifier)
		if err == nil && equivalent(contactFlowIDProperty, flow, got[contactFlowIDProperty]) {
			return
		}
		if err != nil && !errors.Is(err, native.ErrNotFound) {
			return
		}
	}
}

// Create associates the template's flow with the phone number and returns the association's identifier. UCP calls it
// only for a number with no flow, or one AWS doesn't have, which AWS rejects.
func (h *PhoneNumberContactFlowAssociation) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	if err := h.associate(ctx, request.Region, identifier, awsapi.String(request.Properties, contactFlowIDProperty)); err != nil {
		return "", awsapi.Rejected(err)
	}
	return identifier, nil
}

// Update associates the template's flow with the phone number, in place of the flow it has.
func (h *PhoneNumberContactFlowAssociation) Update(ctx context.Context, request native.UpdateRequest) error {
	if !slices.Contains(awsapi.Changed(request), contactFlowIDProperty) {
		return nil
	}
	return awsapi.Rejected(notFound(h.associate(ctx, request.Region, request.Identifier, awsapi.String(request.Desired, contactFlowIDProperty))))
}

// Delete removes the phone number's flow. It returns ErrNotFound when AWS has no such instance or number.
func (h *PhoneNumberContactFlowAssociation) Delete(ctx context.Context, region string, identifier string) error {
	phoneNumber, instance, err := parseIdentifier(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DisassociatePhoneNumberContactFlow(ctx, &connect.DisassociatePhoneNumberContactFlowInput{
		PhoneNumberId: aws.String(phoneNumber),
		InstanceId:    aws.String(instance),
	}, inRegion(region))
	return awsapi.Rejected(notFound(err))
}

// List fails, because associations can be listed only under their instance.
func (h *PhoneNumberContactFlowAssociation) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", associationType)
}

// Progress reports a create as done once a read finds the association, and an update as done, because the create and
// the update already waited until a read showed them. It returns ErrNotFound when the number has no flow, so UCP sends
// a lost create again, and a delete finishes.
func (h *PhoneNumberContactFlowAssociation) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	if _, err := h.Read(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "associated", Waiting: "disassociated"}, nil
	}
	return native.Progress{Done: true}, nil
}
