// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

//TODO: Change subId and ResourceId to scope
type ARMUCPManagementClient struct {
	Connection      *arm.Connection
	EnvironmentName string
	Scope           string
}

var _ clients.AppManagementClient = (*ARMUCPManagementClient)(nil)

func (um *ARMUCPManagementClient) ListEnv(ctx context.Context) ([]v20220315privatepreview.EnvironmentResourceList, error) {

	return nil, nil

}
