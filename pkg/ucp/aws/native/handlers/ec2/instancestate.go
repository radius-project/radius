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

package ec2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const instanceStateType = "AWS::EC2::InstanceState"

// InstanceAPI is the part of the EC2 client the instance state handler uses.
type InstanceAPI interface {
	DescribeInstances(ctx context.Context, params *awsec2.DescribeInstancesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error)
	StartInstances(ctx context.Context, params *awsec2.StartInstancesInput, optFns ...func(*awsec2.Options)) (*awsec2.StartInstancesOutput, error)
	StopInstances(ctx context.Context, params *awsec2.StopInstancesInput, optFns ...func(*awsec2.Options)) (*awsec2.StopInstancesOutput, error)
}

var (
	_ native.Handler           = (*InstanceState)(nil)
	_ native.IdentifierEncoder = (*InstanceState)(nil)
	_ native.UpdatePreparer    = (*InstanceState)(nil)
)

// InstanceState serves AWS::EC2::InstanceState, which keeps an EC2 instance Radius doesn't create running or stopped.
// Its identifier is InstanceId. The instance always exists before the deploy, so UCP takes it over and updates it,
// and calls Create only when a read finds no instance. Its metadata makes a delete change nothing in AWS.
//
// A deploy starts or stops the instance only when its state differs from the template's, and then waits for the
// template's state. An instance that is terminated, or being terminated, reads as gone, because it can never run
// again. AWS's error for an instance it can't stop, such as one with an instance store root volume or a Spot Instance
// of a one-time request, is a 400 naming the instance.
type InstanceState struct {
	api InstanceAPI
}

// NewInstanceState returns the handler for the running state of EC2 instances.
func NewInstanceState(api InstanceAPI) *InstanceState {
	return &InstanceState{api: api}
}

// TypeName returns AWS::EC2::InstanceState.
func (h *InstanceState) TypeName() string {
	return instanceStateType
}

// Options returns how UCP uses the handler. The template gives the identifier, and the type can't be listed.
func (h *InstanceState) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// Identifier returns InstanceId. UCP calls it before any AWS call, so a template whose InstanceId isn't a string, or
// whose State is neither running nor stopped, is rejected here. State may be left out, as for `existing`.
func (h *InstanceState) Identifier(properties map[string]any) (string, error) {
	if value, ok := properties["InstanceId"]; ok {
		if _, ok := value.(string); !ok {
			return "", &native.Failure{Message: "InstanceId of an " + instanceStateType + " must be a string", ClientError: true, Outcome: native.OutcomeRejected}
		}
	}
	if _, ok := properties["State"]; ok {
		if _, err := awsapi.RunningState(properties, instanceStateType); err != nil {
			return "", err
		}
	}
	return awsapi.String(properties, "InstanceId"), nil
}

// instance returns the instance with the ID, or ErrNotFound when EC2 has none, or it is terminated or being
// terminated.
func (h *InstanceState) instance(ctx context.Context, region string, id string) (*types.Instance, error) {
	out, err := h.api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{id}}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "InvalidInstanceID.NotFound")
	}
	// EC2 leaves out an instance the account doesn't own.
	var instance *types.Instance
	for _, reservation := range out.Reservations {
		for i := range reservation.Instances {
			if aws.ToString(reservation.Instances[i].InstanceId) == id {
				instance = &reservation.Instances[i]
			}
		}
	}
	if instance == nil || (instance.State != nil && (instance.State.Name == types.InstanceStateNameShuttingDown || instance.State.Name == types.InstanceStateNameTerminated)) {
		return nil, native.ErrNotFound
	}
	if instance.State == nil {
		// The state is optional in the API, and without it UCP can't tell whether the instance can run.
		return nil, fmt.Errorf("EC2 returned the instance %q without its state. Deploy again", id)
	}
	return instance, nil
}

// Read returns the instance's ID and state, as EC2 reports it.
func (h *InstanceState) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	instance, err := h.instance(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return map[string]any{"InstanceId": identifier, "State": string(instance.State.Name)}, nil
}

// Create changes nothing and fails with 400. UCP calls it only when a read found no instance, and AWS has nothing a
// deploy can create: it says the instance doesn't exist, or, when EC2 reports it now, to deploy again, so the next
// deploy takes it over and waits for the template's state as an update.
func (h *InstanceState) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	if _, err := awsapi.RunningState(request.Properties, instanceStateType); err != nil {
		return "", err
	}
	if _, err := h.instance(ctx, request.Region, id); errors.Is(err, native.ErrNotFound) {
		return "", instanceGone(id)
	} else if err != nil {
		return "", err
	}
	return "", &native.Failure{
		Message:     fmt.Sprintf("EC2 reported no instance %q when the deploy read it, and reports it now. Deploy again.", id),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// instanceGone is the failure for an instance EC2 doesn't have, or that is terminated.
func instanceGone(id string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("AWS has no EC2 instance %q that can run: it doesn't exist, or it is terminated. Set InstanceId to an existing instance.", id),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Update starts or stops the instance, when the template's state differs from the one EC2 reports now. UpdateProgress
// waits for the state that follows the one the instance was in before the update (request.State), so an update whose
// template sets another fails with 400 and calls nothing: a pending instance can't be stopped, and a stopping one
// can't be started, until it gets there.
func (h *InstanceState) Update(ctx context.Context, request native.UpdateRequest) error {
	desired, err := awsapi.RunningState(request.Desired, instanceStateType)
	if err != nil {
		return err
	}
	var before *instanceBefore
	if json.Unmarshal([]byte(request.State), &before) == nil && before != nil {
		state := types.InstanceStateName(before.State)
		if target := updateTarget(state); target != desired {
			message := fmt.Sprintf("The EC2 instance %q changed to %s while the deploy started. Deploy again.", request.Identifier, state)
			if target := instanceTarget(state); target != before.State {
				message = fmt.Sprintf("The EC2 instance %q is %s, so AWS can't make it %s now. Deploy again once it is %s.", request.Identifier, state, desired, target)
			}
			return &native.Failure{Message: message, ClientError: true, Outcome: native.OutcomeRejected}
		}
	}
	return h.apply(ctx, request.Region, request.Identifier, desired, before)
}

// apply starts or stops the instance when it isn't in, or on its way to, the desired state. A call EC2 rejects, such
// as a stop of an instance with an instance store root volume, is a rejected client failure that names the instance
// and its state.
//
// before is the instance as it was before the update, or nil. An instance back in the state it had then, with another
// reason or launch time (see endedAttempt), shows that an earlier attempt of this update reached AWS and ended, such
// as a start whose response UCP lost and that stopped again. apply calls nothing then, and UpdateProgress reports the
// attempt's outcome, so a read that doesn't show a new start yet can't be taken for it.
func (h *InstanceState) apply(ctx context.Context, region string, id string, desired string, before *instanceBefore) error {
	instance, err := h.instance(ctx, region, id)
	if errors.Is(err, native.ErrNotFound) {
		return instanceGone(id)
	} else if err != nil {
		return err
	}
	current := instance.State.Name
	if instanceTarget(current) == desired || (before != nil && endedAttempt(*before, beforeOf(instance))) {
		return nil
	}
	verb := "start"
	if desired == awsapi.StateRunning {
		_, err = h.api.StartInstances(ctx, &awsec2.StartInstancesInput{InstanceIds: []string{id}}, inRegion(region), sendOnce)
	} else {
		verb = "stop"
		_, err = h.api.StopInstances(ctx, &awsec2.StopInstancesInput{InstanceIds: []string{id}}, inRegion(region), sendOnce)
	}
	if err == nil || !ucp_aws.IsClientFault(err) {
		return err
	}
	return &native.Failure{
		Message:     fmt.Sprintf("AWS can't %s the EC2 instance %q, which is %s: %s", verb, id, current, err),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// sendOnce stops the SDK from sending a start or stop again after an error. EC2 may answer a start of an instance that
// isn't stopped, or a stop of one that isn't running, with IncorrectInstanceState, so a retry of a call EC2 acted on
// before its response was lost would read as a call EC2 rejected. The error of the one call leaves the outcome
// unknown instead, and the next deploy finds the instance on its way.
func sendOnce(o *awsec2.Options) {
	o.RetryMaxAttempts = 1
}

// instanceTarget returns the state an instance in state is in or on its way to: running for pending, stopped for
// stopping, and state itself otherwise.
func instanceTarget(state types.InstanceStateName) string {
	switch state {
	case types.InstanceStateNamePending:
		return awsapi.StateRunning
	case types.InstanceStateNameStopping:
		return awsapi.StateStopped
	}
	return string(state)
}

// updateTarget returns the state an update waits for, given the state the instance was in before it: the other of
// running and stopped, because an update is sent only when the template's state differs, or the state a pending or
// stopping instance is on its way to.
func updateTarget(before types.InstanceStateName) string {
	switch before {
	case types.InstanceStateNameRunning:
		return awsapi.StateStopped
	case types.InstanceStateNameStopped:
		return awsapi.StateRunning
	}
	return instanceTarget(before)
}

// Delete changes nothing. The type's metadata makes a delete leave the instance as it is, so UCP never calls it.
func (h *InstanceState) Delete(ctx context.Context, region string, identifier string) error {
	return nil
}

// List is not supported.
func (h *InstanceState) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reports the operation as finished once the instance is running or stopped. UCP waits for an update with
// UpdateProgress, and Create never starts an operation, so UpdateProgress calls it only for an update whose state it
// can't read.
func (h *InstanceState) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	instance, err := h.instance(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	state := string(instance.State.Name)
	if target := instanceTarget(instance.State.Name); target != state {
		return native.Progress{State: state, Waiting: target}, nil
	}
	return native.Progress{Done: true, State: state}, nil
}

// instanceBefore is the instance as it was before an update: its state, the code of the reason for its last change of
// state, such as Client.UserInitiatedShutdown, and the time it was last launched. UCP keeps it on the record
// (native.UpdatePreparer), so the update's progress knows which state it waits for, and can tell a start that failed
// from a read that doesn't show the change yet.
type instanceBefore struct {
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Launched string `json:"launched,omitempty"`
}

// beforeOf returns instance as instanceBefore.
func beforeOf(instance *types.Instance) instanceBefore {
	before := instanceBefore{State: string(instance.State.Name)}
	if instance.StateReason != nil {
		before.Reason = aws.ToString(instance.StateReason.Code)
	}
	if instance.LaunchTime != nil {
		before.Launched = instance.LaunchTime.UTC().Format(time.RFC3339Nano)
	}
	return before
}

// endedAttempt reports whether now, an instance in the state it had before an update, shows that an attempt of the
// update reached AWS and ended: its reason or launch time differs from before. DescribeInstances may leave out both,
// so a value missing on either side is unknown, not a difference.
func endedAttempt(before instanceBefore, now instanceBefore) bool {
	differs := func(a, b string) bool { return a != "" && b != "" && a != b }
	return now.State == before.State && (differs(before.Reason, now.Reason) || differs(before.Launched, now.Launched))
}

// UpdateState returns the instance as it is before the update, as JSON.
func (h *InstanceState) UpdateState(ctx context.Context, region string, identifier string) (string, error) {
	instance, err := h.instance(ctx, region, identifier)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(beforeOf(instance))
	return string(b), err
}

// UpdateProgress waits for the state the update moves the instance to (see updateTarget), given the instance as it
// was before the update in state. A read that still shows the state from before, with no sign of an ended attempt
// (see endedAttempt), doesn't show the change yet. Any other running or stopped instance means the update failed,
// such as a start that ran out of capacity, or whose operating system shut down at once, and stopped again.
func (h *InstanceState) UpdateProgress(ctx context.Context, region string, identifier string, state string) (native.Progress, error) {
	var before instanceBefore
	if err := json.Unmarshal([]byte(state), &before); err != nil {
		return h.Progress(ctx, region, identifier, native.OperationUpdate)
	}
	instance, err := h.instance(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	target := updateTarget(types.InstanceStateName(before.State))
	current := string(instance.State.Name)
	switch {
	case current == target:
		return native.Progress{Done: true, State: current}, nil
	case instanceTarget(instance.State.Name) != current, current == before.State && !endedAttempt(before, beforeOf(instance)):
		return native.Progress{State: current, Waiting: target}, nil
	}
	message := fmt.Sprintf("the EC2 instance %q is %s, not %s", identifier, current, target)
	if instance.StateReason != nil {
		message += ": " + aws.ToString(instance.StateReason.Message)
	}
	return native.Progress{Done: true, State: current, Failure: &native.Failure{Message: message}}, nil
}
