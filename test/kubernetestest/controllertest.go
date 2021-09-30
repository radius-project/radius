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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	bicepcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/bicep"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	"github.com/Azure/radius/pkg/kubernetes/webhook"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	//+kubebuilder:scaffold:imports
)

var options EnvOptions
var testEnv *envtest.Environment

const (
	retries            = 10
	CacheKeyController = "metadata.controller"
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
	err = (&radcontroller.ApplicationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Application"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize application reconciler: %w", err)
	}

	// Index deployments by the owner (any resource besides application)
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &appsv1.Deployment{}, CacheKeyController, extractOwnerKey)
	if err != nil {
		return err
	}

	// Index services by the owner (any resource besides application)
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Service{}, CacheKeyController, extractOwnerKey)
	if err != nil {
		return err
	}

	resourceTypes := []struct {
		client.Object
		client.ObjectList
	}{
		{&radiusv1alpha3.ContainerComponent{}, &radiusv1alpha3.ContainerComponentList{}},
		{&radiusv1alpha3.DaprIOInvokeRoute{}, &radiusv1alpha3.DaprIOInvokeRouteList{}},
		{&radiusv1alpha3.DaprIOPubSubTopicComponent{}, &radiusv1alpha3.DaprIOPubSubTopicComponentList{}},
		{&radiusv1alpha3.DaprIOStateStoreComponent{}, &radiusv1alpha3.DaprIOStateStoreComponentList{}},
		{&radiusv1alpha3.GrpcRoute{}, &radiusv1alpha3.GrpcRouteList{}},
		{&radiusv1alpha3.HttpRoute{}, &radiusv1alpha3.HttpRouteList{}},
		{&radiusv1alpha3.MongoDBComponent{}, &radiusv1alpha3.MongoDBComponentList{}},
		{&radiusv1alpha3.RabbitMQComponent{}, &radiusv1alpha3.RabbitMQComponentList{}},
		{&radiusv1alpha3.RedisComponent{}, &radiusv1alpha3.RedisComponentList{}},
	}

	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	for _, resourceType := range resourceTypes {
		gvks, _, err := scheme.ObjectKinds(resourceType.Object)
		if err != nil {
			return fmt.Errorf("failed to initialize find objects : %w", err)
		}
		for _, gvk := range gvks {
			if gvk.GroupVersion() != radiusv1alpha3.GroupVersion {
				continue
			}
			// Get GVR for corresponding component.
			gvr, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)

			if err != nil {
				return fmt.Errorf("can't find gvr: %w", err)
			}

			if err = (&radcontroller.ResourceReconciler{
				Client:  mgr.GetClient(),
				Log:     ctrl.Log.WithName("controllers").WithName(resourceType.GetName()),
				Scheme:  mgr.GetScheme(),
				Dynamic: dynamicClient,
				GVR:     gvr.Resource,
			}).SetupWithManager(mgr, resourceType.Object, resourceType.ObjectList); err != nil {
				return fmt.Errorf("can't create controller: %w", err)
			}
		}
	}

	err = (&webhook.ResourceWebhook{}).SetupWebhookWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize component webhook: %w", err)
	}

	if err = (&bicepcontroller.DeploymentTemplateReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("DeploymentTemplate"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to initialize arm reconciler: %w", err)
	}

	err = (&radiusv1alpha3.Application{}).SetupWebhookWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to initialize application webhook: %w", err)
	}

	err = (&bicepv1alpha3.DeploymentTemplate{}).SetupWebhookWithManager(mgr)
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

func extractOwnerKey(obj client.Object) []string {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		return nil
	}

	if owner.APIVersion != radiusv1alpha3.GroupVersion.String() || owner.Kind == "Application" {
		return nil
	}

	return []string{owner.Name}
}
