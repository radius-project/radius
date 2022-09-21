// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	ConnectionStringValue = "connectionString"
	DatabaseNameValue     = "database"
	SecretStoreName       = "secretStoreName"
	StateStoreName        = "stateStoreName"
	DatabaseName          = "database"
	ServerNameValue       = "server"
	UsernameStringValue   = "username"
	PasswordStringHolder  = "password"
	Host                  = "host"
	Port                  = "port"
)

var ErrorResourceOrServerNameMissingFromResource = errors.New("either the 'resource' or 'server'/'database' is required")

var ErrResourceMissingForResource = errors.New("the 'resource' field is required")

//go:generate mockgen -destination=./mock_renderer.go -package=renderers github.com/project-radius/radius/pkg/connectorrp/renderers Renderer
type Renderer interface {
	Render(ctx context.Context, resource conv.DataModelInterface, options RenderOptions) (RendererOutput, error)
}
type RenderOptions struct {
	Namespace           string
	RecipeConnectorType string
	RecipeTemplatePath  string
}

type RendererOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]ComputedValueReference
	SecretValues   map[string]rp.SecretValueReference
	RecipeData     RecipeData
}

type RecipeData struct {
	Name               string
	RecipeTemplatePath string
	APIVersion         string
	AzureResourceType  resources.KnownType
	Resources          []string // Resource id's of the resources deployed by the recipe
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
	Value interface{}

	// PropertyReference specifies a property key to look up in the resource's *persisted properties*.
	PropertyReference string

	// JSONPointer specifies a JSON Pointer that cn be used to look up the value in the resource's body.
	JSONPointer string
}
