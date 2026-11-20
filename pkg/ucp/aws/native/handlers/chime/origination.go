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

package chime

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

const (
	originationType     = "AWS::Chime::VoiceConnectorOrigination"
	originationProperty = "Origination"
)

var _ native.Handler = (*Origination)(nil)

// Origination serves AWS::Chime::VoiceConnectorOrigination, a Voice Connector's origination settings, which route
// inbound calls to your SIP hosts. A Voice Connector has none until a Put sets them, and a delete deletes them. AWS
// rejects the delete while the Voice Connector has an emergency calling configuration.
type Origination struct {
	api  API
	wait pause
}

// NewOrigination returns the handler for a Voice Connector's origination settings.
func NewOrigination(api API) *Origination {
	return &Origination{api: api, wait: sleep}
}

// TypeName returns AWS::Chime::VoiceConnectorOrigination.
func (h *Origination) TypeName() string {
	return originationType
}

// Options returns how UCP uses the handler.
func (h *Origination) Options() native.Options {
	return options()
}

// Read returns the Voice Connector's origination settings, or ErrNotFound when it has none or AWS has no such Voice
// Connector.
func (h *Origination) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.api.GetVoiceConnectorOrigination(ctx, &chimesdkvoice.GetVoiceConnectorOriginationInput{VoiceConnectorId: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, notFoundOrRejected(err)
	}
	return properties(identifier, originationProperty, out.Origination), nil
}

// put sets the Voice Connector's origination settings to those properties set, and waits until a read shows them.
func (h *Origination) put(ctx context.Context, region string, identifier string, props map[string]any) error {
	var settings types.Origination
	if err := settingsOf(props, originationProperty, &settings); err != nil {
		return err
	}
	out, err := h.api.PutVoiceConnectorOrigination(ctx, &chimesdkvoice.PutVoiceConnectorOriginationInput{
		VoiceConnectorId: aws.String(identifier),
		Origination:      &settings,
	}, inRegion(region))
	if err != nil {
		return sendFailed(err)
	}
	return confirm(ctx, h.wait, h.Read, region, identifier, properties(identifier, originationProperty, out.Origination))
}

// Create sets the Voice Connector's origination settings to the template's and returns the Voice Connector's ID.
func (h *Origination) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, _ := request.Properties[voiceConnectorIDProperty].(string)
	if err := h.put(ctx, request.Region, identifier, request.Properties); err != nil {
		return "", err
	}
	return identifier, nil
}

// Update sets the Voice Connector's origination settings to the template's. A value the template no longer sets is left
// out of the Put, which clears it or sets it back to AWS's default.
func (h *Origination) Update(ctx context.Context, request native.UpdateRequest) error {
	if err := settingsOf(request.Desired, originationProperty, &types.Origination{}); err != nil {
		return err
	}
	target, err := patched(request)
	if err != nil {
		return err
	}
	return h.put(ctx, request.Region, request.Identifier, target)
}

// Delete deletes the Voice Connector's origination settings. AWS deletes settings a Voice Connector doesn't have without
// an error, and the delete's progress reports them gone. It returns ErrNotFound when AWS has no such Voice Connector.
func (h *Origination) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeleteVoiceConnectorOrigination(ctx, &chimesdkvoice.DeleteVoiceConnectorOriginationInput{VoiceConnectorId: aws.String(identifier)}, inRegion(region))
	return notFoundOrRejected(err)
}

// List fails, because the settings can't be listed.
func (h *Origination) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, cantList(originationType)
}

// Progress reports a create as done once a read finds the settings, after a few tries, an update as done, because the Put waited until a
// read showed it, and a delete as done, with ErrNotFound, once a read no longer finds them.
func (h *Origination) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	switch op {
	case native.OperationCreate:
		return createProgress(ctx, h.wait, h.Read, region, identifier)
	case native.OperationDelete:
		return deleteProgress(ctx, h.Read, region, identifier)
	}
	return native.Progress{Done: true}, nil
}
