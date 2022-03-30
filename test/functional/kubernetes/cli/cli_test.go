// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/kubernetestest"
	"github.com/project-radius/radius/test/radcli"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_CLI_DeploymentParameters(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	application := "kubernetes-cli-parameters"
	template := "testdata/kubernetes-cli-parameters.bicep"
	parameterFile := "testdata/kubernetes-cli-parameters.parameters.json"
	parameterFilePath := filepath.Join(cwd, parameterFile)

	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template, "@"+parameterFilePath, "env=COOL_VALUE"),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "a",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "b",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "a"),
						validation.NewK8sPodForResource(application, "b"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_CLI(t *testing.T) {
	options := kubernetestest.NewTestOptions(t)

	// We deploy a simple app and then run a variety of different CLI commands on it. Emphasis here
	// is on the commands that aren't tested as part of our main flow.
	//
	// We use nested tests so we can skip them if we've already failed deployment.
	application := "kubernetes-cli"
	template := "testdata/kubernetes-cli.bicep"

	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "a",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "b",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "a"),
						validation.NewK8sPodForResource(application, "b"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at kubernetestest.ApplicationTest) {
				// Test all management commands
				// Delete application is implicitly tested by all application tests
				// as it is how we cleanup.
				cli := radcli.NewCLI(t, options.ConfigFilePath)

				t.Run("resource show", func(t *testing.T) {
					output, err := cli.ResourceShow(ctx, application, "Container", "a")
					require.NoError(t, err)
					// We are more interested in the content and less about the formatting, which
					// is already covered by unit tests. The spaces change depending on the input
					// and it takes very long to get a feedback from CI.
					expected := regexp.MustCompile(`RESOURCE\s+TYPE\s+PROVISIONING_STATE\s+HEALTH_STATE
a\s+Container\s+.*Provisioned\s+.*[h|H]ealthy\s*
`)
					require.Regexp(t, expected, output)
				})
				t.Run("resource list", func(t *testing.T) {
					output, err := cli.ResourceList(ctx, application)
					require.NoError(t, err)
					expected := regexp.MustCompile(`RESOURCE\s+TYPE\s+PROVISIONING_STATE\s+HEALTH_STATE
(a|b)\s+Container\s+.*Provisioned\s+.*[h|H]ealthy\s*
(a|b)\s+Container\s+.*Provisioned\s+.*[h|H]ealthy\s*
`)
					require.Regexp(t, expected, output)
				})

				t.Run("application show", func(t *testing.T) {
					output, err := cli.ApplicationShow(ctx, application)
					require.NoError(t, err)
					expected := regexp.MustCompile(`APPLICATION\s+PROVISIONING_STATE\s+HEALTH_STATE
kubernetes-cli\s+.*Provisioned\s+.*[h|H]ealthy\s*
`)
					require.Regexp(t, expected, output)
				})
			},
		},
	})

	test.Test(t)
}
