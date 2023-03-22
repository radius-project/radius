// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/sdk"
	clients "github.com/project-radius/radius/pkg/sdk/clients"
)

func GetUCPDeploymentClient(connection sdk.Connection) (*clients.ResourceDeploymentsClient, error) {
	client, err := clients.NewResourceDeploymentsClient(&clients.Options{
		ARMClientOptions: sdk.NewClientOptions(connection),
		BaseURI:          connection.Endpoint(),
		Cred:             &aztoken.AnonymousCredential{},
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}
