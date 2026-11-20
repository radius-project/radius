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
	"fmt"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// missingPrefix starts the IDs of Voice Connectors AWS doesn't have, and emergencyPrefix the IDs of Voice
	// Connectors with an emergency calling configuration.
	missingPrefix   = "missing"
	emergencyPrefix = "emergency"

	badRequestCode = "BadRequestException"

	// validationCode is the error AWS returns for a value outside the limits of its API model. The SDK doesn't model
	// it, so its fault is unknown.
	validationCode = "ValidationException"

	// cpsQuota is the fake account's quota of calls per second, the default quota.
	cpsQuota = 1

	// maxRoutes is how many origination routes a Voice Connector can have.
	maxRoutes = 10

	// staleReads is how many reads after a Put or a delete still return the earlier settings.
	staleReads = 2
)

// callingRegions are the countries a Voice Connector can call in the fake.
var callingRegions = []string{"US", "CA"}

var (
	phoneNumberPattern = regexp.MustCompile(`^\+?[1-9]\d{1,14}$`)

	// voiceConnectorIDPattern is the form of a Voice Connector ID AWS takes, from its ValidationException.
	voiceConnectorIDPattern = regexp.MustCompile(`^([a-z0-9]{21,22}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)
)

// stale is what reads of a setting return for a while after a Put or a delete changed it: before, or nil for none,
// for reads more reads, while AWS still has after, or nil for none.
type stale struct {
	before map[string]any
	after  map[string]any
	reads  int
}

// fakeAPI is an in-memory Amazon Chime SDK Voice API for a Voice Connector's settings. It keeps them in an awsfake
// store, in their types' property format, so the suite's Cloud Control sees the same resources.
//
// Every Voice Connector exists, except those whose ID starts with missingPrefix. One whose ID starts with
// emergencyPrefix has an emergency calling configuration. A Voice Connector with no logging entry has both logs off.
//
// As AWS does, a read right after a Put or a delete returns the earlier settings, for staleReads reads.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu    sync.Mutex
	calls []string
	stale map[string]*stale

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, stale: map[string]*stale{}, errs: map[string]error{}}
}

// noWait is a pause that doesn't wait.
func noWait(context.Context, time.Duration) {}

// apiError returns the error AWS returns with HTTP status and code.
func apiError(status int, code string, message string) error {
	return &smithy.OperationError{
		ServiceID:     "Chime SDK Voice",
		OperationName: "Call",
		Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
			Err:      &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient},
		},
	}
}

func badRequest(format string, args ...any) error {
	return apiError(http.StatusBadRequest, badRequestCode, fmt.Sprintf(format, args...))
}

// invalid returns AWS's ValidationException. The SDK doesn't model it, so in production its fault is unknown, which
// testdata/errors.json covers. The fake marks it a client fault, as the suite's API rules require.
func invalid(format string, args ...any) error {
	return &smithy.OperationError{
		ServiceID:     "Chime SDK Voice",
		OperationName: "Call",
		Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusBadRequest}},
			Err:      &smithy.GenericAPIError{Code: validationCode, Message: fmt.Sprintf(format, args...), Fault: smithy.FaultClient},
		},
	}
}

func notFound(format string, args ...any) error {
	return apiError(http.StatusNotFound, notFoundCode, fmt.Sprintf(format, args...))
}

// missing returns the error the SDK returns, before it sends anything, for a call without a required parameter.
func missing(operation string, parameter string) error {
	err := smithy.InvalidParamsError{Context: operation}
	err.Add(smithy.NewErrParamRequired(parameter))
	return err
}

// begin records a call and returns the error errs sets for it.
func (f *fakeAPI) begin(method string) error {
	f.calls = append(f.calls, method)
	return f.errs[method]
}

// voiceConnector returns the error AWS returns for a call on the Voice Connector with id, or nil when AWS has it.
func voiceConnector(id *string, operation string) error {
	switch {
	case id == nil:
		return missing(operation, "VoiceConnectorId")
	case !voiceConnectorIDPattern.MatchString(*id):
		return invalid("Value '%s' at 'voiceConnectorId' failed to satisfy constraint", *id)
	case strings.HasPrefix(*id, missingPrefix):
		return notFound("Could not find VoiceConnector with Id '%s'", *id)
	}
	return nil
}

// read returns the settings of typeName for the Voice Connector with id as a read sees them, and whether there are
// any.
func (f *fakeAPI) read(typeName string, id string) (map[string]any, bool) {
	current, ok := f.store.Read(typeName, id)
	key := typeName + "|" + id
	if s := f.stale[key]; s != nil {
		if s.reads > 0 && reflect.DeepEqual(current, s.after) {
			s.reads--
			return s.before, s.before != nil
		}
		delete(f.stale, key)
	}
	return current, ok
}

// write sets the settings of typeName for the Voice Connector with id to props, or deletes them when props is nil.
// Reads return the earlier settings for staleReads reads.
func (f *fakeAPI) write(typeName string, id string, props map[string]any) {
	before, _ := f.store.Read(typeName, id)
	if props == nil {
		f.store.Remove(typeName, id)
	} else {
		f.store.Put(typeName, id, props)
	}
	after, _ := f.store.Read(typeName, id)
	f.stale[typeName+"|"+id] = &stale{before: before, after: after, reads: staleReads}
}

// decode sets out, an AWS SDK struct, from the settings of props under name.
func decode(props map[string]any, name string, out any) {
	settings, _ := props[name].(map[string]any)
	_ = awsapi.Decode(settings, out)
}

// encode returns the properties of settings, an AWS SDK struct, for the Voice Connector with id, under name.
func encode(id string, name string, settings any) map[string]any {
	props, _ := properties(id, name, settings)
	return props
}

func (f *fakeAPI) GetVoiceConnector(ctx context.Context, in *chimesdkvoice.GetVoiceConnectorInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetVoiceConnector"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "GetVoiceConnectorInput"); err != nil {
		return nil, err
	}
	return &chimesdkvoice.GetVoiceConnectorOutput{VoiceConnector: &types.VoiceConnector{VoiceConnectorId: in.VoiceConnectorId}}, nil
}

func (f *fakeAPI) GetVoiceConnectorLoggingConfiguration(ctx context.Context, in *chimesdkvoice.GetVoiceConnectorLoggingConfigurationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorLoggingConfigurationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetVoiceConnectorLoggingConfiguration"); err != nil {
		return nil, err
	}
	if in.VoiceConnectorId == nil {
		return nil, missing("GetVoiceConnectorLoggingConfigurationInput", "VoiceConnectorId")
	}
	// AWS returns both logs off for a Voice Connector it doesn't have, as for one nobody set.
	settings := types.LoggingConfiguration{EnableSIPLogs: aws.Bool(false), EnableMediaMetricLogs: aws.Bool(false)}
	if props, ok := f.read(loggingType, *in.VoiceConnectorId); ok && !strings.HasPrefix(*in.VoiceConnectorId, missingPrefix) {
		decode(props, loggingProperty, &settings)
	}
	return &chimesdkvoice.GetVoiceConnectorLoggingConfigurationOutput{LoggingConfiguration: &settings}, nil
}

func (f *fakeAPI) PutVoiceConnectorLoggingConfiguration(ctx context.Context, in *chimesdkvoice.PutVoiceConnectorLoggingConfigurationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorLoggingConfigurationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutVoiceConnectorLoggingConfiguration"); err != nil {
		return nil, err
	}
	if in.LoggingConfiguration == nil {
		return nil, missing("PutVoiceConnectorLoggingConfigurationInput", "LoggingConfiguration")
	}
	if err := voiceConnector(in.VoiceConnectorId, "PutVoiceConnectorLoggingConfigurationInput"); err != nil {
		return nil, err
	}
	// A Put replaces both logs: one it leaves out is off. It returns only what it was sent.
	stored := types.LoggingConfiguration{EnableSIPLogs: aws.Bool(aws.ToBool(in.LoggingConfiguration.EnableSIPLogs)), EnableMediaMetricLogs: aws.Bool(aws.ToBool(in.LoggingConfiguration.EnableMediaMetricLogs))}
	f.write(loggingType, *in.VoiceConnectorId, encode(*in.VoiceConnectorId, loggingProperty, stored))
	sent := *in.LoggingConfiguration
	return &chimesdkvoice.PutVoiceConnectorLoggingConfigurationOutput{LoggingConfiguration: &sent}, nil
}

func (f *fakeAPI) GetVoiceConnectorStreamingConfiguration(ctx context.Context, in *chimesdkvoice.GetVoiceConnectorStreamingConfigurationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorStreamingConfigurationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetVoiceConnectorStreamingConfiguration"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "GetVoiceConnectorStreamingConfigurationInput"); err != nil {
		return nil, err
	}
	props, ok := f.read(streamingType, *in.VoiceConnectorId)
	if !ok {
		return nil, notFound("Could not find StreamingConfiguration for Voice Connector %s", *in.VoiceConnectorId)
	}
	var settings types.StreamingConfiguration
	decode(props, streamingProperty, &settings)
	return &chimesdkvoice.GetVoiceConnectorStreamingConfigurationOutput{StreamingConfiguration: &settings}, nil
}

// checkStreaming returns the error AWS returns for streaming settings it doesn't take.
func checkStreaming(settings *types.StreamingConfiguration) error {
	switch {
	case settings == nil:
		return missing("PutVoiceConnectorStreamingConfigurationInput", "StreamingConfiguration")
	case settings.DataRetentionInHours == nil:
		return missing("PutVoiceConnectorStreamingConfigurationInput", "StreamingConfiguration.DataRetentionInHours")
	case settings.Disabled == nil:
		return missing("PutVoiceConnectorStreamingConfigurationInput", "StreamingConfiguration.Disabled")
	case *settings.DataRetentionInHours < 0:
		return invalid("DataRetentionInHours must have value greater than or equal to 0")
	case settings.StreamingNotificationTargets != nil && (len(settings.StreamingNotificationTargets) < 1 || len(settings.StreamingNotificationTargets) > 3):
		return invalid("StreamingNotificationTargets must have length from 1 to 3")
	}
	for _, target := range settings.StreamingNotificationTargets {
		if target.NotificationTarget != "" && !slices.Contains(target.NotificationTarget.Values(), target.NotificationTarget) {
			return invalid("NotificationTarget must satisfy enum value set")
		}
	}
	return nil
}

func (f *fakeAPI) PutVoiceConnectorStreamingConfiguration(ctx context.Context, in *chimesdkvoice.PutVoiceConnectorStreamingConfigurationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorStreamingConfigurationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutVoiceConnectorStreamingConfiguration"); err != nil {
		return nil, err
	}
	if err := checkStreaming(in.StreamingConfiguration); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "PutVoiceConnectorStreamingConfigurationInput"); err != nil {
		return nil, err
	}
	settings := *in.StreamingConfiguration
	// AWS sends streaming notifications to EventBridge when a Put leaves the targets out.
	if len(settings.StreamingNotificationTargets) == 0 {
		settings.StreamingNotificationTargets = []types.StreamingNotificationTarget{{NotificationTarget: types.NotificationTargetEventBridge}}
	}
	f.write(streamingType, *in.VoiceConnectorId, encode(*in.VoiceConnectorId, streamingProperty, settings))
	return &chimesdkvoice.PutVoiceConnectorStreamingConfigurationOutput{StreamingConfiguration: &settings}, nil
}

// deleteSettings deletes the settings of typeName for the Voice Connector with id. AWS deletes settings a Voice
// Connector doesn't have without an error.
func (f *fakeAPI) deleteSettings(typeName string, id string) {
	if _, ok := f.store.Read(typeName, id); ok {
		f.write(typeName, id, nil)
	}
}

func (f *fakeAPI) DeleteVoiceConnectorStreamingConfiguration(ctx context.Context, in *chimesdkvoice.DeleteVoiceConnectorStreamingConfigurationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.DeleteVoiceConnectorStreamingConfigurationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteVoiceConnectorStreamingConfiguration"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "DeleteVoiceConnectorStreamingConfigurationInput"); err != nil {
		return nil, err
	}
	f.deleteSettings(streamingType, *in.VoiceConnectorId)
	return &chimesdkvoice.DeleteVoiceConnectorStreamingConfigurationOutput{}, nil
}

func (f *fakeAPI) GetVoiceConnectorOrigination(ctx context.Context, in *chimesdkvoice.GetVoiceConnectorOriginationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorOriginationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetVoiceConnectorOrigination"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "GetVoiceConnectorOriginationInput"); err != nil {
		return nil, err
	}
	props, ok := f.read(originationType, *in.VoiceConnectorId)
	if !ok {
		return nil, notFound("Could not find Origination for VoiceConnector %s", *in.VoiceConnectorId)
	}
	var settings types.Origination
	decode(props, originationProperty, &settings)
	return &chimesdkvoice.GetVoiceConnectorOriginationOutput{Origination: &settings}, nil
}

// checkOrigination returns the error AWS returns for origination settings it doesn't take.
func checkOrigination(settings *types.Origination) error {
	switch {
	case settings == nil:
		return missing("PutVoiceConnectorOriginationInput", "Origination")
	case settings.Routes == nil:
		return badRequest("Value null at 'routes' failed to satisfy constraint: Member must not be null")
	case len(settings.Routes) < 1:
		return badRequest("Must have at least one Origination Route")
	case len(settings.Routes) > maxRoutes:
		return badRequest("Cannot have more than %d Origination Routes", maxRoutes)
	}
	for _, route := range settings.Routes {
		switch {
		case route.Host == nil || route.Weight == nil || route.Priority == nil:
			return badRequest("An origination route must set Host, Weight, and Priority")
		case route.Port != nil && (*route.Port < 0 || *route.Port > 65535):
			return invalid("Port must have value from 0 to 65535")
		case *route.Priority < 1 || *route.Priority > 100:
			return invalid("Priority must have value from 1 to 100")
		case *route.Weight < 1 || *route.Weight > 100:
			return invalid("Weight must have value from 1 to 100")
		case route.Protocol != "" && !slices.Contains(route.Protocol.Values(), route.Protocol):
			return invalid("Protocol must satisfy enum value set")
		}
	}
	return nil
}

func (f *fakeAPI) PutVoiceConnectorOrigination(ctx context.Context, in *chimesdkvoice.PutVoiceConnectorOriginationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorOriginationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutVoiceConnectorOrigination"); err != nil {
		return nil, err
	}
	if err := checkOrigination(in.Origination); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "PutVoiceConnectorOriginationInput"); err != nil {
		return nil, err
	}
	settings := *in.Origination
	settings.Routes = slices.Clone(settings.Routes)
	for i := range settings.Routes {
		// AWS reports a port the Put leaves out as 0.
		if settings.Routes[i].Port == nil {
			settings.Routes[i].Port = aws.Int32(0)
		}
	}
	if settings.Disabled == nil {
		settings.Disabled = aws.Bool(false)
	}
	f.write(originationType, *in.VoiceConnectorId, encode(*in.VoiceConnectorId, originationProperty, settings))
	return &chimesdkvoice.PutVoiceConnectorOriginationOutput{Origination: &settings}, nil
}

// emergencyCalling returns the error AWS returns for deleting settings of a Voice Connector with an emergency calling
// configuration.
func emergencyCalling(id string) error {
	if strings.HasPrefix(id, emergencyPrefix) {
		return badRequest("Delete the emergency calling configuration of Voice Connector %s first", id)
	}
	return nil
}

func (f *fakeAPI) DeleteVoiceConnectorOrigination(ctx context.Context, in *chimesdkvoice.DeleteVoiceConnectorOriginationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.DeleteVoiceConnectorOriginationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteVoiceConnectorOrigination"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "DeleteVoiceConnectorOriginationInput"); err != nil {
		return nil, err
	}
	if err := emergencyCalling(*in.VoiceConnectorId); err != nil {
		return nil, err
	}
	f.deleteSettings(originationType, *in.VoiceConnectorId)
	return &chimesdkvoice.DeleteVoiceConnectorOriginationOutput{}, nil
}

func (f *fakeAPI) GetVoiceConnectorTermination(ctx context.Context, in *chimesdkvoice.GetVoiceConnectorTerminationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.GetVoiceConnectorTerminationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetVoiceConnectorTermination"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "GetVoiceConnectorTerminationInput"); err != nil {
		return nil, err
	}
	props, ok := f.read(terminationType, *in.VoiceConnectorId)
	if !ok {
		return nil, notFound("Could not find Termination for VoiceConnector %s", *in.VoiceConnectorId)
	}
	var settings types.Termination
	decode(props, terminationProperty, &settings)
	return &chimesdkvoice.GetVoiceConnectorTerminationOutput{Termination: &settings}, nil
}

// checkTermination returns the error AWS returns for termination settings it doesn't take.
func checkTermination(settings *types.Termination) error {
	switch {
	case settings == nil:
		return missing("PutVoiceConnectorTerminationInput", "Termination")
	case len(settings.CallingRegions) == 0:
		return badRequest("Cannot put termination without regions")
	case len(settings.CidrAllowedList) == 0:
		return badRequest("Cannot put termination without CidrAllowedList")
	case settings.CpsLimit != nil && *settings.CpsLimit < 1:
		return invalid("CpsLimit must have value greater than or equal to 1")
	case settings.CpsLimit != nil && *settings.CpsLimit > cpsQuota:
		return badRequest("Provided CPS limit (%d) exceeded allowable limit (%d)", *settings.CpsLimit, cpsQuota)
	case settings.DefaultPhoneNumber != nil && !phoneNumberPattern.MatchString(*settings.DefaultPhoneNumber):
		return invalid("DefaultPhoneNumber must satisfy regular expression pattern")
	}
	for _, region := range settings.CallingRegions {
		if !slices.Contains(callingRegions, region) {
			return badRequest("Not all calling region are not allowed")
		}
	}
	for _, cidr := range settings.CidrAllowedList {
		ip, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return badRequest("Invalid CIDR address: %s", cidr)
		}
		if ones, _ := network.Mask.Size(); ones < 27 {
			return badRequest("Invalid CIDR address: %s must have a minimum netmask of 27", cidr)
		}
		if ip.IsPrivate() {
			return badRequest("Invalid CIDR allowed list: %s is a private IP.", cidr)
		}
	}
	return nil
}

func (f *fakeAPI) PutVoiceConnectorTermination(ctx context.Context, in *chimesdkvoice.PutVoiceConnectorTerminationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorTerminationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutVoiceConnectorTermination"); err != nil {
		return nil, err
	}
	if err := checkTermination(in.Termination); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "PutVoiceConnectorTerminationInput"); err != nil {
		return nil, err
	}
	settings := *in.Termination
	if settings.CpsLimit == nil {
		settings.CpsLimit = aws.Int32(1)
	}
	if settings.Disabled == nil {
		settings.Disabled = aws.Bool(false)
	}
	f.write(terminationType, *in.VoiceConnectorId, encode(*in.VoiceConnectorId, terminationProperty, settings))
	return &chimesdkvoice.PutVoiceConnectorTerminationOutput{Termination: &settings}, nil
}

func (f *fakeAPI) DeleteVoiceConnectorTermination(ctx context.Context, in *chimesdkvoice.DeleteVoiceConnectorTerminationInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.DeleteVoiceConnectorTerminationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteVoiceConnectorTermination"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "DeleteVoiceConnectorTerminationInput"); err != nil {
		return nil, err
	}
	if err := emergencyCalling(*in.VoiceConnectorId); err != nil {
		return nil, err
	}
	f.deleteSettings(terminationType, *in.VoiceConnectorId)
	return &chimesdkvoice.DeleteVoiceConnectorTerminationOutput{}, nil
}
