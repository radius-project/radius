// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/kubernetes/controllers"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/stretchr/testify/require"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	//+kubebuilder:scaffold:imports
)

func TestAPIs(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "deploy", "k8s", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(radiusv1alpha1.AddToScheme(scheme))

	err := scheme.AddConversionFunc(&radiusv1alpha1.Component{}, &components.GenericComponent{}, controllers.ConvertComponentToInternal)
	require.NoError(t, err, "failed to add conversion func")

	cfg, err := testEnv.Start()
	require.NoError(t, err, "failed to initialize environment")
	require.NotNil(t, cfg, "failed to initialize environment")

	err = radiusv1alpha1.AddToScheme(scheme)
	require.NoError(t, err, "could not add scheme")

	//+kubebuilder:scaffold:scheme

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "failed to initialize k8s client")
	require.NotNil(t, k8sClient, "failed to initialize k8s client")

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	require.NoError(t, err, "failed to initialize manager")

	err = (&controllers.ApplicationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Application"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	require.NoError(t, err, "failed to initialize application reconciler")

	err = (&controllers.ComponentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Component"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	require.NoError(t, err, "failed to initialize component reconciler")

	err = (&controllers.DeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Deployment"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	require.NoError(t, err, "failed to initialize deployment reconciler")

	go func() {
		err = mgr.Start(ctrl.SetupSignalHandler())
		require.NoError(t, err, "failed to start manager")
	}()

	t.Run("component", func(t *testing.T) {
		const (
			ApplicationName = "radius-frontend-backend"
			ComponentName   = "test-component"
			Namespace       = "default"
			JobName         = "test-job"
			KindName        = "radius.dev/Container@v1alpha1"
			attempts        = 40
		)
		ctx := context.Background()

		hierarchy := []string{ApplicationName, ComponentName}

		// Testing applications
		application := &v1alpha1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "applications.radius.dev/v1alpha1",
				Kind:       "Application",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      ApplicationName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"radius.dev/applications": "frontend-backend",
				},
			},
			Spec: radiusv1alpha1.ApplicationSpec{
				Hierarchy: hierarchy,
			},
		}

		err = k8sClient.Create(ctx, application)
		require.NoError(t, err, "failed to create application")

		applicationLookupKey := types.NamespacedName{Name: ApplicationName, Namespace: Namespace}
		createdApplication := &v1alpha1.Application{}

		GetK8sObject(t, ctx, k8sClient, applicationLookupKey, createdApplication)

		require.Equal(t, "frontend-backend", createdApplication.Annotations["radius.dev/applications"])
		require.Equal(t, ApplicationName, createdApplication.Name)
		require.Equal(t, hierarchy, createdApplication.Spec.Hierarchy)

		bindings := map[string]components.GenericBinding{
			"a": {
				Kind: "http",
			},
		}

		bindingJson, _ := json.Marshal(bindings)

		img := map[string]interface{}{
			"image": "rynowak/frontend:0.5.0-dev",
		}

		run := map[string]interface{}{
			"container": img,
		}

		runJson, _ := json.Marshal(run)

		component := &v1alpha1.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "applications.radius.dev/v1alpha1",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      ComponentName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"radius.dev/applications": "frontend-backend",
					"radius.dev/components":   ComponentName,
				},
			},
			Spec: v1alpha1.ComponentSpec{
				Kind:      KindName,
				Run:       &runtime.RawExtension{Raw: runJson},
				Bindings:  runtime.RawExtension{Raw: bindingJson},
				Hierarchy: hierarchy,
				// config
				// uses
				// traits
			},
		}

		err = k8sClient.Create(ctx, component)
		require.NoError(t, err, "failed to create component")

		componentLookupKey := types.NamespacedName{Name: ComponentName, Namespace: Namespace}
		createdComponent := &v1alpha1.Component{}

		GetK8sObject(t, ctx, k8sClient, componentLookupKey, createdComponent)

		runActual, _ := createdComponent.Spec.Run.MarshalJSON()
		require.Equal(t, "frontend-backend", createdComponent.Annotations["radius.dev/applications"])
		require.Equal(t, ComponentName, createdComponent.Annotations["radius.dev/components"])
		require.Equal(t, KindName, createdComponent.Spec.Kind)
		require.Equal(t, runJson, runActual)
		require.Equal(t, hierarchy, createdComponent.Spec.Hierarchy)

		// Test Services

		// deployments := &appsv1.DeploymentList{}
		// err := r.Client.List(ctx, deployments, client.InNamespace(component.Namespace), client.MatchingFields{CacheKeyController: component.Name})
		// if err != nil {
		// 	log.Error(err, "failed to retrieve deployments")
		// 	return nil, err
		// }

		// for _, d := range (*deployments).Items {
		// 	obj := d
		// 	results = append(results, &obj)
		// }

		// services := &corev1.ServiceList{}
		// err = r.Client.List(ctx, services, client.InNamespace(component.Namespace), client.MatchingFields{CacheKeyController: component.Name})
		// if err != nil {
		// 	log.Error(err, "failed to retrieve services")
		// 	return nil, err
		// }

		// for _, s := range (*services).Items {
		// 	obj := s
		// 	results = append(results, &obj)
		// }

		// Test Deployments
		deployments := &appsv1.DeploymentList{}
		for i := 0; ; i++ {
			err = k8sClient.List(ctx, deployments, client.InNamespace(component.Namespace))

			if len(deployments.Items) > 0 {
				break
			}

			if i >= attempts {
				require.NoError(t, err, "could not get deployments from k8s")
			}
			time.Sleep(1000)
		}

		deployment := deployments.Items[0]
		require.Equal(t, ComponentName, deployment.Name)

		services := &corev1.ServiceList{}
		for i := 0; ; i++ {
			err = k8sClient.List(ctx, services, client.InNamespace(component.Namespace))

			if len(services.Items) == 1 {
				break
			}

			if i >= attempts {
				require.NoError(t, err, "could not get services from k8s")
			}
			time.Sleep(1000)
		}

		service := services.Items[0]
		require.Equal(t, ComponentName, service.Name)
	})

	err = testEnv.Stop()
	require.NoError(t, err, "failed to stop test env")
}

func GetK8sObject(t *testing.T, ctx context.Context, k8sClient client.Client, lookupKey types.NamespacedName, createdApplication client.Object) {
	const (
		attempts = 40
		interval = time.Millisecond * 250
	)
	for i := 0; ; i++ {
		err := k8sClient.Get(ctx, lookupKey, createdApplication)
		if err == nil {
			break
		}
		if i >= attempts {
			require.NoError(t, err, "could not get component from k8s")
		}
		time.Sleep(interval)
	}
}
