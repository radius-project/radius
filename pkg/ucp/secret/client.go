// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secret

import (
	"context"
)

//go:generate mockgen -destination=./mock_client.go -package=secret -self_package github.com/project-radius/radius/pkg/ucp/secret github.com/project-radius/radius/pkg/ucp/secret Client

// Client is an interface to implement secret operations.
type Client interface {
	CreateOrUpdate(ctx context.Context, id string, secrets interface{}) error
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (string, error)
	List(ctx context.Context, planeType string, planeName string, scope string) ([]string, error)
}
