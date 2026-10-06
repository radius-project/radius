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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/output"
)

const (
	testScope        = "/planes/radius/local/resourceGroups/test-group"
	testResourceType = "Applications.Datastores/redisCaches"
)

func testResource(name string) generated.GenericResource {
	return generated.GenericResource{
		ID:   new(testScope + "/providers/" + testResourceType + "/" + name),
		Type: new(testResourceType),
	}
}

func Test_PreviewResourceIDs(t *testing.T) {
	require.Equal(t, testScope+"/providers/Radius.Core/applications/my-app", PreviewApplicationID(testScope, "my-app"))
	require.Equal(t, testScope+"/providers/Radius.Core/environments/my-env", PreviewEnvironmentID(testScope, "my-env"))
}

func Test_DeleteResourcesInParallel(t *testing.T) {
	t.Run("deletes every resource and logs each one", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		first := testResource("a")
		second := testResource("b")

		mock := clients.NewMockApplicationsManagementClient(ctrl)
		mock.EXPECT().DeleteResource(gomock.Any(), testResourceType, *first.ID, false).Return(true, nil).Times(1)
		mock.EXPECT().DeleteResource(gomock.Any(), testResourceType, *second.ID, false).Return(true, nil).Times(1)

		sink := &output.MockOutput{}
		err := DeleteResourcesInParallel(t.Context(), mock, sink, []generated.GenericResource{first, second}, false)
		require.NoError(t, err)
		require.Len(t, sink.Writes, 2)
	})

	t.Run("warns about resources without an ID or type instead of skipping them silently", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		valid := testResource("a")
		noID := generated.GenericResource{Type: new(testResourceType)}
		noType := generated.GenericResource{ID: new(testScope + "/providers/" + testResourceType + "/c")}

		mock := clients.NewMockApplicationsManagementClient(ctrl)
		mock.EXPECT().DeleteResource(gomock.Any(), testResourceType, *valid.ID, false).Return(true, nil).Times(1)

		sink := &output.MockOutput{}
		err := DeleteResourcesInParallel(t.Context(), mock, sink, []generated.GenericResource{valid, noID, noType}, false)
		require.NoError(t, err)

		// The caller reports a count to the user before calling this function, so every resource
		// that is not deleted must produce a message rather than disappearing.
		require.Equal(t, []any{
			output.LogOutput{Format: MsgDeletingResource, Params: []any{*valid.ID}},
			output.LogOutput{Format: MsgSkippingResource, Params: []any{"an unnamed resource"}},
			output.LogOutput{Format: MsgSkippingResource, Params: []any{*noType.ID}},
		}, sink.Writes)
	})

	t.Run("identifies a skipped resource by name when it has no ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock := clients.NewMockApplicationsManagementClient(ctrl)
		named := generated.GenericResource{Name: new("my-resource")}

		sink := &output.MockOutput{}
		err := DeleteResourcesInParallel(t.Context(), mock, sink, []generated.GenericResource{named}, false)
		require.NoError(t, err)

		require.Equal(t, []any{
			output.LogOutput{Format: MsgSkippingResource, Params: []any{"my-resource"}},
		}, sink.Writes)
	})

	t.Run("tolerates resources that are already deleted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resource := testResource("a")

		mock := clients.NewMockApplicationsManagementClient(ctrl)
		mock.EXPECT().
			DeleteResource(gomock.Any(), testResourceType, *resource.ID, false).
			Return(false, &azcore.ResponseError{StatusCode: http.StatusNotFound}).
			Times(1)

		err := DeleteResourcesInParallel(t.Context(), mock, &output.MockOutput{}, []generated.GenericResource{resource}, false)
		require.NoError(t, err)
	})

	t.Run("surfaces deletion failures", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resource := testResource("a")

		mock := clients.NewMockApplicationsManagementClient(ctrl)
		mock.EXPECT().
			DeleteResource(gomock.Any(), testResourceType, *resource.ID, false).
			Return(false, fmt.Errorf("simulated failure")).
			Times(1)

		err := DeleteResourcesInParallel(t.Context(), mock, &output.MockOutput{}, []generated.GenericResource{resource}, false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated failure")
	})

	t.Run("passes force through to the client", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resource := testResource("a")

		mock := clients.NewMockApplicationsManagementClient(ctrl)
		mock.EXPECT().DeleteResource(gomock.Any(), testResourceType, *resource.ID, true).Return(true, nil).Times(1)

		err := DeleteResourcesInParallel(t.Context(), mock, &output.MockOutput{}, []generated.GenericResource{resource}, true)
		require.NoError(t, err)
	})
}

func managedSecretPair() (generated.GenericResource, generated.GenericResource) {
	owner := testResourceWithSecrets("producer", map[string]any{"name": "producer-secret"})
	child := generated.GenericResource{
		ID:   new(testScope + "/providers/Radius.Security/secrets/producer-secret"),
		Type: new("Radius.Security/secrets"),
	}
	return owner, child
}

func testResourceWithSecrets(name string, secrets any) generated.GenericResource {
	resource := testResource(name)
	resource.Properties = map[string]any{"secrets": secrets}
	return resource
}

func Test_SelectedManagedSecrets(t *testing.T) {
	owner, child := managedSecretPair()
	matched := map[string]string{deleteResourceKey(*child.ID): *owner.ID}
	for _, tt := range []struct {
		name     string
		selected []generated.GenericResource
		want     map[string]string
		err      string
	}{
		{name: "selected pair", selected: []generated.GenericResource{owner, child}, want: matched},
		{name: "child first", selected: []generated.GenericResource{child, owner}, want: matched},
		{name: "case insensitive IDs and type", selected: []generated.GenericResource{
			{ID: new(strings.ToUpper(*owner.ID)), Type: owner.Type, Properties: owner.Properties},
			{ID: new(strings.ToUpper(*child.ID)), Type: new(strings.ToUpper(*child.Type))},
		}, want: map[string]string{deleteResourceKey(*child.ID): strings.ToUpper(*owner.ID)}},
		{name: "trailing slash", selected: []generated.GenericResource{
			owner, {ID: new(*child.ID + "/"), Type: child.Type},
		}, want: matched},
		{name: "same name in another group", selected: []generated.GenericResource{
			owner, {ID: new(strings.Replace(*child.ID, "test-group", "other-group", 1)), Type: child.Type},
		}},
		{name: "producer absent", selected: []generated.GenericResource{child}},
		{name: "child absent", selected: []generated.GenericResource{owner}},
		{name: "standalone secret", selected: []generated.GenericResource{testResource("producer"), child}},
		{name: "no name", selected: []generated.GenericResource{testResourceWithSecrets("producer", map[string]any{}), child}},
		{name: "null secrets", selected: []generated.GenericResource{testResourceWithSecrets("producer", nil), child}},
		{name: "unaddressable producer", selected: []generated.GenericResource{
			{ID: owner.ID, Properties: owner.Properties}, child,
		}},
		{name: "invalid secrets object", selected: []generated.GenericResource{
			testResourceWithSecrets("producer", "invalid"), child,
		}, err: "invalid properties.secrets"},
		{name: "invalid producer ID", selected: []generated.GenericResource{
			{ID: new("not-a-resource-id"), Type: owner.Type, Properties: owner.Properties}, child,
		}, err: "cannot resolve managed secret"},
		{name: "wrong child type", selected: []generated.GenericResource{
			owner, {ID: child.ID, Type: owner.Type},
		}, err: "unexpected resource type"},
		{name: "multiple producers", selected: []generated.GenericResource{
			owner, testResourceWithSecrets("producer-second", map[string]any{"name": "producer-secret"}), child,
		}, err: "claimed by both"},
		{name: "self reference", selected: []generated.GenericResource{
			{ID: child.ID, Type: child.Type, Properties: owner.Properties},
		}, err: "ownership cycle"},
		{name: "cycle", selected: []generated.GenericResource{
			{ID: new(testScope + "/providers/Radius.Security/secrets/producer"), Type: child.Type, Properties: owner.Properties},
			{ID: child.ID, Type: child.Type, Properties: map[string]any{"secrets": map[string]any{"name": "producer"}}},
		}, err: "ownership cycle"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			owners, err := selectedManagedSecrets(tt.selected)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
				require.ErrorContains(t, DeleteResourcesInParallel(t.Context(), client, &output.MockOutput{}, tt.selected, false), tt.err)
				return
			}
			require.NoError(t, err)
			if len(tt.want) == 0 {
				require.Empty(t, owners)
			} else {
				require.Equal(t, tt.want, owners)
			}
		})
	}

	for _, name := range []any{nil, 12, "", "../secret", "secret/name", "secret?force=true", "secret#fragment", " secret"} {
		t.Run(fmt.Sprintf("invalid name %v", name), func(t *testing.T) {
			owner := testResourceWithSecrets("producer", map[string]any{"name": name})
			client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
			err := DeleteResourcesInParallel(t.Context(), client, &output.MockOutput{}, []generated.GenericResource{owner, child}, false)
			require.ErrorContains(t, err, "invalid properties.secrets.name")
		})
	}
}

func Test_WaitForManagedSecretDeletion(t *testing.T) {
	for _, tt := range []struct {
		name      string
		states    []any
		getError  error
		wantError string
	}{
		{name: "already absent"},
		{name: "accepted updating deleting then absent", states: []any{"Accepted", "Updating", "Deleting"}},
		{name: "succeeded is not absent", states: []any{"Succeeded"}},
		{name: "missing and unknown states", states: []any{nil, "Unknown"}},
		{name: "failed secret", states: []any{"Failed"}, wantError: `state "Failed"`},
		{name: "canceled secret", states: []any{"canceled"}, wantError: `state "canceled"`},
		{name: "invalid state", states: []any{42}, wantError: "invalid properties.provisioningState"},
		{name: "unauthorized", getError: &azcore.ResponseError{StatusCode: http.StatusUnauthorized}, wantError: "not confirmed deleted"},
		{name: "forbidden", getError: &azcore.ResponseError{StatusCode: http.StatusForbidden}, wantError: "not confirmed deleted"},
		{name: "GET conflict", getError: &azcore.ResponseError{StatusCode: http.StatusConflict}, wantError: "not confirmed deleted"},
		{name: "server error", getError: &azcore.ResponseError{StatusCode: http.StatusInternalServerError}, wantError: "not confirmed deleted"},
		{name: "transport error", getError: errors.New("connection closed"), wantError: "connection closed"},
		{name: "discovery not found", getError: errors.New(`resource provider "Radius.Security" not found in the configured scope`), wantError: "resource provider"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				owner, child := managedSecretPair()
				client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
				calls := 0
				client.EXPECT().GetResource(gomock.Any(), *child.Type, *child.ID).
					DoAndReturn(func(context.Context, string, string) (generated.GenericResource, error) {
						calls++
						if tt.getError != nil {
							return generated.GenericResource{}, tt.getError
						}
						if calls <= len(tt.states) {
							return generated.GenericResource{Properties: map[string]any{"provisioningState": tt.states[calls-1]}}, nil
						}
						return generated.GenericResource{}, &azcore.ResponseError{StatusCode: http.StatusNotFound}
					}).AnyTimes()
				err := waitForManagedSecretDeletion(t.Context(), client, *child.ID, *owner.ID, false, false)
				if tt.wantError != "" {
					require.ErrorContains(t, err, tt.wantError)
					require.ErrorContains(t, err, *child.ID)
					require.ErrorContains(t, err, *owner.ID)
					if tt.getError != nil {
						require.ErrorIs(t, err, tt.getError)
					}
				} else {
					require.NoError(t, err)
					require.Equal(t, len(tt.states)+1, calls)
				}
			})
		})
	}
}

func Test_DeleteResourcesInParallel_MissingProducer(t *testing.T) {
	for _, tt := range []struct {
		name        string
		ownerError  error
		states      []string
		direct      bool
		force       bool
		deleteError error
	}{
		{name: "204 leaves inactive orphan", states: []string{"Succeeded"}, direct: true},
		{name: "404 leaves inactive orphan", ownerError: &azcore.ResponseError{StatusCode: http.StatusNotFound}, states: []string{"Succeeded"}, direct: true},
		{name: "both already absent"},
		{name: "earlier cascade completes", states: []string{"Accepted", "Updating", "Deleting"}},
		{name: "earlier cascade fails", states: []string{"Updating", "Failed"}, direct: true},
		{name: "canceled cleanup", states: []string{"Canceled"}, direct: true},
		{name: "empty terminal state", states: []string{""}, direct: true},
		{name: "force still waits for active operation", states: []string{"Updating", "Succeeded"}, direct: true, force: true},
		{name: "orphan delete fails", states: []string{"Succeeded"}, direct: true, deleteError: errors.New("orphan delete failed")},
		{name: "new concurrent operation conflicts", states: []string{"Succeeded"}, direct: true, deleteError: &azcore.ResponseError{StatusCode: http.StatusConflict}},
		{name: "orphan disappeared before delete", states: []string{"Succeeded"}, direct: true, deleteError: &azcore.ResponseError{StatusCode: http.StatusNotFound}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				owner, child := managedSecretPair()
				id := deleteResourceKey(*child.ID)
				client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
				client.EXPECT().DeleteResource(gomock.Any(), *owner.Type, *owner.ID, tt.force).Return(false, tt.ownerError)
				var sequence []any
				for _, state := range tt.states {
					sequence = append(sequence, client.EXPECT().GetResource(gomock.Any(), *child.Type, id).
						Return(generated.GenericResource{Properties: map[string]any{"provisioningState": state}}, nil))
				}
				if tt.direct {
					sequence = append(sequence, client.EXPECT().DeleteResource(gomock.Any(), *child.Type, id, tt.force).Return(true, tt.deleteError))
				}
				if tt.deleteError == nil || clients.Is404Error(tt.deleteError) {
					sequence = append(sequence, client.EXPECT().GetResource(gomock.Any(), *child.Type, id).
						Return(generated.GenericResource{}, &azcore.ResponseError{StatusCode: http.StatusNotFound}))
				}
				gomock.InOrder(sequence...)
				err := DeleteResourcesInParallel(t.Context(), client, &output.MockOutput{}, []generated.GenericResource{child, owner}, tt.force)
				if tt.deleteError != nil && !clients.Is404Error(tt.deleteError) {
					require.ErrorIs(t, err, tt.deleteError)
					require.ErrorContains(t, err, id)
				} else {
					require.NoError(t, err)
				}
			})
		})
	}
}

func Test_DeleteResourcesInParallel_ProducerErrors(t *testing.T) {
	for _, deleteErr := range []error{&azcore.ResponseError{StatusCode: http.StatusNotFound}, errors.New("producer delete failed")} {
		t.Run(deleteErr.Error(), func(t *testing.T) {
			owner, child := managedSecretPair()
			client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
			client.EXPECT().DeleteResource(gomock.Any(), *owner.Type, *owner.ID, false).Return(false, deleteErr)
			if clients.Is404Error(deleteErr) {
				client.EXPECT().GetResource(gomock.Any(), *child.Type, deleteResourceKey(*child.ID)).
					Return(generated.GenericResource{}, &azcore.ResponseError{StatusCode: http.StatusNotFound})
			}
			err := DeleteResourcesInParallel(t.Context(), client, &output.MockOutput{}, []generated.GenericResource{owner, child}, false)
			if clients.Is404Error(deleteErr) {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, deleteErr)
			}
		})
	}
}

func Test_DeleteResourcesInParallel_StandaloneAndOrphanSecrets(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%v", force), func(t *testing.T) {
			owner, managed := managedSecretPair()
			standalone := generated.GenericResource{ID: new(testScope + "/providers/Radius.Security/secrets/standalone"), Type: managed.Type}
			orphan := generated.GenericResource{ID: new(testScope + "/providers/Radius.Security/secrets/orphan"), Type: managed.Type}
			client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
			for _, resource := range []generated.GenericResource{owner, standalone, orphan} {
				client.EXPECT().DeleteResource(gomock.Any(), *resource.Type, *resource.ID, force).Return(true, nil)
			}
			client.EXPECT().GetResource(gomock.Any(), *managed.Type, deleteResourceKey(*managed.ID)).
				Return(generated.GenericResource{}, &azcore.ResponseError{StatusCode: http.StatusNotFound})
			require.NoError(t, DeleteResourcesInParallel(t.Context(), client, &output.MockOutput{}, []generated.GenericResource{owner, managed, managed, standalone, orphan}, force))

			// After a prior invocation removed the producer, a remaining secret must be deleted directly.
			client.EXPECT().DeleteResource(gomock.Any(), *managed.Type, *managed.ID, force).Return(true, nil)
			require.NoError(t, DeleteResourcesInParallel(t.Context(), client, &output.MockOutput{}, []generated.GenericResource{managed}, force))
		})
	}
}

func Test_DeleteResourcesInParallel_ConcurrencyAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		releaseDeletes := make(chan struct{})
		var deleting, reading atomic.Int32
		var selected []generated.GenericResource
		for i := range maxParallelDeletes + 2 {
			owner := testResource(fmt.Sprintf("producer-%d", i))
			name := fmt.Sprintf("secret-%d", i)
			owner.Properties = map[string]any{"secrets": map[string]any{"name": name}}
			child := generated.GenericResource{ID: new(testScope + "/providers/Radius.Security/secrets/" + name), Type: new("Radius.Security/secrets")}
			selected = append(selected, owner, child)
			client.EXPECT().DeleteResource(gomock.Any(), *owner.Type, *owner.ID, false).
				DoAndReturn(func(context.Context, string, string, bool) (bool, error) {
					deleting.Add(1)
					defer deleting.Add(-1)
					<-releaseDeletes
					return true, nil
				})
		}
		client.EXPECT().GetResource(gomock.Any(), "Radius.Security/secrets", gomock.Any()).
			DoAndReturn(func(ctx context.Context, _, _ string) (generated.GenericResource, error) {
				reading.Add(1)
				defer reading.Add(-1)
				<-ctx.Done()
				return generated.GenericResource{}, ctx.Err()
			}).AnyTimes()
		done := make(chan error, 1)
		go func() {
			done <- DeleteResourcesInParallel(ctx, client, &output.MockOutput{}, selected, false)
		}()
		synctest.Wait()
		require.EqualValues(t, maxParallelDeletes, deleting.Load())
		require.Zero(t, reading.Load())
		close(releaseDeletes)
		synctest.Wait()
		require.Zero(t, deleting.Load())
		require.EqualValues(t, maxParallelDeletes, reading.Load())
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
		synctest.Wait()
		require.Zero(t, reading.Load())
	})

	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
		require.ErrorIs(t, DeleteResourcesInParallel(ctx, client, &output.MockOutput{}, []generated.GenericResource{testResource("a")}, false), context.Canceled)
	})
}

func Test_DeleteResourcesInParallel_ManagedSecretDeadline(t *testing.T) {
	for _, timeout := range []time.Duration{managedSecretDeleteTimeout, 2 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				if timeout < managedSecretDeleteTimeout {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, timeout)
					defer cancel()
				}
				client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
				var selected []generated.GenericResource
				for i := range maxParallelDeletes + 1 {
					name := fmt.Sprintf("producer-%d", i)
					owner := testResource(name)
					owner.Properties = map[string]any{"secrets": map[string]any{"name": name}}
					child := generated.GenericResource{ID: new(testScope + "/providers/Radius.Security/secrets/" + name), Type: new("Radius.Security/secrets")}
					selected = append(selected, owner, child)
					client.EXPECT().DeleteResource(gomock.Any(), *owner.Type, *owner.ID, false).Return(true, nil)
				}
				client.EXPECT().GetResource(gomock.Any(), "Radius.Security/secrets", gomock.Any()).
					Return(generated.GenericResource{Properties: map[string]any{"provisioningState": "Updating"}}, nil).AnyTimes()
				start := time.Now()
				err := DeleteResourcesInParallel(ctx, client, &output.MockOutput{}, selected, false)
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.Equal(t, timeout, time.Since(start), "queued secrets must share one deadline")
			})
		})
	}
}

func Test_DeleteResourcesInParallel_ManagedSecretOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const secretType = "Radius.Security/secrets"
		owner, child := managedSecretPair()
		ctrl := gomock.NewController(t)
		client := clients.NewMockApplicationsManagementClient(ctrl)
		startCascade := make(chan struct{})
		finishCascade := make(chan struct{})
		var directDeletes atomic.Int32
		client.EXPECT().DeleteResource(gomock.Any(), secretType, *child.ID, false).
			DoAndReturn(func(ctx context.Context, _, _ string, _ bool) (bool, error) {
				directDeletes.Add(1)
				<-ctx.Done()
				return false, ctx.Err()
			}).AnyTimes()
		client.EXPECT().DeleteResource(gomock.Any(), testResourceType, *owner.ID, false).
			DoAndReturn(func(context.Context, string, string, bool) (bool, error) {
				<-startCascade
				if directDeletes.Load() != 0 {
					return false, fmt.Errorf("managed secret DELETE: 409 Conflict, state Updating")
				}
				return true, nil
			})
		client.EXPECT().GetResource(gomock.Any(), secretType, deleteResourceKey(*child.ID)).
			DoAndReturn(func(ctx context.Context, _, _ string) (generated.GenericResource, error) {
				select {
				case <-finishCascade:
					return generated.GenericResource{}, &azcore.ResponseError{StatusCode: http.StatusNotFound}
				case <-ctx.Done():
					return generated.GenericResource{}, ctx.Err()
				}
			}).AnyTimes()

		done := make(chan error, 1)
		go func() {
			done <- DeleteResourcesInParallel(t.Context(), client, &output.MockOutput{}, []generated.GenericResource{child, owner}, false)
		}()
		synctest.Wait()
		close(startCascade)
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("deletion completed before server-side secret cleanup: %v", err)
		default:
		}
		close(finishCascade)
		require.NoError(t, <-done)
		require.Zero(t, directDeletes.Load())
	})
}
