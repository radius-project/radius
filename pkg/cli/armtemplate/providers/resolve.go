// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

import (
	"errors"
	"fmt"
	"strings"
)

func GetProvider(providers map[string]Provider, provider string, version string, resourceType string) (Provider, error) {
	if providers == nil {
		return nil, errors.New("providers map was not initialized")
	}

	// Types that use symbolic references should just be looked up via their import.
	if provider != "" {
		p, ok := providers[provider]
		if !ok {
			return nil, fmt.Errorf("could not find a provider matching import %s", provider)
		}

		return p, nil
	}

	// Nested deployments
	if resourceType == "Microsoft.Resources/deployments" {
		p, ok := providers[DeploymentProviderImport]
		if !ok {
			return nil, errors.New("could not find a provider supporting nested deployments")
		}

		return p, nil
	}

	// We use a heuristic for now until symbolic name support fully lands.
	if strings.HasPrefix(resourceType, "kubernetes.") {
		p, ok := providers[KubernetesProviderImport]
		if !ok {
			return nil, errors.New("could not find a provider supporting Kubernetes")
		}

		return p, nil
	}

	// Radius types (allows us to redirect the Radius provider at a different BaseURL)
	if strings.HasPrefix(resourceType, "Microsoft.CustomProviders/resourceProviders") {
		p, ok := providers[RadiusProviderImport]
		if !ok {
			return nil, errors.New("could not find a provider supporting Radius")
		}

		return p, nil
	}

	p, ok := providers[AzureProviderImport]
	if !ok {
		return nil, errors.New("could not find a provider that supports Azure")
	}

	return p, nil
}
