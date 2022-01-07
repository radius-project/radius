// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	context "context"
	"fmt"

	"github.com/go-openapi/jsonpointer"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

func NewSecretValueClient(arm armauth.ArmConfig) SecretValueClient {
	return &client{ARM: arm}
}

var _ SecretValueClient = (*client)(nil)

type client struct {
	ARM armauth.ArmConfig
}

func (c *client) FetchSecret(ctx context.Context, identity resourcemodel.ResourceIdentity, action string, valueSelector string) (interface{}, error) {
	arm, ok := identity.Data.(resourcemodel.ARMIdentity)
	if !ok {
		return nil, fmt.Errorf("unsupported resource type: %+v. Currently only ARM resources are supported", identity)
	}

	custom := clients.NewCustomActionClient(c.ARM.SubscriptionID, c.ARM.Auth)
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
