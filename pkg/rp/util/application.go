/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	resources "github.com/project-radius/radius/pkg/ucp/resources"
)

// # Function Explanation
//
// FetchApplication fetches an application resource from the Azure Resource Manager using the given application ID and
// client options, and returns the application resource or an error if one occurs.
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
