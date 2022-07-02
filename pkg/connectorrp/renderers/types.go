// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
)

const (
	ConnectionStringValue = "connectionString"
	DatabaseNameValue     = "database"
	UsernameStringValue   = "username"
	PasswordStringHolder  = "password"
	Host                  = "host"
	Port                  = "port"
)

var ErrorResourceOrServerNameMissingFromResource = errors.New("either the 'resource' or 'server'/'database' is required")

var ErrResourceMissingForResource = errors.New("the 'resource' field is required")

//go:generate mockgen -destination=./mock_renderer.go -package=renderers github.com/project-radius/radius/pkg/connectorrp/renderers Renderer
type Renderer interface {
	Render(ctx context.Context, resource conv.DataModelInterface) (rp.RendererOutput, error)
}

type RendererOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]rp.ComputedValueReference
	SecretValues   map[string]rp.SecretValueReference
}

// SecretValueTransformer allows transforming a secret value before passing it on to a Resource
// that wants to access it.
//
// This is surprisingly common. For example, it's common for access control/connection strings to apply
// to an 'account' primitive such as a ServiceBus namespace or CosmosDB account. The actual connection
// string that application code consumes will include a database name or queue name, etc. Or the different
// libraries involved might support different connection string formats, and the user has to choose on.
type SecretValueTransformer interface {
	Transform(ctx context.Context, resource conv.DataModelInterface, value interface{}) (interface{}, error)
}

//go:generate mockgen -destination=./mock_secretvalueclient.go -package=renderers -self_package github.com/project-radius/radius/pkg/connectorrp/renderers github.com/project-radius/radius/pkg/connectorrp/renderers SecretValueClient
type SecretValueClient interface {
	FetchSecret(ctx context.Context, identity resourcemodel.ResourceIdentity, action string, valueSelector string) (interface{}, error)
}
