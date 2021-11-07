// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	context "context"
	"fmt"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/go-openapi/jsonpointer"
)

func NewSecretValueClient(auth autorest.Authorizer) SecretValueClient {
	return &client{Auth: auth}
}

var _ SecretValueClient = (*client)(nil)

type client struct {
	Auth autorest.Authorizer
}

func (c *client) FetchSecret(ctx context.Context, identity resourcemodel.ResourceIdentity, action string, valueSelector string) (interface{}, error) {
	arm, ok := identity.Data.(resourcemodel.ARMIdentity)
	if !ok {
		return nil, fmt.Errorf("unsupported resource type: %+v. Currently only ARM resources are supported", identity)
	}

	id, err := azresources.Parse(arm.ID)
	if err != nil {
		return nil, err
	}

	custom := clients.NewCustomActionClient(id.SubscriptionID, c.Auth)
	response, err := custom.InvokeCustomAction(ctx, arm.ID, arm.APIVersion, action, nil)
	if err != nil {
		return nil, err
	}

	pointer, err := jsonpointer.New(valueSelector)
	if err != nil {
		return nil, err
	}

	value, _, err := pointer.Get(response.Body)
	if err != nil {
		return nil, err
	}

	return value, err
}
