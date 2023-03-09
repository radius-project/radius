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

// FetchEnvironment gets the environment resource using environment id
func FetchEnvironment(ctx context.Context, environment string, ucpOptions *arm.ClientOptions) (*v20220315privatepreview.EnvironmentResource, error) {
	environmentID, err := resources.ParseResource(environment)
	if err != nil {
		return nil, err
	}

	client, err := v20220315privatepreview.NewEnvironmentsClient(environmentID.RootScope(), &aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, environmentID.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.EnvironmentResource, nil
}
