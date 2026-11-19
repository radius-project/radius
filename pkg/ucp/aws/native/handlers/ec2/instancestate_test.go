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
	"errors"
	"os"
	"testing"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func handWrittenSchema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func instanceState(state string) map[string]any {
	return map[string]any{"InstanceId": testInstance, "State": state}
}

func Test_Suite_InstanceState(t *testing.T) {
	stopped := instanceState("stopped")
	nativetest.Run(t, nativetest.Suite{
		TypeName:    instanceStateType,
		Schema:      handWrittenSchema(t, "aws-ec2-instancestate.json"),
		Metadata:    handWrittenSchema(t, "aws-ec2-instancestate.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			putInstance(store, testInstance, types.InstanceStateNameRunning, nil)
			return NewInstanceState(newFakeInstances(store))
		},
		IdentifierProperty: "InstanceId",
		Properties:         stopped,
		Identifier:         testInstance,
		Changed:            instanceState("running"),
		Invalid: []map[string]any{
			instanceState("paused"),
			instanceState("Running"),
			{"InstanceId": testInstance, "State": true},
			{"InstanceId": 5, "State": "running"},
		},
		// The instance is started from the console between deploys.
		ChangedOutside: &nativetest.ChangedOutside{
			Properties: stopped,
			Change: func(stored map[string]any) map[string]any {
				return with(stored, map[string]any{"State": "running", "StateReason": nil})
			},
			Restored: func(stored map[string]any) bool { return stored["State"] == "stopped" },
		},
		AlwaysPresent: true,
		CreateFails:   true,
		API:           instanceStateAPI,
	})
}

// newInstanceState returns a handler with a fake API that has testInstance in state, with the extra fields set.
func newInstanceState(state types.InstanceStateName, extra map[string]any) (*InstanceState, *fakeInstances) {
	store := awsfake.NewCloudControl()
	putInstance(store, testInstance, state, extra)
	api := newFakeInstances(store)
	return NewInstanceState(api), api
}

// requireStateRejected requires err to be a rejected client failure whose message contains want.
func requireStateRejected(t *testing.T, err error, want string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, want)
}

func Test_InstanceState_Identifier(t *testing.T) {
	h := NewInstanceState(nil)
	id, err := h.Identifier(instanceState("running"))
	require.NoError(t, err)
	require.Equal(t, testInstance, id)

	id, err = h.Identifier(map[string]any{"InstanceId": testInstance})
	require.NoError(t, err, "`existing` leaves State out")
	require.Equal(t, testInstance, id)

	_, err = h.Identifier(map[string]any{"InstanceId": 5})
	requireStateRejected(t, err, "InstanceId of an AWS::EC2::InstanceState must be a string")
	_, err = h.Identifier(instanceState("paused"))
	requireStateRejected(t, err, `State of an AWS::EC2::InstanceState must be "running" or "stopped"`)
}

func Test_InstanceState_Read(t *testing.T) {
	for _, state := range []types.InstanceStateName{types.InstanceStateNamePending, types.InstanceStateNameRunning, types.InstanceStateNameStopping, types.InstanceStateNameStopped} {
		h, _ := newInstanceState(state, nil)
		got, err := h.Read(t.Context(), testRegion, testInstance)
		require.NoError(t, err)
		require.Equal(t, instanceState(string(state)), got)
	}

	gone := []struct {
		name  string
		state types.InstanceStateName
		extra map[string]any
	}{
		{"terminated", types.InstanceStateNameTerminated, nil},
		{"being terminated", types.InstanceStateNameShuttingDown, nil},
		{"of another account", types.InstanceStateNameRunning, map[string]any{"Owner": "other"}},
	}
	for _, tt := range gone {
		h, _ := newInstanceState(tt.state, tt.extra)
		_, err := h.Read(t.Context(), testRegion, testInstance)
		require.ErrorIs(t, err, native.ErrNotFound, tt.name)
	}

	h, _ := newInstanceState(types.InstanceStateNameRunning, nil)
	_, err := h.Read(t.Context(), testRegion, "i-0fedcba9876543210")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), testRegion, "i-XYZ")
	require.ErrorContains(t, err, "InvalidInstanceID.Malformed")
	require.NotErrorIs(t, err, native.ErrNotFound, "a malformed ID is the template's error, not a missing instance")
}

func Test_InstanceState_Create(t *testing.T) {
	h, api := newInstanceState(types.InstanceStateNameStopped, nil)
	id, err := h.Create(t.Context(), createRequest(instanceState("running")))
	require.NoError(t, err)
	require.Equal(t, testInstance, id)
	require.Equal(t, []string{"DescribeInstances", "StartInstances"}, api.calls)

	h, api = newInstanceState(types.InstanceStateNameTerminated, nil)
	_, err = h.Create(t.Context(), createRequest(instanceState("running")))
	requireStateRejected(t, err, `AWS has no EC2 instance "i-0123456789abcdef0" that can run`)
	require.Equal(t, []string{"DescribeInstances"}, api.calls)

	for _, properties := range []map[string]any{{"InstanceId": 5, "State": "running"}, {"InstanceId": testInstance}} {
		h, api = newInstanceState(types.InstanceStateNameStopped, nil)
		_, err = h.Create(t.Context(), createRequest(properties))
		requireStateRejected(t, err, "of an AWS::EC2::InstanceState must be")
		require.Empty(t, api.calls, "nothing reaches AWS")
	}
}

// update returns the update of testInstance to the desired state, after UpdateState read the instance.
func update(t *testing.T, h *InstanceState, desired string) native.UpdateRequest {
	state, err := h.UpdateState(t.Context(), testRegion, testInstance)
	require.NoError(t, err)
	request := updateRequest(t, testInstance, map[string]any{}, instanceState(desired))
	request.State = state
	return request
}

func Test_InstanceState_Update(t *testing.T) {
	tests := []struct {
		name    string
		state   types.InstanceStateName
		desired string
		call    string
	}{
		{"stop a running instance", types.InstanceStateNameRunning, "stopped", "StopInstances"},
		{"start a stopped instance", types.InstanceStateNameStopped, "running", "StartInstances"},
		{"wait for a pending instance", types.InstanceStateNamePending, "running", ""},
		{"wait for a stopping instance", types.InstanceStateNameStopping, "stopped", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, api := newInstanceState(tt.state, nil)
			request := update(t, h, tt.desired)
			require.NoError(t, h.Update(t.Context(), request))
			want := []string{"DescribeInstances", "DescribeInstances"}
			if tt.call != "" {
				want = append(want, tt.call)
			}
			require.Equal(t, want, api.calls)
			waitInstance(t, h, request.State, tt.desired)
		})
	}
}

// waitInstance reads the update's progress until it finishes, and requires the instance to be in want.
func waitInstance(t *testing.T, h *InstanceState, state string, want string) {
	t.Helper()
	for range 3 {
		progress, err := h.UpdateProgress(t.Context(), testRegion, testInstance, state)
		require.NoError(t, err)
		if progress.Done {
			require.Nil(t, progress.Failure)
			require.Equal(t, want, progress.State)
			return
		}
		require.Equal(t, want, progress.Waiting)
	}
	require.Fail(t, "the update didn't finish")
}

func Test_InstanceState_UpdateConflicts(t *testing.T) {
	tests := []struct {
		name    string
		state   types.InstanceStateName
		desired string
		want    string
	}{
		{"stop a pending instance", types.InstanceStateNamePending, "stopped", `The EC2 instance "i-0123456789abcdef0" is pending, so AWS can't make it stopped now. Deploy again once it is running.`},
		{"start a stopping instance", types.InstanceStateNameStopping, "running", `The EC2 instance "i-0123456789abcdef0" is stopping, so AWS can't make it running now. Deploy again once it is stopped.`},
		// The read before the update saw the instance pending, and UpdateState sees it running.
		{"an instance that changed state while the deploy started", types.InstanceStateNameRunning, "running", `The EC2 instance "i-0123456789abcdef0" changed to running while the deploy started. Deploy again.`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, api := newInstanceState(tt.state, nil)
			request := update(t, h, tt.desired)
			api.calls = nil
			requireStateRejected(t, h.Update(t.Context(), request), tt.want)
			require.Empty(t, api.calls, "nothing reaches AWS")
		})
	}

	t.Run("an update without state from before", func(t *testing.T) {
		h, api := newInstanceState(types.InstanceStateNameRunning, nil)
		require.NoError(t, h.Update(t.Context(), updateRequest(t, testInstance, map[string]any{}, instanceState("stopped"))))
		require.Contains(t, api.calls, "StopInstances")
	})
	t.Run("an invalid state", func(t *testing.T) {
		h, api := newInstanceState(types.InstanceStateNameRunning, nil)
		requireStateRejected(t, h.Update(t.Context(), updateRequest(t, testInstance, map[string]any{}, instanceState("paused"))), "must be")
		require.Empty(t, api.calls)
	})
	t.Run("a terminated instance", func(t *testing.T) {
		h, api := newInstanceState(types.InstanceStateNameRunning, nil)
		request := update(t, h, "stopped")
		api.store.Remove(instanceStateType, testInstance)
		requireStateRejected(t, h.Update(t.Context(), request), "it doesn't exist, or it is terminated")
	})
}

// Test_InstanceState_CantStop checks that AWS's error for an instance it can't stop is a 400 that names the instance,
// and that an error AWS doesn't blame on the request stays as it is.
func Test_InstanceState_CantStop(t *testing.T) {
	for name, extra := range map[string]map[string]any{
		"an instance store root volume":         {"RootDeviceType": "instance-store"},
		"a Spot Instance of a one-time request": {"SpotRequestType": "one-time"},
	} {
		t.Run(name, func(t *testing.T) {
			h, _ := newInstanceState(types.InstanceStateNameRunning, extra)
			err := h.Update(t.Context(), update(t, h, "stopped"))
			requireStateRejected(t, err, `AWS can't stop the EC2 instance "i-0123456789abcdef0", which is running: `)
			require.ErrorContains(t, err, "UnsupportedOperation")
		})
	}

	h, _ := newInstanceState(types.InstanceStateNameRunning, map[string]any{"SpotRequestType": "persistent"})
	require.NoError(t, h.Update(t.Context(), update(t, h, "stopped")), "a Spot Instance of a persistent request can be stopped")

	h, api := newInstanceState(types.InstanceStateNameStopped, nil)
	api.errs["StartInstances"] = nativefake.ServerError("InternalError", "EC2 failed")
	err := h.Update(t.Context(), update(t, h, "running"))
	var failure *native.Failure
	require.False(t, errors.As(err, &failure), "the outcome of a server error is unknown")
	require.ErrorContains(t, err, "EC2 failed")
}

func Test_InstanceState_Progress(t *testing.T) {
	h, api := newInstanceState(types.InstanceStateNameStopped, nil)
	id, err := h.Create(t.Context(), createRequest(instanceState("running")))
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), testRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "pending", Waiting: "running"}, progress)
	progress, err = h.Progress(t.Context(), testRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{Done: true, State: "running"}, progress)

	api.store.Remove(instanceStateType, testInstance)
	_, err = h.Progress(t.Context(), testRegion, id, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_InstanceState_UpdateProgress(t *testing.T) {
	t.Run("a read that doesn't show the change yet", func(t *testing.T) {
		h, _ := newInstanceState(types.InstanceStateNameStopped, map[string]any{"StateReason": "Client.UserInitiatedShutdown"})
		state, err := h.UpdateState(t.Context(), testRegion, testInstance)
		require.NoError(t, err)
		require.JSONEq(t, `{"state": "stopped", "reason": "Client.UserInitiatedShutdown"}`, state)
		progress, err := h.UpdateProgress(t.Context(), testRegion, testInstance, state)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "stopped", Waiting: "running"}, progress)
	})

	t.Run("a start that stops again", func(t *testing.T) {
		h, api := newInstanceState(types.InstanceStateNameStopped, map[string]any{"StateReason": "Client.UserInitiatedShutdown"})
		api.failStart[testInstance] = "Server.InsufficientInstanceCapacity"
		request := update(t, h, "running")
		require.NoError(t, h.Update(t.Context(), request))
		progress, err := h.UpdateProgress(t.Context(), testRegion, testInstance, request.State)
		require.NoError(t, err)
		require.Equal(t, "running", progress.Waiting, "the instance is pending")
		progress, err = h.UpdateProgress(t.Context(), testRegion, testInstance, request.State)
		require.NoError(t, err)
		require.True(t, progress.Done)
		require.NotNil(t, progress.Failure)
		require.Equal(t, `the EC2 instance "i-0123456789abcdef0" is stopped, not running: Server.InsufficientInstanceCapacity: the reason for the last change of state`, progress.Failure.Message)
	})

	t.Run("a running instance with no reason, as before", func(t *testing.T) {
		h, _ := newInstanceState(types.InstanceStateNameRunning, nil)
		state, err := h.UpdateState(t.Context(), testRegion, testInstance)
		require.NoError(t, err)
		progress, err := h.UpdateProgress(t.Context(), testRegion, testInstance, state)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "running", Waiting: "stopped"}, progress)
	})

	t.Run("a pending instance that stopped", func(t *testing.T) {
		h, api := newInstanceState(types.InstanceStateNamePending, nil)
		state, err := h.UpdateState(t.Context(), testRegion, testInstance)
		require.NoError(t, err)
		putInstance(api.store, testInstance, types.InstanceStateNameStopped, nil)
		progress, err := h.UpdateProgress(t.Context(), testRegion, testInstance, state)
		require.NoError(t, err)
		require.True(t, progress.Done)
		require.Equal(t, `the EC2 instance "i-0123456789abcdef0" is stopped, not running`, progress.Failure.Message)
	})

	t.Run("state UCP can't read", func(t *testing.T) {
		h, _ := newInstanceState(types.InstanceStateNamePending, nil)
		progress, err := h.UpdateProgress(t.Context(), testRegion, testInstance, "")
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "pending", Waiting: "running"}, progress, "it waits as Progress does")
	})

	t.Run("a terminated instance", func(t *testing.T) {
		h, api := newInstanceState(types.InstanceStateNameRunning, nil)
		state, err := h.UpdateState(t.Context(), testRegion, testInstance)
		require.NoError(t, err)
		api.store.Remove(instanceStateType, testInstance)
		_, err = h.UpdateProgress(t.Context(), testRegion, testInstance, state)
		require.ErrorIs(t, err, native.ErrNotFound)
		_, err = h.UpdateState(t.Context(), testRegion, testInstance)
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_InstanceState_DeleteAndList(t *testing.T) {
	h, api := newInstanceState(types.InstanceStateNameRunning, nil)
	require.NoError(t, h.Delete(t.Context(), testRegion, testInstance))
	resources, err := h.List(t.Context(), testRegion)
	require.NoError(t, err)
	require.Empty(t, resources)
	require.Empty(t, api.calls, "neither calls AWS")
	require.Equal(t, native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}, h.Options())
}

// describeErr is an EC2 API whose DescribeInstances fails.
type describeErr struct {
	InstanceAPI
}

func (describeErr) DescribeInstances(context.Context, *awsec2.DescribeInstancesInput, ...func(*awsec2.Options)) (*awsec2.DescribeInstancesOutput, error) {
	return nil, nativefake.ServerError("InternalError", "EC2 failed")
}

func Test_InstanceState_DescribeFails(t *testing.T) {
	h := NewInstanceState(describeErr{})
	_, err := h.UpdateState(t.Context(), testRegion, testInstance)
	require.ErrorContains(t, err, "EC2 failed")
	_, err = h.Progress(t.Context(), testRegion, testInstance, native.OperationCreate)
	require.ErrorContains(t, err, "EC2 failed")
	err = h.Update(t.Context(), updateRequest(t, testInstance, map[string]any{}, instanceState("stopped")))
	require.ErrorContains(t, err, "EC2 failed")
}
