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

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/health/handlers"
	"github.com/project-radius/radius/pkg/health/model"
	"github.com/project-radius/radius/pkg/health/model/azure"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"

	k8s "github.com/project-radius/radius/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	fake "k8s.io/client-go/kubernetes/fake"
)

var applicationName = "testApplication"
var resourceName = "testResource"
var objectMeta = metav1.ObjectMeta{
	Name:      resourceName,
	Namespace: applicationName,
}
var deployment = appsv1.Deployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      k8s.MakeResourceName(applicationName, resourceName),
		Namespace: applicationName,
		Labels:    k8s.MakeDescriptiveLabels(applicationName, resourceName),
	},
	Spec: appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: k8s.MakeSelectorLabels(applicationName, resourceName),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{},
			Spec:       corev1.PodSpec{},
		},
	},
}

func getKubernetesClient() kubernetes.Interface {
	return fake.NewSimpleClientset()
}

func Test_RegisterResourceCausesResourceToBeMonitored(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	wg := sync.WaitGroup{}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthModel:                 azure.NewAzureHealthModel(nil, getKubernetesClient(), &wg),
	}
	monitor := NewMonitor(options)
	ctx := logr.NewContext(context.Background(), logger)

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		Resource: healthcontract.HealthResource{
			RadiusResourceID: "abc",
			Identity:         resourcemodel.NewKubernetesIdentity(&deployment, objectMeta),
			ResourceKind:     resourcekinds.Deployment,
		},
	}

	// Wait till the waitgroup is done
	stopCh := make(chan struct{})
	t.Cleanup(func() {
		stopCh <- struct{}{}
		wg.Wait()
	})

	registration := monitor.RegisterResource(ctx, registrationMsg, stopCh)

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
	require.Equal(t, resourcemodel.NewKubernetesIdentity(&deployment, objectMeta), healthInfo.Registration.Identity)
	require.Equal(t, resourcekinds.Deployment, healthInfo.Registration.ResourceKind)
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
		HealthModel:                 azure.NewAzureHealthModel(nil, getKubernetesClient(), &sync.WaitGroup{}),
	}
	monitor := NewMonitor(options)
	ctx := logr.NewContext(context.Background(), logger)
	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		Resource: healthcontract.HealthResource{
			RadiusResourceID: "abc",
			Identity:         resourcemodel.NewKubernetesIdentity(&deployment, objectMeta),
			ResourceKind:     "NotImplementedType",
		},
	}
	monitor.RegisterResource(ctx, registrationMsg, make(chan struct{}, 1))
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
		HealthModel:                 azure.NewAzureHealthModel(nil, getKubernetesClient(), &sync.WaitGroup{}),
	}
	monitor := NewMonitor(options)
	stopCh := make(chan struct{}, 1)
	resource := healthcontract.HealthResource{
		RadiusResourceID: "abc",
		Identity:         resourcemodel.NewKubernetesIdentity(&deployment, objectMeta),
		ResourceKind:     resourcekinds.Deployment,
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
			Identity:         resourcemodel.NewKubernetesIdentity(&deployment, objectMeta),
			ResourceKind:     resourcekinds.Deployment,
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
	wg := sync.WaitGroup{}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hrpc,
		HealthModel:                 azure.NewAzureHealthModel(&armauth.ArmConfig{SubscriptionID: uuid.NewString()}, getKubernetesClient(), &wg),
	}
	monitor := NewMonitor(options)
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
	t.Cleanup(func() {
		stopCh <- struct{}{}
		wg.Wait()
	})

	ctx := logr.NewContext(context.Background(), logger)
	registration := monitor.RegisterResource(ctx, registrationMsg, stopCh)

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
	wg := sync.WaitGroup{}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel: model.NewHealthModel(map[string]handlers.HealthHandler{
			"dummy": mockHandler,
		}, &wg),
	}
	monitor := NewMonitor(options)
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
	wg := sync.WaitGroup{}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel: model.NewHealthModel(map[string]handlers.HealthHandler{
			"dummy": mockHandler,
		}, &wg),
	}
	monitor := NewMonitor(options)
	ctx := logr.NewContext(context.Background(), logger)
	resource := healthcontract.HealthResource{
		RadiusResourceID: "abc",
		Identity:         resourcemodel.NewKubernetesIdentity(&deployment, objectMeta),
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
	t.Cleanup(func() {
		stopCh <- struct{}{}
		wg.Wait()
	})
	monitor.RegisterResource(ctx, registrationMsg, stopCh)

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
	wg := sync.WaitGroup{}
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hpc,
		HealthModel: model.NewHealthModel(map[string]handlers.HealthHandler{
			"dummy": mockHandler,
		}, &wg),
	}
	monitor := NewMonitor(options)
	ctx := logr.NewContext(context.Background(), logger)
	resource := healthcontract.HealthResource{
		RadiusResourceID: "abc",
		Identity:         resourcemodel.NewKubernetesIdentity(&deployment, objectMeta),
		ResourceKind:     "dummy",
	}

	// Wait till the waitgroup is done
	stopCh := make(chan struct{})
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

	monitor.RegisterResource(ctx, registrationMsg, stopCh)

	// Wait till forced health state change notification is received
	notification := <-hpc
	require.Equal(t, "Unknown", notification.HealthState)
	require.Equal(t, "", notification.HealthStateErrorDetails)
}

func Test_NoAzureCredentials_RegisterAzureResourceReturnsNoHandler(t *testing.T) {
	logger, err := radlogger.NewTestLogger(t)
	require.NoError(t, err)

	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage)
	hrpc := make(chan healthcontract.ResourceHealthDataMessage, 1)
	options := MonitorOptions{
		Logger:                      logger,
		ResourceRegistrationChannel: rrc,
		HealthProbeChannel:          hrpc,
		HealthModel:                 azure.NewAzureHealthModel(nil, getKubernetesClient(), &sync.WaitGroup{}),
	}
	monitor := NewMonitor(options)
	ctx := logr.NewContext(context.Background(), logger)

	registrationMsg := healthcontract.ResourceHealthRegistrationMessage{
		Action: healthcontract.ActionRegister,
		Resource: healthcontract.HealthResource{
			RadiusResourceID: "abc",
			Identity:         resourcemodel.NewARMIdentity("xyz", "2020-01-01"),
			ResourceKind:     resourcekinds.AzureServiceBusQueue,
		},
	}

	registration := monitor.RegisterResource(ctx, registrationMsg, make(chan struct{}))
	monitor.activeHealthProbesMutex.RLock()
	defer monitor.activeHealthProbesMutex.RUnlock()
	require.Equal(t, 0, len(monitor.activeHealthProbes))
	notification := <-hrpc
	require.Equal(t, resourcekinds.AzureServiceBusQueue, notification.Resource.ResourceKind)
	require.Equal(t, healthcontract.HealthStateNotSupported, notification.HealthState)
	require.Nil(t, registration)
}
