// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"context"
	"errors"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const (
	ConnectionStringValue = "connectionString"
	DatabaseNameValue     = "database"
	ServerNameValue       = "server"
	UsernameStringValue   = "username"
	PasswordStringHolder  = "password"
	Host                  = "host"
	Port                  = "port"
	ComponentNameKey      = "componentName"
)

var ErrorResourceOrServerNameMissingFromResource = errors.New("either the 'resource' or 'server'/'database' is required")

var ErrResourceMissingForResource = errors.New("the 'resource' field is required")

//go:generate mockgen -destination=./mock_renderer.go -package=renderers github.com/project-radius/radius/pkg/linkrp/renderers Renderer
type Renderer interface {
	Render(ctx context.Context, resource v1.ResourceDataModel, options RenderOptions) (RendererOutput, error)
}
type RenderOptions struct {
	Namespace            string
	RecipeProperties     datamodel.RecipeProperties
	EnvironmentProviders coreDatamodel.Providers
}

type RendererOutput struct {
	Resources      []rpv1.OutputResource
	ComputedValues map[string]ComputedValueReference
	SecretValues   map[string]rpv1.SecretValueReference
	RecipeData     datamodel.RecipeData
	// EnvironmentProviders specifies the providers mapped to the linked environment needed to deploy the recipe
	EnvironmentProviders coreDatamodel.Providers
	// RecipeContext specifies the context parameters for the recipe deployment
	RecipeContext datamodel.RecipeContext
}

// ComputedValueReference represents a non-secret value that can accessed once the output resources
// have been deployed.
type ComputedValueReference struct {
	// ComputedValueReference might hold a static value in `.Value` or might be a reference
	// that needs to be looked up.
	//
	// If `.Value` is set then treat this as a static value.
	//
	// If `.Value == nil` then use the `.PropertyReference` or to look up a property in the property
	// bag returned from deploying the resource via `handler.Put`.
	//
	// If `.Value == nil` && `.PropertyReference` is unset, then use JSONPointer to evaluate a JSON path
	// into the 'resource'.

	// LocalID specifies the output resource to be used for lookup. Does not apply with `.Value`
	LocalID string

	// Value specifies a static value to copy to computed values.
	Value any

	// PropertyReference specifies a property key to look up in the resource's *persisted properties*.
	PropertyReference string

	// JSONPointer specifies a JSON Pointer that can be used to look up the value in the resource's body.
	JSONPointer string
}
