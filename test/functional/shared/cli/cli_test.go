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

	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/radcli"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"

	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
)

const (
	retries = 10
)

func verifyRecipeCLI(ctx context.Context, t *testing.T, test shared.RPTest) {
	options := shared.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	// get the current environment to switch back to after the test since the environment is used
	// for AWS test and has the AWS scope which the environment created in this does not.
	envName := test.Steps[0].RPResources.Resources[0].Name
	recipeName := "recipeName"
	recipeTemplate := "testpublicrecipe.azurecr.io/bicep/modules/testTemplate:v1"
	templateKind := "bicep"
	linkType := "Applications.Link/mongoDatabases"
	file := "testdata/corerp-redis-recipe.bicep"
	target := fmt.Sprintf("br:radiusdev.azurecr.io/test-bicep-recipes/redis-recipe:%s", generateUniqueTag())

	t.Run("Validate rad recipe register", func(t *testing.T) {
		output, err := cli.RecipeRegister(ctx, envName, recipeName, templateKind, recipeTemplate, linkType)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully linked recipe")
	})

	t.Run("Validate rad recipe list", func(t *testing.T) {
		output, err := cli.RecipeList(ctx, envName)
		require.NoError(t, err)
		require.Regexp(t, recipeName, output)
		require.Regexp(t, linkType, output)
		require.Regexp(t, recipeTemplate, output)
	})

	t.Run("Validate rad recipe unregister", func(t *testing.T) {
		output, err := cli.RecipeUnregister(ctx, envName, recipeName, linkType)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully unregistered recipe")
	})

	t.Run("Validate rad recipe show", func(t *testing.T) {
		showRecipeName := "mongodbtest"
		showRecipeTemplate := "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0"
		showRecipeLinkType := "Applications.Link/mongoDatabases"
		output, err := cli.RecipeRegister(ctx, envName, showRecipeName, templateKind, showRecipeTemplate, showRecipeLinkType)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully linked recipe")
		output, err = cli.RecipeShow(ctx, envName, showRecipeName, linkType)
		require.NoError(t, err)
		require.Contains(t, output, showRecipeName)
		require.Contains(t, output, showRecipeTemplate)
		require.Contains(t, output, showRecipeLinkType)
		require.Contains(t, output, "mongodbName")
		require.Contains(t, output, "documentdbName")
		require.Contains(t, output, "location")
		require.Contains(t, output, "string")
		require.Contains(t, output, "resourceGroup().location]")
	})

	t.Run("Validate `rad bicep publish` is publishing the file to the given target", func(t *testing.T) {
		output, err := cli.BicepPublish(ctx, file, target)
		require.NoError(t, err)
		require.Contains(t, output, "Successfully published")
	})

	t.Run("Validate rad recipe register with recipe name conflicting with dev recipe", func(t *testing.T) {
		output, err := cli.RecipeRegister(ctx, envName, "mongo-azure", templateKind, recipeTemplate, linkType)
		require.Contains(t, output, "Successfully linked recipe")
		require.NoError(t, err)
		output, err = cli.RecipeList(ctx, envName)
		require.NoError(t, err)
		require.Regexp(t, recipeTemplate, output)
	})
}

func verifyCLIBasics(ctx context.Context, t *testing.T, test shared.RPTest) {
	options := shared.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	appName := test.Name
	containerName := "containerA"
	//spacing in output will change based on resource names
	showSpacing := ""
	if strings.EqualFold(appName, "kubernetes-cli-json") {
		containerName = "containerA-json"
		showSpacing = "     "
	}

	t.Run("Validate rad application show", func(t *testing.T) {
		output, err := cli.ApplicationShow(ctx, appName)
		require.NoError(t, err)
		expected := regexp.MustCompile(`RESOURCE      ` + showSpacing + `  TYPE\n` + appName + `  Applications.Core/applications\n`)
		match := expected.MatchString(output)
		require.Equal(t, true, match, "output: %s", output)
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
		output, err := cli.ResourceShow(ctx, "containers", containerName)
		require.NoError(t, err)
		// We are more interested in the content and less about the formatting, which
		// is already covered by unit tests. The spaces change depending on the input
		// and it takes very long to get a feedback from CI.
		expected := regexp.MustCompile(`RESOURCE  ` + showSpacing + `  TYPE\n` + containerName + `  Applications.Core/containers\n`)
		match := expected.MatchString(output)
		require.Equal(t, true, match, "output: %s", output)
	})

	t.Run("Validate rad resoure logs containers", func(t *testing.T) {
		output, err := cli.ResourceLogs(ctx, appName, containerName)
		require.NoError(t, err)

		// We don't want to be too fragile so we're not validating the logs in depth
		require.Contains(t, output, "Server running at http://localhost:3000")
	})
	t.Run("Validate rad resource expose Container", func(t *testing.T) {
		t.Skip("https://github.com/project-radius/radius/issues/3232")
		port, err := GetAvailablePort()
		require.NoError(t, err)

		// We open a local port-forward and then make a request to it.
		child, cancel := context.WithCancel(ctx)

		done := make(chan error)
		go func() {
			_, err = cli.ResourceExpose(child, appName, containerName, port, 3000)
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
	for i := 0; i < retries; i++ {
		url := fmt.Sprintf("http://localhost:%d/healthz", port)
		t.Logf("making request to %s", url)
		response, err := http.Get(url)
		if err != nil {
			if i == retries-1 {
				// last retry failed, report failure
				require.NoError(t, err, "failed to get connect to resource after %d retries", retries)
			}
			t.Logf("got error %s", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		if response.Body != nil {
			defer response.Body.Close()
		}

		if response.StatusCode > 299 || response.StatusCode < 200 {
			if i == retries-1 {
				// last retry failed, report failure
				require.NoError(t, err, "status code was a bad response after %d retries %d", retries, response.StatusCode)
			}
			t.Logf("got status %d", response.StatusCode)
			time.Sleep(1 * time.Second)
			continue
		}

		defer response.Body.Close()
		content, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		t.Logf("[response] %s", string(content))
		return
	}
}

func Test_Run_Logger(t *testing.T) {
	// Will be used to cancel `rad run`
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)
	options := shared.NewRPTestOptions(t)

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
	options := shared.NewRPTestOptions(t)

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
		functional.GetMagpieImage(),
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

	rgx := regexp.MustCompile(`.*\[port-forward\] .* from localhost:(.*) -> ::.*`)

	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line)
		output.WriteString("\n")
		matches := rgx.FindSubmatch([]byte(line))
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

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
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

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
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

	options := shared.NewRPTestOptions(t)
	appName := "kubernetes-cli-with-resources"
	appNameEmptyResources := "kubernetes-cli-empty-resources"

	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateWithResources := "testdata/corerp-kubernetes-cli-with-resources.bicep"
	templateFilePathWithResources := filepath.Join(cwd, templateWithResources)

	templateEmptyResources := "testdata/corerp-kubernetes-cli-app-empty-resources.bicep"
	templateFilePathEmptyResources := filepath.Join(cwd, templateEmptyResources)

	cli := radcli.NewCLI(t, options.ConfigFilePath)

	t.Run("Validate rad app delete with non empty resources", func(t *testing.T) {
		t.Logf("deploying %s from file %s", appName, templateWithResources)

		err = cli.Deploy(ctx, templateFilePathWithResources, appName, functional.GetMagpieImage())
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

		err = cli.Deploy(ctx, templateFilePathEmptyResources, appNameEmptyResources)
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

		err := cli.Deploy(ctx, templateFilePathWithResources, appName, functional.GetMagpieImage())
		require.NoErrorf(t, err, "failed to deploy %s", appName)

		validation.ValidateObjectsRunning(ctx, t, options.K8sClient, options.DynamicClient, validation.K8sObjectSet{
			Namespaces: map[string][]validation.K8sObject{
				"default-kubernetes-cli-with-resources": {
					validation.NewK8sPodForResource(appName, "containera-app-with-resources"),
					validation.NewK8sPodForResource(appName, "containerb-app-with-resources"),
				},
			},
		})

		//ignore response for tests
		_, err = options.ManagementClient.DeleteResource(ctx, "Applications.Core/containers", "containerb-app-with-resources")
		require.NoErrorf(t, err, "failed to delete resource containerb-app-with-resources")
		err = DeleteAppWithoutDeletingResources(t, ctx, options, appName)
		require.NoErrorf(t, err, "failed to delete application %s", appName)

		t.Logf("deploying from file %s", templateEmptyResources)
		err = cli.Deploy(ctx, templateFilePathEmptyResources, appName)
		require.NoErrorf(t, err, "failed to deploy %s", appNameEmptyResources)

		err = cli.ApplicationDelete(ctx, appNameEmptyResources)
		require.NoErrorf(t, err, "failed to delete %s", appNameEmptyResources)

		//ignore response for tests
		_, err = options.ManagementClient.DeleteResource(ctx, "Applications.Core/containers", "containera-app-with-resources")
		require.NoErrorf(t, err, "failed to delete resource containera-app-with-resources")

	})
}

func Test_CLI_DeploymentParameters(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	template := "testdata/corerp-kubernetes-cli-parameters.bicep"
	parameterFile := "testdata/corerp-kubernetes-cli-parameters.parameters.json"
	name := "kubernetes-cli-params"
	parameterFilePath := filepath.Join(cwd, parameterFile)

	// corerp-kubernetes-cli-parameters.parameters.json uses radiusdev.azurecr.io as a registry parameter.
	// Use the specified tag only if the test uses radiusdev.azurecr.io registry. Otherwise, use latest tag.
	magpieTag := "magpietag=latest"
	image := functional.GetMagpieImage()
	if !strings.HasPrefix(image, "magpieimage=radiusdev.azurecr.io") {
		magpieTag = functional.GetMagpieTag()
	}

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, "@"+parameterFilePath, magpieTag),
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

	options := shared.NewTestOptions(t)
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

	options := shared.NewTestOptions(t)
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

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
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

func DeleteAppWithoutDeletingResources(t *testing.T, ctx context.Context, options shared.RPTestOptions, applicationName string) error {
	client := options.ManagementClient
	require.IsType(t, client, &clients.UCPApplicationsManagementClient{})
	appManagementClient := client.(*clients.UCPApplicationsManagementClient)
	appDeleteClient, err := v20220315privatepreview.NewApplicationsClient(appManagementClient.RootScope, &aztoken.AnonymousCredential{}, appManagementClient.ClientOptions)
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
