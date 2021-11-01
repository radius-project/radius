// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceprovider

import (
	"fmt"

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

func NewRestRadiusResourceStatus(resourceName string, original db.RadiusResourceStatus) RadiusResourceStatus {
	ors := NewRestOutputResourceStatus(original.OutputResources)

	// Aggregate the resource status
	healthState := healthcontract.HealthStateHealthy
	healthStateErrorDetails := ""
	foundNotSupported := false
	foundHealthyOrUnhealthy := false
	for i, or := range ors {
		userHealthState := healthcontract.InternalToUserHealthStateTranslation[or.Status.HealthState]
		switch or.Status.HealthState {
		case healthcontract.HealthStateNotSupported:
			foundNotSupported = true
			// Show output resource state NotSupported as "" to the users for individual output resources
			// as well as the aggregated status
			ors[i].Status.HealthState = userHealthState
			// Modify the aggregated status
			healthState = userHealthState
		case healthcontract.HealthStateNotApplicable:
			// Show output resource state NotApplicable as Healthy to the users
			// A resource with state = NotApplicable has no effect on the aggregate state
			ors[i].Status.HealthState = userHealthState
		case healthcontract.HealthStateHealthy:
			foundHealthyOrUnhealthy = true
			// No need to modify the aggregate state as it is already Healthy
		case healthcontract.HealthStateUnhealthy:
			foundHealthyOrUnhealthy = true
			// If any of the output resources is not healthy, modify the aggregate health state as unhealthy
			healthState = userHealthState
		case healthcontract.HealthStateUnknown:
			healthState = userHealthState
			healthStateErrorDetails = "Health state unknown"
		default:
			// Unexpected state
			healthState = healthcontract.InternalToUserHealthStateTranslation[healthcontract.HealthStateUnhealthy]
			healthStateErrorDetails = fmt.Sprintf("output resource found in unexpected state: %s", or.Status.HealthState)
		}
	}

	if foundNotSupported && foundHealthyOrUnhealthy {
		// We do not expect a combination of not supported and supported health reporting for output resources
		// This will result in an aggregation logic error
		healthState = healthcontract.InternalToUserHealthStateTranslation[healthcontract.HealthStateError]
		healthStateErrorDetails = fmt.Sprintf("radius resource: %q has a combination of supported and unsupported health reporting for its output resources. Health aggregation error", resourceName)
	}

	provisioningState := rest.Provisioned

forLoop:
	for _, or := range ors {
		// If any of the output resources is not in Provisioned state, mark the resource accordingly
		switch or.Status.ProvisioningState {
		case db.Failed:
			provisioningState = rest.Failed
			break forLoop
		case db.Provisioning, db.NotProvisioned:
			provisioningState = rest.Provisioning
		}
	}

	status := RadiusResourceStatus{
		ProvisioningState:  provisioningState,
		HealthState:        healthState,
		HealthErrorDetails: healthStateErrorDetails,
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
