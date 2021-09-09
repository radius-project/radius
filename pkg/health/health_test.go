// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/handleroptions"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/health/model/azure"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"

	// k8sclient "github.com/kubernetes-sdk-for-go-101/pkg/client"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	fake "k8s.io/client-go/kubernetes/fake"
)

func getKubernetesClient() kubernetes.Interface {
	return fake.NewSimpleClientset()
}

func Test_RegisterResourceCausesResourceToBeMonitored(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureHealthModel(armauth.ArmConfig{}, getKubernetesClient()),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		ResourceInfo: healthcontract.ResourceInfo{
			HealthID:     "abc",
			ResourceID:   "xyz",
			ResourceKind: resourcekinds.AzureServiceBusQueue,
		},
	}

	// Create an unbuffered channel so that the test can wait till the ticker routine is stopped
	stopCh := make(chan struct{})
	t.Cleanup(func() {
		stopCh <- struct{}{}
	})

	monitor.RegisterResource(ctx, registrationMsg, stopCh)

	monitor.activeHealthProbesMutex.RLock()
	probesLen := len(monitor.activeHealthProbes)
	healthInfo, found := monitor.activeHealthProbes["abc"]
	monitor.activeHealthProbesMutex.RUnlock()

	require.Equal(t, 1, probesLen)
	require.Equal(t, true, found)
	handler, mode := monitor.model.LookupHandler(registrationMsg)
	require.Equal(t, handler, healthInfo.handler)
	require.Equal(t, handleroptions.HealthHandlerModePull, mode)
	require.Equal(t, "abc", healthInfo.Resource.HealthID)
	require.Equal(t, "xyz", healthInfo.Resource.ResourceID)
	require.Equal(t, resourcekinds.AzureServiceBusQueue, healthInfo.Resource.ResourceKind)
	require.NotNil(t, healthInfo.ticker)
}

// When a resource kind is not implemented in the health service, it should still be handled with no errors
func Test_RegisterResourceWithResourceKindNotImplemented(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureHealthModel(armauth.ArmConfig{}, getKubernetesClient()),
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
	monitor.RegisterResource(ctx, registrationMsg, make(chan struct{}, 1))
	monitor.activeHealthProbesMutex.RLock()
	defer monitor.activeHealthProbesMutex.RUnlock()
	require.Equal(t, 0, len(monitor.activeHealthProbes))
}

func Test_UnregisterResourceStopsResourceHealthMonitoring(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureHealthModel(armauth.ArmConfig{}, getKubernetesClient()),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	stopCh := make(chan struct{}, 1)
	resourceInfo := healthcontract.ResourceInfo{
		HealthID:     "abc",
		ResourceID:   "xyz",
		ResourceKind: resourcekinds.AzureServiceBusQueue,
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
			ResourceKind: resourcekinds.AzureServiceBusQueue,
		},
	}
	ctx := logr.NewContext(context.Background(), logger)
	monitor.UnregisterResource(ctx, unregistrationMsg)
	monitor.activeHealthProbesMutex.RLock()
	defer monitor.activeHealthProbesMutex.RUnlock()
	require.Equal(t, 0, len(monitor.activeHealthProbes))
	require.NotZero(t, len(stopCh))
}

func Test_HealthServiceConfiguresSpecifiedHealthOptions(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureHealthModel(armauth.ArmConfig{}, getKubernetesClient()),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	optionsInterval := time.Microsecond * 5
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		ResourceInfo: healthcontract.ResourceInfo{
			HealthID:     "abc",
			ResourceID:   "xyz",
			ResourceKind: resourcekinds.AzureServiceBusQueue,
		},
		Options: healthcontract.HealthCheckOptions{
			Interval: optionsInterval,
		},
	}

	// Create an unbuffered channel so that the test can wait till the ticker routine is stopped
	stopCh := make(chan struct{})
	t.Cleanup(func() {
		stopCh <- struct{}{}
	})

	ctx := logr.NewContext(context.Background(), logger)
	monitor.RegisterResource(ctx, registrationMsg, stopCh)

	monitor.activeHealthProbesMutex.RLock()
	hi := monitor.activeHealthProbes["abc"]
	monitor.activeHealthProbesMutex.RUnlock()
	require.Equal(t, optionsInterval, hi.Options.Interval)
}

func Test_HealthServiceSendsNotificationsOnHealthStateChanges(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	hpc := make(chan healthcontract.ResourceHealthDataMessage)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockHandler := handlers.NewMockHealthHandler(ctrl)
	handlers := map[string]handlers.HealthHandler{
		"dummy": mockHandler,
	}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel:                 model.NewHealthModel(handlers),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	ri := healthcontract.ResourceInfo{
		HealthID:     "abc",
		ResourceID:   "xyz",
		ResourceKind: "dummy",
	}
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action:       healthcontract.ActionRegister,
		ResourceInfo: ri,
		Options: healthcontract.HealthCheckOptions{
			Interval: time.Nanosecond * 1,
		},
	}
	// Create an unbuffered channel so that the test can wait till the ticker routine is stopped
	stopCh := make(chan struct{})
	t.Cleanup(func() {
		stopCh <- struct{}{}
	})

	mockHandler.EXPECT().GetHealthState(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().Return(healthcontract.ResourceHealthDataMessage{
		Resource:                ri,
		HealthState:             "Healthy",
		HealthStateErrorDetails: "None",
	})
	monitor.RegisterResource(ctx, registrationMsg, stopCh)
	// Wait till health state change notification is received
	notification := <-hpc

	require.Equal(t, "Healthy", notification.HealthState)
	require.Equal(t, "None", notification.HealthStateErrorDetails)
}

func Test_HealthServiceUpdatesHealthStateBasedOnGetHealthStateReturnValue(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	hpc := make(chan healthcontract.ResourceHealthDataMessage)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockHandler := handlers.NewMockHealthHandler(ctrl)
	handlers := map[string]handlers.HealthHandler{
		"dummy": mockHandler,
	}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel:                 model.NewHealthModel(handlers),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	ri := healthcontract.ResourceInfo{
		HealthID:     "abc",
		ResourceID:   "xyz",
		ResourceKind: "dummy",
	}

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action:       healthcontract.ActionRegister,
		ResourceInfo: ri,
		Options: healthcontract.HealthCheckOptions{
			Interval: time.Nanosecond * 1,
		},
	}
	mockHandler.EXPECT().GetHealthState(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().Return(healthcontract.ResourceHealthDataMessage{
		Resource:                ri,
		HealthState:             "Healthy",
		HealthStateErrorDetails: "None",
	})

	// Create an unbuffered channel so that the test can wait till the ticker routine is stopped
	stopCh := make(chan struct{})
	t.Cleanup(func() {
		stopCh <- struct{}{}
	})
	monitor.RegisterResource(ctx, registrationMsg, stopCh)

	// Wait till health state change notification is received
	<-hpc

	// Read updated state
	monitor.activeHealthProbesMutex.RLock()
	hi := monitor.activeHealthProbes["abc"]
	monitor.activeHealthProbesMutex.RUnlock()

	require.Equal(t, "Healthy", hi.HealthState)
	require.Equal(t, "None", hi.HealthStateErrorDetails)
}
