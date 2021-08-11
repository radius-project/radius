// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/model/revision"
	"github.com/Azure/radius/pkg/radrp/db"
)

// ComponentAction represents a set of deployment actions to take for a component instance.
type ComponentAction struct {
	ApplicationName string
	ComponentName   string
	Operation       DeploymentOperation

	NewRevision revision.Revision
	OldRevision revision.Revision

	// Will be `nil` for a delete
	Definition *db.Component
	// Will be `nil` for a delete
	Component *components.GenericComponent
}

// DependencyItem implementation
func (action ComponentAction) Key() string {
	return action.ComponentName
}

func (action ComponentAction) GetDependencies() ([]string, error) {
	if action.Component == nil {
		return []string{}, nil
	}

	dependencies := []string{}
	for _, dependency := range action.Component.Uses {
		if dependency.Binding.Kind == components.KindStatic {
			continue
		}

		expr := dependency.Binding.Value.(*components.ComponentBindingValue)
		dependencies = append(dependencies, expr.Component)
	}

	return dependencies, nil
}
