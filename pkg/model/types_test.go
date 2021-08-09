// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
)

var rs = map[string]workloads.WorkloadRenderer{
	"radius.dev/Test@v1alpha1": &NoOpRenderer{},
}
var hs = map[string]handlers.ResourceHandler{
	"testresourcetype": &NoOpHandler{},
}

func Test_GetResources(t *testing.T) {
	model := NewModel(rs, hs)

	resources := model.GetResources()
	require.Len(t, resources, 1)

	resourceType := resources[0]
	require.Equal(t, "testresourcetype", resourceType.Type())
	require.IsType(t, &NoOpHandler{}, resourceType.Handler())
}

func Test_LookupResourceType_Found(t *testing.T) {
	model := NewModel(rs, hs)

	resourceType, err := model.LookupResource("testresourcetype")
	require.NotNil(t, resourceType)
	require.Nil(t, err)

	require.Equal(t, "testresourcetype", resourceType.Type())
	require.IsType(t, &NoOpHandler{}, resourceType.Handler())
}

func Test_LookupResourceType_NotFound(t *testing.T) {
	model := NewModel(rs, hs)

	resourceType, err := model.LookupResource("sometype")
	require.Nil(t, resourceType)
	require.NotNil(t, err)

	require.Equal(t, "resource type 'sometype' is unsupported", err.Error())
}

func Test_GetComponents(t *testing.T) {
	model := NewModel(rs, hs)

	components := model.GetComponents()
	require.Len(t, components, 1)

	component := components[0]
	require.Equal(t, "radius.dev/Test@v1alpha1", component.Kind())
	require.IsType(t, &NoOpRenderer{}, component.Renderer())
}

func Test_LookupComponentKind_Found(t *testing.T) {
	model := NewModel(rs, hs)

	componentKind, err := model.LookupComponent("radius.dev/Test@v1alpha1")
	require.NotNil(t, componentKind)
	require.Nil(t, err)

	require.Equal(t, "radius.dev/Test@v1alpha1", componentKind.Kind())
	require.IsType(t, &NoOpRenderer{}, componentKind.Renderer())
}

func Test_LookupComponentKind_NotFound(t *testing.T) {
	model := NewModel(rs, hs)

	resourceType, err := model.LookupComponent("radius.dev/AnotherType@v1alpha1")
	require.Nil(t, resourceType)
	require.NotNil(t, err)

	require.Equal(t, "component kind 'radius.dev/AnotherType@v1alpha1' is unsupported", err.Error())
}

type NoOpRenderer struct {
}

func (*NoOpRenderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	return nil, nil
}

func (*NoOpRenderer) Render(ctx context.Context, workload workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	return nil, nil
}

type NoOpHandler struct {
}

func (*NoOpHandler) Put(ctx context.Context, options handlers.PutOptions) (map[string]string, error) {
	return nil, nil
}

func (*NoOpHandler) Delete(ctx context.Context, options handlers.DeleteOptions) error {
	return nil
}
