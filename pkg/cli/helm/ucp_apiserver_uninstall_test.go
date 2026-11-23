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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/release"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// stagedUCPAPIServerObject returns object name of kind as rad stages it for the Radius release, or, when release is
// empty, without Helm ownership metadata.
func stagedUCPAPIServerObject(kind, name, release string, annotations map[string]string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{}
	object.SetAPIVersion("cert-manager.io/v1")
	object.SetKind(kind)
	object.SetNamespace(RadiusSystemNamespace)
	object.SetName(name)
	object.SetResourceVersion("1")
	object.SetUID(types.UID("uid-" + name))
	if release != "" {
		object.SetLabels(map[string]string{helmManagedByLabel: helmManagedByValue})
		annotations = mergeAnnotations(annotations, map[string]string{
			helmReleaseNameAnnotation:      release,
			helmReleaseNamespaceAnnotation: RadiusSystemNamespace,
		})
	}
	object.SetAnnotations(annotations)
	return object
}

func mergeAnnotations(a, b map[string]string) map[string]string {
	out := maps.Clone(a)
	if out == nil {
		out = map[string]string{}
	}
	maps.Copy(out, b)
	return out
}

func stageObjectExists(t *testing.T, client dynamic.Interface, resource schema.GroupVersionResource, name string) bool {
	t.Helper()
	_, err := client.Resource(resource).Namespace(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

func setupUninstallStore(t *testing.T, impl *Impl) {
	t.Helper()
	kube, dyn, err := impl.ucpAPIServerTrustClients("")
	require.NoError(t, err)
	_, err = kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace, UID: "namespace", ResourceVersion: "1"}}, metav1.CreateOptions{})
	require.NoError(t, err)
	store := bootstrapStoreFixture(t)
	kube.(*kubefake.Clientset).PrependReactor("create", "leases", store.kube.(*kubefake.Clientset).ReactionChain[0].React)
	store.kube, store.dynamic = kube, dyn
	impl.newRotationStore = func(string, string) (*rotationStore, error) { return store, nil }
}

func countDeletes(client *dynamicfake.FakeDynamicClient) int {
	deletes := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == "delete" {
			deletes++
		}
	}
	return deletes
}

// An upgrade that turns authorization on stages the radius-apiserver-ca chain before Helm adopts it. If it stops
// before Helm runs, the chain is outside the release, so Helm's uninstall leaves it; rad deletes it afterwards.
func Test_UninstallRadius_DeletesUnadoptedUCPAPIServerStage(t *testing.T) {
	var objects []runtime.Object
	for _, staged := range ucpAPIServerCAStageObjects {
		objects = append(objects, stagedUCPAPIServerObject(staged.kind, staged.name, radiusReleaseName, nil))
	}
	dynamicClient := fakeCertManagerDynamicClient(nil, objects...)

	mockHelmClient := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	for _, c := range []struct{ releaseName, ns string }{
		{options.Radius.ReleaseName, options.Radius.Namespace},
		{options.Contour.ReleaseName, options.Contour.Namespace},
	} {
		mockHelmClient.EXPECT().RunHelmUninstall(gomock.AssignableToTypeOf(&helm.Configuration{}), c.releaseName, c.ns, true).Return(&release.UninstallReleaseResponse{}, nil)
	}
	for _, name := range []string{certManagerReleaseName} {
		mockHelmClient.EXPECT().RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), name).Return(nil, driver.ErrReleaseNotFound)
	}
	impl := &Impl{
		Helm:                        mockHelmClient,
		removeDefaultContourGateway: (&defaultContourGatewayCalls{}).remove,
		newUCPAPIServerTrustClient:  uninstallTrustClient,
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return dynamicClient, nil },
	}
	setupUninstallStore(t, impl)
	require.NoError(t, impl.UninstallRadius(t.Context(), options, "test-context"))
	for _, staged := range ucpAPIServerCAStageObjects {
		require.False(t, stageObjectExists(t, dynamicClient, staged.resource, staged.name), "uninstall must delete the staged %s %s", staged.kind, staged.name)
	}
}

func Test_deleteUnadoptedUCPAPIServerStage(t *testing.T) {
	certificate := ucpAPIServerCAStageObjects[1]
	require.Equal(t, "Certificate", certificate.kind)

	tests := []struct {
		name string
		// object replaces the staged Certificate; nil leaves it as rad stages it.
		object func() *unstructured.Unstructured
		// changeOnDelete makes the Certificate change between rad's read and its delete.
		changeOnDelete func(*unstructured.Unstructured)
		wantKept       bool
		wantWarning    string
		// wantRerunDeletes is true when a rerun reads the changed Certificate again, still finds it staged, and
		// deletes it.
		wantRerunDeletes bool
	}{
		{name: "staged for the release"},
		{
			name: "without Helm ownership",
			object: func() *unstructured.Unstructured {
				return stagedUCPAPIServerObject("Certificate", certificate.name, "", nil)
			},
			wantKept:    true,
			wantWarning: `which is not an object rad staged for the Radius release. If nothing else uses it, delete it with kubectl --context "test-context" -n radius-system delete certificate radius-apiserver-ca`,
		},
		{
			name: "of another Helm release",
			object: func() *unstructured.Unstructured {
				return stagedUCPAPIServerObject("Certificate", certificate.name, "other", nil)
			},
			wantKept:    true,
			wantWarning: "which is not an object rad staged for the Radius release",
		},
		{
			name: "kept by helm.sh/resource-policy",
			object: func() *unstructured.Unstructured {
				return stagedUCPAPIServerObject("Certificate", certificate.name, radiusReleaseName, map[string]string{helmResourcePolicyAnnotation: "keep"})
			},
			wantKept:    true,
			wantWarning: "which is not an object rad staged for the Radius release",
		},
		{
			name:             "changed after rad read it",
			changeOnDelete:   func(object *unstructured.Unstructured) { object.SetResourceVersion("2") },
			wantKept:         true,
			wantWarning:      "which changed after rad read it. Run the command again to delete it",
			wantRerunDeletes: true,
		},
		{
			name: "recreated by another owner after rad read it",
			changeOnDelete: func(object *unstructured.Unstructured) {
				object.SetUID("recreated")
				object.SetResourceVersion("2")
				object.SetLabels(nil)
				object.SetAnnotations(nil)
			},
			wantKept:    true,
			wantWarning: "which is not an object rad staged for the Radius release",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []runtime.Object
			for _, staged := range ucpAPIServerCAStageObjects {
				object := stagedUCPAPIServerObject(staged.kind, staged.name, radiusReleaseName, nil)
				if staged.name == certificate.name && staged.kind == certificate.kind && tt.object != nil {
					object = tt.object()
				}
				objects = append(objects, object)
			}
			dynamicClient := fakeCertManagerDynamicClient(nil, objects...)
			if tt.changeOnDelete != nil {
				changed := false
				dynamicClient.PrependReactor("delete", certificate.resource.Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
					if changed || action.(k8stesting.DeleteAction).GetName() != certificate.name {
						return false, nil, nil
					}
					changed = true
					live, err := dynamicClient.Tracker().Get(certificate.resource, RadiusSystemNamespace, certificate.name)
					require.NoError(t, err)
					object := live.(*unstructured.Unstructured).DeepCopy()
					tt.changeOnDelete(object)
					require.NoError(t, dynamicClient.Tracker().Update(certificate.resource, object, RadiusSystemNamespace))
					return true, nil, apierrors.NewConflict(certificate.resource.GroupResource(), certificate.name, fmt.Errorf("precondition failed"))
				})
			}
			var logs []string
			impl := &Impl{logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }}

			require.NoError(t, impl.deleteUnadoptedUCPAPIServerStage(t.Context(), "test-context", dynamicClient, RadiusSystemNamespace, radiusReleaseName, nil))
			for _, staged := range ucpAPIServerCAStageObjects {
				want := tt.wantKept && staged.name == certificate.name && staged.kind == certificate.kind
				require.Equal(t, want, stageObjectExists(t, dynamicClient, staged.resource, staged.name), "%s %s", staged.kind, staged.name)
			}
			if tt.wantWarning == "" {
				require.Empty(t, logs)
			} else {
				require.Len(t, logs, 1)
				require.Contains(t, logs[0], "Warning: rad did not delete Certificate radius-system/radius-apiserver-ca")
				require.Contains(t, logs[0], tt.wantWarning)
			}

			deletes := countDeletes(dynamicClient)
			require.NoError(t, impl.deleteUnadoptedUCPAPIServerStage(t.Context(), "test-context", dynamicClient, RadiusSystemNamespace, radiusReleaseName, nil))
			if tt.wantRerunDeletes {
				require.Equal(t, deletes+1, countDeletes(dynamicClient), "a rerun deletes the Certificate it read again")
				require.False(t, stageObjectExists(t, dynamicClient, certificate.resource, certificate.name))
				return
			}
			require.Equal(t, deletes, countDeletes(dynamicClient), "a rerun must not delete anything")
		})
	}

	t.Run("cert-manager CRDs already removed", func(t *testing.T) {
		dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		require.NoError(t, (&Impl{}).deleteUnadoptedUCPAPIServerStage(t.Context(), "test-context", dynamicClient, RadiusSystemNamespace, radiusReleaseName, nil))
	})
}

// Helm uninstall leaves the Secrets that cert-manager issued for the Certificates of the release, since they have no
// owner reference, and Secret ucp-cert when an interrupted switch to authorization off staged it before Helm adopted
// it. rad deletes those it or cert-manager wrote for the release, and warns about any other Secret with their names.
func Test_UninstallRadius_DeletesUCPAPIServerSecrets(t *testing.T) {
	issued := func(name, certificate string) *corev1.Secret {
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: RadiusSystemNamespace, UID: types.UID("uid-" + name), ResourceVersion: "1",
			Annotations: map[string]string{certificateNameAnnotation: certificate}}}
	}
	stagedOffCertificate := func(release string) *unstructured.Unstructured {
		object := stagedUCPAPIServerObject("Secret", ucpOffCertificateSecretName, release, nil)
		object.SetAPIVersion("v1")
		return object
	}
	tests := []struct {
		name       string
		secrets    []runtime.Object
		staged     *unstructured.Unstructured
		wantKept   []string
		wantLogged []string
	}{
		{
			name:    "issued for the release",
			secrets: []runtime.Object{issued(ucpAPIServerCASecretName, ucpAPIServerCASecretName), issued(ucpAPIServerTLSSecretName, ucpAPIServerCertificateName)},
			staged:  stagedOffCertificate(radiusReleaseName),
		},
		{
			name:     "with other owners",
			secrets:  []runtime.Object{issued(ucpAPIServerCASecretName, "other"), issued(ucpAPIServerTLSSecretName, "other")},
			staged:   stagedOffCertificate(""),
			wantKept: []string{ucpAPIServerCASecretName, ucpAPIServerTLSSecretName, ucpOffCertificateSecretName},
			wantLogged: []string{
				"Secret radius-system/ucp-cert, which is not an object rad staged for the Radius release",
				"radius-system/radius-apiserver-ca", "radius-system/ucp-apiserver-tls",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset(tt.secrets...)
			dynamicClient := fakeCertManagerDynamicClient(nil, tt.staged)
			mockHelmClient := NewMockHelmClient(gomock.NewController(t))
			options := NewDefaultClusterOptions()
			for _, c := range []struct{ releaseName, ns string }{
				{options.Radius.ReleaseName, options.Radius.Namespace},
				{options.Contour.ReleaseName, options.Contour.Namespace},
			} {
				mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), c.releaseName, c.ns, true).Return(&release.UninstallReleaseResponse{}, nil)
			}
			mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, driver.ErrReleaseNotFound)
			var logs []string
			impl := &Impl{
				Helm:                        mockHelmClient,
				logger:                      func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
				removeDefaultContourGateway: (&defaultContourGatewayCalls{}).remove,
				newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return kubeClient, nil },
				newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return dynamicClient, nil },
			}
			setupUninstallStore(t, impl)
			require.NoError(t, impl.UninstallRadius(t.Context(), options, "test-context"))

			for _, name := range []string{ucpAPIServerCASecretName, ucpAPIServerTLSSecretName} {
				_, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
				if slices.Contains(tt.wantKept, name) {
					require.NoError(t, err, "uninstall must keep Secret %s", name)
				} else {
					require.True(t, apierrors.IsNotFound(err), "uninstall must delete Secret %s, got %v", name, err)
				}
			}
			require.Equal(t, slices.Contains(tt.wantKept, ucpOffCertificateSecretName),
				stageObjectExists(t, dynamicClient, corev1.SchemeGroupVersion.WithResource("secrets"), ucpOffCertificateSecretName))
			joined := strings.Join(logs, "\n")
			for _, want := range tt.wantLogged {
				require.Contains(t, joined, want)
			}
			if len(tt.wantLogged) == 0 {
				require.Empty(t, logs)
			}
		})
	}
}

// A rollback, or an upgrade, to a chart that still declares part of the radius-apiserver-ca chain, such as the
// radius-selfsigned-bootstrap Issuer of the parent chart, must not delete that part after Helm applies it.
func Test_deleteUnadoptedUCPAPIServerStage_KeepsDeclaredObjects(t *testing.T) {
	var objects []runtime.Object
	for _, staged := range ucpAPIServerCAStageObjects {
		objects = append(objects, stagedUCPAPIServerObject(staged.kind, staged.name, radiusReleaseName, nil))
	}
	dynamicClient := fakeCertManagerDynamicClient(nil, objects...)
	declared, err := manifestObjects("apiVersion: cert-manager.io/v1\nkind: Issuer\nmetadata:\n  name: " + radiusBootstrapIssuerName + "\n---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: ucp-cert\n")
	require.NoError(t, err)

	require.NoError(t, (&Impl{}).deleteUnadoptedUCPAPIServerStage(t.Context(), "test-context", dynamicClient, RadiusSystemNamespace, radiusReleaseName, declared))
	for _, staged := range ucpAPIServerCAStageObjects {
		require.Equal(t, staged.name == radiusBootstrapIssuerName, stageObjectExists(t, dynamicClient, staged.resource, staged.name), "%s %s", staged.kind, staged.name)
	}
}

// Helm removes the Radius release before rad deletes these objects, so a failed delete leaves them with no release. The
// failed uninstall still removes Contour, which a retry would skip. A retried uninstall lists the objects and resumes the
// cleanup; it never lists objects that other owners wrote.
func Test_RadiusUninstallLeftovers_ResumesFailedCleanup(t *testing.T) {
	issued := func(name, certificate string) *corev1.Secret {
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: RadiusSystemNamespace, UID: types.UID("uid-" + name), ResourceVersion: "1",
			Annotations: map[string]string{certificateNameAnnotation: certificate}}}
	}
	trust := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace, UID: "uid-trust", ResourceVersion: "1",
			Annotations: map[string]string{allowDirectInjectionAnnotation: "true"}},
		Data: map[string][]byte{caCertificateKey: []byte("ca")},
	}
	kubeClient := kubefake.NewSimpleClientset(issued(ucpAPIServerCASecretName, ucpAPIServerCASecretName), issued(ucpAPIServerTLSSecretName, ucpAPIServerCertificateName),
		issued("other-secret", "other"), trust)
	failDelete := true
	kubeClient.PrependReactor("delete", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if failDelete && action.(k8stesting.DeleteAction).GetName() == ucpAPIServerTLSSecretName {
			return true, nil, apierrors.NewServiceUnavailable("unavailable")
		}
		return false, nil, nil
	})
	staged := stagedUCPAPIServerObject("Issuer", radiusAPIServerIssuerName, radiusReleaseName, nil)
	foreign := stagedUCPAPIServerObject("Certificate", ucpAPIServerCASecretName, "", nil)
	require.NoError(t, unstructured.SetNestedField(foreign.Object, ucpAPIServerCASecretName, "spec", "secretName"))
	dynamicClient := fakeCertManagerDynamicClient(nil, staged, foreign)
	mockHelmClient := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Radius.ReleaseName, options.Radius.Namespace, true).Return(&release.UninstallReleaseResponse{}, nil)
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), options.Contour.ReleaseName, options.Contour.Namespace, true).Return(nil, errors.New("contour unavailable"))
	gateway := &defaultContourGatewayCalls{}
	impl := &Impl{
		Helm:                        mockHelmClient,
		logger:                      func(string, ...any) {},
		removeDefaultContourGateway: gateway.remove,
		newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return kubeClient, nil },
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return dynamicClient, nil },
	}
	setupUninstallStore(t, impl)
	uninstallErr := impl.UninstallRadius(t.Context(), options, "test-context")
	require.ErrorContains(t, uninstallErr, ucpAPIServerTLSSecretName)
	require.ErrorContains(t, uninstallErr, "contour unavailable")
	require.Equal(t, 1, gateway.removeCalls)

	leftovers, err := impl.RadiusUninstallLeftovers(t.Context(), options, "test-context")
	require.NoError(t, err)
	require.Equal(t, []string{"Secret radius-system/" + ucpAPIServerTLSSecretName}, leftoverNames(leftovers))

	mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), radiusReleaseName).Return(nil, driver.ErrReleaseNotFound)
	failDelete = false
	require.NoError(t, impl.DeleteRadiusUninstallLeftovers(t.Context(), "test-context", leftovers))
	leftovers, err = impl.RadiusUninstallLeftovers(t.Context(), options, "test-context")
	require.NoError(t, err)
	require.Empty(t, leftovers)
	_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), "other-secret", metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, stageObjectExists(t, dynamicClient, certManagerCertificatesResource, ucpAPIServerCASecretName))
}

func Test_RadiusUninstallLeftovers_ProviderGuard(t *testing.T) {
	c := newStageCluster(t, nil, false)
	impl := c.impl(NewMockHelmClient(gomock.NewController(t)))
	options := NewDefaultClusterOptions()
	confirmed := []UninstallLeftover{{Namespace: RadiusSystemNamespace, Kind: "Secret", Name: "test", uid: "old", resourceVersion: "1"}}
	_, err := c.kube.CoreV1().ConfigMaps(RadiusSystemNamespace).Create(t.Context(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: identityProviderJournal}}, metav1.CreateOptions{})
	require.NoError(t, err)
	c.kube.ClearActions()
	c.dynamic.ClearActions()
	_, err = impl.RadiusUninstallLeftovers(t.Context(), options, "test-context")
	require.ErrorContains(t, err, identityProviderJournal)
	require.ErrorContains(t, impl.DeleteRadiusUninstallLeftovers(t.Context(), "test-context", confirmed), identityProviderJournal)
	require.ErrorContains(t, impl.UninstallRadius(t.Context(), options, "test-context"), identityProviderJournal)
	requireOnlyCertificateGuardReads(t, c)
}

// A retried uninstall lists the Secret of a staged radius-apiserver-ca Certificate with it, because deleting the
// Certificate leaves the Secret unused, and deletes only what it listed: a Secret replaced while the user confirms
// survives. A Certificate that Helm keeps still writes the Secret, so neither is listed.
func Test_RadiusUninstallLeftovers_ListsCertificateSecret(t *testing.T) {
	t.Run("Certificate kept", func(t *testing.T) {
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerCASecretName, Namespace: RadiusSystemNamespace, UID: "uid-1",
			ResourceVersion: "1", Annotations: map[string]string{certificateNameAnnotation: ucpAPIServerCASecretName}}}
		kubeClient := kubefake.NewSimpleClientset(secret)
		certificate := stagedUCPAPIServerObject("Certificate", ucpAPIServerCASecretName, radiusReleaseName, map[string]string{helmResourcePolicyAnnotation: "keep"})
		require.NoError(t, unstructured.SetNestedField(certificate.Object, ucpAPIServerCASecretName, "spec", "secretName"))
		dynamicClient := fakeCertManagerDynamicClient(nil, certificate)
		impl := &Impl{
			logger:                      func(string, ...any) {},
			newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return kubeClient, nil },
			newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return dynamicClient, nil },
		}

		leftovers, err := impl.RadiusUninstallLeftovers(t.Context(), NewDefaultClusterOptions(), "test-context")
		require.NoError(t, err)
		require.Empty(t, leftovers)
	})
	for _, changed := range []string{"", "Secret", "Certificate"} {
		t.Run("changed="+changed, func(t *testing.T) {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerCASecretName, Namespace: RadiusSystemNamespace, UID: "uid-1",
				ResourceVersion: "1", Annotations: map[string]string{certificateNameAnnotation: ucpAPIServerCASecretName}}}
			kubeClient := kubefake.NewSimpleClientset(secret)
			kubeClient.PrependReactor("delete", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
				live, err := kubeClient.Tracker().Get(secretsResource, RadiusSystemNamespace, ucpAPIServerCASecretName)
				if err != nil {
					return false, nil, nil
				}
				preconditions := action.(k8stesting.DeleteAction).GetDeleteOptions().Preconditions
				if preconditions != nil && (preconditions.UID == nil || *preconditions.UID != live.(*corev1.Secret).UID) {
					return true, nil, apierrors.NewConflict(corev1.Resource("secrets"), ucpAPIServerCASecretName, errors.New("uid precondition failed"))
				}
				return false, nil, nil
			})
			cert := stagedUCPAPIServerObject("Certificate", ucpAPIServerCASecretName, radiusReleaseName, nil)
			require.NoError(t, unstructured.SetNestedField(cert.Object, ucpAPIServerCASecretName, "spec", "secretName"))
			dynamicClient := fakeCertManagerDynamicClient(nil, cert)
			impl := &Impl{
				logger:                      func(string, ...any) {},
				newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return kubeClient, nil },
				newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return dynamicClient, nil },
			}

			setupUninstallStore(t, impl)
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().RunHelmGet(gomock.Any(), radiusReleaseName).Return(nil, driver.ErrReleaseNotFound)
			impl.Helm = client
			leftovers, err := impl.RadiusUninstallLeftovers(t.Context(), NewDefaultClusterOptions(), "test-context")
			require.NoError(t, err)
			require.Equal(t, []string{"Certificate radius-system/" + ucpAPIServerCASecretName, "Secret radius-system/" + ucpAPIServerCASecretName}, leftoverNames(leftovers))

			late := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ucp-spiffe", Namespace: RadiusSystemNamespace, UID: "late", ResourceVersion: "2", Annotations: map[string]string{certificateNameAnnotation: "ucp-spiffe"}}}
			_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), late, metav1.CreateOptions{})
			require.NoError(t, err)
			switch changed {
			case "Secret":
				require.NoError(t, kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Delete(t.Context(), ucpAPIServerCASecretName, metav1.DeleteOptions{}))
				secret.UID = "uid-2"
				_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), secret, metav1.CreateOptions{})
				require.NoError(t, err)
			case "Certificate":
				certificate := stagedUCPAPIServerObject("Certificate", ucpAPIServerCASecretName, radiusReleaseName, nil)
				certificate.SetResourceVersion("2")
				require.NoError(t, unstructured.SetNestedField(certificate.Object, int64(2), "status", "revision"))
				require.NoError(t, dynamicClient.Tracker().Update(certManagerCertificatesResource, certificate, RadiusSystemNamespace))
			}
			require.NoError(t, impl.DeleteRadiusUninstallLeftovers(t.Context(), "test-context", leftovers))
			_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), late.Name, metav1.GetOptions{})
			require.NoError(t, err, "a newly eligible object was not confirmed")
			require.False(t, stageObjectExists(t, dynamicClient, certManagerCertificatesResource, ucpAPIServerCASecretName))
			_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerCASecretName, metav1.GetOptions{})
			require.Equal(t, changed != "", err == nil, "changed Secret or Secret behind a changed Certificate kept: %v", err)
		})
	}
}

func leftoverNames(leftovers []UninstallLeftover) []string {
	names := []string{}
	for _, leftover := range leftovers {
		names = append(names, leftover.String())
	}
	return names
}
