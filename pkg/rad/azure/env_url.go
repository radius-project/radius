// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"fmt"
)


// GenerateAzureEnvUrl Returns the URL string for an Azure environment based on its subscriptionID and resourceGroup.
// Currently only supports kind=azure. We may generalize this function in the future as necessary. 
func GenerateAzureEnvUrl(subscriptionID string, resourceGroup string) (string, error) { 	
	tenantId := ""
	
	subs, err := LoadSubscriptionsFromProfile() 
	if err != nil {
		return "", err 
	}	
	for _, s := range subs.Subscriptions {
		if s.SubscriptionID == subscriptionID {
			tenantId = s.TenantID
		}
	}
	if tenantId == "" {
		return "Unable to find tenant ID for this subscription.", nil
	}
	
	envUrl := fmt.Sprintf("https://portal.azure.com/#@%s/resource/subscriptions/%s/resourceGroups/%s/overview", 
						tenantId, subscriptionID, resourceGroup)

	return envUrl, nil
}
