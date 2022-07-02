// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/project-radius/radius/pkg/deployment github.com/project-radius/radius/pkg/deployment DeploymentProcessor
type DeploymentProcessor interface {
	Render(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (rp.RendererOutput, error)
	Deploy(ctx context.Context, id resources.ID, rendererOutput rp.RendererOutput) (rp.DeploymentOutput, error)
	Delete(ctx context.Context, id resources.ID, outputResources []outputresource.OutputResource) error
	FetchSecrets(ctx context.Context, resourceData rp.ResourceData) (map[string]interface{}, error)
}

// BaseDeploymentProcessor is the base deployment processor.
type BaseDeploymentProcessor struct {
	AppModel        interface{}
	StorageProvider dataprovider.DataStorageProvider
	SecretClient    renderers.SecretValueClient
	K8S             client.Client
}

func NewBaseDeploymentProcessor(appModel interface{}, storageProvider dataprovider.DataStorageProvider, secretClient renderers.SecretValueClient, k8s client.Client) BaseDeploymentProcessor {
	return BaseDeploymentProcessor{
		AppModel:        appModel,
		StorageProvider: storageProvider,
		SecretClient:    secretClient,
		K8S:             k8s,
	}
}
