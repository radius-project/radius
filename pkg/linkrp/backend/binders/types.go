// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package binders

import (
	"context"

	"github.com/project-radius/radius/pkg/rp"
)

type FetchFunc = func(ctx context.Context, obj interface{}, id string, apiVersion string) error

type Binder[T any] interface {
	Bind(ctx context.Context, id string, fetch FetchFunc, destination T, secrets map[string]rp.SecretValueReference) error
}
