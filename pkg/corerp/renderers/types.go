// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

//go:generate mockgen -destination=./mock_renderer.go -package=renderers github.com/project-radius/radius/pkg/corerp/renderers Renderer
type Renderer interface {
	GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error)
	Render(ctx context.Context, resource conv.DataModelInterface, options RenderOptions) (RendererOutput, error)
}

type RenderOptions struct {
	Dependencies map[string]RendererDependency
	Environment  EnvironmentOptions
}

// Represents a dependency of the resource currently being rendered. Currently dependencies are always Radius resources.
type RendererDependency struct {
	// ResourceID is the resource ID of the Radius resource that is the dependency.
	ResourceID resources.ID

	// Definition is the definition (`properties` node) of the dependency.
	Definition map[string]interface{}

	// ComputedValues is a map of the computed values and secrets of the dependency.
	ComputedValues map[string]interface{}

	// OutputResources is a map of the output resource identities of the dependency. The map is keyed on the LocalID of the output resource.
	OutputResources map[string]resourcemodel.ResourceIdentity
}

type EnvironmentOptions struct {
	Gateway   GatewayOptions
	Namespace string
}

type GatewayOptions struct {
	PublicEndpointOverride bool
	PublicIP               string
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
	Transform(ctx context.Context, dependency RendererDependency, value interface{}) (interface{}, error)
}

//go:generate mockgen -destination=./mock_secretvalueclient.go -package=renderers -self_package github.com/project-radius/radius/pkg/renderers github.com/project-radius/radius/pkg/renderers SecretValueClient
type SecretValueClient interface {
	FetchSecret(ctx context.Context, identity resourcemodel.ResourceIdentity, action string, valueSelector string) (interface{}, error)
}

// HACK remove this once we consolidate handlers between core and connector RP.
var _ SecretValueTransformer = (*AzureTransformer)(nil)

type AzureTransformer struct {
}

func (t *AzureTransformer) Transform(ctx context.Context, dependency RendererDependency, value interface{}) (interface{}, error) {
	// Mongo uses the following format for mongo: mongodb://{accountname}:{key}@{endpoint}:{port}/{database}?...{params}
	//
	// The connection strings that come back from CosmosDB don't include the database name.
	str, ok := value.(string)
	if !ok {
		return nil, errors.New("expected the connection string to be a string")
	}

	// These connection strings won't include the database
	u, err := url.Parse(str)
	if err != nil {
		return "", fmt.Errorf("failed to parse connection string as a URL: %w", err)
	}

	databaseName, ok := dependency.ComputedValues["database"].(string)
	if !ok {
		return nil, errors.New("expected the databaseName to be a string")
	}

	u.Path = "/" + databaseName
	return u.String(), nil
}
