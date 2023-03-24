// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	resources "github.com/project-radius/radius/pkg/ucp/resources"
)

// FetchEnvironment gets the environment resource using environment ID
func FetchEnvironment(ctx context.Context, environmentID string, ucpOptions *arm.ClientOptions) (*v20220315privatepreview.EnvironmentResource, error) {
	envID, err := resources.ParseResource(environmentID)
	if err != nil {
		return nil, err
	}

	client, err := v20220315privatepreview.NewEnvironmentsClient(envID.RootScope(), &aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, envID.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.EnvironmentResource, nil
}
