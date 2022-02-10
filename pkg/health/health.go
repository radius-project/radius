// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/health/db"
	"github.com/project-radius/radius/pkg/health/handlers"
	"github.com/project-radius/radius/pkg/health/model"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

// ChannelBufferSize defines the buffer size for the Watch channel to receive health state changes from push mode watchers
const ChannelBufferSize = 100

// HealthInfo represents the data maintained per resource being tracked by the HealthService
type HealthInfo struct {
	stopProbeForResource    chan struct{}
	ticker                  *time.Ticker
	forcedUpdateTicker      *time.Ticker // We will start a ticker after which we force state updates to the RP even if there are no changes.
	handler                 handlers.HealthHandler
	HealthState             string
	HealthStateErrorDetails string
	Registration            handlers.HealthRegistration
	Options                 healthcontract.HealthCheckOptions
}

// Monitor is the controller for health checks for all output resources
type Monitor struct {
	db                            db.RadHealthDB
	resourceRegistrationChannel   <-chan healthcontract.ResourceHealthRegistrationMessage
	healthToRPNotificationChannel chan<- healthcontract.ResourceHealthDataMessage
	watchHealthChangesChannel     chan handlers.HealthState
	activeHealthProbes            map[string]HealthInfo
	activeHealthProbesMutex       *sync.RWMutex
	model                         model.HealthModel
}

// Run starts the HealthService
func (h Monitor) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	logger := radlogger.GetLogger(ctx)
	logger.Info("RadHealth Service started...")
	for {
		select {
		case msg := <-h.resourceRegistrationChannel:
			// Received a registration/de-registration message
			if msg.Action == healthcontract.ActionRegister {
				h.RegisterResource(ctx, msg, make(chan struct{}, 1))
			} else if msg.Action == healthcontract.ActionUnregister {
				h.UnregisterResource(ctx, msg)
			}
		case newHealthState := <-h.watchHealthChangesChannel:
			if newHealthState.HealthState != "" {
				logger.Info(fmt.Sprintf("Received a health state change event with state: %s", newHealthState.HealthState), newHealthState.Registration.Identity.AsLogValues()...)
				h.handleStateChanges(ctx, newHealthState)
			}
		case <-ctx.Done():
			logger.Info("RadHealth Service stopped...")
			cancel()
			return nil
		}
	}
}

// RegisterResource is called to register an output resource with the health checker
// It should be called at the time of creation of the output resource
//
// The return value here is for testing purposes.
func (h Monitor) RegisterResource(ctx context.Context, registerMsg healthcontract.ResourceHealthRegistrationMessage, stopCh chan struct{}) *handlers.HealthRegistration {
	wg := h.model.GetWaitGroup()
	ctx = radlogger.WrapLogContext(ctx, registerMsg.Resource.Identity.AsLogValues()...)
	logger := radlogger.GetLogger(ctx)

	logger.Info("Registering resource with health service")

	healthHandler, mode := h.model.LookupHandler(ctx, registerMsg)
	if healthHandler == nil {
		// No health handler was found. Return NotSupported state to distinguish from Unhealthy
		// TODO: Convert this log to error once health checks are implemented for all resource kinds
		// https://github.com/project-radius/radius/issues/827
		kind := registerMsg.Resource.ResourceKind
		if registerMsg.Resource.ResourceKind == resourcekinds.Kubernetes {
			kID := registerMsg.Resource.Identity.Data.(resourcemodel.KubernetesIdentity)
			kind += "-" + kID.Kind
		}
		logger.Info(fmt.Sprintf("ResourceKind: %s does not support health checks. Resource not monitored by HealthService", kind))
		msg := healthcontract.ResourceHealthDataMessage{
			Resource:                registerMsg.Resource,
			HealthState:             healthcontract.HealthStateNotSupported,
			HealthStateErrorDetails: "",
		}
		h.SendHealthStateChangeNotification(ctx, msg)
		return nil
	}

	registration, err := handlers.NewHealthRegistration(registerMsg.Resource)
	if err != nil {
		logger.Error(err, "failed to serialize HealthResource")
		return nil
	}

	ho := healthcontract.HealthCheckOptions{}
	getHealthCheckOptions(&ho, &registerMsg.Options)

	h.activeHealthProbesMutex.RLock()
	_, ok := h.activeHealthProbes[registration.Token]
	h.activeHealthProbesMutex.RUnlock()
	if ok {
		logger.Info(fmt.Sprintf("Resource %+v is already registered with the health service. Ignoring this registration message.", registerMsg.Resource.Identity))
		return nil
	}

	healthInfo := HealthInfo{
		stopProbeForResource: stopCh,
		forcedUpdateTicker:   time.NewTicker(ho.ForcedUpdateInterval),
		handler:              healthHandler,
		HealthState:          healthcontract.HealthStateUnknown,
		Registration:         registration,
		Options:              ho,
	}

	ctx, cancel := context.WithCancel(ctx)
	// Lookup whether the health can be watched or needs to be actively probed
	if mode == handlers.HealthHandlerModePush {
		h.activeHealthProbesMutex.Lock()
		h.activeHealthProbes[healthInfo.Registration.Token] = healthInfo
		h.activeHealthProbesMutex.Unlock()

		options := handlers.Options{
			WatchHealthChangesChannel: h.watchHealthChangesChannel,
			ForcedUpdateInterval:      healthInfo.Options.ForcedUpdateInterval,
		}

		// Watch health state
		go healthHandler.GetHealthState(ctx, healthInfo.Registration, options)
	} else if mode == handlers.HealthHandlerModePull {
		// Need to actively probe the health periodically
		h.probeHealth(ctx, healthHandler, healthInfo, wg)
	}

	// Forced updates if there are no state changes for specified forced update
	go h.forcePeriodicUpdates(ctx, healthInfo, h.getWaitGroup(wg))

	go func(stopProbeForResource <-chan struct{}) {
		for range stopProbeForResource {
			cancel()
			wg.Wait()
			return
		}
	}(stopCh)

	logger.Info("Registered resource with health service successfully")
	return &registration
}

func (h Monitor) getWaitGroup(wg *sync.WaitGroup) *sync.WaitGroup {
	wg.Add(1)
	return wg
}

func (h Monitor) forcePeriodicUpdates(ctx context.Context, healthInfo HealthInfo, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-healthInfo.forcedUpdateTicker.C:
			h.activeHealthProbesMutex.Lock()
			currentHealthInfo := h.activeHealthProbes[healthInfo.Registration.Token]
			h.activeHealthProbesMutex.Unlock()
			message := healthcontract.ResourceHealthDataMessage{
				Resource:                currentHealthInfo.Registration.HealthResource,
				HealthState:             currentHealthInfo.HealthState,
				HealthStateErrorDetails: currentHealthInfo.HealthStateErrorDetails,
			}

			h.SendHealthStateChangeNotification(ctx, message)
		case <-ctx.Done():
			return
		}
	}
}

func (h Monitor) probeHealth(ctx context.Context, healthHandler handlers.HealthHandler, healthInfo HealthInfo, wg *sync.WaitGroup) {
	logger := radlogger.GetLogger(ctx)
	// Create a ticker with a period as specified in the health options by the resource
	// TODO: Optimize and not create a ticker per resource
	healthInfo.ticker = time.NewTicker(healthInfo.Options.Interval)
	h.activeHealthProbesMutex.Lock()
	h.activeHealthProbes[healthInfo.Registration.Token] = healthInfo
	h.activeHealthProbesMutex.Unlock()

	// Create a new health handler for the resource
	go func(ticker *time.Ticker, healthHandler handlers.HealthHandler, wg *sync.WaitGroup) {
		defer wg.Done()
		for {
			select {
			case <-ticker.C:
				logger.Info("Probing health...")
				options := handlers.Options{
					Interval: healthInfo.Options.Interval,
				}
				newHealthState := healthHandler.GetHealthState(ctx, healthInfo.Registration, options)
				h.handleStateChanges(ctx, newHealthState)
			case <-ctx.Done():
				return
			}
		}
	}(healthInfo.ticker, healthInfo.handler, h.getWaitGroup(wg))
}

func (h Monitor) handleStateChanges(ctx context.Context, newHealthState handlers.HealthState) {
	logger := radlogger.GetLogger(ctx).WithValues(newHealthState.Registration.Identity.AsLogValues()...)
	// Save the current health state in memory
	h.activeHealthProbesMutex.RLock()
	currentHealthInfo, ok := h.activeHealthProbes[newHealthState.Registration.Token]
	h.activeHealthProbesMutex.RUnlock()
	if !ok {
		logger.Error(errors.New("NotFound"), "Unable to find active health probe for resource")
		return
	}
	// Send a health notification if the health state has changed or if no notification has been sent for DefaultForceHealthStateUpdateInterval time
	if currentHealthInfo.HealthState != newHealthState.HealthState {
		logger.Info(fmt.Sprintf("HealthState changed from :%s to %s. Sending notification...", currentHealthInfo.HealthState, newHealthState.HealthState))

		// Save the new state as current state
		currentHealthInfo.HealthState = newHealthState.HealthState
		currentHealthInfo.HealthStateErrorDetails = newHealthState.HealthStateErrorDetails
		h.activeHealthProbesMutex.Lock()
		h.activeHealthProbes[newHealthState.Registration.Token] = currentHealthInfo
		h.activeHealthProbesMutex.Unlock()

		message := healthcontract.ResourceHealthDataMessage{
			Resource:                newHealthState.Registration.HealthResource,
			HealthState:             newHealthState.HealthState,
			HealthStateErrorDetails: newHealthState.HealthStateErrorDetails,
		}

		h.SendHealthStateChangeNotification(ctx, message)
		logger.Info(fmt.Sprintf("Health state change notification sent and current health state updated to: %s", newHealthState.HealthState))
	}
}

// UnregisterResource should be called when the output resource is deleted
func (h Monitor) UnregisterResource(ctx context.Context, unregisterMsg healthcontract.ResourceHealthRegistrationMessage) {
	ctx = radlogger.WrapLogContext(ctx, unregisterMsg.Resource.Identity.AsLogValues()...)
	logger := radlogger.GetLogger(ctx)

	logger.Info("Unregistering resource with health service")

	registration, err := handlers.NewHealthRegistration(unregisterMsg.Resource)
	if err != nil {
		logger.Error(err, "failed to serialize HealthResource")
		return
	}

	h.activeHealthProbesMutex.Lock()
	defer h.activeHealthProbesMutex.Unlock()
	healthProbe, ok := h.activeHealthProbes[registration.Token]
	if ok {
		if healthProbe.ticker != nil {
			// The ticker could be nil when the health handler mode is push
			healthProbe.ticker.Stop()
		}
		healthProbe.stopProbeForResource <- struct{}{}
		// Remove entry from active health probe map
		delete(h.activeHealthProbes, registration.Token)
		logger.Info("Unregistered resource with health service successfully")
	} else {
		logger.Info("No active probe found for the resource.")
	}
}

// SendHealthStateChangeNotification sends a health update to the RP whenever the health state for a resource changes
func (h Monitor) SendHealthStateChangeNotification(ctx context.Context, message healthcontract.ResourceHealthDataMessage) {
	logger := radlogger.GetLogger(ctx)
	h.healthToRPNotificationChannel <- message
	logger.Info(fmt.Sprintf("Sent notification for change in health state to new value: %s successfully", message.HealthState))
}

// NewMonitor returns a health probe monitor
func NewMonitor(options MonitorOptions, arm armauth.ArmConfig) Monitor {
	m := Monitor{
		db:                            options.DB,
		resourceRegistrationChannel:   options.ResourceRegistrationChannel,
		healthToRPNotificationChannel: options.HealthProbeChannel,
		watchHealthChangesChannel:     options.WatchHealthChangesChannel,
		model:                         options.HealthModel,
	}
	m.activeHealthProbes = make(map[string]HealthInfo)
	m.activeHealthProbesMutex = &sync.RWMutex{}
	return m
}
