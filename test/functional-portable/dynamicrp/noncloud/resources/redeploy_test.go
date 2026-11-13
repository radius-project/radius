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

package resource_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/cli/recipepack"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_DynamicRP_Recipe_RedeployDeletesDroppedOutputResource verifies that a redeploy deletes a recipe output
// resource the recipe no longer returns, and that deleting the Radius resource deletes the current one.
//
// The recipe names a ConfigMap after the physicalName property, so changing physicalName replaces the ConfigMap.
func Test_DynamicRP_Recipe_RedeployDeletesDroppedOutputResource(t *testing.T) {
	template := "testdata/renamableconfigmap.bicep"
	recipePackTemplate := "testdata/renamableconfigmap-recipe-pack.bicep"
	recipePackName := "renamableconfigmap-recipe-pack"
	name := "dynamicrp-redeploy-gc"
	resourceTypeName := "Test.Resources/renamableConfigMaps"
	resourceTypeParam := strings.Split(resourceTypeName, "/")[1]
	resourceName := "renamableconfigmap"
	oldConfigMap := "dynamicrp-redeploy-gc-old"
	newConfigMap := "dynamicrp-redeploy-gc-new"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	// The resource has no application, so the recipe deploys to the environment's namespace.
	preSetup, envID := rp.NewPreviewEnvPreSetup(name, options.Workspace.Scope, name)
	envName := name + "-env"
	defaultPackID := recipepack.DefaultRecipePackID()
	customPackID := fmt.Sprintf("%s/providers/Radius.Core/recipePacks/%s", options.Workspace.Scope, recipePackName)

	configMapExists := func(ctx context.Context, t *testing.T, ct rp.RPTest, configMap string) bool {
		_, err := ct.Options.K8sClient.CoreV1().ConfigMaps(name).Get(ctx, configMap, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false
		}
		require.NoError(t, err)
		return true
	}
	deployStep := func(physicalName string, verify func(ctx context.Context, t *testing.T, ct rp.RPTest)) rp.TestStep {
		return rp.TestStep{
			Executor: step.NewDeployExecutor(template, "environment="+envID, "physicalName="+physicalName),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{{Name: resourceName, Type: resourceTypeName}},
			},
			SkipObjectValidation: true,
			SkipResourceDeletion: true,
			PostStepVerify:       verify,
		}
	}

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, resourceTypeParam, "testdata/testresourcetypes.yaml")
				require.NoError(t, err)
				err = cli.Deploy(ctx, recipePackTemplate, "", "", testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion())
				require.NoError(t, err)
				_, err = cli.EnvironmentUpdatePreview(ctx, envName, "", defaultPackID+","+customPackID, "")
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
		deployStep(oldConfigMap, func(ctx context.Context, t *testing.T, ct rp.RPTest) {
			require.True(t, configMapExists(ctx, t, ct, oldConfigMap))
		}),
		deployStep(newConfigMap, func(ctx context.Context, t *testing.T, ct rp.RPTest) {
			require.True(t, configMapExists(ctx, t, ct, newConfigMap))
			require.False(t, configMapExists(ctx, t, ct, oldConfigMap), "the redeploy should delete the ConfigMap the recipe no longer returns")
		}),
		{
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.RunCommand(ctx, []string{"resource", "delete", resourceTypeName, resourceName, "--yes"})
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				require.False(t, configMapExists(ctx, t, ct, newConfigMap))
				require.False(t, configMapExists(ctx, t, ct, oldConfigMap))
			},
		},
	})
	test.PreSetup = preSetup
	test.Test(t)
}
