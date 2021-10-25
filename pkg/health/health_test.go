// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/health/model/azure"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"

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
		Resource: healthcontract.HealthResource{
			RadiusResourceID: "abc",
			Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
			ResourceKind:     resourcekinds.AzureServiceBusQueue,
		},
	}

	// Wait till the waitgroup is done
	stopCh := make(chan struct{})
	wg := sync.WaitGroup{}
	t.Cleanup(func() {
		stopCh <- struct{}{}
		wg.Wait()
	})

	registration := monitor.RegisterResource(ctx, registrationMsg, stopCh, &wg)

	monitor.activeHealthProbesMutex.RLock()
	probesLen := len(monitor.activeHealthProbes)
	healthInfo, found := monitor.activeHealthProbes[registration.Token]
	monitor.activeHealthProbesMutex.RUnlock()
	require.Equal(t, 1, probesLen)
	require.Equal(t, true, found)

	handler, mode := monitor.model.LookupHandler(ctx, registrationMsg)
	require.Equal(t, handler, healthInfo.handler)
	require.Equal(t, handlers.HealthHandlerModePull, mode)
	require.Equal(t, *registration, healthInfo.Registration)
	require.Equal(t, "abc", healthInfo.Registration.RadiusResourceID)
	require.Equal(t, resourcemodel.NewARMIdentity("xyz", "2020-01-01"), healthInfo.Registration.Identity)
	require.Equal(t, resourcekinds.AzureServiceBusQueue, healthInfo.Registration.ResourceKind)
	require.NotNil(t, healthInfo.ticker)
}

// When a resource kind is not implemented in the health service, it should still be handled with no errors
func Test_RegisterResourceWithResourceKindNotImplemented(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	hrpc := make(chan healthcontract.ResourceHealthDataMessage, 1)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hrpc,
		HealthModel:                 azure.NewAzureHealthModel(armauth.ArmConfig{}, getKubernetesClient()),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		Resource: healthcontract.HealthResource{
			RadiusResourceID: "abc",
			Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
			ResourceKind:     "NotImplementedType",
		},
	}
	monitor.RegisterResource(ctx, registrationMsg, make(chan struct{}, 1), &sync.WaitGroup{})
	monitor.activeHealthProbesMutex.RLock()
	defer monitor.activeHealthProbesMutex.RUnlock()
	require.Equal(t, 0, len(monitor.activeHealthProbes))
	notification := <-hrpc
	require.Equal(t, "NotImplementedType", notification.Resource.ResourceKind)
	require.Equal(t, healthcontract.HealthStateNotSupported, notification.HealthState)
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
	resource := healthcontract.HealthResource{
		RadiusResourceID: "abc",
		Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
		ResourceKind:     resourcekinds.AzureServiceBusQueue,
	}

	registration, err := handlers.NewHealthRegistration(resource)
	require.NoError(t, err)

	monitor.activeHealthProbes[registration.Token] = HealthInfo{
		stopProbeForResource: stopCh,
		ticker:               time.NewTicker(time.Second * 10),
		Registration:         registration,
	}

	unregistrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		Resource: healthcontract.HealthResource{
			RadiusResourceID: "abc",
			Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
			ResourceKind:     resourcekinds.AzureServiceBusQueue,
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
	hrpc := make(chan healthcontract.ResourceHealthDataMessage, 1)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hrpc,
		HealthModel:                 azure.NewAzureHealthModel(armauth.ArmConfig{}, getKubernetesClient()),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	optionsInterval := time.Microsecond * 5
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		Resource: healthcontract.HealthResource{
			RadiusResourceID: "abc",
			Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
			ResourceKind:     resourcekinds.AzureServiceBusQueue,
		},
		Options: healthcontract.HealthCheckOptions{
			Interval: optionsInterval,
		},
	}

	// Wait till the waitgroup is done
	stopCh := make(chan struct{})
	wg := sync.WaitGroup{}
	t.Cleanup(func() {
		stopCh <- struct{}{}
		wg.Wait()
	})

	ctx := logr.NewContext(context.Background(), logger)
	registration := monitor.RegisterResource(ctx, registrationMsg, stopCh, &wg)

	monitor.activeHealthProbesMutex.RLock()
	hi := monitor.activeHealthProbes[registration.Token]
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
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel: model.NewHealthModel(map[string]handlers.HealthHandler{
			"dummy": mockHandler,
		}),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	resource := healthcontract.HealthResource{
		RadiusResourceID: "abc",
		Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
		ResourceKind:     "dummy",
	}
	registration, err := handlers.NewHealthRegistration(resource)
	require.NoError(t, err)

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action:   healthcontract.ActionRegister,
		Resource: resource,
		Options: healthcontract.HealthCheckOptions{
			Interval: time.Nanosecond * 1,
		},
	}
	// Wait till the waitgroup is done
	stopCh := make(chan struct{})
	wg := sync.WaitGroup{}
	t.Cleanup(func() {
		stopCh <- struct{}{}
		wg.Wait()
	})

	mockHandler.EXPECT().GetHealthState(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().Return(handlers.HealthState{
		Registration:            registration,
		HealthState:             "Healthy",
		HealthStateErrorDetails: "None",
	})
	monitor.RegisterResource(ctx, registrationMsg, stopCh, &wg)
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
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel: model.NewHealthModel(map[string]handlers.HealthHandler{
			"dummy": mockHandler,
		}),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	resource := healthcontract.HealthResource{
		RadiusResourceID: "abc",
		Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
		ResourceKind:     "dummy",
	}

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action:   healthcontract.ActionRegister,
		Resource: resource,
		Options: healthcontract.HealthCheckOptions{
			Interval: time.Nanosecond * 1,
		},
	}
	registration, err := handlers.NewHealthRegistration(resource)
	require.NoError(t, err)

	mockHandler.EXPECT().GetHealthState(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().Return(handlers.HealthState{
		Registration:            registration,
		HealthState:             "Healthy",
		HealthStateErrorDetails: "None",
	})

	// Wait till the waitgroup is done
	stopCh := make(chan struct{})
	wg := sync.WaitGroup{}
	t.Cleanup(func() {
		stopCh <- struct{}{}
		wg.Wait()
	})
	monitor.RegisterResource(ctx, registrationMsg, stopCh, &wg)

	// Wait till health state change notification is received
	<-hpc

	// Read updated state
	monitor.activeHealthProbesMutex.RLock()
	hi := monitor.activeHealthProbes[registration.Token]
	monitor.activeHealthProbesMutex.RUnlock()

	require.Equal(t, "Healthy", hi.HealthState)
	require.Equal(t, "None", hi.HealthStateErrorDetails)
}

func Test_HealthServiceSendsNotificationsAfterForcedUpdateInterval(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	hpc := make(chan healthcontract.ResourceHealthDataMessage, 1000) // Using a big buffer size here to ensure that periodic update thread is not stuck in blocking on the message
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockHandler := handlers.NewMockHealthHandler(ctrl)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel: model.NewHealthModel(map[string]handlers.HealthHandler{
			"dummy": mockHandler,
		}),
	}
	monitor := NewMonitor(options, armauth.ArmConfig{})
	ctx := logr.NewContext(context.Background(), logger)
	resource := healthcontract.HealthResource{
		RadiusResourceID: "abc",
		Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
		ResourceKind:     "dummy",
	}

	// Wait till the waitgroup is done
	stopCh := make(chan struct{})
	wg := sync.WaitGroup{}
	t.Cleanup(func() {
		stopCh <- struct{}{}
		wg.Wait()
	})

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action:   healthcontract.ActionRegister,
		Resource: resource,
		Options: healthcontract.HealthCheckOptions{
			Interval:             time.Second * 100,   // Making this very big on purpose so that the period ticker is not fired
			ForcedUpdateInterval: time.Nanosecond * 1, // Making this very small on purpose so that the forced update ticker fires immediately
		},
	}

	monitor.RegisterResource(ctx, registrationMsg, stopCh, &wg)

	// Wait till forced health state change notification is received
	notification := <-hpc
	require.Equal(t, "Unknown", notification.HealthState)
	require.Equal(t, "", notification.HealthStateErrorDetails)
}
