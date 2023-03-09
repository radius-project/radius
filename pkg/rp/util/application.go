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

// FetchApplication gets the environment resource using application id
func FetchApplication(ctx context.Context, application string, ucpOptions *arm.ClientOptions) (*v20220315privatepreview.ApplicationResource, error) {
	applicationID, err := resources.ParseResource(application)
	if err != nil {
		return nil, err
	}

	client, err := v20220315privatepreview.NewApplicationsClient(applicationID.RootScope(), &aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, applicationID.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.ApplicationResource, nil
}
