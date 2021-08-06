// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/google/uuid"
)

type ARMDeploymentClient struct {
	ResourceGroup  string
	SubscriptionID string
	Client         resources.DeploymentsClient
}

var _ clients.DeploymentClient = (*ARMDeploymentClient)(nil)

func (dc *ARMDeploymentClient) Deploy(ctx context.Context, content string) error {
	template := map[string]interface{}{}
	err := json.Unmarshal([]byte(content), &template)
	if err != nil {
		return err
	}

	name := fmt.Sprintf("rad-deploy-%v", uuid.New().String())
	op, err := dc.Client.CreateOrUpdate(ctx, dc.ResourceGroup, name, resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template:   template,
			Parameters: map[string]interface{}{},
			Mode:       resources.DeploymentModeIncremental,
		},
	})
	if err != nil {
		return err
	}

	err = op.WaitForCompletionRef(ctx, dc.Client.Client)
	if err != nil {
		return err
	}

	_, err = op.Result(dc.Client)
	if err != nil {
		return err
	}

	return err
}
