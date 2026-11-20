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
	terminationType     = "AWS::Chime::VoiceConnectorTermination"
	terminationProperty = "Termination"

	// defaultPhoneNumberProperty is write-only: in live calls AWS required a number of the Voice Connector, so the
	// recordings couldn't confirm that GetVoiceConnectorTermination returns it.
	defaultPhoneNumberProperty = "DefaultPhoneNumber"
)

var _ native.Handler = (*Termination)(nil)

// Termination serves AWS::Chime::VoiceConnectorTermination, a Voice Connector's termination settings, which let your
// SIP hosts make outbound calls. A Voice Connector has none until a Put sets them, and a delete deletes them. AWS
// rejects the delete while the Voice Connector has an emergency calling configuration. Its SIP credentials are a
// separate resource, which a delete leaves as they are.
type Termination struct {
	api  API
	wait pause
}

// NewTermination returns the handler for a Voice Connector's termination settings.
func NewTermination(api API) *Termination {
	return &Termination{api: api, wait: sleep}
}

// TypeName returns AWS::Chime::VoiceConnectorTermination.
func (h *Termination) TypeName() string {
	return terminationType
}

// Options returns how UCP uses the handler.
func (h *Termination) Options() native.Options {
	return options()
}

// Read returns the Voice Connector's termination settings, or ErrNotFound when it has none or AWS has no such Voice
// Connector.
func (h *Termination) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.api.GetVoiceConnectorTermination(ctx, &chimesdkvoice.GetVoiceConnectorTerminationInput{VoiceConnectorId: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, notFound(err)
	}
	return terminationProperties(identifier, out.Termination), nil
}

// put sets the Voice Connector's termination settings to those properties set, and waits until a read shows them.
func (h *Termination) put(ctx context.Context, region string, identifier string, props map[string]any) error {
	var settings types.Termination
	if err := settingsOf(props, terminationProperty, &settings); err != nil {
		return err
	}
	out, err := h.api.PutVoiceConnectorTermination(ctx, &chimesdkvoice.PutVoiceConnectorTerminationInput{
		VoiceConnectorId: aws.String(identifier),
		Termination:      &settings,
	}, inRegion(region))
	if err != nil {
		return sendFailed(err)
	}
	return confirm(ctx, h.wait, h.Read, region, identifier, terminationProperties(identifier, out.Termination))
}

// terminationProperties returns the properties of settings, without DefaultPhoneNumber, which is write-only, so a read
// that leaves it out matches the Put that set it.
func terminationProperties(identifier string, settings *types.Termination) map[string]any {
	if settings != nil {
		copied := *settings
		copied.DefaultPhoneNumber = nil
		settings = &copied
	}
	return properties(identifier, terminationProperty, settings)
}

// Create sets the Voice Connector's termination settings to the template's and returns the Voice Connector's ID.
func (h *Termination) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, _ := request.Properties[voiceConnectorIDProperty].(string)
	if err := h.put(ctx, request.Region, identifier, request.Properties); err != nil {
		return "", err
	}
	return identifier, nil
}

// Update sets the Voice Connector's termination settings to the template's. A value the template no longer sets is left
// out of the Put, which clears it or sets it back to AWS's default.
func (h *Termination) Update(ctx context.Context, request native.UpdateRequest) error {
	if err := settingsOf(request.Desired, terminationProperty, &types.Termination{}); err != nil {
		return err
	}
	target, err := patched(request)
	if err != nil {
		return err
	}
	// DefaultPhoneNumber is write-only, so AWS's settings in the patch may not have it. The Put replaces every setting,
	// so it sends the template's, or none when the template has none, which clears it.
	if settings, ok := target[terminationProperty].(map[string]any); ok {
		desired, _ := request.Desired[terminationProperty].(map[string]any)
		if phone, ok := desired[defaultPhoneNumberProperty]; ok {
			settings[defaultPhoneNumberProperty] = phone
		} else {
			delete(settings, defaultPhoneNumberProperty)
		}
	}
	return h.put(ctx, request.Region, request.Identifier, target)
}

// Delete deletes the Voice Connector's termination settings. AWS deletes settings a Voice Connector doesn't have without
// an error, and the delete's progress reports them gone. It returns ErrNotFound when AWS has no such Voice Connector.
func (h *Termination) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeleteVoiceConnectorTermination(ctx, &chimesdkvoice.DeleteVoiceConnectorTerminationInput{VoiceConnectorId: aws.String(identifier)}, inRegion(region))
	return notFoundOrRejected(err)
}

// List fails, because the settings can't be listed.
func (h *Termination) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, cantList(terminationType)
}

// Progress reports a create as done once a read finds the settings, after a few tries, an update as done, because the Put waited until a
// read showed it, and a delete as done, with ErrNotFound, once a read no longer finds them.
func (h *Termination) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	switch op {
	case native.OperationCreate:
		return createProgress(ctx, h.wait, h.Read, region, identifier)
	case native.OperationDelete:
		return deleteProgress(ctx, h.Read, region, identifier)
	}
	return native.Progress{Done: true}, nil
}
