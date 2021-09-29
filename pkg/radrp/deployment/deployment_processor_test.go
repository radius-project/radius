// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func Test_DeploymentProcessor_OrderActions(t *testing.T) {
	// We're not going to render or deploy anything, so an empty model works
	model := model.NewModel(map[string]workloads.WorkloadRenderer{}, map[string]model.Handlers{})
	dp := deploymentProcessor{model, &healthcontract.HealthChannels{}}

	actions := map[string]ComponentAction{
		"A": {
			ComponentName: "A",
			Operation:     UpdateWorkload,
			Component: &components.GenericComponent{
				Uses: []components.GenericDependency{
					{
						Binding: components.NewComponentBindingExpression("myapp", "C", "test", ""),
					},
				},
			},
		},
		"B": {
			ComponentName: "B",
			Operation:     DeleteWorkload,
		},
		"C": {
			ComponentName: "C",
			Operation:     UpdateWorkload,
			Component:     &components.GenericComponent{},
		},
	}
	ordered, err := dp.orderActions(actions)
	require.NoError(t, err)

	expected := []ComponentAction{
		actions["C"],
		actions["A"],
		actions["B"],
	}

	require.Equal(t, expected, ordered)
}

func Test_DeploymentProcessor_RegistersOutputResourcesWithHealthService(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockHealthHandler.EXPECT().GetHealthOptions(gomock.Any()).Times(2).Return(healthcontract.HealthCheckOptions{})
	mockRendererKind := renderers.NewMockWorkloadRenderer(ctrl)
	model := model.NewModel(map[string]workloads.WorkloadRenderer{
		"Dummy1": mockRendererKind,
	}, map[string]model.Handlers{
		"Kind1": {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	})

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{model, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}}

	c := db.Component{
		Kind: "Kind1",
		ResourceBase: db.ResourceBase{
			ID: "SomeRadiusResourceID",
		},
		Properties: db.ComponentProperties{
			Status: db.ComponentStatus{
				OutputResources: []db.OutputResource{
					{
						LocalID:      "L1",
						ResourceKind: "Kind1",
						Identity:     resourcemodel.NewARMIdentity("ResourceID_1", "2021-01-01"),
					},
					{
						LocalID:      "L2",
						ResourceKind: "Kind1",
						Identity:     resourcemodel.NewARMIdentity("ResourceID_2", "2021-01-01"),
					},
				},
			},
		},
	}

	var ctx context.Context
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		ctx = context.Background()
	} else {
		ctx = logr.NewContext(context.Background(), logger)
	}
	err = dp.RegisterForHealthChecks(ctx, "A", c)
	require.NoError(t, err, "Update Deployment failed")

	// Registration for first output resource
	msg1 := <-registrationChannel
	require.Equal(t, healthcontract.ActionRegister, msg1.Action)
	require.Equal(t, resourcemodel.NewARMIdentity("ResourceID_1", "2021-01-01"), msg1.Resource.Identity)
	require.Equal(t, "Kind1", msg1.Resource.ResourceKind)
	require.Equal(t, "SomeRadiusResourceID", msg1.Resource.RadiusResourceID)

	// Registration for second output resource
	msg2 := <-registrationChannel
	require.Equal(t, healthcontract.ActionRegister, msg2.Action)
	require.Equal(t, "Kind1", msg2.Resource.ResourceKind)
	require.Equal(t, resourcemodel.NewARMIdentity("ResourceID_2", "2021-01-01"), msg2.Resource.Identity)
	require.Equal(t, "SomeRadiusResourceID", msg2.Resource.RadiusResourceID)
}

func Test_DeploymentProcessor_UnregistersOutputResourcesWithHealthService(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockResourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRendererKind := renderers.NewMockWorkloadRenderer(ctrl)
	mockRendererKind.EXPECT().Render(gomock.Any(), gomock.Any()).AnyTimes().Return([]outputresource.OutputResource{
		{
			LocalID:      "abc",
			ResourceKind: "Kind1",
			Identity:     resourcemodel.NewARMIdentity("ResourceID_1", "2021-01-01"),
		},
		{
			LocalID:      "xyz",
			ResourceKind: "Kind1",
			Identity: resourcemodel.ResourceIdentity{
				Kind: resourcemodel.IdentityKindKubernetes,
				Data: resourcemodel.KubernetesIdentity{
					Name:      "name1",
					Namespace: "ns1",
				},
			},
		},
	}, nil)
	mockRendererKind.EXPECT().AllocateBindings(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(map[string]components.BindingState{}, nil)
	model := model.NewModel(map[string]workloads.WorkloadRenderer{
		"Dummy1": mockRendererKind,
	}, map[string]model.Handlers{
		"Kind1": {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	})

	radiusResourceID := azresources.MakeID(
		"test-subscription",
		"test-resourcegroup",
		azresources.ResourceType{
			Type: azresources.CustomProvidersResourceProviders,
			Name: "radius",
		}, azresources.ResourceType{
			Type: "Applications",
			Name: "A",
		}, azresources.ResourceType{
			Type: "Components",
			Name: "C",
		})

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{model, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}}

	healthResource1 := healthcontract.HealthResource{
		Identity:         resourcemodel.NewARMIdentity("Resource_1", "2020-01-01"),
		ResourceKind:     "Kind1",
		RadiusResourceID: radiusResourceID,
	}

	healthResource2 := healthcontract.HealthResource{
		Identity: resourcemodel.ResourceIdentity{
			Kind: resourcemodel.IdentityKindKubernetes,
			Data: resourcemodel.KubernetesIdentity{
				Name:      "resource-1",
				Namespace: "ns1-name1",
			},
		},
		ResourceKind:     "Kind1",
		RadiusResourceID: radiusResourceID,
	}

	deployStatus := db.DeploymentStatus{
		Workloads: []db.DeploymentWorkload{
			{
				ComponentName: "C",
				Kind:          "Kind1",
				Resources: []db.DeploymentResource{
					{
						LocalID:          "abc",
						Type:             "Kind1",
						RadiusResourceID: radiusResourceID,
						Identity:         healthResource1.Identity,
						Properties:       map[string]string{},
					},
				},
			},
			{
				ComponentName: "C",
				Kind:          "Kind1",
				Resources: []db.DeploymentResource{
					{
						LocalID:          "xyz",
						Type:             "Kind1",
						RadiusResourceID: radiusResourceID,
						Identity:         healthResource2.Identity,
						Properties:       map[string]string{},
					},
				},
			},
		},
	}

	var ctx context.Context
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		ctx = context.Background()
	} else {
		ctx = logr.NewContext(context.Background(), logger)
	}
	err = dp.DeleteDeployment(ctx, "A", "A", &deployStatus)
	require.NoError(t, err, "Delete Deployment failed")

	// Unregistration for first output resource
	msg1 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg1.Action)
	require.Equal(t, healthResource1, msg1.Resource)

	// Registration for second output resource
	msg2 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg2.Action)
	require.Equal(t, healthResource2, msg2.Resource)
}
