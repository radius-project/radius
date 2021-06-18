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
			RadiusApplicationName = "radius-frontend-backend"
			ApplicationName       = "frontend-backend"
			FrontendComponentName = "frontend"
			BackendComponentName  = "backend"
			Namespace             = "default"
			JobName               = "test-job"
			KindName              = "radius.dev/Container@v1alpha1"
			attempts              = 40
		)
		ctx := context.Background()

		hierarchy := []string{RadiusApplicationName, FrontendComponentName}

		// Testing applications
		application := &v1alpha1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "applications.radius.dev/v1alpha1",
				Kind:       "Application",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      RadiusApplicationName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"radius.dev/applications": ApplicationName,
				},
			},
			Spec: radiusv1alpha1.ApplicationSpec{
				Hierarchy: hierarchy,
			},
		}

		err = k8sClient.Create(ctx, application)
		require.NoError(t, err, "failed to create application")

		applicationLookupKey := types.NamespacedName{Name: RadiusApplicationName, Namespace: Namespace}
		createdApplication := &v1alpha1.Application{}

		GetK8sObject(t, ctx, k8sClient, applicationLookupKey, createdApplication)

		require.Equal(t, ApplicationName, createdApplication.Annotations["radius.dev/applications"])
		require.Equal(t, RadiusApplicationName, createdApplication.Name)
		require.Equal(t, hierarchy, createdApplication.Spec.Hierarchy)

		bindings := map[string]components.GenericBinding{
			"default": {
				Kind: "http",
			},
		}

		bindingJson, _ := json.Marshal(bindings)

		frontendImg := map[string]interface{}{
			"image": "rynowak/frontend:0.5.0-dev",
		}

		frontendRun := map[string]interface{}{
			"container": frontendImg,
		}

		frontendRunJson, _ := json.Marshal(frontendRun)

		usesFrontend := components.GenericDependency{
			Binding: components.BindingExpression{
				Kind: "component",
				Value: &components.ComponentBindingValue{
					Application: ApplicationName,
					Component:   FrontendComponentName,
					Binding:     "default",
				},
			},
			Env: map[string]components.BindingExpression{
				"SERVICE__BACKEND__HOST": {
					Kind: "component",
					Value: &components.ComponentBindingValue{
						Application: ApplicationName,
						Component:   FrontendComponentName,
						Binding:     "default",
						Property:    "host",
					},
				},
				"SERVICE__BACKEND__PORT": {
					Kind: "component",
					Value: &components.ComponentBindingValue{
						Application: ApplicationName,
						Component:   FrontendComponentName,
						Binding:     "default",
						Property:    "port",
					},
				},
			},
		}

		usesFrontendJson, _ := json.Marshal(usesFrontend)

		usesArray := []runtime.RawExtension{
			{
				Raw: usesFrontendJson,
			},
		}

		frontendComponent := &v1alpha1.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "applications.radius.dev/v1alpha1",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      FrontendComponentName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"radius.dev/applications": ApplicationName,
					"radius.dev/components":   FrontendComponentName,
				},
			},
			Spec: v1alpha1.ComponentSpec{
				Kind:      KindName,
				Run:       &runtime.RawExtension{Raw: frontendRunJson},
				Bindings:  runtime.RawExtension{Raw: bindingJson},
				Hierarchy: hierarchy,
				Uses:      &usesArray,
			},
		}

		err = k8sClient.Create(ctx, frontendComponent)
		require.NoError(t, err, "failed to create component")

		frontendComponentLookupKey := types.NamespacedName{Name: FrontendComponentName, Namespace: Namespace}
		createdFrontendComponent := &v1alpha1.Component{}

		GetK8sObject(t, ctx, k8sClient, frontendComponentLookupKey, createdFrontendComponent)

		runActual, _ := createdFrontendComponent.Spec.Run.MarshalJSON()
		require.Equal(t, ApplicationName, createdFrontendComponent.Annotations["radius.dev/applications"])
		require.Equal(t, FrontendComponentName, createdFrontendComponent.Annotations["radius.dev/components"])
		require.Equal(t, KindName, createdFrontendComponent.Spec.Kind)
		require.Equal(t, frontendRunJson, runActual)
		require.Equal(t, hierarchy, createdFrontendComponent.Spec.Hierarchy)

		backendImg := map[string]interface{}{
			"image": "rynowak/backend:0.5.0-dev",
		}

		backendRun := map[string]interface{}{
			"container": backendImg,
		}

		backendRunJson, _ := json.Marshal(backendRun)

		backendComponent := &v1alpha1.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "applications.radius.dev/v1alpha1",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      BackendComponentName,
				Namespace: Namespace,
				Annotations: map[string]string{
					"radius.dev/applications": ApplicationName,
					"radius.dev/components":   BackendComponentName,
				},
			},
			Spec: v1alpha1.ComponentSpec{
				Kind:      KindName,
				Run:       &runtime.RawExtension{Raw: backendRunJson},
				Bindings:  runtime.RawExtension{Raw: bindingJson},
				Hierarchy: hierarchy,
			},
		}

		err = k8sClient.Create(ctx, backendComponent)
		require.NoError(t, err, "failed to create component")

		backendComponentLookupKey := types.NamespacedName{Name: BackendComponentName, Namespace: Namespace}
		createdBackendComponent := &v1alpha1.Component{}

		GetK8sObject(t, ctx, k8sClient, backendComponentLookupKey, createdBackendComponent)

		backendRunActual, _ := createdBackendComponent.Spec.Run.MarshalJSON()
		require.Equal(t, ApplicationName, createdBackendComponent.Annotations["radius.dev/applications"])
		require.Equal(t, BackendComponentName, createdBackendComponent.Annotations["radius.dev/components"])
		require.Equal(t, KindName, createdBackendComponent.Spec.Kind)
		require.Equal(t, backendRunJson, backendRunActual)
		require.Equal(t, hierarchy, createdBackendComponent.Spec.Hierarchy)

		// Test Deployments
		deployments := &appsv1.DeploymentList{}
		for i := 0; ; i++ {
			err = k8sClient.List(ctx, deployments, client.InNamespace(frontendComponent.Namespace))

			if len(deployments.Items) == 2 {
				for _, dep := range deployments.Items {
					if dep.Name != FrontendComponentName && dep.Name != BackendComponentName {
						require.Fail(t, "found unrecongnized deployment")
					}
				}
				break
			}

			if i >= attempts {
				require.Fail(t, "could not get deployment from k8s")
			}
			time.Sleep(time.Second)
		}

		services := &corev1.ServiceList{}
		for i := 0; ; i++ {
			err = k8sClient.List(ctx, services, client.InNamespace(frontendComponent.Namespace))

			if len(services.Items) == 3 {
				for _, serv := range services.Items {
					if serv.Name != FrontendComponentName && serv.Name != BackendComponentName && serv.Name != "kubernetes" {
						require.Fail(t, "found unrecongnized service")
					}
				}
				break
			}

			if i >= attempts {
				require.Fail(t, "could not get services from k8s")
			}
			time.Sleep(time.Second)
		}
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
