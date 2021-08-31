// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/resources"
)

// Radius Resource Types
const (
	RadiusRPName             = "radius"
	ApplicationsResourceType = "Applications"
	ComponentsResourceType   = "Components"
)

// ChangeListener implements functionality for listening for health state changes
type ChangeListener interface {
	ListenForChanges(ctx context.Context)
}

type changeListener struct {
	db     db.RadrpDB
	health *healthcontract.HealthChannels
}

// NewChangeListener initializes a listener that listens for change in health state for resources
func NewChangeListener(db db.RadrpDB, health *healthcontract.HealthChannels) ChangeListener {
	return &changeListener{
		db:     db,
		health: health,
	}
}

func (cl *changeListener) ListenForChanges(ctx context.Context) {
	logger := radlogger.GetLogger(ctx)
	logger.Info("Started listening for Health State change notifications from HealthService...")
	for {
		select {
		case msg := <-cl.health.HealthToRPNotificationChannel:
			updated := cl.UpdateHealth(ctx, msg)
			logger.Info(fmt.Sprintf("Updated application health state changes successfully: %v", updated))
		case <-ctx.Done():
			logger.Info("Stopping to listen for health state change notifications")
			return
		}
	}

}

func (cl *changeListener) UpdateHealth(ctx context.Context, healthUpdateMsg healthcontract.ResourceHealthDataMessage) bool {
	logger := radlogger.GetLogger(ctx).WithValues(
		radlogger.LogFieldResourceID, healthUpdateMsg.Resource.ResourceID,
		radlogger.LogFieldHealthID, healthUpdateMsg.Resource.HealthID)
	logger.Info("Received health update message")
	outputResourceDetails := healthcontract.ParseHealthID(healthUpdateMsg.Resource.HealthID)
	id := azresources.MakeID(
		outputResourceDetails.SubscriptionID,
		outputResourceDetails.ResourceGroup,
		azresources.ResourceType{Type: azresources.CustomProvidersResourceProviders, Name: RadiusRPName},
		azresources.ResourceType{Type: ApplicationsResourceType, Name: outputResourceDetails.ApplicationID},
		azresources.ResourceType{Type: ComponentsResourceType, Name: outputResourceDetails.ComponentID})

	resourceID, err := azresources.Parse(id)
	if err != nil {
		logger.Error(err, "Invalid resource ID")
		return false
	}
	cid := resources.ResourceID{ResourceID: resourceID}
	a, err := cid.Application()
	if err != nil {
		logger.Error(err, "Invalid application ID")
		return false
	}
	c, err := cl.db.GetComponentByApplicationID(ctx, a, cid.Name())
	if err != nil {
		logger.Error(err, "Component not found")
		return false
	}

	for i, o := range c.Properties.Status.OutputResources {
		if o.HealthID == healthUpdateMsg.Resource.HealthID {
			// Update the health state
			c.Properties.Status.OutputResources[i].Status.HealthState = healthUpdateMsg.HealthState
			c.Properties.Status.OutputResources[i].Status.HealthStateErrorDetails = healthUpdateMsg.HealthStateErrorDetails
			patched, err := cl.db.PatchComponentByApplicationID(ctx, a, c.Name, c)
			if err == db.ErrNotFound {
				logger.Error(err, "Component not found in DB")
			} else if err != nil {
				logger.Error(err, "Unable to update Health state in DB")
			}

			// temp, err := cl.db.GetComponentByApplicationID(ctx, a, c.Name)
			// if err != nil {
			// 	logger.Error(err, "Component not found in DB")
			// }
			// fmt.Printf("Updated and requeried component: %v", temp)

			return patched
		}
	}

	logger.Error(db.ErrNotFound, "No output resource found with matching health ID")
	return false
}
