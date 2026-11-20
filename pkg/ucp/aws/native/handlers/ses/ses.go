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

// Package ses has the native handlers for Amazon SES resources that have no CloudFormation type: a sending
// authorization policy of an email identity (AWS::SES::EmailIdentityPolicy), the SNS topic an identity publishes one
// type of notification to (AWS::SES::IdentityNotificationTopic), and the pool a dedicated IP address sends from
// (AWS::SES::DedicatedIpAssignment). The policy and the dedicated IP use the SES v2 API, and the notification topic
// the SES v1 API, which is the only one that sets it.
//
// SES doesn't say when a change shows in a read. So a call that changes something reads until a read shows the
// change, for up to confirmReads reads, and a delete waits until a read no longer finds it. Without that, a deploy
// right after another could read the earlier value and send the same change again, and a create's progress could
// report a create AWS already has as lost.
package ses

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// notFoundCode is the error the SES v2 API returns for an identity, a policy, or a dedicated IP address it
	// doesn't have.
	notFoundCode = "NotFoundException"

	// confirmReads is how many times a change reads before it stops waiting for a read to show it, and confirmPause
	// is how long it waits between reads. SES takes most calls at most once a second.
	confirmReads = 20
	confirmPause = time.Second

	// timeout is how long UCP waits for any operation. Each takes effect in about a second.
	timeout = 10 * time.Minute
)

// V2API is the part of the SES v2 client the handlers use.
type V2API interface {
	CreateEmailIdentityPolicy(ctx context.Context, params *sesv2.CreateEmailIdentityPolicyInput, optFns ...func(*sesv2.Options)) (*sesv2.CreateEmailIdentityPolicyOutput, error)
	UpdateEmailIdentityPolicy(ctx context.Context, params *sesv2.UpdateEmailIdentityPolicyInput, optFns ...func(*sesv2.Options)) (*sesv2.UpdateEmailIdentityPolicyOutput, error)
	GetEmailIdentityPolicies(ctx context.Context, params *sesv2.GetEmailIdentityPoliciesInput, optFns ...func(*sesv2.Options)) (*sesv2.GetEmailIdentityPoliciesOutput, error)
	DeleteEmailIdentityPolicy(ctx context.Context, params *sesv2.DeleteEmailIdentityPolicyInput, optFns ...func(*sesv2.Options)) (*sesv2.DeleteEmailIdentityPolicyOutput, error)
	PutDedicatedIpInPool(ctx context.Context, params *sesv2.PutDedicatedIpInPoolInput, optFns ...func(*sesv2.Options)) (*sesv2.PutDedicatedIpInPoolOutput, error)
	GetDedicatedIp(ctx context.Context, params *sesv2.GetDedicatedIpInput, optFns ...func(*sesv2.Options)) (*sesv2.GetDedicatedIpOutput, error)
}

// V1API is the part of the SES v1 client the handlers use.
type V1API interface {
	SetIdentityNotificationTopic(ctx context.Context, params *ses.SetIdentityNotificationTopicInput, optFns ...func(*ses.Options)) (*ses.SetIdentityNotificationTopicOutput, error)
	SetIdentityHeadersInNotificationsEnabled(ctx context.Context, params *ses.SetIdentityHeadersInNotificationsEnabledInput, optFns ...func(*ses.Options)) (*ses.SetIdentityHeadersInNotificationsEnabledOutput, error)
	GetIdentityNotificationAttributes(ctx context.Context, params *ses.GetIdentityNotificationAttributesInput, optFns ...func(*ses.Options)) (*ses.GetIdentityNotificationAttributesOutput, error)
}

var (
	inRegionV2 = awsapi.InRegion[sesv2.Options]
	inRegionV1 = awsapi.InRegion[ses.Options]
)

// pause waits for d, or until ctx ends. Tests replace it, so they don't wait.
type pause func(ctx context.Context, d time.Duration)

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// reader reads the resource with the identifier, as a handler's Read does.
type reader func(ctx context.Context, region string, identifier string) (map[string]any, error)

// readError returns the error of a read: ErrNotFound for something the SES v2 API doesn't have, and otherwise err as it
// is. A read never reports a rejected request, because Progress reads too, and the worker would take a rejected read,
// such as a throttled one, as the end of an operation AWS may have applied.
func readError(err error) error {
	return awsapi.NotFound(err, notFoundCode)
}

// notFoundOrRejected returns err as ErrNotFound for something the SES v2 API doesn't have, and otherwise as
// awsapi.Rejected does.
func notFoundOrRejected(err error) error {
	return awsapi.Rejected(awsapi.NotFound(err, notFoundCode))
}

// rejected returns a client failure AWS applied none of, with message.
func rejected(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// pair returns the two parts of an identifier that joins two values with "|", such as an identity and a policy name.
// Neither part can contain "|", because the schemas don't allow it.
func pair(identifier string) (string, string, error) {
	first, second, ok := strings.Cut(identifier, "|")
	if !ok || first == "" || second == "" || strings.Contains(second, "|") {
		return "", "", fmt.Errorf("the identifier %q is not two values joined with |", identifier)
	}
	return first, second, nil
}

// identityOf returns the identity property name of properties sets, or a client failure when it can't name an
// identity: SES takes its email address or domain, and the identifier joins it with another value with "|".
func identityOf(properties map[string]any, name string) (string, error) {
	identity := awsapi.String(properties, name)
	if strings.ContainsAny(identity, ":|") {
		return "", rejected("%s must be the identity's email address or domain, not its ARN, and can't contain : or |", name)
	}
	return identity, nil
}

// joined returns the identifier of the properties name1 and name2 set, joined with "|", or "" when they don't set
// both.
func joined(properties map[string]any, name1 string, name2 string) string {
	first, second := awsapi.String(properties, name1), awsapi.String(properties, name2)
	if first == "" || second == "" {
		return ""
	}
	return first + "|" + second
}

// confirm reads until shown reports that a read shows the change, so the next read sees it, for up to confirmReads
// reads. AWS accepted the change, but until a read shows it, a later deploy could read the earlier value and keep, or
// send again, a value the template removed. So when no read shows it, confirm returns a failure whose outcome is
// unknown, which keeps the template's values as possibly applied: the next deploy sends the template's value at each
// of them. what names the resource, such as "the policy".
func confirm(ctx context.Context, wait pause, read reader, region string, identifier string, what string, shown func(map[string]any) bool) error {
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
		if got, err = read(ctx, region, identifier); err == nil && shown(got) {
			return nil
		}
	}
	message := fmt.Sprintf("AWS accepted the change to %s %q, but no read showed it within %s, so UCP can't tell what AWS has. Deploy again.", what, identifier, confirmReads*confirmPause)
	if err != nil {
		message += " The last read failed: " + err.Error()
	}
	return &native.Failure{Message: message, Outcome: native.OutcomeUnknown}
}

// isRejected reports whether err is a request AWS rejected as a client fault, such as any 4xx answer.
func isRejected(err error) bool {
	var failure *native.Failure
	return errors.As(awsapi.Rejected(err), &failure) && failure.ClientError
}

// later returns calls with each call after the first reporting its error as partly applied, because the calls before
// it changed AWS. A 4xx answer stays a client error, even one the SDK doesn't model, and a failure that says the
// whole operation applied stays as it is.
func later(calls ...func() error) []func() error {
	out := make([]func() error, len(calls))
	for i, call := range calls {
		out[i] = call
		if i == 0 {
			continue
		}
		out[i] = func() error {
			err := call()
			var failure *native.Failure
			if err == nil || (errors.As(err, &failure) && failure.Outcome == native.OutcomeApplied) {
				return err
			}
			return &native.Failure{Message: err.Error(), ClientError: isRejected(err), Outcome: native.OutcomePartial}
		}
	}
	return out
}

// createProgress reads how far a create has got: it is done once read finds the resource. It returns ErrNotFound when
// read doesn't find it, so UCP sends a lost create again. The create already waited until a read showed it.
func createProgress(ctx context.Context, read reader, region string, identifier string) (native.Progress, error) {
	if _, err := read(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	return native.Progress{Done: true}, nil
}

// deleteProgress reads how far a delete has got. It returns ErrNotFound, which finishes the delete, once read no
// longer finds the resource.
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

// options returns the options every handler in the package starts from: none can be listed, and each identifier is
// known before the create.
func options() native.Options {
	return native.Options{
		List:          native.ListNone,
		Recovery:      native.RecoverByIdentifier,
		CreateTimeout: timeout,
		UpdateTimeout: timeout,
		DeleteTimeout: timeout,
	}
}
