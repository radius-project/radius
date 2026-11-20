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
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	notificationTopicType = "AWS::SES::IdentityNotificationTopic"

	// headersProperty is the property that says whether notifications include the original email headers.
	headersProperty = "HeadersInNotificationsEnabled"

	// forwardingHint is what a delete adds to AWS's message when SES rejects clearing a Bounce or Complaint topic
	// while email feedback forwarding is off.
	forwardingHint = " SES needs a Bounce and a Complaint topic while email feedback forwarding is off, so turn " +
		"forwarding on first, such as with FeedbackAttributes of the AWS::SES::EmailIdentity, and delete again."
)

// topicARN matches the ARN of an SNS topic.
var topicARN = regexp.MustCompile(`^arn:[a-z0-9-]+:sns:[a-z0-9-]+:[0-9]{12}:[A-Za-z0-9_.-]+$`)

var (
	_ native.Handler           = (*IdentityNotificationTopic)(nil)
	_ native.IdentifierEncoder = (*IdentityNotificationTopic)(nil)
)

// IdentityNotificationTopic serves AWS::SES::IdentityNotificationTopic, the SNS topic an SES identity publishes one
// type of notification to, and whether those notifications include the original email headers. Its identifier is the
// identity and the notification type, joined with "|". It exists while the identity has a topic for the type: a
// delete turns headers off and then clears the topic, because SES takes the headers setting only while a topic is
// set.
type IdentityNotificationTopic struct {
	api  V1API
	wait pause
}

// NewIdentityNotificationTopic returns the handler for the notification topics of SES identities.
func NewIdentityNotificationTopic(api V1API) *IdentityNotificationTopic {
	return &IdentityNotificationTopic{api: api, wait: sleep}
}

// TypeName returns AWS::SES::IdentityNotificationTopic.
func (h *IdentityNotificationTopic) TypeName() string {
	return notificationTopicType
}

// Options returns how UCP uses the handler.
func (h *IdentityNotificationTopic) Options() native.Options {
	return options()
}

// Identifier returns Identity and NotificationType, joined with "|". An identity's ARN, a notification type SES
// doesn't have, or a topic that isn't an SNS topic ARN is rejected before any call.
func (h *IdentityNotificationTopic) Identifier(properties map[string]any) (string, error) {
	if _, err := identityOf(properties, "Identity"); err != nil {
		return "", err
	}
	if kind := awsapi.String(properties, "NotificationType"); kind != "" && !slices.Contains(types.NotificationType("").Values(), types.NotificationType(kind)) {
		return "", rejected("NotificationType must be Bounce, Complaint, or Delivery")
	}
	if topic := awsapi.String(properties, "SnsTopic"); topic != "" && !topicARN.MatchString(topic) {
		return "", rejected("SnsTopic must be the ARN of an SNS topic")
	}
	return joined(properties, "Identity", "NotificationType"), nil
}

// notification returns the identity and notification type of the identifier.
func notification(identifier string) (string, types.NotificationType, error) {
	identity, kind, err := pair(identifier)
	if err != nil {
		return "", "", err
	}
	switch t := types.NotificationType(kind); t {
	case types.NotificationTypeBounce, types.NotificationTypeComplaint, types.NotificationTypeDelivery:
		return identity, t, nil
	}
	return "", "", fmt.Errorf("the identifier %q doesn't end with Bounce, Complaint, or Delivery", identifier)
}

// topicAndHeaders returns the topic of kind in attributes, or "" for none, and whether its notifications include
// the original email headers.
func topicAndHeaders(attributes types.IdentityNotificationAttributes, kind types.NotificationType) (string, bool) {
	switch kind {
	case types.NotificationTypeBounce:
		return aws.ToString(attributes.BounceTopic), attributes.HeadersInBounceNotificationsEnabled
	case types.NotificationTypeComplaint:
		return aws.ToString(attributes.ComplaintTopic), attributes.HeadersInComplaintNotificationsEnabled
	}
	return aws.ToString(attributes.DeliveryTopic), attributes.HeadersInDeliveryNotificationsEnabled
}

// attributes returns the notification attributes of identity, or ErrNotFound when SES has no such identity.
func (h *IdentityNotificationTopic) attributes(ctx context.Context, region string, identity string) (types.IdentityNotificationAttributes, error) {
	out, err := h.api.GetIdentityNotificationAttributes(ctx, &ses.GetIdentityNotificationAttributesInput{Identities: []string{identity}}, inRegionV1(region))
	if err != nil {
		return types.IdentityNotificationAttributes{}, awsapi.Rejected(err)
	}
	attributes, ok := out.NotificationAttributes[identity]
	if !ok {
		return types.IdentityNotificationAttributes{}, native.ErrNotFound
	}
	return attributes, nil
}

// Read returns the topic and the headers setting, or ErrNotFound when the identity has no topic for the type or SES
// has no such identity.
func (h *IdentityNotificationTopic) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	identity, kind, err := notification(identifier)
	if err != nil {
		return nil, err
	}
	attributes, err := h.attributes(ctx, region, identity)
	if err != nil {
		return nil, err
	}
	topic, headers := topicAndHeaders(attributes, kind)
	if topic == "" {
		return nil, native.ErrNotFound
	}
	return map[string]any{"Identity": identity, "NotificationType": string(kind), "SnsTopic": topic, headersProperty: headers}, nil
}

// setTopic sets the topic of the notification type, and waits until a read shows it.
func (h *IdentityNotificationTopic) setTopic(ctx context.Context, region string, identifier string, topic string) error {
	identity, kind, err := notification(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.SetIdentityNotificationTopic(ctx, &ses.SetIdentityNotificationTopicInput{Identity: aws.String(identity), NotificationType: kind, SnsTopic: aws.String(topic)}, inRegionV1(region))
	if err != nil {
		return err
	}
	confirm(ctx, h.wait, h.Read, region, identifier, func(got map[string]any, err error) bool {
		return err == nil && got["SnsTopic"] == topic
	})
	return nil
}

// setHeaders sets whether the notifications include the original email headers, and waits until a read shows it. It
// returns whether a read showed it, which tells a call that applied but returned an error, such as one whose response
// was lost, from one that didn't apply.
func (h *IdentityNotificationTopic) setHeaders(ctx context.Context, region string, identifier string, enabled bool) (bool, error) {
	identity, kind, err := notification(identifier)
	if err != nil {
		return false, err
	}
	_, err = h.api.SetIdentityHeadersInNotificationsEnabled(ctx, &ses.SetIdentityHeadersInNotificationsEnabledInput{Identity: aws.String(identity), NotificationType: kind, Enabled: enabled}, inRegionV1(region))
	// The template leaves the setting out to turn it off, so a deploy that reads it still on would take it as a value
	// AWS set. The call may have applied even when it returned an error, so the read waits for it either way.
	shown := confirm(ctx, h.wait, h.Read, region, identifier, func(got map[string]any, err error) bool {
		return err == nil && got[headersProperty] == enabled
	})
	return shown, err
}

// lastHeaders sets the headers setting as the last call of a create or an update. When the call returns an error but
// a read shows the setting, the operation failed fully applied.
func (h *IdentityNotificationTopic) lastHeaders(ctx context.Context, region string, identifier string, enabled bool) error {
	shown, err := h.setHeaders(ctx, region, identifier, enabled)
	if err != nil && shown {
		return &native.Failure{Message: "SES applied the change, but the call failed: " + err.Error(), Outcome: native.OutcomeApplied}
	}
	return err
}

// desired returns the topic and the headers setting properties set, or a client failure when they set no topic.
func desired(properties map[string]any) (string, bool, error) {
	topic := awsapi.String(properties, "SnsTopic")
	if topic == "" {
		return "", false, rejected("set SnsTopic")
	}
	headers, _ := properties[headersProperty].(bool)
	return topic, headers, nil
}

// Create sets the topic, then the headers setting when SES has another, and returns the identifier. SES takes the
// headers setting only while a topic is set. A failure after the topic is set is partly applied.
func (h *IdentityNotificationTopic) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, _ := h.Identifier(request.Properties)
	if _, _, err := notification(identifier); err != nil {
		return "", rejected("set Identity, and NotificationType to Bounce, Complaint, or Delivery")
	}
	topic, headers, err := desired(request.Properties)
	if err != nil {
		return "", err
	}
	err = awsapi.Calls(later(
		func() error { return h.setTopic(ctx, request.Region, identifier, topic) },
		func() error {
			current, err := h.Read(ctx, request.Region, identifier)
			if err != nil || current[headersProperty] == headers {
				return err
			}
			return h.lastHeaders(ctx, request.Region, identifier, headers)
		},
	)...)
	if err != nil {
		return identifier, awsapi.Rejected(err)
	}
	return identifier, nil
}

// Update sets the topic when the template changed it, then the headers setting when it differs from SES's. A
// headers setting the template no longer sets is turned off. A failure after the first call is partly applied.
func (h *IdentityNotificationTopic) Update(ctx context.Context, request native.UpdateRequest) error {
	topic, headers, err := desired(request.Desired)
	if err != nil {
		return err
	}
	calls := []func() error{}
	if request.Current["SnsTopic"] != topic {
		calls = append(calls, func() error { return h.setTopic(ctx, request.Region, request.Identifier, topic) })
	}
	if current, _ := request.Current[headersProperty].(bool); current != headers {
		calls = append(calls, func() error { return h.lastHeaders(ctx, request.Region, request.Identifier, headers) })
	}
	return awsapi.Rejected(awsapi.Calls(later(calls...)...))
}

// Delete turns headers off when they are on, and then clears the topic. It returns ErrNotFound when the identity has
// no topic for the type or SES has no such identity. When SES rejects clearing a Bounce or Complaint topic while email
// feedback forwarding is off, the error says to turn forwarding on first.
func (h *IdentityNotificationTopic) Delete(ctx context.Context, region string, identifier string) error {
	identity, kind, err := notification(identifier)
	if err != nil {
		return err
	}
	attributes, err := h.attributes(ctx, region, identity)
	if err != nil {
		return err
	}
	topic, headers := topicAndHeaders(attributes, kind)
	if topic == "" {
		return native.ErrNotFound
	}
	calls := []func() error{}
	if headers {
		calls = append(calls, func() error {
			_, err := h.setHeaders(ctx, region, identifier, false)
			return err
		})
	}
	var clearErr error
	calls = append(calls, func() error {
		_, clearErr = h.api.SetIdentityNotificationTopic(ctx, &ses.SetIdentityNotificationTopicInput{Identity: aws.String(identity), NotificationType: kind}, inRegionV1(region))
		return clearErr
	})
	err = awsapi.Rejected(awsapi.Calls(later(calls...)...))
	var failure *native.Failure
	if kind != types.NotificationTypeDelivery && !attributes.ForwardingEnabled && isRejected(clearErr) && errors.As(err, &failure) {
		failure.Message += forwardingHint
	}
	return err
}

// List fails, because the topics can't be listed without reading every identity.
func (h *IdentityNotificationTopic) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, cantList(notificationTopicType)
}

// Progress reports a create as done once a read finds the topic, an update as done, because each call waited until a
// read showed it, and a delete as done, with ErrNotFound, once a read no longer finds the topic.
func (h *IdentityNotificationTopic) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	switch op {
	case native.OperationCreate:
		return createProgress(ctx, h.Read, region, identifier)
	case native.OperationDelete:
		return deleteProgress(ctx, h.Read, region, identifier)
	}
	return native.Progress{Done: true}, nil
}
