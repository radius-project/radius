// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rp

import (
	context "context"
	"fmt"

	"github.com/go-openapi/jsonpointer"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/resourcemodel"
	resources "github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

func NewSecretValueClient(arm armauth.ArmConfig) SecretValueClient {
	return &client{ARM: arm}
}

var _ SecretValueClient = (*client)(nil)

type client struct {
	ARM armauth.ArmConfig
}

func (c *client) FetchSecret(ctx context.Context, identity resourcemodel.ResourceIdentity, action string, valueSelector string) (interface{}, error) {
	arm := &resourcemodel.ARMIdentity{}
	if err := store.DecodeMap(identity.Data, arm); err != nil {
		return nil, fmt.Errorf("unsupported resource type: %+v. Currently only ARM resources are supported", identity)
	}

	parsed, err := resources.ParseResource(arm.ID)
	if err != nil {
		return nil, err
	}

	custom := clients.NewCustomActionClient(parsed.FindScope(resources.SubscriptionsSegment), c.ARM.Auth)
	response, err := custom.InvokeCustomAction(ctx, arm.ID, arm.APIVersion, action, nil)
	if err != nil {
		return nil, err
	}

	pointer, err := jsonpointer.New(valueSelector)
	if err != nil {
		return nil, err
	}

	secretValue, _, err := pointer.Get(response.Body)
	if err != nil {
		return nil, err
	}

	return secretValue, err
}
