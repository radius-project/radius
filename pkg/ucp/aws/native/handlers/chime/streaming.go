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
	streamingType     = "AWS::Chime::VoiceConnectorStreamingConfiguration"
	streamingProperty = "StreamingConfiguration"
)

var _ native.Handler = (*StreamingConfiguration)(nil)

// StreamingConfiguration serves AWS::Chime::VoiceConnectorStreamingConfiguration, a Voice Connector's media streaming
// settings. A Voice Connector has none until a Put sets them, and a delete deletes them.
type StreamingConfiguration struct {
	api  API
	wait pause
}

// NewStreamingConfiguration returns the handler for a Voice Connector's streaming settings.
func NewStreamingConfiguration(api API) *StreamingConfiguration {
	return &StreamingConfiguration{api: api, wait: sleep}
}

// TypeName returns AWS::Chime::VoiceConnectorStreamingConfiguration.
func (h *StreamingConfiguration) TypeName() string {
	return streamingType
}

// Options returns how UCP uses the handler.
func (h *StreamingConfiguration) Options() native.Options {
	return options()
}

// Read returns the Voice Connector's streaming settings, or ErrNotFound when it has none or AWS has no such Voice
// Connector.
func (h *StreamingConfiguration) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.api.GetVoiceConnectorStreamingConfiguration(ctx, &chimesdkvoice.GetVoiceConnectorStreamingConfigurationInput{VoiceConnectorId: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, notFound(err)
	}
	return properties(identifier, streamingProperty, out.StreamingConfiguration), nil
}

// put sets the Voice Connector's streaming settings to those properties set, and waits until a read shows them.
func (h *StreamingConfiguration) put(ctx context.Context, region string, identifier string, props map[string]any) error {
	var settings types.StreamingConfiguration
	if err := settingsOf(props, streamingProperty, &settings); err != nil {
		return err
	}
	out, err := h.api.PutVoiceConnectorStreamingConfiguration(ctx, &chimesdkvoice.PutVoiceConnectorStreamingConfigurationInput{
		VoiceConnectorId:       aws.String(identifier),
		StreamingConfiguration: &settings,
	}, inRegion(region))
	if err != nil {
		return sendFailed(err)
	}
	return confirm(ctx, h.wait, h.Read, region, identifier, properties(identifier, streamingProperty, out.StreamingConfiguration))
}

// Create sets the Voice Connector's streaming settings to the template's and returns the Voice Connector's ID.
func (h *StreamingConfiguration) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, _ := request.Properties[voiceConnectorIDProperty].(string)
	if err := h.put(ctx, request.Region, identifier, request.Properties); err != nil {
		return "", err
	}
	return identifier, nil
}

// Update sets the Voice Connector's streaming settings to the template's. A value the template no longer sets is left
// out of the Put, which clears it or sets it back to AWS's default.
func (h *StreamingConfiguration) Update(ctx context.Context, request native.UpdateRequest) error {
	if err := settingsOf(request.Desired, streamingProperty, &types.StreamingConfiguration{}); err != nil {
		return err
	}
	target, err := patched(request)
	if err != nil {
		return err
	}
	return h.put(ctx, request.Region, request.Identifier, target)
}

// Delete deletes the Voice Connector's streaming settings. AWS deletes settings a Voice Connector doesn't have without
// an error, and the delete's progress reports them gone. It returns ErrNotFound when AWS has no such Voice Connector.
func (h *StreamingConfiguration) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeleteVoiceConnectorStreamingConfiguration(ctx, &chimesdkvoice.DeleteVoiceConnectorStreamingConfigurationInput{VoiceConnectorId: aws.String(identifier)}, inRegion(region))
	return notFoundOrRejected(err)
}

// List fails, because the settings can't be listed.
func (h *StreamingConfiguration) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, cantList(streamingType)
}

// Progress reports a create as done once a read finds the settings, after a few tries, an update as done, because the Put waited until a
// read showed it, and a delete as done, with ErrNotFound, once a read no longer finds them.
func (h *StreamingConfiguration) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	switch op {
	case native.OperationCreate:
		return createProgress(ctx, h.wait, h.Read, region, identifier)
	case native.OperationDelete:
		return deleteProgress(ctx, h.Read, region, identifier)
	}
	return native.Progress{Done: true}, nil
}
