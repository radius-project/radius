// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package healthlistener

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/db"
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
	logger, err := radlogger.GetLogger(ctx)
	if err != nil {
		return err
	}

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
			logger.Info(fmt.Sprintf("Updated application health state changes with health state: %v successfully: %v", msg.HealthState, updated), msg.Resource.Identity.AsLogValues()...)
		case <-ctx.Done():
			logger.Info("Stopping to listen for health state change notifications")
			return nil
		}
	}
}

func (s *Service) UpdateHealth(ctx context.Context, healthUpdateMsg healthcontract.ResourceHealthDataMessage) bool {
	logger, err := radlogger.GetLogger(ctx)
	if err != nil {
		logger.Error(err, "Error getting logger")
		return false
	}
	logger = logger.WithValues(healthUpdateMsg.Resource.Identity.AsLogValues()...)
	logger.Info(fmt.Sprintf("Received health state change notification from health service. Updating health in DB with state: %s", healthUpdateMsg.HealthState))

	// This is the ID of the Radius Resource that 'owns' the output resource being updated.
	resourceID, err := azresources.Parse(healthUpdateMsg.Resource.RadiusResourceID)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Invalid resource ID: %s", healthUpdateMsg.Resource.RadiusResourceID))
		return false
	}

	return s.updateHealth(ctx, logger, healthUpdateMsg, resourceID)
}

func (s *Service) updateHealth(ctx context.Context, logger logr.Logger, healthUpdateMsg healthcontract.ResourceHealthDataMessage, id azresources.ResourceID) bool {
	resource, err := s.db.GetV3Resource(ctx, id)
	if err == db.ErrNotFound {
		logger.Error(err, "Resource not found in DB")
		return false
	} else if err != nil {
		logger.Error(err, "Unable to update Health state in DB")
		return false
	}

	for i, o := range resource.Status.OutputResources {
		if o.Identity.IsSameResource(healthUpdateMsg.Resource.Identity) {

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
