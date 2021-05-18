// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package curp

import (
	"testing"

	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/curp/deployment"
	"github.com/Azure/radius/pkg/curp/metadata"
	"github.com/Azure/radius/pkg/curp/revision"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	"github.com/stretchr/testify/require"
)

func Test_DeploymentCreated_NoComponents(t *testing.T) {
	app := db.NewApplication()
	newer := db.NewDeployment()
	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, nil, newer)
	require.NoError(t, err)

	require.True(t, deploymentIsNoOp(actions))
	require.Empty(t, actions)
}

func Test_DeploymentCreated_ValidationError(t *testing.T) {
	app := db.NewApplication()
	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "", // intentionally empty
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	_, err := rp.computeDeploymentActions(app, nil, newer)
	require.Error(t, err)
}

func Test_DeploymentCreated_ErrMissingComponent(t *testing.T) {
	app := db.NewApplication()
	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	_, err := rp.computeDeploymentActions(app, nil, newer)
	require.Error(t, err)
}

func Test_DeploymentCreated_OneComponent(t *testing.T) {
	app := db.NewApplication()
	app.Components["A"] = db.Component{
		Revision:   revision.Revision("1"),
		Kind:       containerv1alpha1.Kind,
		Properties: *db.NewComponentProperties(),
	}

	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, nil, newer)
	require.NoError(t, err)

	require.False(t, deploymentIsNoOp(actions))

	// Updates to the components are in the actions
	require.Len(t, actions, 1)

	require.Contains(t, actions, "A")
	action := actions["A"]
	require.Equal(t, deployment.CreateWorkload, action.Operation)
	require.Equal(t, "A", action.ComponentName)
	require.Equal(t, app.Components["A"], *action.Definition)
	require.Equal(t, revision.Revision(""), action.OldRevision)
	require.Equal(t, revision.Revision("1"), action.NewRevision)
}

func Test_DeploymentCreated_MultipleComponents(t *testing.T) {
	app := db.NewApplication()
	app.Components["A"] = db.Component{
		Kind:       containerv1alpha1.Kind,
		Revision:   revision.Revision("1"),
		Properties: *db.NewComponentProperties(),
	}
	app.Components["B"] = db.Component{
		Kind:       containerv1alpha1.Kind,
		Revision:   revision.Revision("1"),
		Properties: *db.NewComponentProperties(),
	}

	app.Components["C"] = db.Component{
		Kind:       containerv1alpha1.Kind,
		Revision:   revision.Revision("1"),
		Properties: *db.NewComponentProperties(),
	}

	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
		},
		{
			ComponentName: "B",
		},
		{
			ComponentName: "C",
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, nil, newer)
	require.NoError(t, err)

	require.False(t, deploymentIsNoOp(actions))

	// Revision is updated in the deployment.
	require.Equal(t, revision.Revision("1"), newer.Properties.Components[0].Revision)
	require.Equal(t, revision.Revision("1"), newer.Properties.Components[1].Revision)
	require.Equal(t, revision.Revision("1"), newer.Properties.Components[2].Revision)

	// Updates to the components are in the actions
	require.Len(t, actions, 3)

	require.Contains(t, actions, "A")
	action := actions["A"]
	require.Equal(t, deployment.CreateWorkload, action.Operation)
	require.Equal(t, "A", action.ComponentName)
	require.Equal(t, app.Components["A"], *action.Definition)
	require.Equal(t, revision.Revision(""), action.OldRevision)
	require.Equal(t, revision.Revision("1"), action.NewRevision)

	require.Contains(t, actions, "B")
	action = actions["B"]
	require.Equal(t, deployment.CreateWorkload, action.Operation)
	require.Equal(t, "B", action.ComponentName)
	require.Equal(t, app.Components["B"], *action.Definition)
	require.Equal(t, revision.Revision(""), action.OldRevision)
	require.Equal(t, revision.Revision("1"), action.NewRevision)

	require.Contains(t, actions, "C")
	action = actions["C"]
	require.Equal(t, deployment.CreateWorkload, action.Operation)
	require.Equal(t, "C", action.ComponentName)
	require.Equal(t, app.Components["C"], *action.Definition)
	require.Equal(t, revision.Revision(""), action.OldRevision)
	require.Equal(t, revision.Revision("1"), action.NewRevision)
}

func Test_DeploymentUpdated_OneComponent_Deleted(t *testing.T) {
	app := db.NewApplication()
	app.Components["A"] = db.Component{
		Kind:       containerv1alpha1.Kind,
		Revision:   revision.Revision("1"),
		Properties: *db.NewComponentProperties(),
	}

	older := db.NewDeployment()
	older.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
			Revision:      revision.Revision("1"),
		},
	}

	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, older, newer)
	require.NoError(t, err)

	require.False(t, deploymentIsNoOp(actions))

	// Updates to the components are in the actions
	require.Len(t, actions, 1)

	require.Contains(t, actions, "A")
	action := actions["A"]
	require.Equal(t, deployment.DeleteWorkload, action.Operation)
	require.Equal(t, "A", action.ComponentName)
	require.Nil(t, action.Definition)
	require.Equal(t, revision.Revision("1"), action.OldRevision)
	require.Equal(t, revision.Revision(""), action.NewRevision)
}

func Test_DeploymentUpdated_OneComponent_NoAction(t *testing.T) {
	app := db.NewApplication()
	app.Components["A"] = db.Component{
		Kind:       containerv1alpha1.Kind,
		Revision:   revision.Revision("2"),
		Properties: *db.NewComponentProperties(),
	}

	older := db.NewDeployment()
	older.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
			Revision:      revision.Revision("2"),
		},
	}

	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
			Revision:      revision.Revision("2"),
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, older, newer)
	require.NoError(t, err)

	require.True(t, deploymentIsNoOp(actions))

	// Updates to the components are in the actions
	require.Len(t, actions, 1)

	require.Contains(t, actions, "A")
	action := actions["A"]
	require.Equal(t, deployment.None, action.Operation)
	require.Equal(t, "A", action.ComponentName)
	require.Equal(t, app.Components["A"], *action.Definition)
	require.Equal(t, revision.Revision("2"), action.OldRevision)
	require.Equal(t, revision.Revision("2"), action.NewRevision)
}

func Test_DeploymentUpdated_OneComponent_RevisionUpgraded(t *testing.T) {
	app := db.NewApplication()
	app.Components["A"] = db.Component{
		Kind:       containerv1alpha1.Kind,
		Revision:   revision.Revision("2"),
		Properties: *db.NewComponentProperties(),
	}

	older := db.NewDeployment()
	older.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
			Revision:      revision.Revision("1"),
		},
	}

	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
			Revision:      revision.Revision("2"),
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, older, newer)
	require.NoError(t, err)

	require.False(t, deploymentIsNoOp(actions))

	// Updates to the components are in the actions
	require.Len(t, actions, 1)

	require.Contains(t, actions, "A")
	action := actions["A"]
	require.Equal(t, deployment.UpdateWorkload, action.Operation)
	require.Equal(t, "A", action.ComponentName)
	require.Equal(t, app.Components["A"], *action.Definition)
	require.Equal(t, revision.Revision("1"), action.OldRevision)
	require.Equal(t, revision.Revision("2"), action.NewRevision)
}

func Test_DeploymentCreated_MultipleComponents_ServiceBinding(t *testing.T) {
	app := db.NewApplication()
	app.Components["A"] = db.Component{
		Kind:     containerv1alpha1.Kind,
		Revision: revision.Revision("1"),
		Properties: db.ComponentProperties{
			Build: map[string]interface{}{},
			Run:   map[string]interface{}{},
			Provides: []db.ComponentProvides{
				{
					Name: "A",
					Kind: "http",
				},
			},
			DependsOn: []db.ComponentDependsOn{
				{
					Name: "B",
					Kind: "http",
				},
			},
		},
	}

	app.Components["B"] = db.Component{
		Kind:     containerv1alpha1.Kind,
		Revision: revision.Revision("1"),
		Properties: db.ComponentProperties{
			Build: map[string]interface{}{},
			Run:   map[string]interface{}{},
			Provides: []db.ComponentProvides{
				{
					Name:          "B",
					Kind:          "http",
					ContainerPort: (func() *int { x := 80; return &x })(),
				},
			},
			DependsOn: []db.ComponentDependsOn{
				{
					Name: "A",
					Kind: "http",
				},
			},
		},
	}

	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
			Revision:      revision.Revision("1"),
		},
		{
			ComponentName: "B",
			Revision:      revision.Revision("1"),
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, nil, newer)
	require.NoError(t, err)

	require.False(t, deploymentIsNoOp(actions))

	// Updates to the components are in the actions
	require.Len(t, actions, 2)

	require.Contains(t, actions, "A")
	action := actions["A"]
	require.Equal(t, deployment.CreateWorkload, action.Operation)
	require.Equal(t, "A", action.ComponentName)
	require.Equal(t, map[string]deployment.ServiceBinding{"B": {Name: "B", Kind: "http", Provider: "B"}}, action.ServiceBindings)

	require.Contains(t, actions, "B")
	action = actions["B"]
	require.Equal(t, deployment.CreateWorkload, action.Operation)
	require.Equal(t, "B", action.ComponentName)
	require.Equal(t, map[string]deployment.ServiceBinding{"A": {Name: "A", Kind: "http", Provider: "A"}}, action.ServiceBindings)
}

func Test_DeploymentUpdated_RenderRealisticContainer(t *testing.T) {
	app := db.NewApplication()
	app.Name = "radius/myapp"
	app.Components["A"] = db.Component{
		Kind:     containerv1alpha1.Kind,
		Revision: revision.Revision("1"),
		Properties: db.ComponentProperties{
			Build: map[string]interface{}{},
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "rynowak/frontend:0.5.0-dev",
					"env": []interface{}{
						map[string]interface{}{
							"name":  "SERVICE__BACKEND__HOST",
							"value": "backend",
						},
						map[string]interface{}{
							"name":  "SERVICE__BACKEND__PORT",
							"value": "80",
						},
					},
				},
			},
		},
	}

	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
			Revision:      revision.Revision("1"),
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, nil, newer)
	require.NoError(t, err)

	require.False(t, deploymentIsNoOp(actions))

	// Updates to the components are in the actions
	require.Len(t, actions, 1)

	require.Contains(t, actions, "A")
	action := actions["A"]
	require.Equal(t, deployment.CreateWorkload, action.Operation)
	require.Equal(t, "myapp", action.ApplicationName)
	require.Equal(t, "A", action.ComponentName)
	require.Equal(t, app.Components["A"], *action.Definition)

	// validate the workload
	require.Equal(t, containerv1alpha1.Kind, action.Component.Kind)
	require.Equal(t, "A", action.Component.Name)

	component := containerv1alpha1.ContainerComponent{}
	err = action.Component.AsRequired(containerv1alpha1.Kind, &component)
	require.NoError(t, err)

	cont := component.Run.Container
	require.Equal(t, "rynowak/frontend:0.5.0-dev", cont.Image)

	require.Len(t, cont.Environment, 2)
	require.Equal(t, "SERVICE__BACKEND__HOST", cont.Environment[0].Name)
	require.Equal(t, "backend", *cont.Environment[0].Value)
	require.Equal(t, "SERVICE__BACKEND__PORT", cont.Environment[1].Name)
	require.Equal(t, "80", *cont.Environment[1].Value)
}

func Test_DeploymentCreated_RenderContainerWithDapr(t *testing.T) {
	app := db.NewApplication()
	app.Name = "radius/myapp"
	app.Components["A"] = db.Component{
		Kind:     containerv1alpha1.Kind,
		Revision: revision.Revision("1"),
		Properties: db.ComponentProperties{
			Build: map[string]interface{}{},
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "rynowak/frontend:0.5.0-dev",
				},
			},
			Traits: []db.ComponentTrait{
				{
					Kind: "dapr.io/App@v1alpha1",
					Properties: map[string]interface{}{
						"appId":   "frontend",
						"appPort": 80,
					},
				},
			},
		},
	}

	newer := db.NewDeployment()
	newer.Properties.Components = []*db.DeploymentComponent{
		{
			ComponentName: "A",
			Revision:      revision.Revision("1"),
		},
	}

	rp := rp{
		meta: metadata.NewRegistry(),
	}

	actions, err := rp.computeDeploymentActions(app, nil, newer)
	require.NoError(t, err)

	require.False(t, deploymentIsNoOp(actions))

	// Updates to the components are in the actions
	require.Len(t, actions, 1)

	require.Contains(t, actions, "A")
	action := actions["A"]
	require.Equal(t, deployment.CreateWorkload, action.Operation)
	require.Equal(t, "A", action.ComponentName)
	require.Equal(t, app.Components["A"], *action.Definition)

	require.Equal(t, "dapr.io/App@v1alpha1", action.Component.Traits[0].Kind)
	require.Equal(t, map[string]interface{}{
		"appId":   "frontend",
		"appPort": float64(80),
	}, action.Component.Traits[0].Properties)
}
