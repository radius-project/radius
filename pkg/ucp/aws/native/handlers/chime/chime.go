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

// Package chime has the native handlers for settings of an Amazon Chime SDK Voice Connector that have no
// CloudFormation type: its logging (AWS::Chime::VoiceConnectorLoggingConfiguration), streaming
// (AWS::Chime::VoiceConnectorStreamingConfiguration), origination (AWS::Chime::VoiceConnectorOrigination), and
// termination (AWS::Chime::VoiceConnectorTermination) settings. Each is a setting of a Voice Connector another
// template creates, such as an AWS::Chime::VoiceConnector, and its identifier is the Voice Connector's ID. The handlers
// call the Amazon Chime SDK Voice API, not the older Amazon Chime API.
//
// Every Put call returns the settings it applied, but a Get right after it can still return the earlier settings, or
// none for settings the Put just created, for about a second. So a Put waits until a Get returns what the Put applied,
// for up to confirmReads reads, and a delete waits until a Get no longer finds the settings. Without that, a deploy
// right after another would read the earlier settings and send the same Put again.
package chime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/smithy-go"
	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// voiceConnectorIDProperty is the property that names the Voice Connector. Its value is every type's identifier.
	voiceConnectorIDProperty = "VoiceConnectorId"

	// notFoundCode is the error Amazon Chime SDK Voice returns for a Voice Connector it doesn't have, and for settings
	// a Voice Connector doesn't have.
	notFoundCode = "NotFoundException"

	// confirmReads is how many times a Put reads the settings before it stops waiting for a read to show them, and
	// confirmPause is how long it waits between reads.
	confirmReads = 20
	confirmPause = 500 * time.Millisecond

	// timeout is how long UCP waits for any operation. AWS shows a Put or a delete in about a second.
	timeout = 10 * time.Minute
)

// API is the part of the Amazon Chime SDK Voice client the handlers use.
type API interface {
	GetVoiceConnector(ctx context.Context, params *chimesdkvoice.GetVoiceConnectorInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorOutput, error)
	GetVoiceConnectorLoggingConfiguration(ctx context.Context, params *chimesdkvoice.GetVoiceConnectorLoggingConfigurationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorLoggingConfigurationOutput, error)
	PutVoiceConnectorLoggingConfiguration(ctx context.Context, params *chimesdkvoice.PutVoiceConnectorLoggingConfigurationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorLoggingConfigurationOutput, error)
	GetVoiceConnectorStreamingConfiguration(ctx context.Context, params *chimesdkvoice.GetVoiceConnectorStreamingConfigurationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorStreamingConfigurationOutput, error)
	PutVoiceConnectorStreamingConfiguration(ctx context.Context, params *chimesdkvoice.PutVoiceConnectorStreamingConfigurationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorStreamingConfigurationOutput, error)
	DeleteVoiceConnectorStreamingConfiguration(ctx context.Context, params *chimesdkvoice.DeleteVoiceConnectorStreamingConfigurationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.DeleteVoiceConnectorStreamingConfigurationOutput, error)
	GetVoiceConnectorOrigination(ctx context.Context, params *chimesdkvoice.GetVoiceConnectorOriginationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorOriginationOutput, error)
	PutVoiceConnectorOrigination(ctx context.Context, params *chimesdkvoice.PutVoiceConnectorOriginationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorOriginationOutput, error)
	DeleteVoiceConnectorOrigination(ctx context.Context, params *chimesdkvoice.DeleteVoiceConnectorOriginationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.DeleteVoiceConnectorOriginationOutput, error)
	GetVoiceConnectorTermination(ctx context.Context, params *chimesdkvoice.GetVoiceConnectorTerminationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorTerminationOutput, error)
	PutVoiceConnectorTermination(ctx context.Context, params *chimesdkvoice.PutVoiceConnectorTerminationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorTerminationOutput, error)
	DeleteVoiceConnectorTermination(ctx context.Context, params *chimesdkvoice.DeleteVoiceConnectorTerminationInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.DeleteVoiceConnectorTerminationOutput, error)
}

var inRegion = awsapi.InRegion[chimesdkvoice.Options]

// pause waits for d, or until ctx ends. Tests replace it, so they don't wait.
type pause func(ctx context.Context, d time.Duration)

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// reader reads the settings of the Voice Connector with the identifier, as a handler's Read does.
type reader func(ctx context.Context, region string, identifier string) (map[string]any, error)

// notFoundOrRejected returns err as ErrNotFound for a Voice Connector, or settings, AWS doesn't have, and otherwise as
// awsapi.Rejected does.
func notFoundOrRejected(err error) error {
	return awsapi.Rejected(awsapi.NotFound(err, notFoundCode))
}

// sendFailed returns the error of a call that changes AWS. An error the SDK returns before it sends the call, for a
// required parameter the call leaves out, is a client failure AWS applied none of, as awsapi.Rejected makes a 4xx.
func sendFailed(err error) error {
	var params smithy.InvalidParamsError
	if errors.As(err, &params) {
		return rejected("%s", err.Error())
	}
	return awsapi.Rejected(err)
}

// rejected returns a client failure AWS applied none of, with message.
func rejected(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// settingsOf sets out, an AWS SDK struct, from the settings object properties set under name. It returns a client
// failure when they set none, such as a template that leaves out StreamingConfiguration.
func settingsOf(properties map[string]any, name string, out any) error {
	settings, ok := properties[name].(map[string]any)
	if !ok {
		return rejected("set %s", name)
	}
	if err := awsapi.Decode(settings, out); err != nil {
		return rejected("%s: %s", name, err.Error())
	}
	return nil
}

// properties returns the properties of a Read: the Voice Connector's ID, and settings, an AWS SDK struct, under name.
// EncodeObject fails only for a value JSON can't encode, and an SDK struct always encodes.
func properties(identifier string, name string, settings any) map[string]any {
	encoded, _ := awsapi.EncodeObject(settings)
	return map[string]any{voiceConnectorIDProperty: identifier, name: encoded}
}

// patched returns the properties an update sends: the settings AWS has, with the update's patch applied. A Put
// replaces all of the settings, so values AWS filled in that the template never set, such as a CpsLimit of 1, are sent
// as AWS has them, and a value the template no longer sets is left out, which clears it.
func patched(request native.UpdateRequest) (map[string]any, error) {
	// Both are JSON values already, so they always encode.
	current, _ := json.Marshal(request.Current)
	ops, _ := json.Marshal(request.Patch)
	patch, err := jsonpatch.DecodePatch(ops)
	if err == nil {
		current, err = patch.Apply(current)
	}
	if err != nil {
		return nil, &native.Failure{Message: "UCP's update doesn't apply to the settings AWS has: " + err.Error() + ". Deploy again.", Outcome: native.OutcomeRejected}
	}
	out := map[string]any{}
	_ = json.Unmarshal(current, &out)
	return out, nil
}

// confirm reads the settings until a read returns want, what the Put returned, so the next read shows the Put, for up
// to confirmReads reads. AWS accepted the Put, but until a read shows it, a later deploy could read the earlier
// settings and keep, or send again, a value the template removed. So when no read shows it, confirm returns a failure
// whose outcome is unknown, which keeps the template's values as possibly applied: the next deploy sends the
// template's value at each of them.
func confirm(ctx context.Context, wait pause, read reader, region string, identifier string, want map[string]any) error {
	var err error
	for i := range confirmReads {
		if i > 0 {
			wait(ctx, confirmPause)
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
		var got map[string]any
		if got, err = read(ctx, region, identifier); err == nil && reflect.DeepEqual(got, want) {
			return nil
		}
	}
	message := fmt.Sprintf("AWS accepted the change to the settings of Voice Connector %q, but no read showed it within %s, so UCP can't tell what AWS has. Deploy again.", identifier, confirmReads*confirmPause)
	if err != nil {
		message += " The last read failed: " + err.Error()
	}
	return &native.Failure{Message: message, Outcome: native.OutcomeUnknown}
}

// voiceConnectorExists reports whether AWS has the Voice Connector with id.
func voiceConnectorExists(ctx context.Context, api API, region string, id string) (bool, error) {
	_, err := api.GetVoiceConnector(ctx, &chimesdkvoice.GetVoiceConnectorInput{VoiceConnectorId: aws.String(id)}, inRegion(region))
	err = notFoundOrRejected(err)
	if errors.Is(err, native.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// createProgress reads how far a create of settings has got: it is done once read finds them. AWS shows a Put about a
// second after it accepts it, and a create whose response UCP lost never waited for that, so read gets up to
// confirmReads tries before createProgress returns ErrNotFound, which makes UCP send the create again, or a DELETE
// remove only the record.
func createProgress(ctx context.Context, wait pause, read reader, region string, identifier string) (native.Progress, error) {
	var err error
	for i := range confirmReads {
		if i > 0 {
			wait(ctx, confirmPause)
		}
		if ctx.Err() != nil {
			return native.Progress{}, ctx.Err()
		}
		if _, err = read(ctx, region, identifier); !errors.Is(err, native.ErrNotFound) {
			break
		}
	}
	if err != nil {
		return native.Progress{}, err
	}
	return native.Progress{Done: true}, nil
}

// deleteProgress reads how far a delete of settings has got. It returns ErrNotFound, which finishes the delete, once
// read no longer finds them.
func deleteProgress(ctx context.Context, read reader, region string, identifier string) (native.Progress, error) {
	if _, err := read(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	return native.Progress{State: "present", Waiting: "absent"}, nil
}

// cantList returns the error of List for typeName, which can't be listed.
func cantList(typeName string) error {
	return fmt.Errorf("the AWS type %s can't be listed", typeName)
}

// options returns the options of every handler in the package: settings can't be listed, and their identifier, the
// Voice Connector's ID, is known before the create.
func options() native.Options {
	return native.Options{
		List:          native.ListNone,
		Recovery:      native.RecoverByIdentifier,
		CreateTimeout: timeout,
		UpdateTimeout: timeout,
		DeleteTimeout: timeout,
	}
}
