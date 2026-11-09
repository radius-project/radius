//go:build !windows

/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helm

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type bootstrapIssuanceClient struct {
	dynamic.Interface
	kube      kubernetes.Interface
	cert, key []byte
}

func (c *bootstrapIssuanceClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	delegate := c.Interface.Resource(resource)
	if resource == rotationCertificateResource {
		return bootstrapIssuanceResource{NamespaceableResourceInterface: delegate, client: c}
	}
	return delegate
}

type bootstrapIssuanceResource struct {
	dynamic.NamespaceableResourceInterface
	client *bootstrapIssuanceClient
}

func (r bootstrapIssuanceResource) Namespace(namespace string) dynamic.ResourceInterface {
	return bootstrapIssuanceWriter{ResourceInterface: r.NamespaceableResourceInterface.Namespace(namespace), client: r.client, namespace: namespace}
}

type bootstrapIssuanceWriter struct {
	dynamic.ResourceInterface
	client    *bootstrapIssuanceClient
	namespace string
}

func (r bootstrapIssuanceWriter) Create(ctx context.Context, object *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	created, err := r.ResourceInterface.Create(ctx, object, options, subresources...)
	if err != nil {
		return nil, err
	}
	name, _, err := unstructured.NestedString(created.Object, "spec", "secretName")
	if err != nil {
		return nil, err
	}
	_, err = r.client.kube.CoreV1().Secrets(r.namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.namespace},
		Type:       corev1.SecretTypeTLS, Data: map[string][]byte{"tls.crt": r.client.cert, "tls.key": r.client.key},
	}, metav1.CreateOptions{})
	return created, err
}

func TestBootstrapNativeInstallRequiresRunningEvidence(t *testing.T) {
	s, config, _, _ := newRotationTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.dynamic.Resource(apiServiceResource).Delete(ctx, radiusAPIService, metav1.DeleteOptions{}))
	installRotationTestCRDs(t, s)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca, private, root := testSigningCA(t, key, nil, nil, nil)
	leaf, leafKey, _ := testSigningCA(t, nil, func(cert *x509.Certificate) {
		cert.IsCA, cert.KeyUsage = false, x509.KeyUsageDigitalSignature
		cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		cert.DNSNames = []string{"ucp." + s.namespace + ".svc"}
		cert.NotAfter = time.Now().Add(24 * time.Hour)
	}, root, key)
	_, err = s.kube.CoreV1().Secrets(s.namespace).Create(ctx, caTLSSecret("operator-ca", ca, private), metav1.CreateOptions{})
	require.NoError(t, err)
	// Cert-manager availability and issuance are simulated. The public install
	// path, Helm resources, ownership, UIDs, resource versions and incomplete
	// evidence barrier use the native API server.
	s.dynamic = &bootstrapIssuanceClient{Interface: s.dynamic, kube: s.kube, cert: leaf, key: leafKey}
	selected, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	selected.Templates = slices.DeleteFunc(selected.Templates, func(file *common.File) bool {
		return strings.HasPrefix(file.Name, "templates/control-plane-readiness/") ||
			strings.HasPrefix(file.Name, "templates/pre-upgrade/")
	})
	selected.Files = slices.DeleteFunc(selected.Files, func(file *common.File) bool { return strings.HasPrefix(file.Name, "crds/") })
	archive, err := chartutil.Save(selected, t.TempDir())
	require.NoError(t, err)
	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, clientcmd.WriteToFile(clientcmdapi.Config{
		Clusters:  map[string]*clientcmdapi.Cluster{"test": {Server: config.Host, CertificateAuthorityData: config.CAData}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"test": {ClientCertificateData: config.CertData, ClientKeyData: config.KeyData}},
		Contexts:  map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}}, CurrentContext: "test",
	}, kubeconfig))
	t.Setenv("KUBECONFIG", kubeconfig)
	options := NewDefaultClusterOptions()
	options.Radius.Wait, options.Radius.Timeout = false, time.Minute
	options.Radius.ChartPath, options.Contour.Disabled, options.Certificates.CASecret = archive, true, "operator-ca"
	options.Radius.SetArgs = []string{
		"global.rbac.enabled=true", "global.rbac.mtls.provider=external",
		"global.prometheus.enabled=true", "global.prometheus.port=9090",
	}
	impl := &Impl{
		Helm: NewHelmClient(),
		newRotationStore: func(string, string) (*rotationStore, error) {
			return s, nil
		},
		newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
			kube, extensions := (fakeCertManagerCluster{crd: true,
				controller: certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")}).clients()
			return kube, extensions, nil
		},
	}
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	impl.logger = func(format string, arguments ...any) {
		if strings.Contains(fmt.Sprintf(format, arguments...), "fresh aggregation and component evidence") {
			cancel()
		}
	}
	require.ErrorContains(t, impl.InstallRadius(runContext, options, "test"), "interrupted")
	_, pending, err := s.readBootstrap(ctx)
	require.NoError(t, err)
	require.Equal(t, bootstrapVerifying, pending.Phase, "Secret creation and Helm success cannot complete bootstrap")
	require.NotEmpty(t, pending.Resources[bootstrapResourceKey(apiServiceResource, radiusAPIService)].UID)
	_, err = s.kube.CoreV1().Secrets(s.namespace).Get(ctx, "ucp-cert", metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err), "fresh enabled install must not create legacy aggregation material")
	_, _, err = s.read(ctx)
	require.True(t, apierrors.IsNotFound(err), "no completed lifecycle without process and API evidence")
}

func TestBootstrapNativeCreationAndFences(t *testing.T) {
	s, _, _, _ := newRotationTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.dynamic.Resource(apiServiceResource).Delete(ctx, radiusAPIService, metav1.DeleteOptions{}))
	installRotationTestCRDs(t, s)
	_, requested := bootstrapStoreFixture(t)
	requested.Namespace = s.namespace
	require.NoError(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
		s.guardWrite = func(ctx context.Context) error { return operation.checkProviderJournal(ctx, s.release, nil) }
		s.activeBootstrap = requested.ID
		journal, state, err := s.beginBootstrap(ctx, requested)
		require.NoError(t, err)
		require.ErrorContains(t, withIdentityOperation(ctx, s.kube, s.namespace, func(*identityOperation) error {
			t.Fatal("an existing identity Lease must never be stolen")
			return nil
		}), "lease")
		require.ErrorContains(t, (&Impl{newRotationStore: func(string, string) (*rotationStore, error) {
			return s, nil
		}}).guardCertificateRotation(ctx, "", s.namespace, "upgrade", nil), "unfinished")
		_, err = s.aggregationTransitionSnapshot(ctx)
		require.ErrorContains(t, err, "bootstrap")
		desired := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
			"metadata": map[string]any{"name": "radius-selfsigned-bootstrap", "namespace": s.namespace},
			"spec":     map[string]any{"selfSigned": map[string]any{}},
		}}
		check := s.guardWrite
		calls := 0
		s.guardWrite = func(ctx context.Context) error {
			calls++
			if calls == 2 {
				return errors.New("lost journal write after create")
			}
			return check(ctx)
		}
		require.ErrorContains(t, s.ensureBootstrapResource(ctx, state.ID, rotationIssuerResource, desired, true), "lost journal write")
		s.guardWrite = check
		created, err := s.dynamic.Resource(rotationIssuerResource).Namespace(s.namespace).Get(ctx, desired.GetName(), metav1.GetOptions{})
		require.NoError(t, err)
		require.NoError(t, s.ensureBootstrapResource(ctx, state.ID, rotationIssuerResource, desired, true))
		_, resumed, err := s.readBootstrap(ctx)
		require.NoError(t, err)
		require.Equal(t, created.GetUID(), resumed.Resources[bootstrapResourceKey(rotationIssuerResource, created.GetName())].UID)
		next := state
		next.Resources = resumed.Resources
		_, err = s.updateBootstrap(ctx, journal, state, next)
		require.ErrorContains(t, err, "intent changed")
		current, err := s.dynamic.Resource(rotationIssuerResource).Namespace(s.namespace).Get(ctx, desired.GetName(), metav1.GetOptions{})
		require.NoError(t, err)
		require.NoError(t, s.dynamic.Resource(rotationIssuerResource).Namespace(s.namespace).Delete(ctx, current.GetName(), metav1.DeleteOptions{}))
		current.SetUID("")
		current.SetResourceVersion("")
		_, err = s.dynamic.Resource(rotationIssuerResource).Namespace(s.namespace).Create(ctx, current, metav1.CreateOptions{})
		require.NoError(t, err)
		require.ErrorContains(t, s.checkBootstrapResources(ctx, resumed), "changed")
		changed := requested
		changed.IntentSHA256 = strings.Repeat("c", 64)
		_, _, err = s.beginBootstrap(ctx, changed)
		require.ErrorContains(t, err, "retry differs")
		return nil
	}))
}
