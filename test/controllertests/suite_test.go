// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/kubernetes/controllers"
	"github.com/stretchr/testify/require"
	//+kubebuilder:scaffold:imports
)

func TestAPIs(t *testing.T) {

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "deploy", "k8s", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err, "failed to initialize environment")
	require.NotNil(t, cfg, "failed to initialize environment")

	err = radiusv1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err, "could not add scheme")

	//+kubebuilder:scaffold:scheme

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	require.NoError(t, err, "failed to initialize k8s client")
	require.NotNil(t, k8sClient, "failed to initialize k8s client")

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
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

	err = (&controllers.ScopeReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Scope"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	require.NoError(t, err, "failed to initialize scope reconciler")

	err = (&controllers.TemplateReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Template"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	require.NoError(t, err, "failed to initialize template reconciler")

	go func() {
		err = mgr.Start(ctrl.SetupSignalHandler())
		require.NoError(t, err, "failed to start manager")
	}()

	// TODO EXECUTE TESTS HERE
	t.Run("component", func(t *testing.T) {
		const (
			ApplicationName    = "frontend-backend"
			ComponentName      = "test-component"
			ComponentNamespace = "default"
			JobName            = "test-job"
			KindName           = "radius.dev/Container@v1alpha1"
			Name               = "frontend"
			attempts           = 40
			interval           = time.Millisecond * 250
		)
		ctx := context.Background()

		application := &v1alpha1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "applications.radius.dev/v1alpha1",
				Kind:       "Application",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("radius-%s", ApplicationName),
				Namespace: ComponentNamespace,
			},
		}

		bindings := map[string]interface{}{
			"kind": "http",
			"name": "backend",
		}

		bindingJson, _ := json.Marshal(bindings)

		img := map[string]interface{}{
			"image": "rynowak/frontend:0.5.0-dev",
		}

		err = k8sClient.Create(ctx, application)
		require.NoError(t, err, "failed to create application")

		run := map[string]interface{}{}
		run["container"] = img

		runJson, _ := json.Marshal(run)

		component := &v1alpha1.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "applications.radius.dev/v1alpha1",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      ComponentName,
				Namespace: ComponentNamespace,
				Annotations: map[string]string{
					"radius.dev/applications": ApplicationName,
					"radius.dev/components":   ComponentName,
				},
			},
			Spec: v1alpha1.ComponentSpec{
				Kind:     KindName,
				Run:      &runtime.RawExtension{Raw: runJson},
				Bindings: runtime.RawExtension{Raw: bindingJson},
			},
		}

		err = k8sClient.Create(ctx, component)
		require.NoError(t, err, "failed to create component")

		componentLookupKey := types.NamespacedName{Name: ComponentName, Namespace: ComponentNamespace}
		createdComponent := &v1alpha1.Component{}

		for i := 0; ; i++ {
			err := k8sClient.Get(ctx, componentLookupKey, createdComponent)
			if err == nil {
				break
			}
			if i >= attempts {
				require.NoError(t, err, "could not get component from k8s")
			}
			time.Sleep(interval)
		}

		runActual, _ := createdComponent.Spec.Run.MarshalJSON()
		require.Equal(t, ApplicationName, createdComponent.Annotations["radius.dev/applications"])
		require.Equal(t, ComponentName, createdComponent.Annotations["radius.dev/components"])
		require.Equal(t, KindName, createdComponent.Spec.Kind)
		require.Equal(t, runActual, runJson)
	})

	err = testEnv.Stop()
	require.NoError(t, err, "failed to stop test env")
}
