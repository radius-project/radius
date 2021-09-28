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
		Properties: db.ComponentProperties{
			Status: db.ComponentStatus{
				OutputResources: []db.OutputResource{
					{
						LocalID:            "L1",
						HealthID:           "HealthID_1",
						OutputResourceType: outputresource.TypeARM,
						ResourceKind:       "Kind1",
						OutputResourceInfo: outputresource.ARMInfo{
							ID:           "ResourceID_1",
							ResourceType: "Dummy1",
							APIVersion:   "2021-01-01",
						},
					},
					{
						LocalID:            "L2",
						HealthID:           "HealthID_2",
						OutputResourceType: outputresource.TypeARM,
						ResourceKind:       "Kind1",
						OutputResourceInfo: outputresource.ARMInfo{
							ID:           "ResourceID_2",
							ResourceType: "Dummy2",
							APIVersion:   "2021-01-01",
						},
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
	require.Equal(t, "ResourceID_1", msg1.ResourceInfo.ResourceID)
	require.Equal(t, "Kind1", msg1.ResourceInfo.ResourceKind)
	require.Equal(t, "HealthID_1", msg1.ResourceInfo.HealthID)

	// Registration for second output resource
	msg2 := <-registrationChannel
	require.Equal(t, healthcontract.ActionRegister, msg2.Action)
	require.Equal(t, "Kind1", msg2.ResourceInfo.ResourceKind)
	require.Equal(t, "ResourceID_2", msg2.ResourceInfo.ResourceID)
	require.Equal(t, "HealthID_2", msg2.ResourceInfo.HealthID)
}

func Test_DeploymentProcessor_UnregistersOutputResourcesWithHealthService(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockResourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRendererKind := renderers.NewMockWorkloadRenderer(ctrl)
	mockRendererKind.EXPECT().Render(gomock.Any(), gomock.Any()).AnyTimes().Return([]outputresource.OutputResource{
		{
			LocalID:  "abc",
			Kind:     "Kind1",
			HealthID: "HealthID_1",
			Type:     outputresource.TypeARM,
			Info: outputresource.ARMInfo{
				ID: "ResourceID_1",
			},
		},
		{
			LocalID:  "xyz",
			Kind:     "Kind1",
			HealthID: "HealthID_2",
			Type:     "Kind1",
			Info: outputresource.K8sInfo{
				Name:      "name1",
				Namespace: "ns1",
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

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{model, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}}

	outputResourceInfo1 := healthcontract.ResourceDetails{
		ResourceID:   "ResourceID_1",
		ResourceKind: "Kind1",
		OwnerID: azresources.MakeID(
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
			}),
	}

	outputResourceInfo2 := healthcontract.ResourceDetails{
		ResourceID:   "ns1-name1",
		ResourceKind: "Kind1",
		OwnerID: azresources.MakeID(
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
			}),
	}

	deployStatus := db.DeploymentStatus{
		Workloads: []db.DeploymentWorkload{
			{
				ComponentName: "C",
				Kind:          "Kind1",
				Resources: []db.DeploymentResource{
					{
						LocalID:    "abc",
						Type:       "Kind1",
						Properties: map[string]string{healthcontract.HealthIDKey: outputResourceInfo1.GetHealthID()},
					},
				},
			},
			{
				ComponentName: "C",
				Kind:          "Kind1",
				Resources: []db.DeploymentResource{
					{
						LocalID:    "xyz",
						Type:       "Kind1",
						Properties: map[string]string{healthcontract.HealthIDKey: outputResourceInfo2.GetHealthID()},
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
	require.Equal(t, outputResourceInfo1.GetHealthID(), msg1.ResourceInfo.HealthID)

	// Registration for second output resource
	msg2 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg2.Action)
	require.Equal(t, outputResourceInfo2.GetHealthID(), msg2.ResourceInfo.HealthID)
}
