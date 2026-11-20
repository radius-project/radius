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

// Package chimevoice has the native handler for the SIP credentials of an Amazon Chime SDK Voice Connector
// (AWS::Chime::VoiceConnectorTerminationCredentials), which have no CloudFormation type. The handler calls the Amazon
// Chime SDK Voice API, not the older Amazon Chime API.
//
// A list of the credentials right after a Put or a delete can still return the earlier usernames for about a second.
// So a Put or a delete waits until a list shows it, for up to confirmReads lists. Without that, a deploy right after
// another would read the earlier usernames and send the same change again.
package chimevoice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// notFoundCode is the error Amazon Chime SDK Voice returns for a Voice Connector it doesn't have.
	notFoundCode = "NotFoundException"

	// confirmReads is how many times a Put or a delete lists the credentials before it stops waiting for a list to
	// show it, and confirmPause is how long it waits between lists.
	confirmReads = 20
	confirmPause = 500 * time.Millisecond
)

// API is the part of the Amazon Chime SDK Voice client the handler uses.
type API interface {
	PutVoiceConnectorTerminationCredentials(ctx context.Context, params *chimesdkvoice.PutVoiceConnectorTerminationCredentialsInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorTerminationCredentialsOutput, error)
	ListVoiceConnectorTerminationCredentials(ctx context.Context, params *chimesdkvoice.ListVoiceConnectorTerminationCredentialsInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.ListVoiceConnectorTerminationCredentialsOutput, error)
	DeleteVoiceConnectorTerminationCredentials(ctx context.Context, params *chimesdkvoice.DeleteVoiceConnectorTerminationCredentialsInput, optFns ...func(*chimesdkvoice.Options)) (*chimesdkvoice.DeleteVoiceConnectorTerminationCredentialsOutput, error)
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

// notFound returns err as ErrNotFound for a Voice Connector AWS doesn't have, and err otherwise. Reads use it and not
// awsapi.Rejected, because the worker takes a *native.Failure from Progress as the operation's end: a throttled read
// must leave the operation waiting, not fail it as rejected.
func notFound(err error) error {
	return awsapi.NotFound(err, notFoundCode)
}

// notFoundOrRejected returns err as ErrNotFound for a Voice Connector AWS doesn't have, and otherwise as
// awsapi.Rejected does, for a call that changes AWS.
func notFoundOrRejected(err error) error {
	return awsapi.Rejected(awsapi.NotFound(err, notFoundCode))
}

// rejected returns a client failure AWS applied none of, with message.
func rejected(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// patched returns the properties an update applies: the request's current properties with its patch applied. They
// are the template's, without the write-only values UCP leaves out because their fingerprints show they didn't
// change.
func patched(request native.UpdateRequest) (map[string]any, error) {
	current, err := json.Marshal(request.Current)
	if err != nil {
		return nil, err
	}
	ops, err := json.Marshal(request.Patch)
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.DecodePatch(ops)
	if err == nil {
		current, err = patch.Apply(current)
	}
	if err != nil {
		return nil, &native.Failure{Message: "UCP's update doesn't apply to the credentials AWS has: " + err.Error() + ". Deploy again.", Outcome: native.OutcomeRejected}
	}
	out := map[string]any{}
	if err := json.Unmarshal(current, &out); err != nil {
		return nil, err
	}
	return out, nil
}
