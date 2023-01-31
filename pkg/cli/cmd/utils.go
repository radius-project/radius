// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

// CreateEnvProviders forms the provider scope from the given
func CreateEnvProviders(providersList []any) (corerp.Providers, error) {
	var res corerp.Providers
	for _, provider := range providersList {
		if provider != nil {
			switch p := provider.(type) {
			case azure.Provider:
				if res.Azure != nil {
					return res, &cli.FriendlyError{Message: "Only one azure provider can be configured to a scope"}
				}
				res.Azure = &corerp.ProvidersAzure{
					Scope: to.Ptr("/subscriptions/" + p.SubscriptionID + "/resourceGroups/" + p.ResourceGroup),
				}
			case aws.Provider:
				if res.Aws != nil {
					return res, &cli.FriendlyError{Message: "Only one aws provider can be configured to a scope"}
				}
				res.Aws = &corerp.ProvidersAws{
					Scope: to.Ptr("planes/aws/aws/accounts/" + p.AccountId + "/regions/" + p.TargetRegion),
				}
			}
		}
	}
	return res, nil
}

func GetNamespace(envResource corerp.EnvironmentResource) string {
	switch v := envResource.Properties.Compute.(type) {
	case *corerp.KubernetesCompute:
		return *v.Namespace
	}
	return ""
}
