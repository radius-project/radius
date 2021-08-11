// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azclients

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/go-autorest/autorest"
)

func GetDefaultAPIVersion(ctx context.Context, subscriptionId string, authorizer autorest.Authorizer, resourceType string) (string, error) {
	parts := strings.Split(resourceType, "/")
	provider := parts[0]
	typeWithoutProvider := parts[1]

	providerc := NewProvidersClient(subscriptionId, authorizer)

	p, err := providerc.Get(ctx, provider, "")
	if err != nil {
		return "", err
	}

	// For preview API versions, the DefaultAPIVersion isn't set,
	// so get the first from the list of APIVersions instead.
	for _, rt := range *p.ResourceTypes {
		if strings.EqualFold(*rt.ResourceType, typeWithoutProvider) {
			if rt.DefaultAPIVersion != nil {
				return *rt.DefaultAPIVersion, nil
			} else if rt.APIVersions != nil && len(*rt.APIVersions) > 0 {
				return (*rt.APIVersions)[0], nil
			} else {
				return "", fmt.Errorf("no valid api version for type %s", resourceType)
			}
		}
	}

	return "", nil // unreachable
}
