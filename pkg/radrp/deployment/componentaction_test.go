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

	require.Empty(t, action.GetDependencies())
}

func Test_ComponentAction_GetDependencies_None(t *testing.T) {
	action := ComponentAction{
		Component: &components.GenericComponent{},
	}

	require.Empty(t, action.GetDependencies())
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

	require.Equal(t, []string{"A", "C"}, action.GetDependencies())
}
