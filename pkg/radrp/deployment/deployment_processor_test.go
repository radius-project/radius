// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"testing"

	"github.com/Azure/radius/mocks/mockhandlers"
	"github.com/Azure/radius/mocks/mockrenderers"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
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
	mockResourceHandler := mockhandlers.NewMockResourceHandler(ctrl)
	mockResourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(2).Return(map[string]string{}, nil)
	mockHealthHandler := mockhandlers.NewMockHealthHandler(ctrl)
	mockHealthHandler.EXPECT().GetHealthOptions(gomock.Any()).Times(2).Return(healthcontract.HealthCheckOptions{})
	mockRendererKind := mockrenderers.NewMockWorkloadRenderer(ctrl)
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
			Type:     outputresource.TypeKubernetes,
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

	actions := map[string]ComponentAction{
		"C": {
			ComponentName: "C",
			Operation:     CreateWorkload,
			Component: &components.GenericComponent{
				Kind: "Dummy1",
			},
			Definition: &db.Component{
				Kind: "Kind1",
				Properties: db.ComponentProperties{
					Uses: []db.ComponentDependency{},
				},
			},
		},
	}

	deployStatus := db.DeploymentStatus{}

	var ctx context.Context
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		ctx = context.Background()
	}
	ctx = logr.NewContext(context.Background(), logger)
	dp.UpdateDeployment(ctx, "A", "A", &deployStatus, actions)

	// Registration for first output resource
	msg1 := <-registrationChannel
	require.Equal(t, healthcontract.ActionRegister, msg1.Action)
	require.Equal(t, "ResourceID_1", msg1.ResourceInfo.ResourceID)
	require.Equal(t, "Kind1", msg1.ResourceInfo.ResourceKind)
	outputResourceInfo1 := healthcontract.ResourceDetails{
		ResourceID:    "ResourceID_1",
		ResourceKind:  "Kind1",
		ApplicationID: "A",
		ComponentID:   "C",
	}
	require.Equal(t, outputResourceInfo1.GetHealthID(), msg1.ResourceInfo.HealthID)

	// Registration for second output resource
	msg2 := <-registrationChannel
	require.Equal(t, healthcontract.ActionRegister, msg2.Action)
	require.Equal(t, "ns1-name1", msg2.ResourceInfo.ResourceID)
	require.Equal(t, "Kind1", msg2.ResourceInfo.ResourceKind)
	outputResourceInfo2 := healthcontract.ResourceDetails{
		ResourceID:    "ns1-name1",
		ResourceKind:  "Kind1",
		ApplicationID: "A",
		ComponentID:   "C",
	}
	require.Equal(t, outputResourceInfo2.GetHealthID(), msg2.ResourceInfo.HealthID)
}

func Test_DeploymentProcessor_UnregistersOutputResourcesWithHealthService(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockResourceHandler := mockhandlers.NewMockResourceHandler(ctrl)
	mockResourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockHealthHandler := mockhandlers.NewMockHealthHandler(ctrl)
	mockRendererKind := mockrenderers.NewMockWorkloadRenderer(ctrl)
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
			Type:     outputresource.TypeKubernetes,
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
		ResourceID:    "ResourceID_1",
		ResourceKind:  "Kind1",
		ApplicationID: "A",
		ComponentID:   "C",
	}

	outputResourceInfo2 := healthcontract.ResourceDetails{
		ResourceID:    "ns1-name1",
		ResourceKind:  "Kind1",
		ApplicationID: "A",
		ComponentID:   "C",
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
	}
	ctx = logr.NewContext(context.Background(), logger)
	dp.DeleteDeployment(ctx, "A", "A", &deployStatus)

	// Unregistration for first output resource
	msg1 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg1.Action)
	require.Equal(t, outputResourceInfo1.GetHealthID(), msg1.ResourceInfo.HealthID)

	// Registration for second output resource
	msg2 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg2.Action)
	require.Equal(t, outputResourceInfo2.GetHealthID(), msg2.ResourceInfo.HealthID)
}
