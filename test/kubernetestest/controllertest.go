// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetestest

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
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	//+kubebuilder:scaffold:imports
)

var options EnvOptions
var testEnv *envtest.Environment

const retries = 10

func StartController() error {
	assetsDirectory := os.Getenv("KUBEBUILDER_ASSETS")

	if assetsDirectory == "" {
		// run setup-envtest to get the path to binary assets
		var err error
		assetsDirectory, err = getEnvTestBinaryPath()
		if err != nil {
			return fmt.Errorf("failed to call setup-envtest to find path: %w", err)
		}
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "deploy", "Chart", "crds")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: assetsDirectory,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "kubernetestest")},
		},
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(radiusv1alpha1.AddToScheme(scheme))

	err := scheme.AddConversionFunc(&radiusv1alpha1.Component{}, &components.GenericComponent{}, converters.ConvertComponentToInternal)
	if err != nil {
		return fmt.Errorf("failed to add conversion func: %w", err)
	}

	cfg, err := testEnv.Start()
	if err != nil {
		return fmt.Errorf("failed to initialize environment: %w", err)
	}

	err = radiusv1alpha1.AddToScheme(scheme)
	if err != nil {
		return fmt.Errorf("could not add scheme: %w", err)
	}

	//+kubebuilder:scaffold:scheme

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic kubernetes client: %w", err)
	}

	k8s, err := k8s.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	webhookInstallOptions := &testEnv.WebhookInstallOptions

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	if err != nil {
		return fmt.Errorf("failed to initialize manager: %w", err)
	}
	err = (&controllers.ApplicationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Application"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize application reconciler: %w", err)
	}

	err = (&controllers.ComponentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Component"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize component reconciler: %w", err)
	}
	err = (&controllers.DeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Deployment"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize deployment reconciler: %w", err)
	}

	if err = (&controllers.ArmReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("Arm"),
		Scheme:        mgr.GetScheme(),
		DynamicClient: dynamicClient,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to initialize arm reconciler: %w", err)
	}

	err = (&radiusv1alpha1.Application{}).SetupWebhookWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize application webhook: %w", err)
	}
	err = (&radiusv1alpha1.Component{}).SetupWebhookWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize component webhook: %w", err)
	}
	err = (&radiusv1alpha1.Deployment{}).SetupWebhookWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize deployment webhook: %w", err)
	}
	err = (&radiusv1alpha1.Arm{}).SetupWebhookWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize arm webhook: %w", err)
	}

	go func() {
		_ = mgr.Start(ctrl.SetupSignalHandler())
	}()

	// Make sure the webhook is started
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	for i := 0; i < retries; i++ {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			if i == retries-1 {
				// if we can't connect after 10 attempts, fail
				return fmt.Errorf("failed to connect to webhook: %w", err)
			}
			time.Sleep(time.Second)
			continue
		}
		conn.Close()
		break
	}

	options = EnvOptions{
		K8s:     k8s,
		Dynamic: dynamicClient,
	}

	return nil
}

func StopController() error {
	return testEnv.Stop()
}

func (ct ControllerTest) Test(t *testing.T) error {
	// Make sure namespace exists
	err := kubernetes.CreateNamespace(ct.Context, ct.Options.K8s, ct.ControllerStep.Namespace)
	require.NoError(t, err, "failed to create namespace")

	items, err := ioutil.ReadDir(ct.ControllerStep.TemplateFolder)
	require.NoError(t, err, "failed to read directory")

	for _, item := range items {
		unst, err := GetUnstructured(path.Join(ct.ControllerStep.TemplateFolder, item.Name()))
		require.NoError(t, err, "failed to get unstructured")

		gvr, err := gvr(unst)
		require.NoError(t, err, "failed to get gvr")

		data, err := unst.MarshalJSON()
		require.NoError(t, err, "failed to marshal json")

		name := unst.GetName()

		_, err = ct.Options.Dynamic.Resource(gvr).Namespace(ct.ControllerStep.Namespace).Patch(
			ct.Context,
			name,
			types.ApplyPatchType,
			data,
			v1.PatchOptions{FieldManager: "rad"})
		if err != nil {
			return err
		}
	}
	return nil
}

func (ct ControllerTest) ValidateDeploymentsRunning(t *testing.T) {

	// ValidateDeploymentsRunning triggers its own assertions, no need to handle errors
	validation.ValidateDeploymentsRunning(ct.Context, t, ct.Options.K8s, ct.ControllerStep.Deployments)
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
	} else if unst.GroupVersionKind().Kind == "Arm" {
		return schema.GroupVersionResource{
			Group:    "radius.dev",
			Version:  "v1alpha1",
			Resource: "arms",
		}, nil
	}

	return schema.GroupVersionResource{}, fmt.Errorf("unsupported resource  '%s'", unst.GroupVersionKind().Kind)
}

func getEnvTestBinaryPath() (string, error) {
	// TODO https://github.com/Azure/radius/issues/698, remove hard coded version
	cmd := exec.Command("setup-envtest", "use", "-p", "path", "1.19.x", "--arch", "amd64")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	return out.String(), err
}

type ControllerStep struct {
	TemplateFolder string
	Namespace      string
	Deployments    validation.K8sObjectSet
}

type ControllerTest struct {
	Options        EnvOptions
	Context        context.Context
	ControllerStep ControllerStep
}

type EnvOptions struct {
	K8s     *k8s.Clientset
	Dynamic dynamic.Interface
}

func NewControllerTest(ctx context.Context, row ControllerStep) ControllerTest {
	return ControllerTest{options, ctx, row}
}
