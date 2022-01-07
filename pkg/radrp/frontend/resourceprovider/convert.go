// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceprovider

import (
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/db"
	rest "github.com/Azure/radius/pkg/radrp/rest"
)

func NewDBApplicationResource(id azresources.ResourceID, application ApplicationResource) db.ApplicationResource {
	return db.ApplicationResource{
		ID:              id.ID,
		Type:            id.Type(),
		SubscriptionID:  id.SubscriptionID,
		ResourceGroup:   id.ResourceGroup,
		ApplicationName: id.Types[1].Name,
		Tags:            application.Tags,
		Location:        application.Location,

		// NOTE: status is intentionally not set here.
		// This isn't accepted over the wire as an input.
	}
}

func NewDBRadiusResource(id azresources.ResourceID, resource RadiusResource) db.RadiusResource {
	return db.RadiusResource{
		ID:              id.ID,
		Type:            id.Type(),
		SubscriptionID:  id.SubscriptionID,
		ResourceGroup:   id.ResourceGroup,
		ApplicationName: id.Types[1].Name,
		ResourceName:    id.Types[2].Name,
		Definition:      resource.Properties,
		Status:          db.RadiusResourceStatus{},

		// NOTE: status and provisioning state are intentionally not set here.
		// These aren't accepted over the wire as inputs.
	}
}

func NewRestApplicationResource(application db.ApplicationResource) ApplicationResource {
	// Properties are built from a combination of fields we store in the database
	// This allows us to separate the stateful info from the user-supplied definition.
	return ApplicationResource{
		ID:         application.ID,
		Type:       application.Type,
		Name:       application.ApplicationName,
		Tags:       application.Tags,
		Location:   application.Location,
		Properties: map[string]interface{}{},
	}
}

func NewRestRadiusResource(resource db.RadiusResource) RadiusResource {
	// Properties are built from a combination of fields we store in the database
	// This allows us to separate the stateful info from the user-supplied definition.
	properties := map[string]interface{}{}

	// We're copying things in order of priority even though we don't expect conflicts.
	properties["status"] = NewRestRadiusResourceStatus(resource.ResourceName, resource.Status)
	if resource.Definition != nil {
		for k, v := range resource.Definition {
			properties[k] = v
		}
	}
	if resource.ComputedValues != nil {
		for k, v := range resource.ComputedValues {
			properties[k] = v
		}
	}
	properties["provisioningState"] = resource.ProvisioningState

	return RadiusResource{
		ID:         resource.ID,
		Type:       resource.Type,
		Name:       resource.ResourceName,
		Properties: properties,
	}
}

func NewRestRadiusResourceFromAzureResource(azResource db.AzureResource) RadiusResource {
	// Currently properties for Azure resources is empty, once we implement health check or decide to store additional
	// metadata about azure resources, it should be included as a part of the properties field.
	return RadiusResource{
		ID:         azResource.ID,
		Type:       azResource.Type,
		Name:       azResource.ResourceName,
		Properties: map[string]interface{}{},
	}
}

func NewRestRadiusResourceStatus(resourceName string, original db.RadiusResourceStatus) RadiusResourceStatus {
	ors := NewRestOutputResourceStatus(original.OutputResources)

	aggregateHealthState, aggregateHealthStateErrorDetails := rest.GetUserFacingResourceHealthState(ors)
	aggregateProvisioningState := rest.GetUserFacingResourceProvisioningState(ors)

	status := RadiusResourceStatus{
		ProvisioningState:  aggregateProvisioningState,
		HealthState:        aggregateHealthState,
		HealthErrorDetails: aggregateHealthStateErrorDetails,
		OutputResources:    ors,
	}
	return status
}

func NewRestOutputResourceStatus(original []db.OutputResource) []rest.OutputResource {
	rrs := []rest.OutputResource{}
	for _, r := range original {
		rr := rest.OutputResource{
			LocalID:            r.LocalID,
			ResourceKind:       r.ResourceKind,
			OutputResourceInfo: r.Identity,
			OutputResourceType: string(r.Identity.Kind),
			Managed:            r.Managed,
			Status: rest.OutputResourceStatus{
				HealthState:              r.Status.HealthState,
				HealthErrorDetails:       r.Status.HealthStateErrorDetails,
				ProvisioningState:        r.Status.ProvisioningState,
				ProvisioningErrorDetails: r.Status.ProvisioningErrorDetails,
			},
			// Resource includes the body of the resource which would make the REST
			// response too verbose. Hence excluded
		}
		rrs = append(rrs, rr)
	}
	return rrs
}
