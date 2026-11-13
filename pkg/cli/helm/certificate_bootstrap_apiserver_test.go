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
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestBootstrapNativeInstallBlocksBeforeMutation(t *testing.T) {
	s, config, _, _ := newRotationTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.dynamic.Resource(apiServiceResource).Delete(ctx, radiusAPIService, metav1.DeleteOptions{}))
	installRotationTestCRDs(t, s)
	ca, private, _ := testSigningCA(t, nil, nil, nil, nil)
	operator, err := s.kube.CoreV1().Secrets(s.namespace).Create(ctx, caTLSSecret("operator-ca", ca, private), metav1.CreateOptions{})
	require.NoError(t, err)
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
		"global.rbac.enabled=true", "global.rbac.tls.provider=external",
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
			require.NoError(t, kube.AppsV1().Deployments(CertManagerNamespace).Delete(ctx, "cert-manager-cainjector", metav1.DeleteOptions{}))
			deployment, replicaSet, pod := injectorFixture("cert-manager-cainjector")
			_, err := kube.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
			require.NoError(t, err)
			_, err = kube.AppsV1().ReplicaSets(replicaSet.Namespace).Create(ctx, replicaSet, metav1.CreateOptions{})
			require.NoError(t, err)
			_, err = kube.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
			require.NoError(t, err)
			return kube, extensions, nil
		},
	}
	require.ErrorIs(t, impl.InstallRadius(ctx, options, "test"), errFreshCertificateBootstrapUnavailable)
	secrets, err := s.kube.CoreV1().Secrets(s.namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, secrets.Items, 1)
	require.Equal(t, *operator, secrets.Items[0])
	journals, err := s.kube.CoreV1().ConfigMaps(s.namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, journals.Items)
	leases, err := s.kube.CoordinationV1().Leases(s.namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, leases.Items)
	_, err = s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
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
		}}).guardCertificateRotation(ctx, "", s.namespace, "upgrade", nil), "legacy aggregation bootstrap")
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
