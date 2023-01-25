// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretvalue

import (
	"context"

	"github.com/project-radius/radius/pkg/resourcemodel"
)

// SecretValueTransformer allows transforming a secret value before passing it on to a Resource
// that wants to access it.
//
// This is surprisingly common. For example, it's common for access control/connection strings to apply
// to an 'account' primitive such as a ServiceBus namespace or CosmosDB account. The actual connection
// string that application code consumes will include a database name or queue name, etc. Or the different
// libraries involved might support different connection string formats, and the user has to choose on.
type SecretValueTransformer interface {
	Transform(ctx context.Context, resourceComputedValues map[string]any, secretValue any) (any, error)
}

//go:generate mockgen -destination=./mock_secretvalueclient.go -package=secretvalue -self_package github.com/project-radius/radius/pkg/rp/secretvalue github.com/project-radius/radius/pkg/rp/secretvalue SecretValueClient
type SecretValueClient interface {
	FetchSecret(ctx context.Context, identity resourcemodel.ResourceIdentity, action string, valueSelector string) (any, error)
}
