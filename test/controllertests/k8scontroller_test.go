// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/kubernetes/controllers"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	//+kubebuilder:scaffold:imports
)

func TestK8sController(t *testing.T) {
	ctx, cancel := utils.GetContext(t)
	defer cancel()

	assetsDirectory := os.Getenv("KUBEBUILDER_ASSETS")

	if assetsDirectory == "" {
		// run setup-envtest to get the path to binary assets
		var err error
		assetsDirectory, err = getEnvTestBinaryPath()
		require.NoError(t, err, "failed to call setup-envtest to find path")
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "deploy", "Chart", "crds")},
		ErrorIfCRDPathMissing:    true,
		AttachControlPlaneOutput: true,
		BinaryAssetsDirectory:    assetsDirectory,
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(radiusv1alpha1.AddToScheme(scheme))

	err := scheme.AddConversionFunc(&radiusv1alpha1.Component{}, &components.GenericComponent{}, controllers.ConvertComponentToInternal)
	require.NoError(t, err, "failed to add conversion func")

	cfg, err := testEnv.Start()
	require.NoError(t, err, "failed to initialize environment")
	require.NotNil(t, cfg, "failed to initialize environment")

	defer func() {
		err := testEnv.Stop()
		require.NoError(t, err, "failed to clean up resources")
	}()

	err = radiusv1alpha1.AddToScheme(scheme)
	require.NoError(t, err, "could not add scheme")

	//+kubebuilder:scaffold:scheme

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "failed to initialize k8s client")
	require.NotNil(t, k8sClient, "failed to initialize k8s client")

	k8s, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err, "failed to create kubernetes client")

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

	options := Options{
		Context: ctx,
		Client:  k8sClient,
		K8s:     k8s,
	}

	table := []Row{
		{
			// Testing applications
			Description: "frontend-backend",
			Application: &v1alpha1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "applications.radius.dev/v1alpha1",
					Kind:       "Application",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "radius-frontend-backend",
					Namespace: "default",
					Annotations: map[string]string{
						"radius.dev/applications": "frontend-backend",
					},
				},
				Spec: radiusv1alpha1.ApplicationSpec{
					Hierarchy: []string{"radius", "frontend-backend"},
				},
			},
			Components: &[]TestComponent{
				{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "applications.radius.dev/v1alpha1",
						Kind:       "Component",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "frontend",
						Namespace: "default",
						Annotations: map[string]string{
							"radius.dev/applications": "frontend-backend",
							"radius.dev/components":   "frontend",
						},
					},
					Spec: TestComponentSpec{
						Kind: "radius.dev/Container@v1alpha1",
						Run: map[string]interface{}{
							"container": map[string]interface{}{
								"image": "rynowak/frontend:0.5.0-dev",
							},
						},
						Bindings: map[string]interface{}{
							"default": map[string]interface{}{
								"kind": "http",
							},
						},
						Hierarchy: []string{"radius", "frontend-backend", "frontend"},
						Uses: []map[string]interface{}{
							{
								"binding": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]",
								"env": map[string]interface{}{
									"SERVICE__BACKEND__HOST": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default.host]",
									"SERVICE__BACKEND__PORT": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default.port]",
								},
							},
						},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "applications.radius.dev/v1alpha1",
						Kind:       "Component",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
						Annotations: map[string]string{
							"radius.dev/applications": "frontend-backend",
							"radius.dev/components":   "backend",
						},
					},
					Spec: TestComponentSpec{
						Kind: "radius.dev/Container@v1alpha1",
						Run: map[string]interface{}{
							"container": map[string]interface{}{
								"image": "rynowak/backend:0.5.0-dev",
							},
						},
						Bindings: map[string]interface{}{
							"default": map[string]interface{}{
								"kind": "http",
							},
						},
						Hierarchy: []string{"radius", "frontend-backend", "backend"},
					},
				},
			},
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForComponent("frontend-backend", "frontend"),
						validation.NewK8sObjectForComponent("frontend-backend", "backend"),
					},
				},
			},
		},
	}

	// Nest parallel subtests into outer Run to have function wait for all tests
	// to finish before returning.
	// See: https://golang.org/pkg/testing/#hdr-Subtests_and_Sub_benchmarks
	t.Run("deploytests", func(t *testing.T) {
		for _, row := range table {
			test := NewControllerTest(options, row)
			t.Run(row.Description, test.Test)
		}
	})
}

type Row struct {
	Application *radiusv1alpha1.Application
	Components  *[]TestComponent
	Description string
	Pods        validation.K8sObjectSet
}

func (r Row) GetComponents() (*[]radiusv1alpha1.Component, error) {
	var components []radiusv1alpha1.Component

	for _, testComponent := range *r.Components {
		component, err := testComponent.GetComponent()
		if err != nil {
			return nil, err
		}
		components = append(components, component)
	}

	return &components, nil
}

// A test only representation of a component, making it easier
// to write input for (don't need to muck with RawExtension for json)
type TestComponent struct {
	TypeMeta   metav1.TypeMeta
	ObjectMeta metav1.ObjectMeta
	Spec       TestComponentSpec
}

type TestComponentSpec struct {
	Kind      string
	Hierarchy []string
	Run       map[string]interface{}
	Bindings  map[string]interface{}
	Config    map[string]interface{}
	Uses      []map[string]interface{}
	Traits    []map[string]interface{}
}

func (tc TestComponent) GetComponent() (radiusv1alpha1.Component, error) {

	bindingJson, err := json.Marshal(tc.Spec.Bindings)
	if err != nil {
		return radiusv1alpha1.Component{}, err
	}
	runJson, err := json.Marshal(tc.Spec.Run)
	if err != nil {
		return radiusv1alpha1.Component{}, err
	}

	uses := []runtime.RawExtension{}

	for _, use := range tc.Spec.Uses {
		useJson, err := json.Marshal(use)
		if err != nil {
			return radiusv1alpha1.Component{}, err
		}
		uses = append(uses, runtime.RawExtension{Raw: useJson})
	}

	traits := []runtime.RawExtension{}
	for _, trait := range tc.Spec.Traits {
		traitJson, err := json.Marshal(trait)
		if err != nil {
			return radiusv1alpha1.Component{}, err
		}
		traits = append(traits, runtime.RawExtension{Raw: traitJson})
	}

	return v1alpha1.Component{
		TypeMeta:   tc.TypeMeta,
		ObjectMeta: tc.ObjectMeta,
		Spec: v1alpha1.ComponentSpec{
			Kind:      tc.Spec.Kind,
			Run:       &runtime.RawExtension{Raw: runJson},
			Bindings:  runtime.RawExtension{Raw: bindingJson},
			Hierarchy: tc.Spec.Hierarchy,
			Uses:      &uses,
			Traits:    &traits,
		},
	}, nil

}

type ControllerTest struct {
	Options Options
	Row     Row
}

type Options struct {
	Client  client.Client
	Context context.Context
	K8s     *kubernetes.Clientset
}

func NewControllerTest(options Options, row Row) ControllerTest {
	return ControllerTest{Options: options, Row: row}
}

func (ct ControllerTest) Test(t *testing.T) {
	// This runs each application deploy as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.
	//
	// In the future we can extend this to multi-phase tests that do more than just deploy and delete by adding more
	// intermediate sub-tests.

	// Each of our tests are isolated to a single application, so they can run in parallel.
	t.Parallel()

	// Create Application
	err := ct.Options.Client.Create(ct.Options.Context, ct.Row.Application)
	require.NoError(t, err, "failed to create application")

	// Create Components
	components, err := ct.Row.GetComponents()
	require.NoError(t, err, "failed to get component list")

	for _, component := range *components {
		err := ct.Options.Client.Create(ct.Options.Context, &component)
		require.NoError(t, err, "failed to create component")
	}
	// ValidatePodsRunning triggers its own assertions, no need to handle errors
	validation.ValidateDeploymentsRunning(ct.Options.Context, t, ct.Options.K8s, ct.Row.Pods)
}

func getEnvTestBinaryPath() (string, error) {
	// TODO https://github.com/Azure/radius/issues/698, remove hard coded version
	cmd := exec.Command("setup-envtest", "use", "-p", "path", "1.19.x", "--arch", "amd64")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	return out.String(), err
}
