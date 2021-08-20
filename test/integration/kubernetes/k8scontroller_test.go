// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/Azure/radius/pkg/cli/kubernetes"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/kubernetes/controllers"
	"github.com/Azure/radius/pkg/kubernetes/converters"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
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
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "deploy", "Chart", "crds")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: assetsDirectory,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "deploy", "Chart", "templates", "webhook")},
		},
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(radiusv1alpha1.AddToScheme(scheme))

	err := scheme.AddConversionFunc(&radiusv1alpha1.Component{}, &components.GenericComponent{}, converters.ConvertComponentToInternal)
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

	dynamicClient, err := dynamic.NewForConfig(cfg)
	require.NoError(t, err, "failed to create dynamic kubernetes client")

	k8s, err := k8s.NewForConfig(cfg)
	require.NoError(t, err, "failed to create kubernetes client")

	webhookInstallOptions := &testEnv.WebhookInstallOptions

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
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

	err = (&radiusv1alpha1.Application{}).SetupWebhookWithManager(mgr)
	require.NoError(t, err, "failed to initialize application webhook")

	err = (&radiusv1alpha1.Component{}).SetupWebhookWithManager(mgr)
	require.NoError(t, err, "failed to initialize component webhook")

	err = (&radiusv1alpha1.Deployment{}).SetupWebhookWithManager(mgr)
	require.NoError(t, err, "failed to initialize deployment webhook")

	go func() {
		err = mgr.Start(ctrl.SetupSignalHandler())
		require.NoError(t, err, "failed to start manager")
	}()

	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	for i := 0; i < 10; i++ {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			if i == 9 {
				// if we can't connect after 10 attempts, fail
				require.NoError(t, err, "failed to connect to webhook")
			}
			time.Sleep(time.Second)
			continue
		}
		conn.Close()
		break
	}

	options := Options{
		Context: ctx,
		K8s:     k8s,
		Dynamic: dynamicClient,
	}

	table := []Row{
		// {
		// 	// Testing applications
		// 	Namespace:      "frontend-backend",
		// 	TemplateFolder: "testdata/frontend-backend/",
		// 	Pods: validation.K8sObjectSet{
		// 		Namespaces: map[string][]validation.K8sObject{
		// 			"frontend-backend": {
		// 				validation.NewK8sObjectForComponent("frontend-backend", "frontend"),
		// 				validation.NewK8sObjectForComponent("frontend-backend", "backend"),
		// 			},
		// 		},
		// 	},
		// },
		{
			Namespace:      "invalidcomponent",
			TemplateFolder: "testdata/invalidcomponent/",
			// TODO write func that verifies error here.
		},
	}

	// Nest parallel subtests into outer Run to have function wait for all tests
	// to finish before returning.
	// See: https://golang.org/pkg/testing/#hdr-Subtests_and_Sub_benchmarks
	t.Run("deploytests", func(t *testing.T) {
		for _, row := range table {
			test := NewControllerTest(options, row)
			t.Run(row.Namespace, test.Test)
		}
	})
}

type Row struct {
	TemplateFolder  string
	Namespace       string
	Pods            validation.K8sObjectSet
	ExpectedFailure string
}

type ControllerTest struct {
	Options Options
	Row     Row
}

type Options struct {
	Context context.Context
	K8s     *k8s.Clientset
	Dynamic dynamic.Interface
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

	// Make sure namespace exists

	err := kubernetes.CreateNamespace(ct.Options.Context, ct.Options.K8s, ct.Row.Namespace)
	require.NoError(t, err, "failed to create namespace")

	items, err := ioutil.ReadDir(ct.Row.TemplateFolder)
	require.NoError(t, err, "failed to read directory")

	for _, item := range items {
		unst, err := GetUnstructured(path.Join(ct.Row.TemplateFolder, item.Name()))
		require.NoError(t, err, "failed to get unstructured")

		gvr, err := gvr(unst)
		require.NoError(t, err, "failed to get gvr")

		data, err := unst.MarshalJSON()
		require.NoError(t, err, "failed to marshal json")

		name := unst.GetName()

		_, err = ct.Options.Dynamic.Resource(gvr).Namespace(ct.Row.Namespace).Patch(
			ct.Options.Context,
			name,
			types.ApplyPatchType,
			data,
			v1.PatchOptions{FieldManager: "rad"})
		require.NoError(t, err, "failed to patch")
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

func GetUnstructured(filePath string) (*unstructured.Unstructured, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	uns := &unstructured.Unstructured{}
	err = json.Unmarshal(content, uns)
	return uns, err
}

func gvr(unst *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	if unst.GroupVersionKind().Kind == "Application" {
		return schema.GroupVersionResource{
			Group:    "radius.dev",
			Version:  "v1alpha1",
			Resource: "applications",
		}, nil
	} else if unst.GroupVersionKind().Kind == "Component" {
		return schema.GroupVersionResource{
			Group:    "radius.dev",
			Version:  "v1alpha1",
			Resource: "components",
		}, nil
	} else if unst.GroupVersionKind().Kind == "Deployment" {
		return schema.GroupVersionResource{
			Group:    "radius.dev",
			Version:  "v1alpha1",
			Resource: "deployments",
		}, nil
	}

	return schema.GroupVersionResource{}, fmt.Errorf("unsupported resource  '%s'", unst.GroupVersionKind().Kind)
}
