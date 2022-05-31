// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/functional/kubernetes"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_Gateway(t *testing.T) {
	template := "testdata/kubernetes-resources-gateway.bicep"
	application := "kubernetes-resources-gateway"
	test := kubernetes.NewApplicationTest(t, application, []kubernetes.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, rest.ResourceType{Type: resourcekinds.Service, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "backend"),
						validation.NewK8sHTTPProxyForResource(application, "backendgateway"),
						validation.NewK8sHTTPProxyForResource(application, "frontendhttp"),
						validation.NewK8sServiceForResource(application, "frontendhttp"),
						validation.NewK8sHTTPProxyForResource(application, "backendhttp"),
						validation.NewK8sServiceForResource(application, "backendhttp"),
					},
				},
			},
		},
	})
	test.Test(t)
}
