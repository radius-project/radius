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
	"fmt"
	"net/http"
	"testing"

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
