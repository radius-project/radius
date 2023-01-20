// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli/output"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Scheme = k8s_runtime.NewScheme()
)

func init() {
	// Adds all types to the client.Client scheme
	// Any time we add a new type to to radius,
	// we need to add it here.
	// TODO centralize these calls.
	_ = apiextv1.AddToScheme(Scheme)
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = contourv1.AddToScheme(Scheme)
}

func ReadKubeConfig() (*api.Config, error) {
	var kubeConfig string
	if home := homeDir(); home != "" {
		kubeConfig = filepath.Join(home, ".kube", "config")
	} else {
		return nil, errors.New("no HOME directory, cannot find kubeconfig")
	}

	config, err := clientcmd.LoadFromFile(kubeConfig)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func CreateExtensionClient(context string) (clientset.Interface, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	client, err := clientset.NewForConfig(merged)
	if err != nil {
		return nil, err
	}

	return client, err
}

func CreateRestRoundTripper(context string, group string, overrideURL string) (http.RoundTripper, error) {
	if overrideURL != "" {
		return http.DefaultTransport, nil
	}

	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}
	gv := schema.GroupVersion{Group: group, Version: "v1alpha3"}
	merged.GroupVersion = &gv
	merged.APIPath = "/apis"
	merged.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	client, err := rest.TransportFor(merged)
	if err != nil {
		return nil, err
	}

	return NewLocationRewriteRoundTripper(merged.Host, client), err
}

func CreateAPIServerPipeline(context string, overrideURL string) (string, runtime.Pipeline, error) {
	baseURL, transporter, err := CreateAPIServerTransporter(context, overrideURL)
	if err != nil {
		return baseURL, runtime.Pipeline{}, err
	}

	pipeline := runtime.NewPipeline(module, version, runtime.PipelineOptions{}, &policy.ClientOptions{
		Cloud: cloud.Configuration{
			Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
				cloud.ResourceManager: {
					Endpoint: baseURL,
					Audience: "https://management.core.windows.net",
				},
			},
		},
		Transport: transporter,
	})

	return baseURL, pipeline, nil
}

func GetBaseUrlForDeploymentEngine(overrideURL string) string {
	return strings.TrimSuffix(overrideURL, "/") + UCPAPIServerBasePath
}

func GetBaseUrlAndRoundTripperForDeploymentEngine(ucpURL string, context string) (string, http.RoundTripper, error) {
	var baseURL string
	var roundTripper http.RoundTripper
	basePath := UCPAPIServerBasePath

	if ucpURL != "" {
		baseURL = strings.TrimSuffix(ucpURL, "/") + basePath
		roundTripper = NewLocationRewriteRoundTripper(ucpURL, http.DefaultTransport)
	} else {
		restConfig, err := GetConfig(context)
		if err != nil {
			return "", nil, err
		}

		k8sType := UCPType

		roundTripper, err = CreateRestRoundTripper(context, k8sType, ucpURL)
		if err != nil {
			return "", nil, err
		}

		baseURL = strings.TrimSuffix(restConfig.Host+restConfig.APIPath, "/") + basePath
		roundTripper = NewLocationRewriteRoundTripper(restConfig.Host, roundTripper)
	}
	return baseURL, roundTripper, nil
}

func GetBaseUrlAndRoundTripper(overrideURL string, group string, context string) (string, http.RoundTripper, error) {
	var baseURL string
	var roundTripper http.RoundTripper
	if overrideURL != "" {
		baseURL = strings.TrimSuffix(overrideURL, "/") + UCPAPIServerBasePath
		roundTripper = NewLocationRewriteRoundTripper(overrideURL, http.DefaultTransport)
	} else {
		restConfig, err := GetConfig(context)
		if err != nil {
			return "", nil, err
		}
		roundTripper, err = CreateRestRoundTripper(context, group, overrideURL)
		if err != nil {
			return "", nil, err
		}
		baseURL = strings.TrimSuffix(restConfig.Host+restConfig.APIPath, "/") + UCPAPIServerBasePath
		roundTripper = NewLocationRewriteRoundTripper(restConfig.Host, roundTripper)
	}
	return baseURL, roundTripper, nil
}

func CreateRestConfig(context string) (*rest.Config, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	return merged, err
}

func CreateDynamicClient(context string) (dynamic.Interface, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(merged)
	if err != nil {
		return nil, err
	}

	return client, err
}

func CreateTypedClient(context string) (*k8s.Clientset, *rest.Config, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, nil, err
	}

	client, err := k8s.NewForConfig(merged)
	if err != nil {
		return nil, nil, err
	}

	return client, merged, err
}

func CreateRuntimeClient(context string, scheme *k8s_runtime.Scheme) (client.Client, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	var c client.Client
	for i := 0; i < 2; i++ {
		c, err = client.New(merged, client.Options{Scheme: scheme})
		if err != nil {
			output.LogInfo(fmt.Errorf("failed to get a kubernetes client: %w", err).Error())
			time.Sleep(15 * time.Second)
		}
	}
	if err != nil {
		output.LogInfo("aborting runtime client creation after 3 retries")
		return nil, err
	}

	return c, nil
}

func CreateRESTMapper(context string) (meta.RESTMapper, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	d, err := discovery.NewDiscoveryClientForConfig(merged)
	if err != nil {
		return nil, err
	}

	return restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(d)), nil
}

func EnsureNamespace(ctx context.Context, client k8s.Interface, namespace string) error {
	namespaceApply := applycorev1.Namespace(namespace)

	// Use Apply instead of Create to avoid failures on a namespace already existing.
	_, err := client.CoreV1().Namespaces().Apply(ctx, namespaceApply, metav1.ApplyOptions{FieldManager: "rad"})
	if err != nil {
		return err
	}
	return nil
}

func GetConfig(context string) (*rest.Config, error) {
	config, err := ReadKubeConfig()
	if err != nil {
		return nil, err
	}

	clientconfig := clientcmd.NewNonInteractiveClientConfig(*config, context, nil, nil)
	merged, err := clientconfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return merged, err
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

var _ policy.Transporter = &KubernetesTransporter{}

type KubernetesTransporter struct {
	Client http.RoundTripper
}

func (t *KubernetesTransporter) Do(req *http.Request) (*http.Response, error) {
	resp, err := t.Client.RoundTrip(req)
	return resp, err
}

var _ http.RoundTripper = (*LocationRewriteRoundTripper)(nil)

// LocationRewriteRoundTripper rewrites the value of the HTTP Location header on responses
// to match the expected Host/Scheme.
//
// There is a blocking behavior bug when combining the ARM-RPC protocol and a Kubernetes APIService.
// Kubernetes does not forward the original hostname when proxying requests (we get the wrong value in
// X-Forwarded-Host). See: https://github.com/kubernetes/kubernetes/issues/107435
//
// ARM-RPC requires the Location header to contain a fully-qualified absolute URL (it must start
// with https://...). Combining this requirement with the broken behavior of APIService proxying means
// that we generate the wrong URL.
//
// So this is a temporary solution, until we can solve this at the protocol level. We rewrite the Location
// header on the client.
type LocationRewriteRoundTripper struct {
	Inner  http.RoundTripper
	Host   string
	Scheme string
}

func (t *LocationRewriteRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	res, err := t.Inner.RoundTrip(request)
	if err != nil {
		return nil, err
	}

	value, ok := res.Header[textproto.CanonicalMIMEHeaderKey(Location)]
	if ok && len(value) > 0 {
		u := GetFixedHeader(value, t.Scheme, t.Host)
		if u != nil {
			res.Header[textproto.CanonicalMIMEHeaderKey(Location)] = []string{u.String()}
		}
	}

	valueAsync, ok := res.Header[textproto.CanonicalMIMEHeaderKey(AzureAsyncOperation)]
	if ok && len(valueAsync) > 0 {
		u := GetFixedHeader(valueAsync, t.Scheme, t.Host)
		if u != nil {
			res.Header[textproto.CanonicalMIMEHeaderKey(AzureAsyncOperation)] = []string{u.String()}
		}
	}

	return res, nil
}

func GetFixedHeader(value []string, scheme string, host string) *url.URL {
	// OK we have a location value, try to parse as a URL and then rewrite.
	// Location is specified to contain a single value so just use the first one.
	u, err := url.Parse(value[0])
	if err != nil {
		// If we fail to parse the location value just skip rewiting. Our usage of Location should always be valid.
		return nil
	}

	if u.Scheme == "" {
		// If we don't have a fully-qualified URL then just skip rewriting. Our usage of Location should always be fully-qualified.
		return nil
	}

	if scheme != "" {
		u.Scheme = scheme
	}
	u.Host = host

	return u
}

func NewLocationRewriteRoundTripper(prefix string, inner http.RoundTripper) *LocationRewriteRoundTripper {
	// NOTE: while we get the value from RESTConfig.Host - it's NOT always a host:port combo. Sometimes
	// it is a URL including the scheme portion. JUST FOR FUN.
	//
	// We do our best to handle all of those cases here and degrade silently if we can't.
	if strings.Contains(prefix, "://") {
		// If we get here this is likely a fully-qualified URL.
		u, err := url.Parse(prefix)
		if err != nil {
			// We failed to parse this as a URL, just treat it as a hostname then.
			return &LocationRewriteRoundTripper{Inner: inner, Host: prefix}
		}

		// OK we have a URL
		return &LocationRewriteRoundTripper{Inner: inner, Host: u.Host, Scheme: u.Scheme}
	}

	// If we get here it's likely not a fully-qualified URL. Treat it as a hostname.
	return &LocationRewriteRoundTripper{Inner: inner, Host: prefix}
}

func CreateAPIServerTransporter(kubeContext string, overrideURL string) (string, policy.Transporter, error) {
	baseURL, roundTripper, err := GetBaseUrlAndRoundTripper(overrideURL, "api.ucp.dev", kubeContext)
	if err != nil {
		return "", nil, err
	}

	return baseURL, &KubernetesTransporter{Client: roundTripper}, nil
}

// Creating a Kubernetes client
func CreateKubernetesClients(contextName string) (k8s.Interface, runtime_client.Client, string, error) {
	k8sConfig, err := ReadKubeConfig()
	if err != nil {
		return nil, nil, "", err
	}

	if contextName == "" && k8sConfig.CurrentContext == "" {
		return nil, nil, "", errors.New("no kubernetes context is set")
	} else if contextName == "" {
		contextName = k8sConfig.CurrentContext
	}

	context := k8sConfig.Contexts[contextName]
	if context == nil {
		return nil, nil, "", fmt.Errorf("kubernetes context '%s' could not be found", contextName)
	}

	client, _, err := CreateTypedClient(contextName)
	if err != nil {
		return nil, nil, "", err
	}

	runtimeClient, err := CreateRuntimeClient(contextName, Scheme)
	if err != nil {
		return nil, nil, "", err
	}

	return client, runtimeClient, contextName, nil
}

//go:generate mockgen -destination=./mock_kubernetes.go -package=kubernetes -self_package github.com/project-radius/radius/pkg/cli/kubernetes github.com/project-radius/radius/pkg/cli/kubernetes Interface
type Interface interface {
	GetKubeContext() (*api.Config, error)
}

type Impl struct {
}

// Fetches the kubecontext from the system
func (i *Impl) GetKubeContext() (*api.Config, error) {
	return ReadKubeConfig()
}
