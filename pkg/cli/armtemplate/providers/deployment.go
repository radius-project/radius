// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

import (
	"context"
	"errors"
)

var _ Provider = (*DeploymentProvider)(nil)

type DeploymentProvider struct {
	DeployFunc func(ctx context.Context, id string, version string, body map[string]interface{}) (map[string]interface{}, error)
}

func (store *DeploymentProvider) GetDeployedResource(ctx context.Context, id string, version string) (map[string]interface{}, error) {
	return nil, errors.New("the deployment provider does not support existing resources")
}

func (p *DeploymentProvider) DeployResource(ctx context.Context, id string, version string, body map[string]interface{}) (map[string]interface{}, error) {
	return p.DeployFunc(ctx, id, version, body)
}

func (p *DeploymentProvider) InvokeCustomAction(ctx context.Context, id string, version string, action string, body interface{}) (map[string]interface{}, error) {
	return nil, errors.New("the deployment provider does not support custom actions")
}
