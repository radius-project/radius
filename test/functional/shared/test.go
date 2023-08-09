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

package shared

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/stretchr/testify/require"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/testcontext"
)

// # Function Explanation
// 
// NewRPTestOptions sets up the test environment by loading configs, creating a test context, creating an 
// ApplicationsManagementClient, creating an AWSCloudControlClient, and returning an RPTestOptions struct.
func NewRPTestOptions(t *testing.T) RPTestOptions {
	registry, tag := functional.SetDefault()
	t.Logf("Using container registry: %s - set DOCKER_REGISTRY to override", registry)
	t.Logf("Using container tag: %s - set REL_VERSION to override", tag)
	t.Logf("Using magpie image: %s/magpiego:%s", registry, tag)

	_, bicepRecipeRegistry, _ := strings.Cut(functional.GetBicepRecipeRegistry(), "=")
	_, bicepRecipeTag, _ := strings.Cut(functional.GetBicepRecipeVersion(), "=")
	t.Logf("Using recipe registry: %s - set BICEP_RECIPE_REGISTRY to override", bicepRecipeRegistry)
	t.Logf("Using recipe tag: %s - set BICEP_RECIPE_TAG_VERSION to override", bicepRecipeTag)

	_, terraformRecipeModuleServerURL, _ := strings.Cut(functional.GetTerraformRecipeModuleServerURL(), "=")
	t.Logf("Using terraform recipe module server URL: %s - set TF_RECIPE_MODULE_SERVER_URL to override", terraformRecipeModuleServerURL)

	ctx := testcontext.New(t)

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	workspace, err := cli.GetWorkspace(config, "")
	require.NoError(t, err, "failed to read default workspace")

	t.Logf("Loaded workspace: %s (%s)", workspace.Name, workspace.FmtConnection())

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(ctx, *workspace)
	require.NoError(t, err, "failed to create ApplicationsManagementClient")

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	var awsClient aws.AWSCloudControlClient = cloudcontrol.NewFromConfig(cfg)

	return RPTestOptions{
		TestOptions:      test.NewTestOptions(t),
		ManagementClient: client,
		AWSClient:        awsClient,
	}
}

type RPTestOptions struct {
	test.TestOptions
	ManagementClient clients.ApplicationsManagementClient
	AWSClient        aws.AWSCloudControlClient
}
