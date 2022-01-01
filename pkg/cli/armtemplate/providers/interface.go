// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

import (
	"context"
)

type Provider interface {
	GetDeployedResource(ctx context.Context, id string, version string) (map[string]interface{}, error)
	DeployResource(ctx context.Context, id string, version string, body map[string]interface{}) (map[string]interface{}, error)
	InvokeCustomAction(ctx context.Context, id string, version string, action string, body interface{}) (map[string]interface{}, error)
}
