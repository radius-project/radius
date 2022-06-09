// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/cli/clients"
)
//TODO: Change subId and ResourceId to scope
type ARMUCPManagementClient struct {
	Connection      *arm.Connection
	ResourceGroup   string
	SubscriptionID  string
	EnvironmentName string
}

var _ clients.FirstPartyServiceManagementClient = (*ARMUCPManagementClient)(nil)

func (um *ARMUCPManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) (error) {
	return nil
}
