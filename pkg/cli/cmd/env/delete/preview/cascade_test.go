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

package preview

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
)

const (
	testScope         = "/planes/radius/local/resourceGroups/test-group"
	testEnvironmentID = testScope + "/providers/Radius.Core/environments/test-env"
	testResourceType  = "Applications.Datastores/redisCaches"
)

func testWorkspace() *workspaces.Workspace {
	return &workspaces.Workspace{
		Name:  "test-workspace",
		Scope: testScope,
	}
}

// deletedApplications records the applications deleted through the fake Applications server.
type deletedApplications struct {
	mu    sync.Mutex
	names []string
}

func (d *deletedApplications) add(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.names = append(d.names, name)
}

func (d *deletedApplications) list() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string{}, d.names...)
}

// applicationsServerWithEnvironment builds a fake Applications server whose list pager returns the
// given applications, and which records deletes into the supplied recorder.
func applicationsServerWithEnvironment(applications []*corerpv20250801.ApplicationResource, deleted *deletedApplications, deleteErr bool) func() fake.ApplicationsServer {
	return func() fake.ApplicationsServer {
		return fake.ApplicationsServer{
			NewListByScopePager: func(rootScope string, options *corerpv20250801.ApplicationsClientListByScopeOptions) (resp azfake.PagerResponder[corerpv20250801.ApplicationsClientListByScopeResponse]) {
				resp.AddPage(
					http.StatusOK,
					corerpv20250801.ApplicationsClientListByScopeResponse{
						ApplicationResourceListResult: corerpv20250801.ApplicationResourceListResult{
							Value: applications,
						},
					},
					nil,
				)
				return
			},
			Delete: func(
				ctx context.Context,
				rootScope string,
				applicationName string,
				options *corerpv20250801.ApplicationsClientDeleteOptions,
			) (resp azfake.Responder[corerpv20250801.ApplicationsClientDeleteResponse], errResp azfake.ErrorResponder) {
				if deleteErr {
					errResp.SetResponseError(http.StatusInternalServerError, "InternalServerError")
					return
				}

				if deleted != nil {
					deleted.add(applicationName)
				}
				resp.SetResponse(http.StatusNoContent, corerpv20250801.ApplicationsClientDeleteResponse{}, nil)
				return
			},
		}
	}
}

// application builds a Radius.Core application resource in the test scope pointing at environmentID.
func application(name string, environmentID string) *corerpv20250801.ApplicationResource {
	return &corerpv20250801.ApplicationResource{
		Name: to.Ptr(name),
		ID:   to.Ptr(testScope + "/providers/Radius.Core/applications/" + name),
		Properties: &corerpv20250801.ApplicationProperties{
			Environment: to.Ptr(environmentID),
		},
	}
}

// resource builds a generic resource with the given name.
func resource(name string) generated.GenericResource {
	return generated.GenericResource{
		ID:   to.Ptr(testScope + "/providers/" + testResourceType + "/" + name),
		Type: to.Ptr(testResourceType),
	}
}

func resourceIDFor(name string) string {
	return testScope + "/providers/" + testResourceType + "/" + name
}

// logFormats extracts the format strings of the logs written to the sink.
func logFormats(sink *output.MockOutput) []string {
	formats := []string{}
	for _, write := range sink.Writes {
		if log, ok := write.(output.LogOutput); ok {
			formats = append(formats, log.Format)
		}
	}
	return formats
}

func Test_Run_Cascade(t *testing.T) {
	t.Run("Success: environment not found is a no-op", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(testScope, test_client_factory.WithEnvironmentServer404OnGet, nil)
		require.NoError(t, err)

		// A management client that fails on any call proves nothing is enumerated after the 404.
		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			EnvironmentName:         "test-env",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Equal(t, []any{
			output.LogOutput{
				Format: msgEnvironmentNotFoundPreview,
				Params: []any{"test-env"},
			},
		}, outputSink.Writes)
	})

	t.Run("Success: empty environment is deleted without cascade logs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(nil, nil, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{}, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			EnvironmentName:         "test-env",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Equal(t, []any{
			output.LogOutput{
				Format: msgEnvironmentDeletedPreview,
				Params: []any{"test-env"},
			},
		}, outputSink.Writes)
	})

	t.Run("Success: resources and applications are cascade deleted before the environment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		apps := []*corerpv20250801.ApplicationResource{
			application("app-a", testEnvironmentID),
			// Belongs to a different environment and must be left alone.
			application("app-b", testScope+"/providers/Radius.Core/environments/other-env"),
		}

		deleted := &deletedApplications{}
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(apps, deleted, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{resource("env-scoped")}, nil).
			Times(1)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), testScope+"/providers/Radius.Core/applications/app-a").
			Return([]generated.GenericResource{resource("app-owned")}, nil).
			Times(1)
		mockMgmt.EXPECT().
			DeleteResource(gomock.Any(), testResourceType, resourceIDFor("env-scoped"), false).
			Return(true, nil).
			Times(1)
		mockMgmt.EXPECT().
			DeleteResource(gomock.Any(), testResourceType, resourceIDFor("app-owned"), false).
			Return(true, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			EnvironmentName:         "test-env",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)

		// Only the application in this environment is deleted.
		require.Equal(t, []string{"app-a"}, deleted.list())

		// Resources are deleted, then applications, then the environment.
		formats := logFormats(outputSink)
		require.Equal(t, []string{
			msgDeletingResources,
			cmd.MsgDeletingResource,
			cmd.MsgDeletingResource,
			msgDeletingApplications,
			msgDeletingApplication,
			msgEnvironmentDeletedPreview,
		}, formats)
	})

	t.Run("Success: a resource in both queries is deleted once", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		apps := []*corerpv20250801.ApplicationResource{application("app-a", testEnvironmentID)}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(apps, &deletedApplications{}, false),
		)
		require.NoError(t, err)

		shared := resource("shared")
		// The same resource, reported with different casing by the two queries.
		sharedUpper := generated.GenericResource{
			ID:   to.Ptr(strings.ToUpper(*shared.ID)),
			Type: shared.Type,
		}

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{shared}, nil).
			Times(1)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), gomock.Any()).
			Return([]generated.GenericResource{sharedUpper}, nil).
			Times(1)
		// Exactly one delete, using the ID from the first list.
		mockMgmt.EXPECT().
			DeleteResource(gomock.Any(), testResourceType, resourceIDFor("shared"), false).
			Return(true, nil).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  &output.MockOutput{},
			EnvironmentName:         "test-env",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
	})

	t.Run("Success: prompt reports the resource count and deletion proceeds on yes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput(
				[]string{prompt.ConfirmNo, prompt.ConfirmYes},
				"The environment test-env contains 1 deployed resource(s). Are you sure you want to delete the environment and its resources?",
			).
			Return(prompt.ConfirmYes, nil).
			Times(1)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(nil, nil, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{resource("env-scoped")}, nil).
			Times(1)
		mockMgmt.EXPECT().
			DeleteResource(gomock.Any(), testResourceType, resourceIDFor("env-scoped"), false).
			Return(true, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			InputPrompter:           promptMock,
			EnvironmentName:         "test-env",
			Confirm:                 false,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Contains(t, logFormats(outputSink), msgEnvironmentDeletedPreview)
	})

	t.Run("Success: prompt reports applications when the environment has no enumerable resources", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// An environment holding an application but no enumerable resources must not be described
		// as empty, or the user would consent to deleting the application without being told.
		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput(
				[]string{prompt.ConfirmNo, prompt.ConfirmYes},
				"The environment test-env contains 1 application(s) and 0 deployed resource(s). Are you sure you want to delete the environment, its applications and its resources?",
			).
			Return(prompt.ConfirmNo, nil).
			Times(1)

		deleted := &deletedApplications{}
		apps := []*corerpv20250801.ApplicationResource{application("app-a", testEnvironmentID)}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(apps, deleted, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{}, nil).
			Times(1)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), gomock.Any()).
			Return([]generated.GenericResource{}, nil).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  &output.MockOutput{},
			InputPrompter:           promptMock,
			EnvironmentName:         "test-env",
			Confirm:                 false,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Empty(t, deleted.list())
	})

	t.Run("Success: prompt reports an empty environment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput(
				[]string{prompt.ConfirmNo, prompt.ConfirmYes},
				"The environment test-env is empty. Are you sure you want to delete the environment?",
			).
			Return(prompt.ConfirmNo, nil).
			Times(1)

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(nil, nil, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{}, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			InputPrompter:           promptMock,
			EnvironmentName:         "test-env",
			Confirm:                 false,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Empty(t, outputSink.Writes)
	})

	t.Run("Success: declining the prompt deletes nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		promptMock := prompt.NewMockInterface(ctrl)
		promptMock.EXPECT().
			GetListInput(gomock.Any(), gomock.Any()).
			Return(prompt.ConfirmNo, nil).
			Times(1)

		deleted := &deletedApplications{}
		apps := []*corerpv20250801.ApplicationResource{application("app-a", testEnvironmentID)}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(apps, deleted, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{resource("env-scoped")}, nil).
			Times(1)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), gomock.Any()).
			Return([]generated.GenericResource{}, nil).
			Times(1)
		// No DeleteResource call is expected.

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			InputPrompter:           promptMock,
			EnvironmentName:         "test-env",
			Confirm:                 false,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Empty(t, deleted.list())
		require.Empty(t, outputSink.Writes)
	})

	t.Run("Success: --force is passed through to resource deletes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(nil, nil, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{resource("env-scoped")}, nil).
			Times(1)
		mockMgmt.EXPECT().
			DeleteResource(gomock.Any(), testResourceType, resourceIDFor("env-scoped"), true).
			Return(true, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			EnvironmentName:         "test-env",
			Confirm:                 true,
			Force:                   true,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		require.Equal(t, msgForceWarning, logFormats(outputSink)[0])
	})

	t.Run("Failure: resource delete failure stops before the environment is deleted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(nil, nil, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{resource("env-scoped")}, nil).
			Times(1)
		mockMgmt.EXPECT().
			DeleteResource(gomock.Any(), testResourceType, resourceIDFor("env-scoped"), false).
			Return(false, fmt.Errorf("simulated delete failure")).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			EnvironmentName:         "test-env",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed to delete resources in environment 'test-env'")
		require.NotContains(t, logFormats(outputSink), msgEnvironmentDeletedPreview)
	})

	t.Run("Failure: application delete failure stops before the environment is deleted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		apps := []*corerpv20250801.ApplicationResource{application("app-a", testEnvironmentID)}

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(apps, nil, true),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return([]generated.GenericResource{}, nil).
			Times(1)
		mockMgmt.EXPECT().
			ListResourcesInApplication(gomock.Any(), gomock.Any()).
			Return([]generated.GenericResource{}, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  outputSink,
			EnvironmentName:         "test-env",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed to delete application 'app-a' in environment 'test-env'")
		require.NotContains(t, logFormats(outputSink), msgEnvironmentDeletedPreview)
	})

	t.Run("Failure: environment resource enumeration failure surfaces error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			testScope,
			test_client_factory.WithEnvironmentServerNoError,
			nil,
			applicationsServerWithEnvironment(nil, nil, false),
		)
		require.NoError(t, err)

		mockMgmt := clients.NewMockApplicationsManagementClient(ctrl)
		mockMgmt.EXPECT().
			ListResourcesInEnvironment(gomock.Any(), testEnvironmentID).
			Return(nil, fmt.Errorf("simulated list error")).
			Times(1)

		runner := &Runner{
			RadiusCoreClientFactory: factory,
			ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: mockMgmt},
			Workspace:               testWorkspace(),
			Output:                  &output.MockOutput{},
			EnvironmentName:         "test-env",
			Confirm:                 true,
		}

		err = runner.Run(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated list error")
	})
}
