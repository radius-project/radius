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
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
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
		ID:   to.Ptr(testScope + "/providers/" + testResourceType + "/" + name),
		Type: to.Ptr(testResourceType),
	}
}

func Test_PreviewResourceIDs(t *testing.T) {
	require.Equal(t, testScope+"/providers/Radius.Core/applications/my-app", PreviewApplicationID(testScope, "my-app"))
	require.Equal(t, testScope+"/providers/Radius.Core/environments/my-env", PreviewEnvironmentID(testScope, "my-env"))
}

func Test_MergeResourcesByID(t *testing.T) {
	first := testResource("a")
	second := testResource("b")

	// Same resource as first, reported with different casing.
	duplicate := generated.GenericResource{ID: to.Ptr(strings.ToUpper(*first.ID)), Type: first.Type}

	// Resources without an ID cannot be deleted and are dropped.
	noID := generated.GenericResource{Type: to.Ptr(testResourceType)}

	merged := MergeResourcesByID(
		[]generated.GenericResource{first, noID},
		[]generated.GenericResource{duplicate, second},
	)

	require.Equal(t, []generated.GenericResource{first, second}, merged)
}

func Test_MergeResourcesByID_PrefersRepresentationWithType(t *testing.T) {
	// A resource can be reported by one enumeration without its type, which makes it
	// undeletable. When another enumeration reports the same ID with a type, the deletable
	// representation must win regardless of which one is seen first.
	typed := testResource("a")
	untyped := generated.GenericResource{ID: to.Ptr(strings.ToUpper(*typed.ID))}

	require.Equal(t,
		[]generated.GenericResource{typed},
		MergeResourcesByID([]generated.GenericResource{untyped}, []generated.GenericResource{typed}),
		"a later typed duplicate should replace an earlier untyped one")

	require.Equal(t,
		[]generated.GenericResource{typed},
		MergeResourcesByID([]generated.GenericResource{typed}, []generated.GenericResource{untyped}),
		"an earlier typed entry should not be replaced by a later untyped duplicate")
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

	t.Run("skips resources without an ID or type", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		valid := testResource("a")

		mock := clients.NewMockApplicationsManagementClient(ctrl)
		mock.EXPECT().DeleteResource(gomock.Any(), testResourceType, *valid.ID, false).Return(true, nil).Times(1)

		resources := []generated.GenericResource{
			valid,
			{Type: to.Ptr(testResourceType)},
			{ID: to.Ptr(testScope + "/providers/" + testResourceType + "/c")},
		}

		sink := &output.MockOutput{}
		err := DeleteResourcesInParallel(t.Context(), mock, sink, resources, false)
		require.NoError(t, err)
		require.Len(t, sink.Writes, 1)
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
