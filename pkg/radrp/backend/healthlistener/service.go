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
	"github.com/go-logr/logr"
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

	// This is the ID of the Radius Resource (Component/Scope/Route) that 'owns' the output resource being updated.
	resourceID, err := azresources.Parse(outputResourceDetails.OwnerID)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Invalid resource ID: %s", outputResourceDetails.OwnerID))
		return false
	}

	if resourceID.Types[0].Name == azresources.CustomRPV3Name {
		return s.updateV3Health(ctx, logger, healthUpdateMsg, resourceID)
	} else {
		return s.updateV1Health(ctx, logger, healthUpdateMsg, resourceID)
	}
}

func (s *Service) updateV1Health(ctx context.Context, logger logr.Logger, healthUpdateMsg healthcontract.ResourceHealthDataMessage, id azresources.ResourceID) bool {
	cid := resources.ResourceID{ResourceID: id}
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

func (s *Service) updateV3Health(ctx context.Context, logger logr.Logger, healthUpdateMsg healthcontract.ResourceHealthDataMessage, id azresources.ResourceID) bool {
	resource, err := s.db.GetV3Resource(ctx, id)
	if err == db.ErrNotFound {
		logger.Error(err, "Resource not found in DB")
		return false
	} else if err != nil {
		logger.Error(err, "Unable to update Health state in DB")
		return false
	}

	for i, o := range resource.Status.OutputResources {
		if o.HealthID == healthUpdateMsg.Resource.HealthID {

			// Update the health state
			resource.Status.OutputResources[i].Status.HealthState = healthUpdateMsg.HealthState
			resource.Status.OutputResources[i].Status.HealthStateErrorDetails = healthUpdateMsg.HealthStateErrorDetails

			err := s.db.UpdateV3ResourceStatus(ctx, id, resource)
			if err == db.ErrNotFound {
				logger.Error(err, "Resource not found in DB")
				return false
			} else if err != nil {
				logger.Error(err, "Unable to update Health state in DB")
				return false
			}

			return true
		}
	}

	logger.Error(db.ErrNotFound, "No output resource found with matching health ID")
	return false
}
