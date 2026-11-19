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

// Package lambda has the native handler for AWS Lambda MicroVMs, which have no CloudFormation type.
package lambda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const microvmType = "AWS::Lambda::Microvm"

// API is the part of the AWS Lambda MicroVMs client the handler uses.
type API interface {
	RunMicrovm(ctx context.Context, params *lambdamicrovms.RunMicrovmInput, optFns ...func(*lambdamicrovms.Options)) (*lambdamicrovms.RunMicrovmOutput, error)
	GetMicrovm(ctx context.Context, params *lambdamicrovms.GetMicrovmInput, optFns ...func(*lambdamicrovms.Options)) (*lambdamicrovms.GetMicrovmOutput, error)
	TerminateMicrovm(ctx context.Context, params *lambdamicrovms.TerminateMicrovmInput, optFns ...func(*lambdamicrovms.Options)) (*lambdamicrovms.TerminateMicrovmOutput, error)
	ListMicrovms(ctx context.Context, params *lambdamicrovms.ListMicrovmsInput, optFns ...func(*lambdamicrovms.Options)) (*lambdamicrovms.ListMicrovmsOutput, error)
}

var inRegion = awsapi.InRegion[lambdamicrovms.Options]

var (
	_ native.Handler           = (*Microvm)(nil)
	_ native.IdentifierEncoder = (*Microvm)(nil)
)

// gone reports whether a MicroVM in state is gone or going: AWS has terminated it, or is terminating it. AWS also
// terminates a MicroVM on its own, at its maximum duration or after its idle policy's suspended duration.
func gone(state types.MicrovmState) bool {
	return state == types.MicrovmStateTerminating || state == types.MicrovmStateTerminated
}

// Microvm serves AWS::Lambda::Microvm: a MicroVM run from a MicroVM image. AWS assigns its identifier, MicrovmId, and
// RunMicrovm takes an idempotency token, so UCP sends a lost create again with the same token. AWS has no call that
// changes a MicroVM, so every property is create-only. A MicroVM AWS is terminating, or has terminated, reads as not
// found, so the next deploy runs a new one.
type Microvm struct {
	api API
}

// NewMicrovm returns the handler for Lambda MicroVMs.
func NewMicrovm(api API) *Microvm {
	return &Microvm{api: api}
}

// TypeName returns AWS::Lambda::Microvm.
func (h *Microvm) TypeName() string {
	return microvmType
}

// Options returns how UCP uses the handler. AWS reports the image as its ARN, so a template that names it by its name
// matches the ARN when the record shows the last deploy sent that name. Without a record, such as on a take-over, the
// name could be of an image in another account, so the template must give the ARN.
func (h *Microvm) Options() native.Options {
	return native.Options{List: native.ListRegion, Recovery: native.RecoverByResend, RecordedEquivalent: recordedEquivalent}
}

// recordedEquivalent reports whether AWS's image ARN is of the image the template names by its name.
func recordedEquivalent(path string, template any, aws any) bool {
	t, okT := template.(string)
	a, okA := aws.(string)
	return okT && okA && path == "ImageIdentifier" && strings.HasSuffix(a, ":microvm-image:"+t)
}

// template is a MicroVM as a template gives it, in the shape of the schema.
type template struct {
	MicrovmId                *string
	ImageIdentifier          *string
	ImageVersion             *string
	ExecutionRoleArn         *string
	IdlePolicy               *idlePolicy
	IngressNetworkConnectors []string
	EgressNetworkConnectors  []string
	Logging                  *logging
	MaximumDurationInSeconds *int32
	RunHookPayload           *string
}

type idlePolicy struct {
	AutoResumeEnabled        *bool
	MaxIdleDurationSeconds   *int32
	SuspendedDurationSeconds *int32
}

type logging struct {
	CloudWatch *struct {
		LogGroup  *string
		LogStream *string
	}
	Disabled *struct{}
}

// clientFailure is a failure for a template value AWS would reject or can't take, which UCP reports with 400.
func clientFailure(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// parse returns the MicroVM the properties give. A value of the wrong type, or a Logging that doesn't set exactly
// one kind, is a client failure, so it is never sent as an empty or dropped value.
func parse(properties map[string]any) (template, error) {
	var t template
	if err := awsapi.Decode(properties, &t); err != nil {
		return template{}, clientFailure("the template of an %s is not valid: %s", microvmType, typeError(err))
	}
	if l := t.Logging; l != nil && (l.CloudWatch == nil) == (l.Disabled == nil) {
		return template{}, clientFailure("Logging of an %s must set exactly one of CloudWatch and Disabled", microvmType)
	}
	return t, nil
}

// typeError says which property has the wrong type, from the error decoding a template.
func typeError(err error) string {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return err.Error()
	}
	return fmt.Sprintf("%s must be %s, not a %s", typeErr.Field, kindOf(typeErr.Type), typeErr.Value)
}

// kindOf names the JSON kind of a value of type t.
func kindOf(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "a boolean"
	case reflect.Slice:
		return "a list of strings"
	case reflect.Struct:
		return "an object"
	}
	return "an integer"
}

// Identifier returns the template's MicrovmId, which only `existing` sets, or "". UCP calls it before any AWS call,
// so a value of the wrong type is rejected here.
func (h *Microvm) Identifier(properties map[string]any) (string, error) {
	t, err := parse(properties)
	return aws.ToString(t.MicrovmId), err
}

// get returns the MicroVM, or ErrNotFound when AWS has no such MicroVM.
func (h *Microvm) get(ctx context.Context, region string, identifier string) (*lambdamicrovms.GetMicrovmOutput, error) {
	out, err := h.api.GetMicrovm(ctx, &lambdamicrovms.GetMicrovmInput{MicrovmIdentifier: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "ResourceNotFoundException")
	}
	return out, nil
}

// Read returns the MicroVM's properties. A MicroVM AWS is terminating, or has terminated, is not found.
func (h *Microvm) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.get(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	if gone(out.State) {
		return nil, fmt.Errorf("%w: the Lambda MicroVM %q is %s", native.ErrNotFound, identifier, out.State)
	}
	return propertiesOf(out), nil
}

// propertiesOf returns the properties of m, with the image's ARN as ImageIdentifier. Values AWS doesn't return are
// left out.
func propertiesOf(m *lambdamicrovms.GetMicrovmOutput) map[string]any {
	properties := map[string]any{
		"MicrovmId":       aws.ToString(m.MicrovmId),
		"ImageIdentifier": aws.ToString(m.ImageArn),
		"ImageArn":        aws.ToString(m.ImageArn),
		"State":           string(m.State),
	}
	for name, v := range map[string]*string{
		"ImageVersion":     m.ImageVersion,
		"ExecutionRoleArn": m.ExecutionRoleArn,
		"Endpoint":         m.Endpoint,
		"StateReason":      m.StateReason,
	} {
		if s := aws.ToString(v); s != "" {
			properties[name] = s
		}
	}
	for name, list := range map[string][]string{
		"IngressNetworkConnectors": m.IngressNetworkConnectors,
		"EgressNetworkConnectors":  m.EgressNetworkConnectors,
	} {
		if len(list) > 0 {
			items := []any{}
			for _, v := range list {
				items = append(items, v)
			}
			properties[name] = items
		}
	}
	if p := m.IdlePolicy; p != nil {
		properties["IdlePolicy"] = map[string]any{
			"AutoResumeEnabled":        aws.ToBool(p.AutoResumeEnabled),
			"MaxIdleDurationSeconds":   aws.ToInt32(p.MaxIdleDurationSeconds),
			"SuspendedDurationSeconds": aws.ToInt32(p.SuspendedDurationSeconds),
		}
	}
	if m.MaximumDurationInSeconds != nil {
		properties["MaximumDurationInSeconds"] = *m.MaximumDurationInSeconds
	}
	return properties
}

// Create runs the MicroVM with the record's client token, so a create sent again returns the first MicroVM.
func (h *Microvm) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	t, err := parse(request.Properties)
	if err != nil {
		return "", err
	}
	if aws.ToString(t.ImageIdentifier) == "" {
		return "", clientFailure("an %s needs ImageIdentifier", microvmType)
	}
	input := &lambdamicrovms.RunMicrovmInput{
		ClientToken:              aws.String(request.ClientToken),
		ImageIdentifier:          t.ImageIdentifier,
		ImageVersion:             t.ImageVersion,
		ExecutionRoleArn:         t.ExecutionRoleArn,
		IngressNetworkConnectors: t.IngressNetworkConnectors,
		EgressNetworkConnectors:  t.EgressNetworkConnectors,
		MaximumDurationInSeconds: t.MaximumDurationInSeconds,
		RunHookPayload:           t.RunHookPayload,
	}
	if p := t.IdlePolicy; p != nil {
		input.IdlePolicy = &types.IdlePolicy{
			AutoResumeEnabled:        p.AutoResumeEnabled,
			MaxIdleDurationSeconds:   p.MaxIdleDurationSeconds,
			SuspendedDurationSeconds: p.SuspendedDurationSeconds,
		}
	}
	switch l := t.Logging; {
	case l == nil:
	case l.Disabled != nil:
		input.Logging = &types.LoggingMemberDisabled{}
	default:
		input.Logging = &types.LoggingMemberCloudWatch{Value: types.CloudWatchLogging{LogGroup: l.CloudWatch.LogGroup, LogStream: l.CloudWatch.LogStream}}
	}
	out, err := h.api.RunMicrovm(ctx, input, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	return aws.ToString(out.MicrovmId), nil
}

// Update changes nothing, because every property is create-only.
func (h *Microvm) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete terminates the MicroVM. AWS succeeds for a MicroVM it already terminated.
func (h *Microvm) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.TerminateMicrovm(ctx, &lambdamicrovms.TerminateMicrovmInput{MicrovmIdentifier: aws.String(identifier)}, inRegion(region))
	return awsapi.NotFound(err, "ResourceNotFoundException")
}

// List returns the MicroVMs in the region that AWS isn't terminating and hasn't terminated, with what AWS lists about
// them.
func (h *Microvm) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	paginator := lambdamicrovms.NewListMicrovmsPaginator(h.api, &lambdamicrovms.ListMicrovmsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if gone(item.State) {
				continue
			}
			properties := propertiesOf(&lambdamicrovms.GetMicrovmOutput{MicrovmId: item.MicrovmId, ImageArn: item.ImageArn, ImageVersion: item.ImageVersion, State: item.State})
			out = append(out, native.Resource{Identifier: aws.ToString(item.MicrovmId), Properties: properties})
		}
	}
	return out, nil
}

// Progress reads the MicroVM's state. A create waits for RUNNING, and is done once the MicroVM runs, including when
// AWS has since suspended it for lack of traffic. A MicroVM that terminated before it ran fails the create. The
// create still counts as applied, so UCP saves the MicroVM, and the next deploy finds it gone and runs a new one. A
// delete waits until AWS has terminated the MicroVM. An update changes nothing, so it is done.
func (h *Microvm) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	out, err := h.get(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	state := string(out.State)
	switch {
	case op == native.OperationDelete && out.State == types.MicrovmStateTerminated:
		return native.Progress{}, fmt.Errorf("%w: the Lambda MicroVM %q is TERMINATED", native.ErrNotFound, identifier)
	case op == native.OperationDelete:
		return native.Progress{State: state, Waiting: string(types.MicrovmStateTerminated)}, nil
	case gone(out.State):
		message := fmt.Sprintf("AWS reports the Lambda MicroVM %q as %s before it ran", identifier, state)
		if reason := aws.ToString(out.StateReason); reason != "" {
			message += " (" + reason + ")"
		}
		return native.Progress{Done: true, State: state, Failure: &native.Failure{
			Message:     message + ". Delete the resource, then deploy again to run a new MicroVM",
			ClientError: true,
			Outcome:     native.OutcomeApplied,
		}}, nil
	case slices.Contains(ran, out.State):
		return native.Progress{Done: true, State: state}, nil
	}
	// PENDING, or a state this handler doesn't know, which UCP waits on until the create's timeout names it.
	return native.Progress{State: state, Waiting: string(types.MicrovmStateRunning)}, nil
}

// ran are the states of a MicroVM that has run: RUNNING, and suspended for lack of traffic.
var ran = []types.MicrovmState{types.MicrovmStateRunning, types.MicrovmStateSuspending, types.MicrovmStateSuspended}
