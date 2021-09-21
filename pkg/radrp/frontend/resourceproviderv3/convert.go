// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceproviderv3

import (
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/healthcontract"
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
	properties := map[string]interface{}{}

	// We're copying things in order of priority even though we don't expect conflicts.
	properties["status"] = rest.ApplicationStatus{
		ProvisioningState:        application.Status.ProvisioningState,
		ProvisioningErrorDetails: application.Status.ProvisioningErrorDetails,
		HealthState:              application.Status.HealthState,
		HealthErrorDetails:       application.Status.HealthErrorDetails,
	}

	return ApplicationResource{
		ID:         application.ID,
		Type:       application.Type,
		Name:       application.ApplicationName,
		Tags:       application.Tags,
		Location:   application.Location,
		Properties: properties,
	}
}

func NewRestRadiusResource(resource db.RadiusResource) RadiusResource {
	// Properties are built from a combination of fields we store in the database
	// This allows us to separate the stateful info from the user-supplied definition.
	properties := map[string]interface{}{}

	// We're copying things in order of priority even though we don't expect conflicts.
	properties["status"] = NewRestRadiusResourceStatus(resource.Status)
	if resource.Definition != nil {
		for k, v := range resource.Definition {
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

func NewRestRadiusResourceStatus(original db.RadiusResourceStatus) RadiusResourceStatus {
	ors := NewRestOutputResourceStatus(original.OutputResources)

	// Aggregate the component status
	healthState := healthcontract.HealthStateHealthy
	provisioningState := rest.Provisioned
forLoop:
	for _, or := range ors {
		// If any of the output resources is not healthy, mark the component as unhealthy
		if or.Status.HealthState != healthcontract.HealthStateHealthy {
			healthState = healthcontract.HealthStateUnhealthy
		}

		// If any of the output resources is not in Provisioned state, mark the component accordingly
		switch or.Status.ProvisioningState {
		case db.Failed:
			provisioningState = rest.Failed
			break forLoop
		case db.Provisioning, db.NotProvisioned:
			provisioningState = rest.Provisioning
		}
	}
	status := RadiusResourceStatus{
		ProvisioningState: provisioningState,
		HealthState:       healthState,
		OutputResources:   ors,
	}
	return status
}

func NewRestOutputResourceStatus(original []db.OutputResource) []rest.OutputResource {
	rrs := []rest.OutputResource{}
	for _, r := range original {
		rr := rest.OutputResource{
			LocalID:            r.LocalID,
			ResourceKind:       r.ResourceKind,
			OutputResourceInfo: r.OutputResourceInfo,
			OutputResourceType: r.OutputResourceType,
			Managed:            r.Managed,
			HealthID:           r.HealthID,
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
