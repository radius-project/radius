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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/cmd/radinit"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/version"

	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
)

const (
	retries = 10
)

func verifyRecipeCLI(ctx context.Context, t *testing.T, test rp.RPTest) {
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	envName := test.Steps[0].RPResources.Resources[0].Name
	registry := strings.TrimPrefix(testutil.GetBicepRecipeRegistry(), "registry=")
	version := strings.TrimPrefix(testutil.GetBicepRecipeVersion(), "version=")
	resourceType := "Applications.Datastores/redisCaches"
	file := "../../../corerp/noncloud/resources/testdata/recipes/test-bicep-recipes/corerp-redis-recipe.bicep"
	target := fmt.Sprintf("br:ghcr.io/radius-project/dev/test-bicep-recipes/redis-recipe:%s", generateUniqueTag())

	recipeName := "recipeName"
	recipeTemplate := fmt.Sprintf("%s/recipes/local-dev/rediscaches:%s", registry, version)

	bicepRecipe := "recipe1"
	bicepRecipeTemplate := fmt.Sprintf("%s/test/functional-portable/corerp/noncloud/resources/testdata/recipes/test-bicep-recipes/corerp-redis-recipe.bicep:%s", registry, version)
	templateKindBicep := "bicep"

	terraformRecipe := "recipe2"
	terraformRecipeTemplate := "Azure/cosmosdb/azurerm"
	templateKindTerraform := "terraform"

	t.Run("Validate rad recipe register", func(t *testing.T) {
		output, err := cli.RecipeRegister(ctx, envName, recipeName, templateKindBicep, recipeTemplate, resourceType, false)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully linked recipe")
	})

	t.Run("Validate rad recipe register with insecure registry", func(t *testing.T) {
		output, err := cli.RecipeRegister(ctx, envName, recipeName, templateKindBicep, recipeTemplate, resourceType, true)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully linked recipe")
	})

	t.Run("Validate rad recipe list", func(t *testing.T) {
		output, err := cli.RecipeList(ctx, envName)
		require.NoError(t, err)
		require.Regexp(t, bicepRecipe, output)
		require.Regexp(t, terraformRecipe, output)
		require.Regexp(t, recipeName, output)
		require.Regexp(t, resourceType, output)
		require.Regexp(t, bicepRecipeTemplate, output)
		require.Regexp(t, terraformRecipeTemplate, output)
		require.Regexp(t, recipeTemplate, output)
		require.Regexp(t, templateKindBicep, output)
		require.Regexp(t, templateKindTerraform, output)
	})

	t.Run("Validate rad recipe unregister", func(t *testing.T) {
		output, err := cli.RecipeUnregister(ctx, envName, recipeName, resourceType)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully unregistered recipe")
	})

	t.Run("Validate rad recipe show", func(t *testing.T) {
		output, err := cli.RecipeShow(ctx, envName, bicepRecipe, resourceType)
		require.NoError(t, err)
		require.Contains(t, output, bicepRecipe)
		require.Contains(t, output, bicepRecipeTemplate)
		require.Contains(t, output, resourceType)
		require.Contains(t, output, "redisName")
		require.Contains(t, output, "string")
	})

	t.Run("Validate rad recipe show - terraform recipe", func(t *testing.T) {
		showRecipeName := "redistesttf"
		moduleServer := strings.TrimPrefix(testutil.GetTerraformRecipeModuleServerURL(), "moduleServer=")
		showRecipeTemplate := fmt.Sprintf("%s/kubernetes-redis.zip//modules", moduleServer)
		showRecipeResourceType := "Applications.Datastores/redisCaches"
		output, err := cli.RecipeRegister(ctx, envName, showRecipeName, "terraform", showRecipeTemplate, showRecipeResourceType, false)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully linked recipe")
		output, err = cli.RecipeShow(ctx, envName, showRecipeName, showRecipeResourceType)
		require.NoError(t, err)
		require.Contains(t, output, showRecipeName)
		require.Contains(t, output, showRecipeTemplate)
		require.Contains(t, output, showRecipeResourceType)
		require.Contains(t, output, "redis_cache_name")
		require.Contains(t, output, "string")
	})

	t.Run("Validate `rad bicep publish` is publishing the file to the given target", func(t *testing.T) {
		output, err := cli.BicepPublish(ctx, file, target)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully published")
	})

	t.Run("Validate rad recipe register with recipe name conflicting with existing recipe", func(t *testing.T) {
		output, err := cli.RecipeRegister(ctx, envName, bicepRecipe, templateKindBicep, recipeTemplate, resourceType, false)
		require.Contains(t, output, "Successfully linked recipe")
		require.NoError(t, err)
		output, err = cli.RecipeList(ctx, envName)
		require.NoError(t, err)
		require.Regexp(t, recipeTemplate, output)
	})
}

func verifyCLIBasics(ctx context.Context, t *testing.T, test rp.RPTest) {
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	appName := test.Name
	containerName := "containerA"
	if strings.EqualFold(appName, "kubernetes-cli-json") {
		containerName = "containerA-json"
	}

	scope, err := resources.ParseScope(options.Workspace.Scope)
	require.NoError(t, err)

	t.Run("Validate rad application show", func(t *testing.T) {
		actualOutput, err := cli.ApplicationShow(ctx, appName)
		require.NoError(t, err)

		lines := strings.Split(actualOutput, "\n")
		require.GreaterOrEqual(t, len(lines), 2, "Actual output should have 2 lines")

		headers := strings.Fields(lines[0])
		require.Equal(t, "RESOURCE", headers[0], "First header should be RESOURCE")
		require.Equal(t, "TYPE", headers[1], "Second header should be TYPE")
		require.Equal(t, "GROUP", headers[2], "Third header should be GROUP")
		require.Equal(t, "STATE", headers[3], "Fourth header should be STATE")

		values := strings.Fields(lines[1])
		require.Equal(t, appName, values[0], "First value should be %s", appName)
		require.Equal(t, "Applications.Core/applications", values[1], "Second value should be Applications.Core/applications")
		require.Equal(t, scope.Name(), values[2], "Third value should be %s", scope.Name())
		require.Equal(t, "Succeeded", values[3], "Fourth value should be Succeeded")
	})

	t.Run("Validate rad resource list", func(t *testing.T) {
		output, err := cli.ResourceList(ctx, appName)
		require.NoError(t, err)

		// Resource ordering can vary so we don't assert exact output.
		if strings.EqualFold(appName, "kubernetes-cli") {
			require.Regexp(t, `containerA`, output)
			require.Regexp(t, `containerB`, output)
		} else {
			require.Regexp(t, `containerA-json`, output)
			require.Regexp(t, `containerB-json`, output)
		}
	})

	t.Run("Validate rad resource show", func(t *testing.T) {
		actualOutput, err := cli.ResourceShow(ctx, "containers", containerName)
		require.NoError(t, err)

		lines := strings.Split(actualOutput, "\n")
		require.GreaterOrEqual(t, len(lines), 2, "Actual output should have 2 lines")

		headers := strings.Fields(lines[0])
		require.Equal(t, "RESOURCE", headers[0], "First header should be RESOURCE")
		require.Equal(t, "TYPE", headers[1], "Second header should be TYPE")
		require.Equal(t, "GROUP", headers[2], "Third header should be GROUP")
		require.Equal(t, "STATE", headers[3], "Fourth header should be STATE")

		values := strings.Fields(lines[1])
		require.Equal(t, containerName, values[0], "First value should be %s", containerName)
		require.Equal(t, "Applications.Core/containers", values[1], "Second value should be Applications.Core/applications")
		require.Equal(t, scope.Name(), values[2], "Third value should be %s", scope.Name())
		require.Equal(t, "Succeeded", values[3], "Fourth value should be Succeeded")
	})

	t.Run("Validate rad resoure logs containers", func(t *testing.T) {
		output, err := cli.ResourceLogs(ctx, appName, containerName)
		require.NoError(t, err)

		// We don't want to be too fragile so we're not validating the logs in depth
		require.Contains(t, output, "Server running at http://localhost:3000")
	})

	t.Run("Validate rad resource expose Container", func(t *testing.T) {
		port, err := GetAvailablePort()
		require.NoError(t, err)

		// We open a local port-forward and then make a request to it.
		child, cancel := context.WithCancel(ctx)

		done := make(chan error)
		go func() {
			output, err := cli.ResourceExpose(child, appName, containerName, port, 3000)
			t.Logf("ResourceExpose - output: %s", output)
			done <- err
		}()

		callHealthEndpointOnLocalPort(t, retries, port)

		cancel()
		err = <-done

		// The error should be due to cancellation (we can canceled the command).
		require.Equal(t, context.Canceled, err)
	})
}

// callHealthEndpointOnLocalPort calls the magpie health endpoint '/healthz' with retries. It will fail the
// test if the exceed the number of retries without success.
func callHealthEndpointOnLocalPort(t *testing.T, retries int, port int) {
	healthzURL := fmt.Sprintf("http://localhost:%d/healthz", port)

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = retries
	retryClient.RetryWaitMin = 5 * time.Second
	retryClient.RetryWaitMax = 20 * time.Second
	retryClient.Backoff = retryablehttp.LinearJitterBackoff
	retryClient.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, retry int) {
		t.Logf("retry calling healthz endpoint %s, retry: %d", healthzURL, retry)
	}

	resp, err := retryClient.Get(healthzURL)
	require.NoError(t, err, "failed to get connect to resource after %d retries", retries)
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	t.Logf("[response] %s", string(content))
}

func Test_Run_Logger(t *testing.T) {
	// Will be used to cancel `rad run`
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)
	options := rp.NewRPTestOptions(t)

	template := "testdata/corerp-kubernetes-cli-run.bicep"
	applicationName := "cli-run-logger"

	cwd, err := os.Getwd()
	require.NoError(t, err)

	cli := radcli.NewCLI(t, options.ConfigFilePath)

	args := []string{
		"run",
		filepath.Join(cwd, template),
		"--application",
		applicationName,
		"--parameters",
		testutil.GetMagpieImage(),
	}

	// 'rad run' streams logs until canceled by the user. This is why we can't 'just' run the command in
	// the test, because we have to decide when to shut down.
	//
	// The challenge with this command is that we want to stream the log output as it comes out so we can find
	// the output we expect and shut down the test. We don't want to use a fixed timeout and make a test
	// that takes forever to run even on the happy path.
	cmd, heartbeat, description := cli.CreateCommand(ctx, args)

	// Read from stdout to get the logs.
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	// Start heartbeat to trigger logging
	done := make(chan struct{})
	go heartbeat(done)

	// Read the text line-by-line while the command is running, but store it so we can report failures.
	output := bytes.Buffer{}
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line)
		output.WriteString("\n")
		if strings.Contains(line, "hello from the streaming logs!") {
			cancel() // Stop the command, but keep reading to drain all output.
		}
	}

	// It's only safe to call wait when we've read all of the output.
	err = cmd.Wait()
	err = cli.ReportCommandResult(ctx, output.String(), description, err)

	// Now we can delete the application (before we report pass/fail)
	t.Run("delete application", func(t *testing.T) {
		// Create a new context since we canceled the outer one.
		ctx, cancel := testcontext.NewWithCancel(t)
		t.Cleanup(cancel)

		err := cli.ApplicationDelete(ctx, applicationName)
		require.NoErrorf(t, err, "failed to delete %s", applicationName)
	})

	// We should have an error, but only because we canceled the context.
	require.Errorf(t, err, "rad run should have been canceled")
	require.Equal(t, err, ctx.Err(), "rad run should have been canceled")
}

func Test_Run_Portforward(t *testing.T) {
	// Will be used to cancel `rad run`
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)
	options := rp.NewRPTestOptions(t)

	template := "testdata/corerp-kubernetes-cli-run-portforward.bicep"
	applicationName := "cli-run-portforward"

	cwd, err := os.Getwd()
	require.NoError(t, err)

	cli := radcli.NewCLI(t, options.ConfigFilePath)

	args := []string{
		"run",
		filepath.Join(cwd, template),
		"--application",
		applicationName,
		"--parameters",
		testutil.GetMagpieImage(),
	}

	// 'rad run' streams logs until canceled by the user. This is why we can't 'just' run the command in
	// the test, because we have to decide when to shut down.
	//
	// The challenge with this command is that we want to stream the log output as it comes out so we can find
	// the output we expect and shut down the test. We don't want to use a fixed timeout and make a test
	// that takes forever to run even on the happy path.
	cmd, heartbeat, description := cli.CreateCommand(ctx, args)

	// Read from stdout to get the logs.
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	// Start heartbeat to trigger logging
	done := make(chan struct{})
	go heartbeat(done)

	// Read the text line-by-line while the command is running, but store it so we can report failures.
	output := bytes.Buffer{}
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)

	dashboardRegex := regexp.MustCompile(`.* dashboard \[port-forward\] .* from localhost:(.*) -> ::.*`)
	appRegex := regexp.MustCompile(`.* k8s-cli-run-portforward \[port-forward\] .* from localhost:(.*) -> ::.*`)

	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line)
		output.WriteString("\n")

		dashboardMatches := dashboardRegex.FindSubmatch([]byte(line))
		if len(dashboardMatches) == 2 {
			t.Log("found matching output", line)

			// Found the portforward local port.
			port, err := strconv.Atoi(string(dashboardMatches[1]))
			require.NoErrorf(t, err, "port is not an integer")
			t.Logf("found local port %d", port)
			require.Equal(t, 7007, port, "dashboard port should be 7007")
		}

		matches := appRegex.FindSubmatch([]byte(line))
		if len(matches) == 2 {
			t.Log("found matching output", line)

			// Found the portforward local port.
			port, err := strconv.Atoi(string(matches[1]))
			require.NoErrorf(t, err, "port is not an integer")
			t.Logf("found local port %d", port)
			callHealthEndpointOnLocalPort(t, retries, port)

			cancel() // Stop the command, but keep reading to drain all output.
		}
	}

	// It's only safe to call wait when we've read all of the output.
	err = cmd.Wait()
	err = cli.ReportCommandResult(ctx, output.String(), description, err)

	// Now we can delete the application (before we report pass/fail)
	t.Run("delete application", func(t *testing.T) {
		// Create a new context since we canceled the outer one.
		ctx, cancel := testcontext.NewWithCancel(t)
		t.Cleanup(cancel)

		err := cli.ApplicationDelete(ctx, applicationName)
		require.NoErrorf(t, err, "failed to delete %s", applicationName)
	})

	// We should have an error, but only because we canceled the context.
	require.Errorf(t, err, "rad run should have been canceled")
	require.Equal(t, err, ctx.Err(), "rad run should have been canceled")
}

func Test_CLI(t *testing.T) {
	template := "testdata/corerp-kubernetes-cli.bicep"
	name := "kubernetes-cli"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "kubernetes-cli",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "containerA",
						Type: validation.ContainersResource,
						App:  "kubernetes-cli",
					},
					{
						Name: "containerB",
						Type: validation.ContainersResource,
						App:  "kubernetes-cli",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default-kubernetes-cli": {
						validation.NewK8sPodForResource(name, "containera"),
						validation.NewK8sPodForResource(name, "containerb"),
					},
				},
			},
			PostStepVerify: verifyCLIBasics,
		},
	})

	test.Test(t)
}

func Test_CLI_JSON(t *testing.T) {
	template := "testdata/corerp-kubernetes-cli.json"
	name := "kubernetes-cli-json"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "kubernetes-cli-json",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "containerA-json",
						Type: validation.ContainersResource,
						App:  "kubernetes-cli-json",
					},
					{
						Name: "containerB-json",
						Type: validation.ContainersResource,
						App:  "kubernetes-cli-json",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default-kubernetes-cli-json": {
						validation.NewK8sPodForResource(name, "containera-json"),
						validation.NewK8sPodForResource(name, "containerb-json"),
					},
				},
			},
			PostStepVerify: verifyCLIBasics,
		},
	})

	test.Test(t)
}

func Test_CLI_Delete(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	options := rp.NewRPTestOptions(t)
	appName := "kubernetes-cli-with-resources"
	appNameUnassociatedResources := "kubernetes-cli-with-unassociated-resources"
	appNameEmptyResources := "kubernetes-cli-empty-resources"

	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateWithResources := "testdata/corerp-kubernetes-cli-with-resources.bicep"
	templateFilePathWithResources := filepath.Join(cwd, templateWithResources)

	templateWithResourcesUnassociated := "testdata/corerp-kubernetes-cli-with-unassociated-resources.bicep"
	templateFilePathWithResourcesUnassociated := filepath.Join(cwd, templateWithResourcesUnassociated)

	templateEmptyResources := "testdata/corerp-kubernetes-cli-app-empty-resources.bicep"
	templateFilePathEmptyResources := filepath.Join(cwd, templateEmptyResources)

	cli := radcli.NewCLI(t, options.ConfigFilePath)

	t.Run("Validate rad app delete with non empty resources", func(t *testing.T) {
		t.Logf("deploying %s from file %s", appName, templateWithResources)

		err = cli.Deploy(ctx, templateFilePathWithResources, "", appName, testutil.GetMagpieImage())
		require.NoErrorf(t, err, "failed to deploy %s", appName)

		validation.ValidateObjectsRunning(ctx, t, options.K8sClient, options.DynamicClient, validation.K8sObjectSet{
			Namespaces: map[string][]validation.K8sObject{
				"default-kubernetes-cli-with-resources": {
					validation.NewK8sPodForResource(appName, "containera-app-with-resources"),
					validation.NewK8sPodForResource(appName, "containerb-app-with-resources"),
				},
			},
		})

		err = cli.ApplicationDelete(ctx, appName)
		require.NoErrorf(t, err, "failed to delete %s", appName)
	})

	t.Run("Validate rad app delete with empty resources", func(t *testing.T) {
		t.Logf("deploying %s from file %s", appNameEmptyResources, templateEmptyResources)

		err = cli.Deploy(ctx, templateFilePathEmptyResources, "", appNameEmptyResources)
		require.NoErrorf(t, err, "failed to deploy %s", appNameEmptyResources)

		err = cli.ApplicationDelete(ctx, appNameEmptyResources)
		require.NoErrorf(t, err, "failed to delete %s", appNameEmptyResources)
	})

	t.Run("Validate rad app delete with non existent app", func(t *testing.T) {
		err = cli.ApplicationDelete(ctx, appName)
		require.NoErrorf(t, err, "failed to delete %s", appName)
	})

	t.Run("Validate rad app delete with resources not associated with any application", func(t *testing.T) {
		t.Logf("deploying from file %s", templateWithResources)

		err := cli.Deploy(ctx, templateFilePathWithResourcesUnassociated, "", appNameUnassociatedResources, testutil.GetMagpieImage())
		require.NoErrorf(t, err, "failed to deploy %s", appNameUnassociatedResources)

		validation.ValidateObjectsRunning(ctx, t, options.K8sClient, options.DynamicClient, validation.K8sObjectSet{
			Namespaces: map[string][]validation.K8sObject{
				"default-kubernetes-cli-with-unassociated-resources": {
					validation.NewK8sPodForResource(appNameUnassociatedResources, "containerX"),
					validation.NewK8sPodForResource(appNameUnassociatedResources, "containerY"),
				},
			},
		})

		//ignore response for tests
		_, err = options.ManagementClient.DeleteResource(ctx, "Applications.Core/containers", "containerY")
		require.NoErrorf(t, err, "failed to delete resource containerY")
		err = DeleteAppWithoutDeletingResources(t, ctx, options, appNameUnassociatedResources)
		require.NoErrorf(t, err, "failed to delete application %s", appNameUnassociatedResources)

		t.Logf("deploying from file %s", templateEmptyResources)
		err = cli.Deploy(ctx, templateFilePathEmptyResources, "", appNameEmptyResources)
		require.NoErrorf(t, err, "failed to deploy %s", appNameEmptyResources)

		err = cli.ApplicationDelete(ctx, appNameEmptyResources)
		require.NoErrorf(t, err, "failed to delete %s", appNameEmptyResources)

		//ignore response for tests
		_, err = options.ManagementClient.DeleteResource(ctx, "Applications.Core/containers", "containerX")
		require.NoErrorf(t, err, "failed to delete resource containerX")

	})
}

func Test_CLI_DeploymentParameters(t *testing.T) {
	template := "testdata/corerp-kubernetes-cli-parameters.bicep"
	name := "kubernetes-cli-params"

	registry, _ := testutil.SetDefault()
	parameterFilePath := testutil.WriteBicepParameterFile(t, map[string]any{"registry": registry})

	// corerp-kubernetes-cli-parameters.parameters.json uses ghcr.io/radius-project/dev as a registry parameter.
	// Use the specified tag only if the test uses ghcr.io/radius-project/dev registry. Otherwise, use latest tag.

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, "@"+parameterFilePath, testutil.GetMagpieTag()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "kubernetes-cli-params",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "containerC",
						Type: validation.ContainersResource,
						App:  "kubernetes-cli-params",
					},
					{
						Name: "containerD",
						Type: validation.ContainersResource,
						App:  "kubernetes-cli-params",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default-kubernetes-cli-params": {
						validation.NewK8sPodForResource(name, "containerc"),
						validation.NewK8sPodForResource(name, "containerd"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_CLI_version(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	options := rp.NewTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	output, err := cli.Version(ctx)
	require.NoError(t, err)

	// Matching logic:
	//
	// Release: any word or number characters or hyphen or dot
	// Version: any work or number characters or hyphen or dot
	// Bicep: Semver
	// Commit: any lowercase word or number characters
	matcher := fmt.Sprintf(`RELEASE\s+VERSION\s+BICEP\s+COMMIT\s*([a-zA-Z0-9-\.]+)\s+([a-zA-Z0-9-\.]+)\s+(%s)\s+([a-z0-9]+)`, bicep.SemanticVersionRegex)
	expected := regexp.MustCompile(matcher)
	require.Regexp(t, expected, objectformats.TrimSpaceMulti(output))
}

func Test_CLI_Only_version(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	options := rp.NewTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	output, err := cli.CliVersion(ctx)
	require.NoError(t, err)

	// Matching logic:
	//
	// Version: any work or number characters or hyphen or dot
	matcher := `([a-zA-Z0-9-\.]+)`
	expected := regexp.MustCompile(matcher)
	require.Regexp(t, expected, objectformats.TrimSpaceMulti(output))
}

func Test_RecipeCommands(t *testing.T) {
	template := "testdata/corerp-resources-recipe-env.bicep"
	name := "corerp-resources-recipe-env"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-recipe-env",
						Type: validation.EnvironmentsResource,
					},
				},
			},
			// Environment should not render any K8s Objects directly
			K8sObjects:     &validation.K8sObjectSet{},
			PostStepVerify: verifyRecipeCLI,
		},
	})

	test.Test(t)
}

// This test creates an environment by directly calling the CreateEnvironment function to test dev recipes.
// After dev recipes are confirmed, the environment is deleted.
func Test_DevRecipes(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	options := rp.NewTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	envName := "test-dev-recipes"
	envNamespace := "test-dev-recipes"

	basicRunner := radinit.NewRunner(
		&framework.Impl{
			ConnectionFactory: connections.DefaultFactory,
		},
	)
	basicRunner.UpdateEnvironmentOptions(true, envName, envNamespace)
	basicRunner.UpdateRecipePackOptions(true)
	basicRunner.DevRecipeClient = radinit.NewDevRecipeClient()
	basicRunner.Workspace = &workspaces.Workspace{
		Name: envName,
		Connection: map[string]any{
			"kind": workspaces.KindKubernetes,
		},
		Environment: fmt.Sprintf("/planes/radius/local/resourceGroups/kind-radius/providers/Applications.Core/environments/%s", envName),
		Scope:       "/planes/radius/local/resourceGroups/kind-radius",
	}

	// Create the environment
	err := basicRunner.CreateEnvironment(ctx)
	require.NoError(t, err)

	output, err := cli.RecipeList(ctx, envName)
	require.NoError(t, err)
	require.Regexp(t, "default", output)

	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}

	for _, devRecipe := range radinit.AvailableDevRecipes() {
		require.Regexp(t, devRecipe.ResourceType, output)
		require.Regexp(t, devRecipe.RepoPath+":"+tag, output)
	}

	err = cli.EnvDelete(ctx, envName)
	require.NoError(t, err)
}

// GetAvailablePort attempts to find an available port on the localhost and returns it, or returns an error if it fails.
func GetAvailablePort() (int, error) {
	address, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", address)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// DeleteAppWithoutDeletingResources creates a client to delete an application without deleting its resources and returns
// an error if one occurs.
func DeleteAppWithoutDeletingResources(t *testing.T, ctx context.Context, options rp.RPTestOptions, applicationName string) error {
	client := options.ManagementClient
	require.IsType(t, client, &clients.UCPApplicationsManagementClient{})
	appManagementClient := client.(*clients.UCPApplicationsManagementClient)
	appDeleteClient, err := v20231001preview.NewApplicationsClient(appManagementClient.RootScope, &aztoken.AnonymousCredential{}, appManagementClient.ClientOptions)
	require.NoError(t, err)
	// We don't care about the response for tests
	_, err = appDeleteClient.Delete(ctx, applicationName, nil)
	return err
}

func generateUniqueTag() string {
	timestamp := time.Now().Unix()
	random := rand.Intn(1000)
	tag := fmt.Sprintf("test-%d-%d", timestamp, random)
	return tag
}
