// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package healthlistener

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
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

type Service struct {
	Options ServiceOptions
	db      db.RadrpDB
}

func NewService(options ServiceOptions) *Service {
	return &Service{
		Options: options,
	}
}

func (s *Service) Name() string {
	return "backend.health-listener"
}

func (s *Service) Run(ctx context.Context) error {
	logger := radlogger.GetLogger(ctx)

	dbclient, err := s.Options.DBClientFactory(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	s.db = db.NewRadrpDB(dbclient)

	logger.Info("Started listening for Health State change notifications from HealthService...")
	for {
		select {
		case msg := <-s.Options.HealthChannels.HealthToRPNotificationChannel:
			updated := s.UpdateHealth(ctx, msg)
			logger.Info(fmt.Sprintf("Updated application health state changes successfully: %v", updated))
		case <-ctx.Done():
			logger.Info("Stopping to listen for health state change notifications")
			return nil
		}
	}
}

func (s *Service) UpdateHealth(ctx context.Context, healthUpdateMsg healthcontract.ResourceHealthDataMessage) bool {
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
	c, err := s.db.GetComponentByApplicationID(ctx, a, cid.Name())
	if err != nil {
		logger.Error(err, "Component not found")
		return false
	}

	for i, o := range c.Properties.Status.OutputResources {
		if o.HealthID == healthUpdateMsg.Resource.HealthID {
			// Update the health state
			c.Properties.Status.OutputResources[i].Status.HealthState = healthUpdateMsg.HealthState
			c.Properties.Status.OutputResources[i].Status.HealthStateErrorDetails = healthUpdateMsg.HealthStateErrorDetails
			patched, err := s.db.PatchComponentByApplicationID(ctx, a, c.Name, c)
			if err == db.ErrNotFound {
				logger.Error(err, "Component not found in DB")
			} else if err != nil {
				logger.Error(err, "Unable to update Health state in DB")
			}

			return patched
		}
	}

	logger.Error(db.ErrNotFound, "No output resource found with matching health ID")
	return false
}
