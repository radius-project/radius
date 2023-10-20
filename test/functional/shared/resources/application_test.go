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
	"fmt"
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
	name := "corerp-application-simple"
	appNamespace := "corerp-application-simple"

	frontCntrID := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/http-front-ctnr-simple"
	frontCntrName := "http-front-ctnr-simple"
	frontCntrType := "Applications.Core/containers"

	backCntrID := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/http-back-ctnr-simple"
	backCntrName := "http-back-ctnr-simple"
	backCntrType := "Applications.Core/containers"

	backRteID := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/http-back-rte-simple"
	backRteName := "http-back-rte-simple"
	backRteType := "Applications.Core/httpRoutes"

	ProvisioningStateSuccess := "Succeeded"
	DirectionInbound := v20231001preview.DirectionInbound
	DirectionOutbound := v20231001preview.DirectionOutbound

	expectedGraphResp := v20231001preview.ApplicationsClientGetGraphResponse{
		ApplicationGraphResponse: v20231001preview.ApplicationGraphResponse{
			Resources: []*v20231001preview.ApplicationGraphResource{
				{
					ID:                &frontCntrID,
					Name:              &frontCntrName,
					Type:              &frontCntrType,
					ProvisioningState: &ProvisioningStateSuccess,
					OutputResources:   []*v20231001preview.ApplicationGraphOutputResource{},
					Connections: []*v20231001preview.ApplicationGraphConnection{
						{
							Direction: &DirectionInbound,
							ID:        &backRteID,
						},
					},
				},
				{
					ID:                &backCntrID,
					Name:              &backCntrName,
					Type:              &backCntrType,
					ProvisioningState: &ProvisioningStateSuccess,
					OutputResources:   []*v20231001preview.ApplicationGraphOutputResource{},
					Connections: []*v20231001preview.ApplicationGraphConnection{
						{
							Direction: &DirectionOutbound,
							ID:        &backRteID,
						},
					},
				},
				{
					ID:                &backRteID,
					Name:              &backRteName,
					Type:              &backRteType,
					ProvisioningState: &ProvisioningStateSuccess,
					Connections: []*v20231001preview.ApplicationGraphConnection{
						{
							Direction: &DirectionInbound,
							ID:        &backRteID,
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
						Name: "http-front-ctnr-simple",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "http-back-rte-simple",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-back-ctnr-simple",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "http-front-ctnr-simple"),
						validation.NewK8sPodForResource(name, "http-back-ctnr-simple"),
						validation.NewK8sServiceForResource(name, "http-back-rte-simple"),
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
				res, err := appGraphClient.GetGraph(ctx, "corerp-application-simple", map[string]any{}, nil)
				//require(res, )
				require.NoError(t, err)

				sort.Slice(expectedGraphResp.ApplicationGraphResponse.Resources, func(i, j int) bool {
					return *expectedGraphResp.ApplicationGraphResponse.Resources[i].ID < *expectedGraphResp.ApplicationGraphResponse.Resources[j].ID
				})

				sort.Slice(res.ApplicationGraphResponse.Resources, func(i, j int) bool {
					return *res.ApplicationGraphResponse.Resources[i].ID < *res.ApplicationGraphResponse.Resources[j].ID
				})

				//iterate over each of the element in res.ApplicationGraphResponse.Resources and compare with expectedGraphResp.ApplicationGraphResponse.Resources
				for i, resResource := range res.ApplicationGraphResponse.Resources {
					expectedResource := expectedGraphResp.ApplicationGraphResponse.Resources[i]

					// Compare the ID field of the two resources
					if *resResource.ID != *expectedResource.ID {
						t.Errorf("Unexpected ID: got %v, want %v", *resResource.ID, *expectedResource.ID)
					}

					// Compare the Name field of the two resources
					if resResource.Name != expectedResource.Name {
						t.Errorf("Unexpected Name: got %v, want %v", resResource.Name, expectedResource.Name)
					}

					// Compare the Type field of the two resources
					if resResource.Type != expectedResource.Type {
						t.Errorf("Unexpected Type: got %v, want %v", resResource.Type, expectedResource.Type)
					}

					// Compare the Connections field of the two resources
					if len(resResource.Connections) != len(expectedResource.Connections) {
						t.Errorf("Unexpected number of Connections: got %v, want %v", len(resResource.Connections), len(expectedResource.Connections))
					} else {
						for j, resConn := range resResource.Connections {
							expectedConn := expectedResource.Connections[j]

							// Compare the ID field of the two connections
							if *resConn.ID != *expectedConn.ID {
								t.Errorf("Unexpected Connection ID: got %v, want %v", *resConn.ID, *expectedConn.ID)
							}

							// Compare the Name field of the two connections
							if resConn.Direction != expectedConn.Direction {
								t.Errorf("Unexpected Connection Name: got %v, want %v", resConn.Direction, expectedConn.Direction)
							}
						}
					}
				}

				s, _ := json.MarshalIndent(res, "", "\t")
				fmt.Print(string(s))

			},
		},
	})

	test.Test(t)

	// getGraph on the app and verify it's what we expect
}
