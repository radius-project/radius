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
)

const (
	ConnectionStringValue = "connectionString"
	DatabaseNameValue     = "database"
	UsernameStringValue   = "username"
	PasswordStringHolder  = "password"
)

var ErrorResourceOrServerNameMissingFromResource = errors.New("either the 'resource' or 'server'/'database' is required")

type Renderer interface {
	Render(ctx context.Context, resource conv.DataModelInterface) (RendererOutput, error)
}

type RendererOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]ComputedValueReference
	SecretValues   map[string]SecretValueReference
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

// SecretValueReference represents a secret value that can accessed on the output resources
// have been deployed.
type SecretValueReference struct {
	// SecretValueReference always needs to be resolved against a deployed resource. These
	// are secrets so we don't want to store them.

	// LocalID is used to resolve the 'target' output resource for retrieving the secret value.
	LocalID string

	// Action refers to a named custom action used to fetch the secret value. Maybe be empty in the case of Kubernetes since there's
	// no concept of 'action'. Will always be set for an ARM resource.
	Action string

	// ValueSelector is a JSONPointer used to resolve the secret value.
	ValueSelector string

	// Transformer is a reference to a SecretValueTransformer that can be looked up by name.
	// By-convention this is the Resource Type of the resource (eg: Microsoft.DocumentDB/databaseAccounts).
	// If there are multiple kinds of transformers per Resource Type, then add a unique suffix.
	//
	// NOTE: the transformer is a string key because it has to round-trip from
	// the database. We don't store the secret value, so we have to be able to process it later.
	Transformer resourcemodel.ResourceType

	// Value is the secret value itself
	Value string
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

//go:generate mockgen -destination=./mock_secretvalueclient.go -package=renderers -self_package github.com/project-radius/radius/pkg/renderers github.com/project-radius/radius/pkg/renderers SecretValueClient
type SecretValueClient interface {
	FetchSecret(ctx context.Context, identity resourcemodel.ResourceIdentity, action string, valueSelector string) (interface{}, error)
}
