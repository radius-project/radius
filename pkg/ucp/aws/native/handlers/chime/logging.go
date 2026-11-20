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
	"reflect"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	loggingType     = "AWS::Chime::VoiceConnectorLoggingConfiguration"
	loggingProperty = "LoggingConfiguration"
)

var (
	_ native.Handler  = (*LoggingConfiguration)(nil)
	_ native.Resetter = (*LoggingConfiguration)(nil)
)

// LoggingConfiguration serves AWS::Chime::VoiceConnectorLoggingConfiguration, whether a Voice Connector sends SIP
// message logs and media metric logs to CloudWatch Logs. Every Voice Connector has the setting, so UCP takes it over
// and updates it, and the type's metadata makes a delete call Reset, which turns both logs off, AWS's default.
//
// GetVoiceConnectorLoggingConfiguration returns both logs off for a Voice Connector AWS doesn't have, instead of an
// error, so Read checks the Voice Connector first. Otherwise UCP would report a deleted Voice Connector's setting as
// there, and a delete would never finish.
type LoggingConfiguration struct {
	api  API
	wait pause
}

// NewLoggingConfiguration returns the handler for a Voice Connector's logging setting.
func NewLoggingConfiguration(api API) *LoggingConfiguration {
	return &LoggingConfiguration{api: api, wait: sleep}
}

// TypeName returns AWS::Chime::VoiceConnectorLoggingConfiguration.
func (h *LoggingConfiguration) TypeName() string {
	return loggingType
}

// Options returns how UCP uses the handler.
func (h *LoggingConfiguration) Options() native.Options {
	return options()
}

// Read returns the Voice Connector's logging setting, or ErrNotFound when AWS has no such Voice Connector.
func (h *LoggingConfiguration) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	exists, err := voiceConnectorExists(ctx, h.api, region, identifier)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, native.ErrNotFound
	}
	out, err := h.api.GetVoiceConnectorLoggingConfiguration(ctx, &chimesdkvoice.GetVoiceConnectorLoggingConfigurationInput{VoiceConnectorId: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, notFoundOrRejected(err)
	}
	return properties(identifier, loggingProperty, loggingOf(out.LoggingConfiguration))
}

// loggingOf returns settings with both logs set, off where AWS leaves one out, so a template that leaves one out
// matches AWS.
func loggingOf(settings *types.LoggingConfiguration) types.LoggingConfiguration {
	out := types.LoggingConfiguration{EnableSIPLogs: aws.Bool(false), EnableMediaMetricLogs: aws.Bool(false)}
	if settings != nil {
		if settings.EnableSIPLogs != nil {
			out.EnableSIPLogs = settings.EnableSIPLogs
		}
		if settings.EnableMediaMetricLogs != nil {
			out.EnableMediaMetricLogs = settings.EnableMediaMetricLogs
		}
	}
	return out
}

// put sets the Voice Connector's logging to settings, sending both logs, off where settings leave one out, and waits
// until a read shows it.
func (h *LoggingConfiguration) put(ctx context.Context, region string, identifier string, settings types.LoggingConfiguration) error {
	settings = loggingOf(&settings)
	out, err := h.api.PutVoiceConnectorLoggingConfiguration(ctx, &chimesdkvoice.PutVoiceConnectorLoggingConfigurationInput{
		VoiceConnectorId:     aws.String(identifier),
		LoggingConfiguration: &settings,
	}, inRegion(region))
	if err != nil {
		return err
	}
	want, err := properties(identifier, loggingProperty, loggingOf(out.LoggingConfiguration))
	if err != nil {
		return err
	}
	confirm(ctx, h.wait, h.Read, region, identifier, want)
	return nil
}

// Create sets the Voice Connector's logging to the template's and returns the Voice Connector's ID. UCP calls it only
// for a Voice Connector AWS doesn't have, so AWS rejects it.
func (h *LoggingConfiguration) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	var settings types.LoggingConfiguration
	if err := settingsOf(request.Properties, loggingProperty, &settings); err != nil {
		return "", err
	}
	identifier, _ := request.Properties[voiceConnectorIDProperty].(string)
	if err := h.put(ctx, request.Region, identifier, settings); err != nil {
		return "", awsapi.Rejected(err)
	}
	return identifier, nil
}

// Update sets the Voice Connector's logging to the template's. A log the template no longer sets is turned off.
func (h *LoggingConfiguration) Update(ctx context.Context, request native.UpdateRequest) error {
	var settings types.LoggingConfiguration
	if err := settingsOf(request.Desired, loggingProperty, &settings); err != nil {
		return err
	}
	target, err := patched(request)
	if err != nil {
		return err
	}
	if err := settingsOf(target, loggingProperty, &settings); err != nil {
		return err
	}
	return awsapi.Rejected(h.put(ctx, request.Region, request.Identifier, settings))
}

// Reset turns both logs off, AWS's default. It returns ErrNotFound when AWS has no such Voice Connector.
func (h *LoggingConfiguration) Reset(ctx context.Context, region string, identifier string) error {
	return notFoundOrRejected(h.put(ctx, region, identifier, types.LoggingConfiguration{}))
}

// Delete resets the setting, as Reset does. The type's metadata makes UCP call Reset.
func (h *LoggingConfiguration) Delete(ctx context.Context, region string, identifier string) error {
	return h.Reset(ctx, region, identifier)
}

// List fails, because the setting can't be listed.
func (h *LoggingConfiguration) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, cantList(loggingType)
}

// Progress reports a create or update as done, because the Put waited until a read showed it. A delete, a reset, is
// done once both logs read as off, and returns ErrNotFound when the Voice Connector is gone.
func (h *LoggingConfiguration) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op != native.OperationDelete {
		return native.Progress{Done: true}, nil
	}
	current, err := h.Read(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	off, err := properties(identifier, loggingProperty, loggingOf(nil))
	if err != nil {
		return native.Progress{}, err
	}
	if reflect.DeepEqual(current, off) {
		return native.Progress{Done: true}, nil
	}
	return native.Progress{State: "on", Waiting: "off"}, nil
}
