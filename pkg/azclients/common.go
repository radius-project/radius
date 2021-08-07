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

func GetDefaultAPIVersion(ctx context.Context, subscriptionId string, authorizer autorest.Authorizer, entireType string) (string, error) {
	parts := strings.Split(entireType, "/")
	provider := parts[0]
	resourceType := parts[1]

	providerc := NewProvidersClient(subscriptionId, authorizer)

	p, err := providerc.Get(ctx, provider, "")
	if err != nil {
		return "", err
	}

	for _, rt := range *p.ResourceTypes {
		if strings.EqualFold(*rt.ResourceType, resourceType) {
			if rt.DefaultAPIVersion != nil {
				return *rt.DefaultAPIVersion, nil
			} else if rt.APIProfiles != nil && len(*rt.APIVersions) > 0 {
				return (*rt.APIVersions)[0], nil
			} else {
				return "", fmt.Errorf("no valid api version for type %s", entireType)
			}
		}
	}

	return "", nil // unreachable
}
