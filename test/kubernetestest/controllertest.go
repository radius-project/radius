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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/Azure/radius/pkg/cli/kubernetes"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	controllers "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	kubernetesmodel "github.com/Azure/radius/pkg/model/kubernetes"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
	//+kubebuilder:scaffold:imports
)

var options EnvOptions
var testEnv *envtest.Environment

const (
	retries = 10
)

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

	opts := zap.Options{
		Development: true,
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

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
	utilruntime.Must(gatewayv1alpha1.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	utilruntime.Must(bicepv1alpha3.AddToScheme(scheme))

	cfg, err := testEnv.Start()
	if err != nil {
		return fmt.Errorf("failed to initialize environment: %w", err)
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

	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

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

	controllerOptions := radcontroller.Options{
		AppModel:      kubernetesmodel.NewKubernetesModel(mgr.GetClient()),
		Client:        mgr.GetClient(),
		Dynamic:       dynamicClient,
		Scheme:        scheme,
		Log:           ctrl.Log,
		Recorder:      mgr.GetEventRecorderFor("radius"),
		RestConfig:    cfg,
		RestMapper:    mapper,
		ResourceTypes: radcontroller.DefaultResourceTypes,
		WatchTypes: map[string]struct {
			Object        client.Object
			ObjectList    client.ObjectList
			HealthHandler func(ctx context.Context, r *controllers.ResourceReconciler, a client.Object) (string, string)
		}{
			resourcekinds.Service:     {&corev1.Service{}, &corev1.ServiceList{}, nil},
			resourcekinds.Deployment:  {&appsv1.Deployment{}, &appsv1.DeploymentList{}, nil},
			resourcekinds.Secret:      {&corev1.Secret{}, &corev1.SecretList{}, nil},
			resourcekinds.StatefulSet: {&appsv1.StatefulSet{}, &appsv1.StatefulSetList{}, nil},
		},
		SkipWebhooks: false,
	}

	controller := radcontroller.NewRadiusController(&controllerOptions)
	err = controller.SetupWithManager(mgr)
	if err != nil {
		return err
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
		Scheme:  mgr.GetScheme(),
		Mapper:  mapper,
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

		gvks, _, err := ct.Options.Scheme.ObjectKinds(unst)
		if err != nil {
			return fmt.Errorf("failed to initialize find objects : %w", err)
		}
		for _, gvk := range gvks {
			// Get GVR for corresponding component.
			gvr, err := ct.Options.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
			require.NoError(t, err, "failed to marshal json")

			data, err := unst.MarshalJSON()
			require.NoError(t, err, "failed to marshal json")

			name := unst.GetName()

			_, err = ct.Options.Dynamic.Resource(gvr.Resource).Namespace(ct.ControllerStep.Namespace).Patch(
				ct.Context,
				name,
				types.ApplyPatchType,
				data,
				v1.PatchOptions{FieldManager: "rad"})
			if err != nil {
				return err
			}
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
	Scheme  *runtime.Scheme
	Mapper  *restmapper.DeferredDiscoveryRESTMapper
}

func NewControllerTest(ctx context.Context, row ControllerStep) ControllerTest {
	return ControllerTest{options, ctx, row}
}
