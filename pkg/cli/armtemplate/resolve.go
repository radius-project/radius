// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/armtemplate/providers"
)

func GetProvider(ps map[string]providers.Provider, provider string, version string, resourceType string) (providers.Provider, error) {
	if ps == nil {
		return nil, errors.New("providers map was not initialized")
	}

	// Types that use symbolic references should just be looked up via their import.
	if provider != "" {
		p, ok := ps[provider]
		if !ok {
			return nil, fmt.Errorf("could not find a provider matching import %s", provider)
		}

		return p, nil
	}

	// Nested deployments
	if resourceType == DeploymentResourceType {
		p, ok := ps[providers.DeploymentProviderImport]
		if !ok {
			return nil, errors.New("could not find a provider supporting nested deployments")
		}

		return p, nil
	}

	// We use a heuristic for now until symbolic name support fully lands.
	if strings.HasPrefix(resourceType, "kubernetes.") {
		p, ok := ps[providers.KubernetesProviderImport]
		if !ok {
			return nil, errors.New("could not find a provider supporting Kubernetes")
		}

		return p, nil
	}

	// Radius types (allows us to redirect the Radius provider at a different BaseURL)
	if strings.HasPrefix(resourceType, "Microsoft.CustomProviders/resourceProviders") {
		p, ok := ps[providers.RadiusProviderImport]
		if !ok {
			return nil, errors.New("could not find a provider supporting Radius")
		}

		return p, nil
	}

	p, ok := ps[providers.AzureProviderImport]
	if !ok {
		return nil, errors.New("could not find a provider that supports Azure")
	}

	return p, nil
}
