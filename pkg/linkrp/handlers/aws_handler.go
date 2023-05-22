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

package handlers

import (
	"context"
	"encoding/json"

	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// NewAWSHandler creates a new ResourceHandler for AWS resources.
func NewAWSHandler(connection sdk.Connection) ResourceHandler {
	return &awsHandler{connection: connection}
}

type awsHandler struct {
	connection sdk.Connection
}

// Put implements Put for AWS resources.
func (handler *awsHandler) Put(ctx context.Context, resource *rpv1.OutputResource) (resourcemodel.ResourceIdentity, map[string]string, error) {
	id, err := resource.Identity.RequireAWS()
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	parsed, err := resources.ParseResource(id)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	client, err := generated.NewGenericResourcesClient(parsed.RootScope(), parsed.Type(), &aztoken.AnonymousCredential{}, sdk.NewClientOptions(handler.connection))
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	response, err := client.Get(ctx, parsed.Name(), nil)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// Serialize to map format since that's what the JSON-pointer library expects
	b, err := response.MarshalJSON()
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	data := map[string]any{}
	err = json.Unmarshal(b, &data)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	resource.Resource = data

	return resource.Identity, map[string]string{}, nil
}

// Delete implementes Delete for AWS resources.
func (handler *awsHandler) Delete(ctx context.Context, resource *rpv1.OutputResource) error {
	if !resource.IsRadiusManaged() {
		return nil
	}

	id, err := resource.Identity.RequireAWS()
	if err != nil {
		return err
	}

	parsed, err := resources.ParseResource(id)
	if err != nil {
		return err
	}

	client, err := generated.NewGenericResourcesClient(parsed.RootScope(), parsed.Type(), &aztoken.AnonymousCredential{}, sdk.NewClientOptions(handler.connection))
	if err != nil {
		return err
	}

	poller, err := client.BeginDelete(ctx, parsed.Name(), nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}
