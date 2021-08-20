// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/mocks"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/health/model/azure"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func Test_RegisterResourceCausesResourceToBeMonitored(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return
	}

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureModel(armauth.ArmConfig{}),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		ResourceInfo: healthcontract.ResourceInfo{
			HealthID:     "abc",
			ResourceID:   "xyz",
			ResourceKind: azure.ResourceKindAzureServiceBusQueue,
		},
	}
	t.Cleanup(func() {
		monitor.activeHealthProbes["abc"].stopProbeForResource <- os.Interrupt
	})

	monitor.RegisterResource(ctx, registrationMsg)
	require.Equal(t, 1, len(monitor.activeHealthProbes))
	healthInfo, found := monitor.activeHealthProbes["abc"]
	require.Equal(t, true, found)
	require.Equal(t, monitor.model.LookupHandler(azure.ResourceKindAzureServiceBusQueue), healthInfo.handler)
	require.Equal(t, "abc", healthInfo.Resource.HealthID)
	require.Equal(t, "xyz", healthInfo.Resource.ResourceID)
	require.Equal(t, azure.ResourceKindAzureServiceBusQueue, healthInfo.Resource.ResourceKind)
	require.NotNil(t, healthInfo.ticker)
}

// When a resource kind is not implemented in the health service, it should still be handled with no errors
func Test_RegisterResourceWithResourceKindNotImplemented(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return
	}

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureModel(armauth.ArmConfig{}),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		ResourceInfo: healthcontract.ResourceInfo{
			HealthID:     "abc",
			ResourceID:   "xyz",
			ResourceKind: "NotImplementedType",
		},
	}
	monitor.RegisterResource(ctx, registrationMsg)
	require.Equal(t, 0, len(monitor.activeHealthProbes))
}

func Test_UnregisterResourceStopsResourceHealthMonitoring(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return
	}

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureModel(armauth.ArmConfig{}),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	monitor.activeHealthProbes = make(map[string]HealthInfo)
	stopCh := make(chan os.Signal, 1)
	resourceInfo := healthcontract.ResourceInfo{
		HealthID:     "abc",
		ResourceID:   "xyz",
		ResourceKind: azure.ResourceKindAzureServiceBusQueue,
	}

	monitor.activeHealthProbes["abc"] = HealthInfo{
		stopProbeForResource: stopCh,
		ticker:               time.NewTicker(time.Second * 10),
		Resource:             resourceInfo,
	}

	unregistrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		ResourceInfo: healthcontract.ResourceInfo{
			HealthID:     "abc",
			ResourceID:   "xyz",
			ResourceKind: azure.ResourceKindAzureServiceBusQueue,
		},
	}
	ctx := logr.NewContext(context.Background(), logger)
	monitor.UnregisterResource(ctx, unregistrationMsg)
	require.Equal(t, 0, len(monitor.activeHealthProbes))
	require.NotZero(t, len(stopCh))
}

func Test_HealthServiceConfiguresSpecifiedHealthOptions(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return
	}

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureModel(armauth.ArmConfig{}),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	optionsInterval := time.Microsecond * 5
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		ResourceInfo: healthcontract.ResourceInfo{
			HealthID:     "abc",
			ResourceID:   "xyz",
			ResourceKind: azure.ResourceKindAzureServiceBusQueue,
		},
		Options: healthcontract.HealthCheckOptions{
			Interval: optionsInterval,
		},
	}
	t.Cleanup(func() {
		monitor.activeHealthProbes["abc"].stopProbeForResource <- os.Interrupt
	})
	ctx := logr.NewContext(context.Background(), logger)
	monitor.RegisterResource(ctx, registrationMsg)

	hi := monitor.activeHealthProbes["abc"]
	require.Equal(t, optionsInterval, hi.Options.Interval)
}

func Test_HealthServiceCallsHealthHandlerBasedOnResourceKind(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return
	}

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockHandler := mocks.NewMockHealthHandler(ctrl)
	handlers := map[string]handlers.HealthHandler{
		"dummy": mockHandler,
	}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 model.NewModel(handlers),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	ri := healthcontract.ResourceInfo{
		HealthID:     "abc",
		ResourceID:   "xyz",
		ResourceKind: "dummy",
	}

	t.Cleanup(func() {
		monitor.activeHealthProbes["abc"].stopProbeForResource <- os.Interrupt
	})

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action:       healthcontract.ActionRegister,
		ResourceInfo: ri,
		Options: healthcontract.HealthCheckOptions{
			Interval: time.Nanosecond * 1,
		},
	}
	mockHandler.EXPECT().GetHealthState(gomock.Any(), gomock.Any()).
		AnyTimes().Return(healthcontract.ResourceHealthDataMessage{})

	monitor.RegisterResource(ctx, registrationMsg)
}

func Test_HealthServiceSendsNotificationsOnHealthStateChanges(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return
	}

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	hpc := make(chan healthcontract.ResourceHealthDataMessage)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockHandler := mocks.NewMockHealthHandler(ctrl)
	handlers := map[string]handlers.HealthHandler{
		"dummy": mockHandler,
	}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel:                 model.NewModel(handlers),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	ri := healthcontract.ResourceInfo{
		HealthID:     "abc",
		ResourceID:   "xyz",
		ResourceKind: "dummy",
	}

	t.Cleanup(func() {
		monitor.activeHealthProbes["abc"].stopProbeForResource <- os.Interrupt
	})

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action:       healthcontract.ActionRegister,
		ResourceInfo: ri,
		Options: healthcontract.HealthCheckOptions{
			Interval: time.Nanosecond * 1,
		},
	}
	mockHandler.EXPECT().GetHealthState(gomock.Any(), gomock.Any()).
		AnyTimes().Return(healthcontract.ResourceHealthDataMessage{
		Resource:                ri,
		HealthState:             "Healthy",
		HealthStateErrorDetails: "None",
	})
	monitor.RegisterResource(ctx, registrationMsg)
	// Wait till health state change notification is received
	notification := <-hpc

	require.Equal(t, "Healthy", notification.HealthState)
	require.Equal(t, "None", notification.HealthStateErrorDetails)
}

func Test_HealthServiceUpdatesHealthStateBasedOnGetHealthStateReturnValue(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return
	}

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	hpc := make(chan healthcontract.ResourceHealthDataMessage)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockHandler := mocks.NewMockHealthHandler(ctrl)
	handlers := map[string]handlers.HealthHandler{
		"dummy": mockHandler,
	}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel:                 model.NewModel(handlers),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	ri := healthcontract.ResourceInfo{
		HealthID:     "abc",
		ResourceID:   "xyz",
		ResourceKind: "dummy",
	}
	t.Cleanup(func() {
		monitor.activeHealthProbes["abc"].stopProbeForResource <- os.Interrupt
	})

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action:       healthcontract.ActionRegister,
		ResourceInfo: ri,
		Options: healthcontract.HealthCheckOptions{
			Interval: time.Nanosecond * 1,
		},
	}
	mockHandler.EXPECT().GetHealthState(gomock.Any(), gomock.Any()).
		AnyTimes().Return(healthcontract.ResourceHealthDataMessage{
		Resource:                ri,
		HealthState:             "Healthy",
		HealthStateErrorDetails: "None",
	})
	monitor.RegisterResource(ctx, registrationMsg)

	// Wait till health state change notification is received
	<-hpc

	// Read updated state
	monitor.activeHealthProbesMutex.RLock()
	hi := monitor.activeHealthProbes["abc"]
	monitor.activeHealthProbesMutex.RUnlock()

	require.Equal(t, "Healthy", hi.HealthState)
	require.Equal(t, "None", hi.HealthStateErrorDetails)
}
