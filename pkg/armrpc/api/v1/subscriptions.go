/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

const (
	SubscriptionAPIVersion = "2.0"
)

const (
	// Registered means the subscription is entitled to use the namespace
	Registered SubscriptionState = "Registered"
	// Unregistered means the subscription is not entitled to use the namespace
	Unregistered SubscriptionState = "Unregistered"
	// Warned means the subscription has been warned
	Warned SubscriptionState = "Warned"
	// Suspended means the subscription has been suspended from the system
	Suspended SubscriptionState = "Suspended"
	// Deleted means the subscription has been deleted
	Deleted SubscriptionState = "Deleted"
)

const (
	// FeatureRegistered means the feature is registered to a certain subscription
	FeatureRegistered FeatureState = "Registered"
	// FeatureUnregistered means the feature is unregistered to a certain subscription
	FeatureUnregistered FeatureState = "Unregistered"
)

// SubscriptionState represents the state of the subscription
type SubscriptionState string

// FeatureState represents the state of a feature for certain subscription
type FeatureState string

// Subscriptions data model
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md
// Sample json data in ./testdata directory
type Subscription struct {
	State            SubscriptionState       `json:"state"`
	RegistrationDate string                  `json:"registrationDate"`
	Properties       *SubscriptionProperties `json:"properties"`
}

type SubscriptionProperties struct {
	TenantID             string                            `json:"tenantId,omitempty"`
	LocationPlacementID  string                            `json:"locationPlacementId,omitempty"`
	QuotaID              string                            `json:"quotaId,omitempty"`
	AccountOwner         *SubscriptionAccountOwner         `json:"accountOwner,omitempty"`
	RegisteredFeatures   []*SubscriptionRegisteredFeatures `json:"registeredFeatures,omitempty"`
	ManagedByTenants     []*SubscriptionManagedTenants     `json:"managedByTenants,omitempty"`
	AdditionalProperties *SubscriptionAdditionalProperties `json:"additionalProperties"`
}

type SubscriptionAccountOwner struct {
	PUID  string `json:"puid,omitempty"`
	Email string `json:"email,omitempty"`
}

type SubscriptionRegisteredFeatures struct {
	Name  string       `json:"name,omitempty"`
	State FeatureState `json:"state,omitempty"`
}

type SubscriptionZoneMappings struct {
	LogicalZone  string `json:"logicalZone,omitempty"`
	PhysicalZone string `json:"physicalZone,omitempty"`
}

type SubscriptionManagedTenants struct {
	TenantID string `json:"tenantId,omitempty"`
}

type SubscriptionAdditionalProperties struct {
	ResourceProviderProperties string                         `json:"resourceProviderProperties,omitempty"`
	BillingProperties          *SubscriptionbillingProperties `json:"billingProperties,omitempty"`
	Promotions                 []*SubscriptionPromotions      `json:"promotions,omitempty"`
}

type SubscriptionbillingProperties struct {
	ChannelType    string                      `json:"channelType,omitempty"`
	PaymentType    string                      `json:"paymentType,omitempty"`
	WorkloadType   string                      `json:"workloadType,omitempty"`
	BillingType    string                      `json:"billingType,omitempty"`
	Tier           string                      `json:"tier,omitempty"`
	BillingAccount *SubscriptionBillingAccount `json:"billingAccount,omitempty"`
}

type SubscriptionBillingAccount struct {
	ID string `json:"id,omitempty"`
}

type SubscriptionPromotions struct {
	Category    string `json:"category,omitempty"`
	EndDateTime string `json:"endDateTime,omitempty"`
}
