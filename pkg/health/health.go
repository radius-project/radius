// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/db"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/health/model/azure"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/go-logr/logr"
	"go.mongodb.org/mongo-driver/mongo"
)

// HealthInfo represents the data maintained per resource being tracked by the HealthService
type HealthInfo struct {
	stopProbeForResource    chan os.Signal
	ticker                  *time.Ticker
	handler                 handlers.HealthHandler
	HealthState             string
	HealthStateErrorDetails string
	Resource                healthcontract.ResourceInfo
	Options                 healthcontract.HealthCheckOptions
}

// Monitor is the controller for health checks for all output resources
type Monitor struct {
	db                            db.RadHealthDB
	resourceRegistrationChannel   <-chan healthcontract.ResourceHealthRegistrationMessage
	healthToRPNotificationChannel chan<- healthcontract.ResourceHealthDataMessage
	activeHealthProbes            map[string]HealthInfo
	activeHealthProbesMutex       *sync.RWMutex
	model                         model.HealthModel
}

// Run starts the HealthService
func (h Monitor) Run(ctx context.Context) {
	logger := radlogger.GetLogger(ctx)
	logger.Info("RadHealth Service started...")
	for {
		select {
		case msg := <-h.resourceRegistrationChannel:
			// Received a registration/de-registration message
			if msg.Action == healthcontract.ActionRegister {
				h.RegisterResource(ctx, msg)
			} else if msg.Action == healthcontract.ActionUnregister {
				h.UnregisterResource(ctx, msg)
			}
		case <-ctx.Done():
			logger.Info("RadHealth Service stopped...")
			return
		}
	}
}

// RegisterResource is called to register an output resource with the health checker
// It should be called at the time of creation of the output resource
func (h Monitor) RegisterResource(ctx context.Context, registerMsg healthcontract.ResourceHealthRegistrationMessage) {
	ctx = radlogger.WrapLogContext(
		ctx,
		radlogger.LogFieldResourceID, registerMsg.ResourceInfo.ResourceID,
		radlogger.LogFieldHealthID, registerMsg.ResourceInfo.HealthID,
		radlogger.LogFieldResourceType, registerMsg.ResourceInfo.ResourceKind)
	logger := radlogger.GetLogger(ctx)

	logger.Info("Registering resource with health service")

	if h.model.LookupHandler(registerMsg.ResourceInfo.ResourceKind) == nil {
		// TODO: Convert this log to error once health checks are implemented for all resource kinds
		logger.Info(fmt.Sprintf("ResourceKind: %s does not support health checks. Resource: %s not monitored by HealthService", registerMsg.ResourceInfo.ResourceKind, registerMsg.ResourceInfo.ResourceID))
		return
	}

	ho := healthcontract.HealthCheckOptions{}
	getHealthCheckOptions(&ho, &registerMsg.Options)

	h.activeHealthProbesMutex.RLock()
	_, ok := h.activeHealthProbes[registerMsg.ResourceInfo.HealthID]
	h.activeHealthProbesMutex.RUnlock()
	if ok {
		logger.Info("Resource is already registered with the health service. Ignoring this registration message.", registerMsg.ResourceInfo.ResourceID)
		return
	}

	healthInfo := HealthInfo{
		stopProbeForResource: make(chan os.Signal, 1),
		// Create a new ticker for the resource which will start the health check at the specified interval
		// TODO: Optimize and not create a ticker per resource
		handler:     h.model.LookupHandler(registerMsg.ResourceInfo.ResourceKind),
		HealthState: healthcontract.HealthStateUnhealthy,
		Resource:    registerMsg.ResourceInfo,
		Options:     ho,
	}
	// Create a ticker with a period as specified in the health options by the resource
	healthInfo.ticker = time.NewTicker(healthInfo.Options.Interval)
	// Create a new health handler for the resource
	h.activeHealthProbesMutex.Lock()
	h.activeHealthProbes[registerMsg.ResourceInfo.HealthID] = healthInfo
	h.activeHealthProbesMutex.Unlock()

	go func(ticker *time.Ticker, healthHandler handlers.HealthHandler, stopProbeForResource chan os.Signal) {
		for {
			select {
			case <-ticker.C:
				logger.Info("Probing health...")
				newHealthInfo := healthHandler.GetHealthState(ctx, registerMsg.ResourceInfo)
				logger.Info(fmt.Sprintf("HealthState: %s, HealthStateErrorDetails: %s", newHealthInfo.HealthState, newHealthInfo.HealthStateErrorDetails))

				// Save the current health state in memory
				h.activeHealthProbesMutex.RLock()
				currentHealthInfo, ok := h.activeHealthProbes[registerMsg.ResourceInfo.HealthID]
				h.activeHealthProbesMutex.RUnlock()
				if !ok {
					logger.Error(errors.New("NotFound"), "Unable to find active health probe for resource")
					continue
				}
				if currentHealthInfo.HealthState != newHealthInfo.HealthState {
					logger.Info(fmt.Sprintf("HealthState changed from :%s to %s. Sending notification...", currentHealthInfo.HealthState, newHealthInfo.HealthState))

					// Save the new state as current state
					currentHealthInfo.HealthState = newHealthInfo.HealthState
					currentHealthInfo.HealthStateErrorDetails = newHealthInfo.HealthStateErrorDetails
					h.activeHealthProbesMutex.Lock()
					h.activeHealthProbes[registerMsg.ResourceInfo.HealthID] = currentHealthInfo
					h.activeHealthProbesMutex.Unlock()

					h.SendHealthStateChangeNotification(ctx, registerMsg.ResourceInfo, newHealthInfo)

					logger.Info(fmt.Sprintf("Health state change notification sent and current health state updated to: %s", newHealthInfo.HealthState))
				}
			case _, ok := <-stopProbeForResource:
				if !ok {
					return
				}
				logger.Info("Health Probe stopped.")
				return
			}
		}
	}(healthInfo.ticker, healthInfo.handler, healthInfo.stopProbeForResource)
	logger.Info("Registered resource with health service successfully")
}

// UnregisterResource should be called when the output resource is deleted
func (h Monitor) UnregisterResource(ctx context.Context, unregisterMsg healthcontract.ResourceHealthRegistrationMessage) {
	ctx = radlogger.WrapLogContext(ctx,
		radlogger.LogFieldHealthID, unregisterMsg.ResourceInfo.HealthID,
		radlogger.LogFieldResourceType, unregisterMsg.ResourceInfo.ResourceKind)
	logger := radlogger.GetLogger(ctx)
	logger.Info("Unregistering resource with health service")
	h.activeHealthProbesMutex.Lock()
	defer h.activeHealthProbesMutex.Unlock()
	healthProbe, ok := h.activeHealthProbes[unregisterMsg.ResourceInfo.HealthID]
	if ok {
		healthProbe.ticker.Stop()
		healthProbe.stopProbeForResource <- os.Interrupt
		// Remove entry from active health probe map
		delete(h.activeHealthProbes, unregisterMsg.ResourceInfo.HealthID)
		logger.Info("Unregistered resource with health service successfully")
	} else {
		logger.Info("No active probe found for the resource.")
	}
}

// GetHealthState returns the in-memory health state for a resource tracked by the health service
func (h Monitor) GetHealthState(ctx context.Context, msg healthcontract.ResourceHealthDataMessage) healthcontract.ResourceHealthDataMessage {
	logger := radlogger.GetLogger(ctx)
	h.activeHealthProbesMutex.RLock()
	healthProbe, ok := h.activeHealthProbes[msg.Resource.HealthID]
	h.activeHealthProbesMutex.RUnlock()
	var healthStatus healthcontract.ResourceHealthDataMessage
	if !ok {
		logger.Error(errors.New("NotFound"), "Unable to find active health probe for resource")
		healthStatus = healthcontract.ResourceHealthDataMessage{
			Resource:                msg.Resource,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: "Resource not tracked by HealthService",
		}
	} else {
		healthStatus = healthcontract.ResourceHealthDataMessage{
			Resource:                msg.Resource,
			HealthState:             healthProbe.HealthState,
			HealthStateErrorDetails: healthProbe.HealthStateErrorDetails,
		}
	}
	return healthStatus
}

// SendHealthStateChangeNotification sends a health update to the RP whenever the health state for a resource changes
func (h Monitor) SendHealthStateChangeNotification(ctx context.Context, resource healthcontract.ResourceInfo, healthData healthcontract.ResourceHealthDataMessage) {
	logger := radlogger.GetLogger(ctx)
	msg := healthcontract.ResourceHealthDataMessage{
		Resource:                resource,
		HealthState:             healthData.HealthState,
		HealthStateErrorDetails: healthData.HealthStateErrorDetails,
	}
	h.healthToRPNotificationChannel <- msg
	logger.Info(fmt.Sprintf("Sent notification for change in health state to new value: %s successfully", healthData.HealthState))
}

// NewMonitor returns a health probe monitor
func NewMonitor(options MonitorOptions, arm armauth.ArmConfig) Monitor {
	m := Monitor{
		db:                            options.DB,
		resourceRegistrationChannel:   options.ResourceRegistrationChannel,
		healthToRPNotificationChannel: options.HealthProbeChannel,
		model:                         options.HealthModel,
	}
	m.activeHealthProbes = make(map[string]HealthInfo)
	m.activeHealthProbesMutex = &sync.RWMutex{}
	return m
}

// StartRadHealth creates and starts the Radius Health Monitor
func StartRadHealth(ctx context.Context, arm armauth.ArmConfig, dbClient *mongo.Client, dbName string, healthChannels healthcontract.HealthChannels) {
	// Create logger to log health events
	logger, flushHealthLogs, err := radlogger.NewLogger(fmt.Sprintf("Health-ARM-%s-%s", arm.SubscriptionID, arm.ResourceGroup))
	if err != nil {
		panic(err)
	}
	logger = logger.WithValues(
		radlogger.LogFieldResourceGroup, arm.ResourceGroup,
		radlogger.LogFieldSubscriptionID, arm.SubscriptionID)

	defer flushHealthLogs()

	// Create a DB to store health events
	db := db.NewRadHealthDB(dbClient.Database(dbName))

	model := azure.NewAzureHealthModel(arm)

	options := MonitorOptions{
		Logger:                      logger,
		DB:                          db,
		ResourceRegistrationChannel: healthChannels.ResourceRegistrationWithHealthChannel,
		HealthProbeChannel:          healthChannels.HealthToRPNotificationChannel,
		HealthModel:                 model,
	}

	ctx = logr.NewContext(ctx, logger)
	healthMonitor := NewMonitor(options, arm)
	healthMonitor.Run(ctx)
}
