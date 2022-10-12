// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secrets

import (
	"context"
)

type Interface interface {
	CreateSecrets(ctx context.Context, id string, secrets interface{}) error
	DeleteSecrets(ctx context.Context, id string) error
	GetSecrets(ctx context.Context, id string) (string, error)
	ListSecrets(ctx context.Context, planeType string, planeName string, scope string) ([]string, error)
}
