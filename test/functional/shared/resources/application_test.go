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
	"encoding/json"
	"sort"
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Application(t *testing.T) {
	template := "testdata/corerp-resources-application.bicep"
	name := "corerp-resources-application"
	appNamespace := "corerp-resources-application-app"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-application-app",
						Type: validation.ApplicationsResource,
					},
				},
			},
			// Application should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				_, err := test.Options.K8sClient.CoreV1().Namespaces().Get(ctx, appNamespace, metav1.GetOptions{})
				require.NoErrorf(t, err, "%s must be created", appNamespace)
			},
		},
	})
	test.Test(t)
}

func Test_ApplicationGraph(t *testing.T) {
	// Deploy a simple app
	template := "testdata/corerp-resources-application-graph.bicep"
	name := "corerp-application-simple1"
	appNamespace := "default-corerp-application-simple1"

	frontCntrID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/containers/http-front-ctnr-simple1"
	frontCntrName := "http-front-ctnr-simple1"
	frontCntrType := "Applications.Core/containers"

	backCntrID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/containers/http-back-ctnr-simple1"
	backCntrName := "http-back-ctnr-simple1"
	backCntrType := "Applications.Core/containers"

	backRteID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/httpRoutes/http-back-rte-simple1"
	backRteName := "http-back-rte-simple1"
	backRteType := "Applications.Core/httpRoutes"

	backOutputResourceID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/apps/Deployment/http-back-ctnr-simple1"
	backOutputResourceServiceAccountID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/core/ServiceAccount/http-back-ctnr-simple1"
	backOutputResourceRoleID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/rbac.authorization.k8s.io/Role/http-back-ctnr-simple1"
	backOutputResourceRoleBindingID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/rbac.authorization.k8s.io/RoleBinding/http-back-ctnr-simple1"

	rteOutputResourceID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/core/Service/http-back-rte-simple1"

	frontOutputResourceID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/apps/Deployment/http-front-ctnr-simple1"
	frontOutputResourceSecretID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/core/Secret/http-front-ctnr-simple1"
	frontOutputResourceServiceID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/core/Service/http-front-ctnr-simple1"
	frontOutputResourcesServiceAccountID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/core/ServiceAccount/http-front-ctnr-simple1"
	frontOutputResourceRoleID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/rbac.authorization.k8s.io/Role/http-front-ctnr-simple1"
	frontOutputResourceRoleBindingID := "/planes/kubernetes/local/namespaces/default-corerp-application-simple1/providers/rbac.authorization.k8s.io/RoleBinding/http-front-ctnr-simple1"

	provisioningStateSuccess := "Succeeded"
	directionInbound := v20231001preview.DirectionInbound
	directionOutbound := v20231001preview.DirectionOutbound

	deploymentType := "apps/Deployment"
	serviceAccountType := "core/ServiceAccount"
	roleType := "rbac.authorization.k8s.io/Role"
	serviceType := "core/Service"
	secretType := "core/Secret"
	roleBindingType := "rbac.authorization.k8s.io/RoleBinding"

	expectedGraphResp := v20231001preview.ApplicationsClientGetGraphResponse{
		ApplicationGraphResponse: v20231001preview.ApplicationGraphResponse{
			Resources: []*v20231001preview.ApplicationGraphResource{
				{
					ID:                &frontCntrID,
					Name:              &frontCntrName,
					Type:              &frontCntrType,
					ProvisioningState: &provisioningStateSuccess,
					OutputResources: []*v20231001preview.ApplicationGraphOutputResource{
						{
							ID:   &frontOutputResourceID,
							Name: &frontCntrName,
							Type: &deploymentType,
						},
						{
							ID:   &frontOutputResourceSecretID,
							Name: &frontCntrName,
							Type: &secretType,
						},
						{
							ID:   &frontOutputResourceServiceID,
							Name: &frontCntrName,
							Type: &serviceType,
						},
						{
							ID:   &frontOutputResourcesServiceAccountID,
							Name: &frontCntrName,
							Type: &serviceAccountType,
						},
						{
							ID:   &frontOutputResourceRoleID,
							Name: &frontCntrName,
							Type: &roleType,
						},
						{
							ID:   &frontOutputResourceRoleBindingID,
							Name: &frontCntrName,
							Type: &roleBindingType,
						},
					},
					Connections: []*v20231001preview.ApplicationGraphConnection{
						{
							Direction: &directionInbound,
							ID:        &backRteID,
						},
					},
				},
				{
					ID:                &backCntrID,
					Name:              &backCntrName,
					Type:              &backCntrType,
					ProvisioningState: &provisioningStateSuccess,
					OutputResources: []*v20231001preview.ApplicationGraphOutputResource{
						{
							ID:   &backOutputResourceID,
							Name: &backCntrName,
							Type: &deploymentType,
						},
						{
							ID:   &backOutputResourceServiceAccountID,
							Name: &backCntrName,
							Type: &serviceAccountType,
						},
						{
							ID:   &backOutputResourceRoleID,
							Name: &backCntrName,
							Type: &roleType,
						},
						{
							ID:   &backOutputResourceRoleBindingID,
							Name: &backCntrName,
							Type: &roleBindingType,
						},
					},
					Connections: []*v20231001preview.ApplicationGraphConnection{
						{
							Direction: &directionOutbound,
							ID:        &backRteID,
						},
					},
				},
				{
					ID:                &backRteID,
					Name:              &backRteName,
					Type:              &backRteType,
					ProvisioningState: &provisioningStateSuccess,
					Connections: []*v20231001preview.ApplicationGraphConnection{
						{
							Direction: &directionInbound,
							ID:        &backCntrID,
						},
					},
					OutputResources: []*v20231001preview.ApplicationGraphOutputResource{
						{
							ID:   &rteOutputResourceID,
							Name: &backRteName,
							Type: &serviceType,
						},
					},
				},
			},
		},
	}

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "http-front-ctnr-simple1",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "http-back-rte-simple1",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-back-ctnr-simple1",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "http-front-ctnr-simple1"),
						validation.NewK8sPodForResource(name, "http-back-ctnr-simple1"),
						validation.NewK8sServiceForResource(name, "http-back-rte-simple1"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Verify the application graph
				options := shared.NewRPTestOptions(t)
				client := options.ManagementClient
				require.IsType(t, client, &clients.UCPApplicationsManagementClient{})
				appManagementClient := client.(*clients.UCPApplicationsManagementClient)
				appGraphClient, err := v20231001preview.NewApplicationsClient(appManagementClient.RootScope, &aztoken.AnonymousCredential{}, appManagementClient.ClientOptions)
				require.NoError(t, err)
				res, err := appGraphClient.GetGraph(ctx, "corerp-application-simple1", map[string]any{}, nil)
				require.NoError(t, err)
				arr, _ := json.Marshal(res)

				sort.Slice(expectedGraphResp.ApplicationGraphResponse.Resources, func(i, j int) bool {
					return *expectedGraphResp.ApplicationGraphResponse.Resources[i].ID < *expectedGraphResp.ApplicationGraphResponse.Resources[j].ID
				})

				sort.Slice(res.ApplicationGraphResponse.Resources, func(i, j int) bool {
					return *res.ApplicationGraphResponse.Resources[i].ID < *res.ApplicationGraphResponse.Resources[j].ID
				})

				sort.Slice(expectedGraphResp.ApplicationGraphResponse.Resources, func(i, j int) bool {
					return *expectedGraphResp.ApplicationGraphResponse.Resources[i].ID < *expectedGraphResp.ApplicationGraphResponse.Resources[j].ID
				})

				sort.Slice(res.ApplicationGraphResponse.Resources, func(i, j int) bool {
					return *res.ApplicationGraphResponse.Resources[i].ID < *res.ApplicationGraphResponse.Resources[j].ID
				})

				require.Equal(t, expectedGraphResp, res, "actual: %s", string(arr))
			},
		},
	})

	test.Test(t)
}
