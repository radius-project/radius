// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/kubernetestest"
	"github.com/Azure/radius/test/validation"
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
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "b",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForResource(application, "a"),
						validation.NewK8sObjectForResource(application, "b"),
					},
				},
			},
		},
	})

	test.Test(t)
}
