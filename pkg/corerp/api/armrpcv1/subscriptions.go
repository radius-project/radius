// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armrpcv1

const (
	ArmApiVersion = "2"
)

// Subscriptions data model
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md
type Subscription struct {
	State            string                  `json:"state"`
	RegistrationDate string                  `json:"registrationDate"`
	Properties       *SubscriptionProperties `json:"properties"`
}

type SubscriptionProperties struct {
	TenantId                  string                            `json:"tenantId,omitempty"`
	LocationPlacementId       string                            `json:"locationPlacementId,omitempty"`
	QuotaId                   string                            `json:"quotaId,omitempty"`
	RegisteredFeatures        []*SubscriptionRegisteredFeatures `json:"registeredFeatures,omitempty"`
	AvailabilityZones         *SubscriptionAvailabilityZones    `json:"availabilityZones,omitempty"`
	SubscriptionSpendingLimit string                            `json:"subscriptionSpendingLimit,omitempty"`
	SubscriptionAccountOwner  string                            `json:"subscriptionAccountOwner,omitempty"`
	ManagedByTenants          []*SubscriptionManagedTenants     `json:"managedByTenants,omitempty"`
	AdditionalProperties      map[string]string                 `json:"additionalProperties"`
}

type SubscriptionRegisteredFeatures struct {
	Name  string `json:"name,omitempty"`
	State string `json:"state,omitempty"`
}

type SubscriptionAvailabilityZones struct {
	Location     string                      `json:"location,omitempty"`
	ZoneMappings []*SubscriptionZoneMappings `json:"zoneMappings,omitempty"`
}

type SubscriptionZoneMappings struct {
	LogicalZone  string `json:"logicalZone,omitempty"`
	PhysicalZone string `json:"physicalZone,omitempty"`
}

type SubscriptionManagedTenants struct {
	TenantId string `json:"tenantId,omitempty"`
}
