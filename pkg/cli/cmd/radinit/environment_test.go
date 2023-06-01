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

package radinit

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_enterEnvironmentOptions(t *testing.T) {
	// Intentionally out of order to test sorting.
	environments := []corerp.EnvironmentResource{
		{
			Name: to.Ptr(defaultEnvironmentName),
		},
		{
			Name: to.Ptr("test-env2"),
		},
		{
			Name: to.Ptr("test-env3"),
		},
		{
			Name: to.Ptr("test-env1"),
		},
	}

	t.Run("radius not installed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter}

		initEnvNamePrompt(prompter, "test-env")
		initNamespacePrompt(prompter, "test-namespace")

		options := initOptions{Cluster: clusterOptions{Install: true}}
		err := runner.enterEnvironmentOptions(context.Background(), &workspaces.Workspace{}, &options)
		require.NoError(t, err)

		expected := environmentOptions{
			Create:    true,
			Name:      "test-env",
			Namespace: "test-namespace",
		}
		require.Equal(t, expected, options.Environment)
	})

	t.Run("create new environment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		applicationsClient := clients.NewMockApplicationsManagementClient(ctrl)
		connectionFactory := connections.MockFactory{
			ApplicationsManagementClient: applicationsClient,
		}
		runner := Runner{Prompter: prompter, ConnectionFactory: &connectionFactory}

		setExistingEnvironments(applicationsClient, environments)
		initExistingEnvironmentSelection(prompter, selectExistingEnvironmentCreateSentinel)
		initEnvNamePrompt(prompter, "test-env")
		initNamespacePrompt(prompter, "test-namespace")

		options := initOptions{}
		err := runner.enterEnvironmentOptions(context.Background(), &workspaces.Workspace{}, &options)
		require.NoError(t, err)

		expected := environmentOptions{
			Create:    true,
			Name:      "test-env",
			Namespace: "test-namespace",
		}
		require.Equal(t, expected, options.Environment)
	})

	t.Run("select existing environment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		applicationsClient := clients.NewMockApplicationsManagementClient(ctrl)
		connectionFactory := connections.MockFactory{
			ApplicationsManagementClient: applicationsClient,
		}
		runner := Runner{Prompter: prompter, ConnectionFactory: &connectionFactory}

		setExistingEnvironments(applicationsClient, environments)
		initExistingEnvironmentSelection(prompter, "test-env1")

		options := initOptions{}
		err := runner.enterEnvironmentOptions(context.Background(), &workspaces.Workspace{}, &options)
		require.NoError(t, err)

		expected := environmentOptions{
			Create: false,
			Name:   "test-env1",
		}
		require.Equal(t, expected, options.Environment)
	})
}

func Test_selectExistingEnvironment(t *testing.T) {
	// Intentionally out of order to test sorting.
	environments := []corerp.EnvironmentResource{
		{
			Name: to.Ptr(defaultEnvironmentName),
		},
		{
			Name: to.Ptr("test-env2"),
		},
		{
			Name: to.Ptr("test-env3"),
		},
		{
			Name: to.Ptr("test-env1"),
		},
	}

	t.Run("dev - chooses default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		applicationsClient := clients.NewMockApplicationsManagementClient(ctrl)
		connectionFactory := connections.MockFactory{
			ApplicationsManagementClient: applicationsClient,
		}
		runner := Runner{Prompter: prompter, ConnectionFactory: &connectionFactory, Dev: true}

		setExistingEnvironments(applicationsClient, environments)

		name, err := runner.selectExistingEnvironment(context.Background(), &workspaces.Workspace{})
		require.NoError(t, err)
		require.Equal(t, defaultEnvironmentName, *name)
	})

	t.Run("dev - no default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		applicationsClient := clients.NewMockApplicationsManagementClient(ctrl)
		connectionFactory := connections.MockFactory{
			ApplicationsManagementClient: applicationsClient,
		}
		runner := Runner{Prompter: prompter, ConnectionFactory: &connectionFactory, Dev: true}

		environments := environments[1:]
		setExistingEnvironments(applicationsClient, environments)
		initExistingEnvironmentSelection(prompter, "test-env1")

		name, err := runner.selectExistingEnvironment(context.Background(), &workspaces.Workspace{})
		require.NoError(t, err)
		require.Equal(t, "test-env1", *name)
	})

	t.Run("no existing environments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		applicationsClient := clients.NewMockApplicationsManagementClient(ctrl)
		connectionFactory := connections.MockFactory{
			ApplicationsManagementClient: applicationsClient,
		}
		runner := Runner{Prompter: prompter, ConnectionFactory: &connectionFactory}

		environments := []corerp.EnvironmentResource{}
		setExistingEnvironments(applicationsClient, environments)

		name, err := runner.selectExistingEnvironment(context.Background(), &workspaces.Workspace{})
		require.NoError(t, err)
		require.Nil(t, name)
	})

	t.Run("choose existing environment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		applicationsClient := clients.NewMockApplicationsManagementClient(ctrl)
		connectionFactory := connections.MockFactory{
			ApplicationsManagementClient: applicationsClient,
		}
		runner := Runner{Prompter: prompter, ConnectionFactory: &connectionFactory}

		setExistingEnvironments(applicationsClient, environments)
		initExistingEnvironmentSelection(prompter, "test-env1")

		name, err := runner.selectExistingEnvironment(context.Background(), &workspaces.Workspace{})
		require.NoError(t, err)
		require.Equal(t, "test-env1", *name)
	})

	t.Run("choose create new", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		applicationsClient := clients.NewMockApplicationsManagementClient(ctrl)
		connectionFactory := connections.MockFactory{
			ApplicationsManagementClient: applicationsClient,
		}
		runner := Runner{Prompter: prompter, ConnectionFactory: &connectionFactory}

		setExistingEnvironments(applicationsClient, environments)
		initExistingEnvironmentSelection(prompter, selectExistingEnvironmentCreateSentinel)

		name, err := runner.selectExistingEnvironment(context.Background(), &workspaces.Workspace{})
		require.NoError(t, err)
		require.Nil(t, name)
	})
}

func Test_buildExistingEnvironmentList(t *testing.T) {
	// Intentionally out of order to test sorting.
	environments := []corerp.EnvironmentResource{
		{
			Name: to.Ptr("test-env2"),
		},
		{
			Name: to.Ptr("test-env3"),
		},
		{
			Name: to.Ptr("test-env1"),
		},
		{
			Name: to.Ptr(defaultEnvironmentName),
		},
	}

	runner := Runner{}
	names := runner.buildExistingEnvironmentList(environments)
	require.Equal(t, []string{defaultEnvironmentName, "test-env1", "test-env2", "test-env3", selectExistingEnvironmentCreateSentinel}, names)
}

func Test_enterEnvironmentName(t *testing.T) {
	t.Run("dev", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, Dev: true}

		name, err := runner.enterEnvironmentName(context.Background())
		require.NoError(t, err)
		require.Equal(t, defaultEnvironmentName, name)
	})

	t.Run("non-dev", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter}

		initEnvNamePrompt(prompter, "test-name")

		name, err := runner.enterEnvironmentName(context.Background())
		require.NoError(t, err)
		require.Equal(t, "test-name", name)
	})
}

func Test_enterEnvironmentNamespace(t *testing.T) {
	t.Run("dev", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, Dev: true}

		namespace, err := runner.enterEnvironmentNamespace(context.Background())
		require.NoError(t, err)
		require.Equal(t, defaultEnvironmentNamespace, namespace)
	})

	t.Run("non-dev", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter}

		initNamespacePrompt(prompter, "test-namespace")

		namespace, err := runner.enterEnvironmentNamespace(context.Background())
		require.NoError(t, err)
		require.Equal(t, "test-namespace", namespace)
	})
}
