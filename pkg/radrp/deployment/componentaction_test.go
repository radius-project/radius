// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"testing"

	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/stretchr/testify/require"
)

func Test_ComponentAction_GetDependencies_Null(t *testing.T) {
	// action.Component will be null for a delete action
	action := ComponentAction{}
	dependencies, err := action.GetDependencies()
	require.NoError(t, err)

	require.Empty(t, dependencies)
}

func Test_ComponentAction_GetDependencies_None(t *testing.T) {
	action := ComponentAction{
		Component: &components.GenericComponent{},
	}

	dependencies, err := action.GetDependencies()
	require.NoError(t, err)

	require.Empty(t, dependencies)
}

func Test_ComponentAction_GetDependencies_Some(t *testing.T) {
	action := ComponentAction{
		Component: &components.GenericComponent{
			Uses: []components.GenericDependency{
				{
					Binding: components.NewComponentBindingExpression("myapp", "A", "test", ""),
				},
				{
					// Should be ignored
					Binding: components.BindingExpression{
						Kind: components.KindStatic,
					},
				},
				{
					Binding: components.NewComponentBindingExpression("myapp", "C", "test", ""),
				},
			},
		},
	}

	dependencies, err := action.GetDependencies()
	require.NoError(t, err)

	require.Equal(t, []string{"A", "C"}, dependencies)
}
