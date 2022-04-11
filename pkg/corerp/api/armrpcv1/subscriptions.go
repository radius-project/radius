// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armrpcv1

// Subscriptions data model
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md
type Subscription struct {
	State         					string          `json:"state"`
	RegistrationDate 				string			`json:"registrationDate"`
	SubscriptionProperties 			*Properties		`json:"properties"`
}

type Properties struct {
	SubscriptionAccountOwner			string					`json:"subscriptionAccountOwner"`
	SubscriptionSpendingLimit			string					`json:"subscriptionSpendingLimit"`
	AdditionalSubscriptionProperties	*AdditionalProperties	`json:"additionalProperties"`
}

type AdditionalProperties struct {
	SubscriptionResourceProviderProperties	*ResourceProviderProperties		`json:"resourceProviderProperties"`
}

type ResourceProviderProperties struct {
	ResourceProviderNamespace	string	`json:"resourceProviderNamespace"`
}