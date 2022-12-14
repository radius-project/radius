// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rp

import (
	context "context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/go-openapi/jsonpointer"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
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

	subscriptionID := parsed.FindScope(resources.SubscriptionsSegment)
	client, err := clientv2.NewCustomActionClient(subscriptionID, c.ARM.ClientOption.Cred)
	if err != nil {
		return nil, err
	}

	options := clientv2.NewClientBeginCustomActionOptions(arm.ID, action, arm.APIVersion)
	poller, err := client.BeginCustomAction(ctx, options)
	if err != nil {
		return nil, err
	}

	response, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{
		Frequency: 0, // Pass zero to accept the default value (30s).
	})
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
