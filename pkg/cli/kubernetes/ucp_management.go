// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/cli/clients"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

//TODO: Change subId and ResourceId to scope
type ARMUCPManagementClient struct {
	Connection      *arm.Connection
	EnvironmentName string
	RootScope       string
}

var _ clients.AppManagementClient = (*ARMUCPManagementClient)(nil)

func (um *ARMUCPManagementClient) ListEnv(ctx context.Context) ([]v20220315privatepreview.EnvironmentResource, error) {

	envClient := v20220315privatepreview.NewEnvironmentsClient(um.Connection, um.RootScope)
	envListPager := envClient.ListByScope(&v20220315privatepreview.EnvironmentsListByScopeOptions{})
	envResourceList := []v20220315privatepreview.EnvironmentResource{}
	for envListPager.NextPage(ctx) {
		currEnvPage := envListPager.PageResponse().EnvironmentResourceList.Value
		for _, env := range currEnvPage {
			envResourceList = append(envResourceList, *env)
		}
	}

	return envResourceList, nil

}
