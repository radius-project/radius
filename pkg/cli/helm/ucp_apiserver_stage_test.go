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
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	kubefakehelm "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/radius-project/radius/pkg/authz"
)

var (
	stagedCertificatesResource = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	stagedIssuersResource      = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
)

// stagedCAObject is an object of the radius-apiserver-ca chain that rad must apply before Helm reissues ucp-cert.
type stagedCAObject struct {
	resource schema.GroupVersionResource
	kind     string
	name     string
}

var stagedCAObjects = []stagedCAObject{
	{resource: stagedIssuersResource, kind: "Issuer", name: RadiusBootstrapIssuerName},
	{resource: stagedCertificatesResource, kind: "Certificate", name: ucpAPIServerCASecretName},
	{resource: stagedIssuersResource, kind: "Issuer", name: RadiusAPIServerIssuerName},
}

// stageCluster is a cluster where UCP serves a certificate from the Helm-generated ucp-ca, and where cert-manager
// issues radius-apiserver-ca when its Certificate is created, if issueCA is set.
type stageCluster struct {
	kube        *kubefake.Clientset
	dynamic     *dynamicfake.FakeDynamicClient
	ucpCA       testCA
	apiServerCA testCA
	issueCA     bool
	prepare     func()
}

func newStageCluster(t *testing.T, apiService *unstructured.Unstructured, issueCA bool) *stageCluster {
	t.Helper()
	c := &stageCluster{
		kube:        kubefake.NewSimpleClientset(),
		ucpCA:       newTestCA(t, "ucp-ca"),
		apiServerCA: newTestCA(t, "radius-apiserver-ca"),
		issueCA:     issueCA,
	}
	var revision atomic.Uint64
	stamp := func(object runtime.Object, resource string) {
		metadata, err := meta.Accessor(object)
		require.NoError(t, err)
		version := fmt.Sprint(revision.Add(1))
		if metadata.GetUID() == "" {
			metadata.SetUID(types.UID("fixture-" + resource + "-" + metadata.GetName() + "-" + version))
		}
		metadata.SetResourceVersion(version)
		if resource == "apiservices" || resource == "deployments" {
			labels := metadata.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[helmManagedByLabel] = helmManagedByValue
			metadata.SetLabels(labels)
			annotations := metadata.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[helmReleaseNameAnnotation] = radiusReleaseName
			annotations[helmReleaseNamespaceAnnotation] = RadiusSystemNamespace
			metadata.SetAnnotations(annotations)
		}
	}
	require.NoError(t, c.kube.Tracker().Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: RadiusSystemNamespace, UID: "fixture-namespace", ResourceVersion: "1",
	}}))
	objects := []runtime.Object{}
	if apiService != nil {
		stamp(apiService, "apiservices")
		objects = append(objects, apiService)
	}
	c.dynamic = dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		apiServicesResource:        "APIServiceList",
		stagedCertificatesResource: "CertificateList",
		stagedIssuersResource:      "IssuerList",
	}, objects...)
	c.prepare = func() {
		object, err := c.dynamic.Tracker().Get(apiServicesResource, "", ucpAPIServiceName)
		if apierrors.IsNotFound(err) {
			return
		}
		require.NoError(t, err)
		metadata, err := meta.Accessor(object)
		require.NoError(t, err)
		if metadata.GetUID() == "" {
			stamp(object, "apiservices")
			require.NoError(t, c.dynamic.Tracker().Update(apiServicesResource, object, ""))
		}
	}
	stampMutation := func(action clienttesting.Action) (bool, runtime.Object, error) {
		if write, ok := action.(interface{ GetObject() runtime.Object }); ok {
			stamp(write.GetObject(), action.GetResource().Resource)
		}
		return false, nil, nil
	}
	c.kube.PrependReactor("*", "*", stampMutation)
	c.dynamic.PrependReactor("*", "*", stampMutation)
	issue := func(action clienttesting.Action) (bool, runtime.Object, error) {
		// UpdateAction has the same method set as CreateAction, so this matches both verbs.
		name := action.(clienttesting.CreateAction).GetObject().(*unstructured.Unstructured).GetName()
		if name == ucpAPIServerCASecretName && c.issueCA {
			c.issueAPIServerCA(t)
		}
		return false, nil, nil
	}
	c.dynamic.PrependReactor("create", "certificates", issue)
	c.dynamic.PrependReactor("update", "certificates", issue)
	return c
}

// offAPIService is the UCP APIService that a chart with authorization off renders: its caBundle holds ucp-ca, and
// the ca.crt of ucp-cert also holds the leaf, which rad must not trust.
func (c *stageCluster) offAPIService(t *testing.T) *unstructured.Unstructured {
	t.Helper()
	bundle := append(append([]byte{}, c.ucpCA.pem...), issueUCPCertificate(t, c.ucpCA)...)
	return ucpAPIService(base64.StdEncoding.EncodeToString(bundle), "True", "Passed")
}

func (c *stageCluster) issueAPIServerCA(t *testing.T) {
	t.Helper()
	_, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), apiServerCASecret(c.apiServerCA), metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
}

// helmReissuesUCPCertificate does what Helm and cert-manager do once Helm applies the chart with authorization on:
// cert-manager issues radius-apiserver-ca, reissues ucp-cert from radius-apiserver-issuer, and UCP rolls out.
func (c *stageCluster) helmReissuesUCPCertificate(t *testing.T) {
	t.Helper()
	c.issueAPIServerCA(t)
	secrets := c.kube.CoreV1().Secrets(RadiusSystemNamespace)
	_, err := secrets.Create(t.Context(), apiServerTLSSecret(t, c.apiServerCA), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = c.kube.AppsV1().Deployments(RadiusSystemNamespace).Create(t.Context(), ucpDeployment(true), metav1.CreateOptions{})
	require.NoError(t, err)
}

// offAPIServiceVariants are the APIServices an upgrade or rollback to authorization on can find while UCP serves a
// certificate from ucp-ca. After an earlier on→off cycle that stopped before ReconcileUCPOffAPIServerTrust, or that an
// older rad ran, Helm's off release keeps the inject-ca-from-secret annotation rad set, and cainjector may already have
// merged the leftover radius-apiserver-ca into the bundle.
var offAPIServiceVariants = []string{"off", "stale annotation, ucp-ca only", "stale annotation, leftover CA merged"}

func (c *stageCluster) addOffAPIService(t *testing.T, variant string) {
	t.Helper()
	apiService := c.offAPIService(t)
	switch variant {
	case "stale annotation, ucp-ca only":
		apiService.SetAnnotations(map[string]string{injectCAFromSecretAnnotation: ucpAPIServerCASource(RadiusSystemNamespace)})
	case "stale annotation, leftover CA merged":
		apiService.SetAnnotations(map[string]string{injectCAFromSecretAnnotation: ucpAPIServerCASource(RadiusSystemNamespace)})
		bundle := append(append([]byte{}, c.ucpCA.pem...), c.apiServerCA.pem...)
		require.NoError(t, unstructured.SetNestedField(apiService.Object, base64.StdEncoding.EncodeToString(bundle), "spec", "caBundle"))
		// cert-manager keeps the Secret after the off release removes its Certificate.
		c.issueAPIServerCA(t)
	}
	require.NoError(t, c.dynamic.Tracker().Add(apiService))
}

// cainjectorCASource makes the APIService look like one cert-manager already maintains from radius-apiserver-ca.
func (c *stageCluster) cainjectorAPIService() *unstructured.Unstructured {
	apiService := ucpAPIService(base64.StdEncoding.EncodeToString(c.apiServerCA.pem), "True", "Passed")
	apiService.SetAnnotations(map[string]string{injectCAFromSecretAnnotation: RadiusSystemNamespace + "/" + ucpAPIServerCASecretName})
	return apiService
}

func (c *stageCluster) impl(client HelmClient, certManagerControllerSA string) *Impl {
	c.prepare()
	return &Impl{
		Helm:           client,
		logger:         func(string, ...any) {},
		contourCluster: &fakeContourCluster{},
		newRotationStore: func(string, string) (*rotationStore, error) {
			return &rotationStore{kube: c.kube, dynamic: c.dynamic, namespace: RadiusSystemNamespace, release: radiusReleaseName, now: time.Now}, nil
		},
		newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
			kubeClient, apiextClient := fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")}.clients()
			return kubeClient, apiextClient, nil
		},
		newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) {
			controller := certManagerControllerWithServiceAccount(certManagerController("security", CertManagerChartDefaultVersion, ""), certManagerControllerSA)
			return sarClient(nil, nil, controller), nil
		},
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return c.dynamic, nil },
		newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return c.kube, nil },
		// Serving here is simulated by the fixture's rollout and certificate
		// checks. Exact-Pod TLS observation has separate tests.
		observeUCPOffServing: func(ctx context.Context, _, namespace string, kube kubernetes.Interface) error {
			secret, err := kube.CoreV1().Secrets(namespace).Get(ctx, ucpOffCertificateSecretName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			cas, err := caCertificates(secret.Data[caCertificateKey])
			if err != nil || len(cas) == 0 {
				return fmt.Errorf("fixture Off CA missing: %v", err)
			}
			leaf, problem, err := ucpServingCertificate(ctx, kube, namespace, ucpOffCertificateSecretName, "fixture Off CA", cas[0])
			if err != nil {
				return err
			}
			if leaf == nil {
				return errors.New(problem)
			}
			return nil
		},
	}
}

// certManagerWrites returns the create, update and patch requests rad sent for cert-manager objects.
func (c *stageCluster) certManagerWrites() []string {
	writes := []string{}
	for _, action := range c.dynamic.Actions() {
		if action.GetResource().Group != "cert-manager.io" || action.GetVerb() == "get" || action.GetVerb() == "list" || action.GetVerb() == "watch" {
			continue
		}
		writes = append(writes, action.GetVerb()+" "+action.GetResource().Resource)
	}
	return writes
}

// requireStagedCA checks, before Helm changes the release, that rad applied the radius-apiserver-ca chain as the
// chart renders it, owned by the Radius release so Helm adopts it, and that the API server trusts both ucp-ca, which
// the running UCP pods serve, and radius-apiserver-ca, which UCP serves once Helm reissues ucp-cert.
func (c *stageCluster) requireStagedCA(t *testing.T, rendered map[string]map[string]any) {
	t.Helper()
	for _, object := range stagedCAObjects {
		staged, err := c.dynamic.Resource(object.resource).Namespace(RadiusSystemNamespace).Get(t.Context(), object.name, metav1.GetOptions{})
		require.NoError(t, err, "rad must apply %s %s before Helm reissues ucp-cert", object.kind, object.name)
		require.Equal(t, "Helm", staged.GetLabels()["app.kubernetes.io/managed-by"], "%s %s", object.kind, object.name)
		require.Equal(t, "radius", staged.GetAnnotations()["meta.helm.sh/release-name"], "%s %s", object.kind, object.name)
		require.Equal(t, RadiusSystemNamespace, staged.GetAnnotations()["meta.helm.sh/release-namespace"], "%s %s", object.kind, object.name)
		require.Equal(t, rendered[object.kind+"/"+object.name], staged.Object["spec"], "%s %s must match the rendered chart", object.kind, object.name)
		for key, value := range rendered[object.kind+"/"+object.name+"#labels"] {
			require.Equal(t, value, staged.GetLabels()[key], "%s %s label %s", object.kind, object.name, key)
		}
	}
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, trustedSubjects(t, c.kube))
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
}

func (c *stageCluster) requireNotStaged(t *testing.T) {
	t.Helper()
	require.Empty(t, c.certManagerWrites(), "rad must not stage cert-manager objects")
	requireNoTrustSecret(t, c.kube)
}

func loadStageTestChart(t *testing.T) *chart.Chart {
	t.Helper()
	c, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	c.Files = slices.DeleteFunc(c.Files, func(file *common.File) bool {
		return strings.HasPrefix(file.Name, "crds/")
	})
	return c
}

// renderStageTestManifest renders the Radius chart with values as a fresh install does.
func renderStageTestManifest(t *testing.T, values map[string]any) string {
	t.Helper()
	install := helm.NewInstall(newTestRenderConfiguration(nil))
	install.DryRunStrategy = helm.DryRunServer
	install.ReleaseName = "radius"
	install.Namespace = RadiusSystemNamespace
	rendered, err := install.RunWithContext(t.Context(), loadStageTestChart(t), values)
	require.NoError(t, err)
	accessor, err := release.NewAccessor(rendered)
	require.NoError(t, err)
	return accessor.Manifest()
}

// renderedStagedCA returns the spec, by kind/name, and the labels, by kind/name#labels, of each object of the
// radius-apiserver-ca chain in manifest.
func renderedStagedCA(t *testing.T, manifest string) map[string]map[string]any {
	t.Helper()
	result := map[string]map[string]any{}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(&object.Object)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		for _, staged := range stagedCAObjects {
			if object.GetKind() == staged.kind && object.GetName() == staged.name {
				result[staged.kind+"/"+staged.name] = object.Object["spec"].(map[string]any)
				labels := map[string]any{}
				for key, value := range object.GetLabels() {
					labels[key] = value
				}
				result[staged.kind+"/"+staged.name+"#labels"] = labels
			}
		}
	}
	for _, staged := range stagedCAObjects {
		require.Contains(t, result, staged.kind+"/"+staged.name)
	}
	return result
}

var authzOnValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}

var authzDryRunValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}

// onUCP makes the cluster look like Radius with authorization on: cert-manager issued radius-apiserver-ca and
// ucp-cert into ucp-apiserver-tls, which the ucp Deployment mounts, and cainjector maintains the APIService.
func (c *stageCluster) onUCP(t *testing.T) {
	t.Helper()
	c.issueAPIServerCA(t)
	secrets := c.kube.CoreV1().Secrets(RadiusSystemNamespace)
	_, err := secrets.Create(t.Context(), apiServerTLSSecret(t, c.apiServerCA), metav1.CreateOptions{})
	require.NoError(t, err)
	deployment := ucpDeployment(true)
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: ucpCertificateVolumeName,
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ucpAPIServerTLSSecretName}}}}
	_, err = c.kube.AppsV1().Deployments(RadiusSystemNamespace).Create(t.Context(), deployment, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NoError(t, c.dynamic.Tracker().Add(c.cainjectorAPIService()))
}

// servedLeaf returns the certificate UCP serves from the Secret that the ucp Deployment mounts.
func (c *stageCluster) servedLeaf(t *testing.T) *x509.Certificate {
	t.Helper()
	deployment, err := c.kube.AppsV1().Deployments(RadiusSystemNamespace).Get(t.Context(), "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	name := ""
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == ucpCertificateVolumeName && volume.Secret != nil {
			name = volume.Secret.SecretName
		}
	}
	secret, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
	require.NoError(t, err, "the ucp Deployment mounts Secret %q", name)
	leaf, err := firstCertificate(secret.Data[corev1.TLSCertKey])
	require.NoError(t, err)
	return leaf
}

// requireTrusts checks that the APIService caBundle verifies leaf as UCP's serving certificate.
func (c *stageCluster) requireTrusts(t *testing.T, leaf *x509.Certificate, who string) {
	t.Helper()
	apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
	require.NoError(t, err)
	encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	for _, ca := range testCACertificates(t, bundle) {
		roots.AddCert(ca)
	}
	_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "ucp." + RadiusSystemNamespace + ".svc", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	require.NoError(t, err, "the API server must trust the certificate %s serve (issuer %s), but its caBundle holds %v",
		who, leaf.Issuer.CommonName, apiServiceCABundle(t, c.dynamic))
}

// helmAppliesOff does what Helm does when it applies a release with authorization off: it writes Secret ucp-cert and
// sets the APIService caBundle to the CA of that Secret, and then the ucp Deployment rolls out with ucp-cert mounted.
// As live, Helm cannot remove the inject-ca-from-secret annotation that rad set, so cainjector keeps merging the CA of
// the annotated Secret, while it exists, into the caBundle. target is the ucp-cert Secret and caBundle that a rollback
// applies from the target revision; for an upgrade it is nil and, as the chart does, Helm keeps a live ucp-cert or generates ucp-ca. It checks
// that the API server trusts the certificate UCP serves before and after the rollout.
func (c *stageCluster) helmAppliesOff(t *testing.T, target *corev1.Secret, caBundle []byte) {
	t.Helper()
	before := c.servedLeaf(t)
	secrets := c.kube.CoreV1().Secrets(RadiusSystemNamespace)
	switch existing, err := secrets.Get(t.Context(), ucpOffCertificateSecretName, metav1.GetOptions{}); {
	case target != nil && err == nil:
		existing.Data = target.Data
		_, err = secrets.Update(t.Context(), existing, metav1.UpdateOptions{})
		require.NoError(t, err)
	case target != nil:
		_, err = secrets.Create(t.Context(), target.DeepCopy(), metav1.CreateOptions{})
		require.NoError(t, err)
	case err == nil:
		caBundle = existing.Data[caCertificateKey]
	default:
		require.True(t, apierrors.IsNotFound(err))
		_, err = secrets.Create(t.Context(), tlsSecret(ucpOffCertificateSecretName, issueUCPCertificate(t, c.ucpCA), c.ucpCA.pem), metav1.CreateOptions{})
		require.NoError(t, err)
		caBundle = c.ucpCA.pem
	}
	apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(apiService.Object, base64.StdEncoding.EncodeToString(c.cainjectorMerges(t, apiService, caBundle)), "spec", "caBundle"))
	_, err = c.dynamic.Resource(apiServicesResource).Update(t.Context(), apiService, metav1.UpdateOptions{})
	require.NoError(t, err)
	c.requireTrusts(t, before, "the running UCP pods")

	deployments := c.kube.AppsV1().Deployments(RadiusSystemNamespace)
	deployment, err := deployments.Get(t.Context(), "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: ucpCertificateVolumeName,
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ucpOffCertificateSecretName}}}}
	_, err = deployments.Update(t.Context(), deployment, metav1.UpdateOptions{})
	require.NoError(t, err)
	c.requireTrusts(t, c.servedLeaf(t), "the UCP pods Helm rolls out")
}

// cainjectorMerges returns bundle with the ca.crt of the Secret that the inject-ca-from-secret annotation of
// apiService names, as cainjector leaves the caBundle after Helm applies bundle, when that Secret exists.
func (c *stageCluster) cainjectorMerges(t *testing.T, apiService *unstructured.Unstructured, bundle []byte) []byte {
	t.Helper()
	source := apiService.GetAnnotations()[injectCAFromSecretAnnotation]
	if source == "" {
		return bundle
	}
	namespace, name, _ := strings.Cut(source, "/")
	secret, err := c.kube.CoreV1().Secrets(namespace).Get(t.Context(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return bundle
	}
	require.NoError(t, err)
	return encodeCertificates(unionCertificates(testCACertificates(t, bundle), testCACertificates(t, secret.Data[caCertificateKey])))
}

// requireOffTrust checks the state rad leaves after authorization turns off: the APIService has none of the
// cert-manager inject annotations and its caBundle holds only the CA in Secret ucp-cert, which UCP serves, and the
// Secrets of the radius-apiserver-ca chain that only authorization on uses are gone.
func (c *stageCluster) requireOffTrust(t *testing.T) {
	t.Helper()
	apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
	require.NoError(t, err)
	for _, annotation := range []string{"cert-manager.io/inject-ca-from", injectCAFromSecretAnnotation, "cert-manager.io/inject-apiserver-ca"} {
		require.NotContains(t, apiService.GetAnnotations(), annotation, "rad must remove %s once authorization is off", annotation)
	}
	offCertificate, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpOffCertificateSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, pemSubjects(t, encodeCertificates(testCACertificates(t, offCertificate.Data[caCertificateKey]))), apiServiceCABundle(t, c.dynamic),
		"the caBundle must hold only the CA of Secret %s", ucpOffCertificateSecretName)
	c.requireTrusts(t, c.servedLeaf(t), "UCP")
	for _, name := range []string{ucpAPIServerCASecretName, ucpAPIServerTLSSecretName, ucpAPIServerTrustSecretName} {
		_, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err), "Secret %s must be removed once authorization is off, got %v", name, err)
	}
}

// stagedOffCertificate returns Secret ucp-cert as rad left it before Helm, or nil.
func (c *stageCluster) stagedOffCertificate(t *testing.T) *corev1.Secret {
	t.Helper()
	secret, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpOffCertificateSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return secret
}

// requireStagedOffCertificate checks that rad applied Secret ucp-cert before Helm, owned by the Radius release so Helm
// adopts it, and that its certificate is valid for UCP.
func requireStagedOffCertificate(t *testing.T, secret *corev1.Secret) {
	t.Helper()
	require.NotNil(t, secret, "rad must apply Secret %s before Helm turns authorization off", ucpOffCertificateSecretName)
	require.Equal(t, "Helm", secret.Labels["app.kubernetes.io/managed-by"])
	require.Equal(t, "radius", secret.Annotations["meta.helm.sh/release-name"])
	require.Equal(t, RadiusSystemNamespace, secret.Annotations["meta.helm.sh/release-namespace"])
	leaf, err := firstCertificate(secret.Data[corev1.TLSCertKey])
	require.NoError(t, err)
	cas := testCACertificates(t, secret.Data[caCertificateKey])
	require.Len(t, cas, 1)
	require.True(t, verifiesUCPCertificate(leaf, cas[0], RadiusSystemNamespace))
}

// renderedOffCertificate returns Secret ucp-cert and the APIService caBundle in a manifest with authorization off.
func renderedOffCertificate(t *testing.T, manifest string) (*corev1.Secret, []byte) {
	t.Helper()
	var secret *corev1.Secret
	var caBundle []byte
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(&object.Object)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch {
		case object.GetKind() == "Secret" && object.GetName() == ucpOffCertificateSecretName:
			secret = &corev1.Secret{}
			require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, secret))
		case object.GetKind() == "APIService":
			encoded, _, _ := unstructured.NestedString(object.Object, "spec", "caBundle")
			var err error
			caBundle, err = base64.StdEncoding.DecodeString(encoded)
			require.NoError(t, err)
		}
	}
	require.NotNil(t, secret)
	require.NotEmpty(t, caBundle)
	return secret, caBundle
}

// stageUpgrade is rad upgrade kubernetes, or rad install kubernetes --reinstall, from a deployed release with values
// deployed, with Helm storage in memory.
type stageUpgrade struct {
	configuration *helm.Configuration
	helmClient    HelmClient
	command       string
}

func newStageUpgrade(t *testing.T, deployed map[string]any, command string) *stageUpgrade {
	t.Helper()
	u := &stageUpgrade{
		configuration: &helm.Configuration{
			Releases:     storage.Init(driver.NewMemory()),
			KubeClient:   &kubefakehelm.PrintingKubeClient{Out: io.Discard},
			Capabilities: common.DefaultCapabilities,
		},
		helmClient: NewHelmClient(),
		command:    command,
	}
	if deployed != nil {
		_, err := u.helmClient.RunHelmInstall(u.configuration, loadStageTestChart(t), deployed, "radius", RadiusSystemNamespace, false, DefaultInstallTimeout, nil)
		require.NoError(t, err)
	}
	return u
}

// mock returns a Helm client that reads Radius from the in-memory storage and installs csi-driver-spiffe. It
// expects upgrades times Radius upgrades (installs for a fresh install) and calls beforeHelm before each.
func (u *stageUpgrade) mock(t *testing.T, upgrades int, beforeHelm func()) *MockHelmClient {
	t.Helper()
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ *helm.Configuration, name string) (*releasev1.Release, error) {
			return u.helmClient.RunHelmGet(u.configuration, name)
		})
	client.EXPECT().RunHelmHistory(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ *helm.Configuration, name string) ([]*releasev1.Release, error) {
			return u.helmClient.RunHelmHistory(u.configuration, name)
		})
	client.EXPECT().LoadChart(gomock.Any()).AnyTimes().
		DoAndReturn(func(string) (*chart.Chart, error) { return loadStageTestChart(t), nil })
	// rad installs csi-driver-spiffe only when authorization will be on.
	expectCSIDriverSPIFFEPull(t, client).AnyTimes()
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true, DefaultInstallTimeout, radManagedByLabels()).
		AnyTimes().Return(&releasev1.Release{}, nil)
	if u.command == "install" {
		client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), "radius", RadiusSystemNamespace, gomock.Any(), gomock.Any(), gomock.Any()).Times(upgrades).
			DoAndReturn(func(_ *helm.Configuration, c *chart.Chart, vals map[string]any, name, namespace string, _ bool, timeout time.Duration, labels map[string]string) (*releasev1.Release, error) {
				beforeHelm()
				return u.helmClient.RunHelmInstall(u.configuration, c, vals, name, namespace, false, timeout, labels)
			})
		return client
	}
	migration := expectControllerStrategyMigration(client, u.command != "upgrade --reset-values").Times(upgrades)
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), "radius", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).After(migration).Times(upgrades).
		DoAndReturn(func(_ *helm.Configuration, c *chart.Chart, vals map[string]any, name, namespace string, _ bool, reuse bool, timeout time.Duration) (*releasev1.Release, error) {
			beforeHelm()
			return u.helmClient.RunHelmUpgrade(u.configuration, c, vals, name, namespace, false, reuse, timeout)
		})
	return client
}

func (u *stageUpgrade) run(t *testing.T, impl *Impl, setArgs []string) error {
	t.Helper()
	state, err := impl.CheckRadiusInstall("test-context")
	require.NoError(t, err)
	options := NewDefaultClusterOptions()
	options.Radius.ChartPath = "chart"
	options.Radius.SetArgs = setArgs
	options.Contour.Disabled = true
	options.RadiusReleaseValues = state.RadiusValues
	switch u.command {
	case "upgrade":
		return impl.UpgradeRadius(t.Context(), options, "test-context")
	case "upgrade --reset-values":
		options.ResetValues = true
		return impl.UpgradeRadius(t.Context(), options, "test-context")
	case "install --reinstall":
		options.Radius.Reinstall = true
		return impl.InstallRadius(t.Context(), options, "test-context")
	default:
		return impl.InstallRadius(t.Context(), options, "test-context")
	}
}

func Test_UpgradeRadius_BlocksLegacyAggregationMigrationBeforeWrites(t *testing.T) {
	for _, command := range []string{"upgrade", "install --reinstall"} {
		for _, variant := range offAPIServiceVariants {
			t.Run(command+"/"+variant, func(t *testing.T) {
				shortenUCPAPIServiceWait(t)
				u := newStageUpgrade(t, map[string]any{}, command)
				c := newStageCluster(t, nil, true)
				c.addOffAPIService(t, variant)

				helmCalls := 0
				client := u.mock(t, 0, func() {
					helmCalls++
				})

				impl := c.impl(client, "cert-manager")
				before, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
				require.NoError(t, err)
				require.ErrorIs(t, u.run(t, impl, []string{"global.rbac.enabled=true"}), errCertificateObserverUnavailable)
				require.Zero(t, helmCalls)
				c.requireNotStaged(t)
				after, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, before, after)
				requireNoTrustSecret(t, c.kube)
			})
		}
	}

	// When an upgrade turns authorization off, rad stages Secret ucp-cert and moves UCP onto it while the API server
	// trusts both radius-apiserver-ca and ucp-ca, so the ucp-ca caBundle that Helm then applies matches what UCP serves.
	// With --reset-values rad drops the stored values, but the deployed release still has authorization on.
	for _, command := range []string{"upgrade", "upgrade --reset-values", "install --reinstall"} {
		for name, deployed := range map[string]map[string]any{"enforce": authzOnValues, "dryRun": authzDryRunValues} {
			t.Run(command+"/"+name+" to off", func(t *testing.T) {
				shortenUCPAPIServiceWait(t)
				u := newStageUpgrade(t, deployed, command)
				c := newStageCluster(t, nil, true)
				c.onUCP(t)

				helmCalls := 0
				client := u.mock(t, 1, func() {
					helmCalls++
					staged := c.stagedOffCertificate(t)
					c.helmAppliesOff(t, nil, nil)
					requireStagedOffCertificate(t, staged)
				})

				require.NoError(t, u.run(t, c.impl(client, "cert-manager"), []string{"global.rbac.enabled=false", "global.rbac.dryRun=false"}))
				require.Equal(t, 1, helmCalls)
				require.Equal(t, []string{"ucp-ca"}, apiServiceCABundle(t, c.dynamic))
				c.requireOffTrust(t)
			})
		}
	}
}

func Test_UpgradeRadius_RequiresIndependentPublicSource(t *testing.T) {
	t.Run("on to on", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		u := newStageUpgrade(t, authzOnValues, "upgrade")
		c := newStageCluster(t, nil, true)
		c.issueAPIServerCA(t)
		require.NoError(t, c.dynamic.Tracker().Add(c.cainjectorAPIService()))
		client := u.mock(t, 0, func() { t.Fatal("missing public source must stop Helm") })

		require.ErrorIs(t, u.run(t, c.impl(client, "cert-manager"), nil), errCertificateObserverUnavailable)
		c.requireNotStaged(t)
	})

}

// Without --reinstall, rad install leaves an installed release unchanged, so the authorization mode in the values it
// was given does not take effect: rad must not stage, wait for, or reconcile a CA the release does not use.
func Test_InstallRadius_InstalledReleaseKeepsAuthzMode(t *testing.T) {
	t.Run("off release with authorization on values", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		u := newStageUpgrade(t, map[string]any{}, "install")
		c := newStageCluster(t, nil, true)
		apiService := c.offAPIService(t)
		require.NoError(t, c.dynamic.Tracker().Add(apiService))

		impl := c.impl(u.mock(t, 0, func() {}), "cert-manager")
		apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		require.NoError(t, u.run(t, impl, []string{"global.rbac.enabled=true"}))
		require.Empty(t, c.clusterWrites())
		live, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, apiService.Object, live.Object)
	})

	t.Run("on release with authorization off values", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		u := newStageUpgrade(t, authzOnValues, "install")
		c := newStageCluster(t, nil, true)
		c.onUCP(t)
		// A ucp-cert Secret left from an earlier release with authorization off.
		_, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(),
			tlsSecret(ucpOffCertificateSecretName, issueUCPCertificate(t, c.ucpCA), c.ucpCA.pem), metav1.CreateOptions{})
		require.NoError(t, err)
		writes := len(c.clusterWrites())

		require.NoError(t, u.run(t, c.impl(u.mock(t, 0, func() {}), "cert-manager"), []string{"global.rbac.enabled=false", "global.rbac.dryRun=false"}))
		require.Empty(t, c.clusterWrites()[writes:])
		require.Equal(t, RadiusSystemNamespace+"/"+ucpAPIServerCASecretName, apiServiceCASource(t, c.dynamic))
		require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
	})
}

// (e): when cert-manager does not issue radius-apiserver-ca in time, rad stops before Helm, so UCP keeps serving
// ucp-cert from ucp-ca, which the API server still trusts. Running the command again after cert-manager issues the CA
// finishes the stage and the upgrade.
func Test_UpgradeRadius_LateIssuanceDoesNotAuthorizeMigration(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	u := newStageUpgrade(t, map[string]any{}, "upgrade")
	c := newStageCluster(t, nil, false)
	offAPIService := c.offAPIService(t)
	require.NoError(t, c.dynamic.Tracker().Add(offAPIService))

	err := u.run(t, c.impl(u.mock(t, 0, func() {}), "cert-manager"), []string{"global.rbac.enabled=true"})
	require.ErrorIs(t, err, errCertificateObserverUnavailable)
	apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, offAPIService.Object["spec"], apiService.Object["spec"], "the API server must keep trusting the CA UCP serves")
	deployed, err := u.helmClient.RunHelmGet(u.configuration, "radius")
	require.NoError(t, err)
	require.Equal(t, 1, deployed.Version, "rad must not call Helm")

	// Issuance alone is not public-source staging or process acknowledgment.
	c.issueCA = true
	c.issueAPIServerCA(t)
	helmCalls := 0
	client := u.mock(t, 0, func() {
		helmCalls++
	})
	require.ErrorIs(t, u.run(t, c.impl(client, "cert-manager"), []string{"global.rbac.enabled=true"}), errCertificateObserverUnavailable)
	require.Zero(t, helmCalls)
	c.requireNotStaged(t)
}

// Even Off-to-Off must exclude retained lifecycle intent before Helm, but it does
// not need to render a certificate transition.
func Test_StageUCPAPIServerCA_OffToOffChecksLifecycle(t *testing.T) {
	impl := &Impl{newUCPAPIServerTrustClient: func(string) (kubernetes.Interface, error) { return nil, errors.New("no cluster") }}
	render := func(context.Context) (string, bool, error) {
		t.Fatal("rad must not render the chart")
		return "", false, nil
	}
	require.ErrorContains(t, impl.stageUCPAPIServerCA(t.Context(), "test-context", RadiusSystemNamespace, "radius", authz.ModeOff, authz.ModeOff, render), "no cluster")
	for _, deployed := range []authz.Mode{authz.ModeEnforce, authz.ModeDryRun} {
		require.ErrorContains(t, impl.stageUCPAPIServerCA(t.Context(), "test-context", RadiusSystemNamespace, "radius", deployed, authz.ModeOff, render), "no cluster")
	}
}

// An earlier switch to authorization off that stopped after Helm, or ran an older rad, leaves the inject annotation and
// the radius-apiserver-ca chain behind. Running the command again with authorization off removes them.
func Test_UpgradeRadius_OffToOffRemovesLeftoverTrust(t *testing.T) {
	for _, command := range []string{"upgrade", "install --reinstall"} {
		t.Run(command, func(t *testing.T) {
			shortenUCPAPIServiceWait(t)
			u := newStageUpgrade(t, map[string]any{}, command)
			c := newStageCluster(t, nil, true)
			c.onUCP(t)
			c.helmAppliesOff(t, nil, nil)
			require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))

			client := u.mock(t, 1, func() { c.helmAppliesOff(t, nil, nil) })
			require.NoError(t, u.run(t, c.impl(client, "cert-manager"), []string{"global.rbac.enabled=false"}))
			c.requireOffTrust(t)
		})
	}
}

// setUCPRolledOut sets the status of the ucp Deployment to one that has, or has not, finished rolling out.
func (c *stageCluster) setUCPRolledOut(t *testing.T, rolledOut bool) {
	t.Helper()
	deployments := c.kube.AppsV1().Deployments(RadiusSystemNamespace)
	deployment, err := deployments.Get(t.Context(), "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	deployment.Status = ucpDeployment(rolledOut).Status
	_, err = deployments.UpdateStatus(t.Context(), deployment, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func Test_UpgradeRadius_StageUCPOffCertificateTimeout(t *testing.T) {
	expireUCPAPIServiceWait(t)
	u := newStageUpgrade(t, authzOnValues, "upgrade")
	c := newStageCluster(t, nil, true)
	c.onUCP(t)
	c.setUCPRolledOut(t, false)
	off := []string{"global.rbac.enabled=false", "global.rbac.dryRun=false"}

	err := u.run(t, c.impl(u.mock(t, 0, func() {}), "cert-manager"), off)
	require.ErrorContains(t, err, "UCP does not serve the certificate in Secret ucp-cert")
	require.ErrorContains(t, err, `kubectl --context "test-context" -n radius-system rollout status deployment/ucp`)
	require.Equal(t, []string{"radius-apiserver-ca", "ucp-ca"}, apiServiceCABundle(t, c.dynamic), "the API server must trust both CAs while pods move")
	deployed, err := u.helmClient.RunHelmGet(u.configuration, "radius")
	require.NoError(t, err)
	require.Equal(t, 1, deployed.Version, "rad must not call Helm")

	// The ucp Deployment template already mounts ucp-cert, but its pods have not rolled out, so some still serve the
	// radius-apiserver-ca certificate. A retry must wait for them again rather than let Helm drop radius-apiserver-ca.
	// The test render cannot look up the live ucp-cert, as rad's server-side render does, so the retry renders a new
	// ucp-ca; the rollback test checks that a retry with the same rendered Secret writes nothing.
	onSecret, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTLSSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	onLeaf, err := firstCertificate(onSecret.Data[corev1.TLSCertKey])
	require.NoError(t, err)
	err = u.run(t, c.impl(u.mock(t, 0, func() {}), "cert-manager"), off)
	require.ErrorContains(t, err, "UCP does not serve the certificate in Secret ucp-cert")
	c.requireTrusts(t, onLeaf, "the UCP pods that have not rolled out")
	c.requireTrusts(t, c.servedLeaf(t), "the UCP pods that rolled out")

	c.setUCPRolledOut(t, true)
	helmCalls := 0
	client := u.mock(t, 1, func() {
		helmCalls++
		c.helmAppliesOff(t, nil, nil)
	})
	require.NoError(t, u.run(t, c.impl(client, "cert-manager"), off))
	require.Equal(t, 1, helmCalls)
	require.Equal(t, []string{"ucp-ca"}, apiServiceCABundle(t, c.dynamic))
	c.requireOffTrust(t)
}

// (b): a rollback from a revision with authorization off to one with it on stages radius-apiserver-ca from the
// target revision's manifest before Helm rolls back, after the rollback issuer guard and the dependency reconcile.
func Test_RollbackRadius_RejectsLegacyTrustBeforeDependencyChanges(t *testing.T) {
	manifest := renderStageTestManifest(t, authzOnValues)
	history := func(targetManifest string) []*releasev1.Release {
		return []*releasev1.Release{
			{Version: 1, Config: authzOnValues, Manifest: targetManifest, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
			{Version: 2, Config: map[string]any{}, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
		}
	}
	impl := func(c *stageCluster, m *MockHelmClient) *Impl {
		return c.impl(m, "cert-manager")
	}

	for _, toRevision := range []bool{false, true} {
		name := "RollbackRadius"
		if toRevision {
			name = "RollbackRadiusToRevision"
		}
		for _, variant := range offAPIServiceVariants {
			t.Run(name+"/"+variant, func(t *testing.T) {
				shortenUCPAPIServiceWait(t)
				c := newStageCluster(t, nil, true)
				c.addOffAPIService(t, variant)
				m := NewMockHelmClient(gomock.NewController(t))
				m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history(manifest), nil)

				var err error
				if toRevision {
					err = impl(c, m).RollbackRadiusToRevision(t.Context(), "test-context", 1)
				} else {
					err = impl(c, m).RollbackRadius(t.Context(), "test-context")
				}
				require.ErrorIs(t, err, errCertificateObserverUnavailable)
				c.requireNotStaged(t)
				requireNoTrustSecret(t, c.kube)
			})
		}
	}

	// A rollback from a revision with authorization on to one with it off stages the target revision's Secret
	// ucp-cert and moves UCP onto it before Helm applies that revision's ucp-ca caBundle.
	offManifest := renderStageTestManifest(t, map[string]any{})
	offSecret, offCABundle := renderedOffCertificate(t, offManifest)
	for _, toRevision := range []bool{false, true} {
		name := "RollbackRadius"
		if toRevision {
			name = "RollbackRadiusToRevision"
		}
		for current, values := range map[string]map[string]any{"enforce": authzOnValues, "dryRun": authzDryRunValues} {
			t.Run(name+"/"+current+" to off", func(t *testing.T) {
				shortenUCPAPIServiceWait(t)
				c := newStageCluster(t, nil, true)
				c.onUCP(t)
				m := NewMockHelmClient(gomock.NewController(t))
				m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return([]*releasev1.Release{
					{Version: 1, Config: map[string]any{}, Manifest: offManifest, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
					{Version: 2, Config: values, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
				}, nil)
				m.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, driver.ErrReleaseNotFound)
				rollbacks := 0
				m.EXPECT().RunHelmRollback(gomock.Any(), radiusReleaseName, 1, true).DoAndReturn(func(*helm.Configuration, string, int, bool) error {
					rollbacks++
					staged := c.stagedOffCertificate(t)
					c.helmAppliesOff(t, offSecret, offCABundle)
					requireStagedOffCertificate(t, staged)
					require.Equal(t, offSecret.Data, staged.Data, "rad must stage the target revision's Secret %s", ucpOffCertificateSecretName)
					return nil
				})

				var err error
				if toRevision {
					err = impl(c, m).RollbackRadiusToRevision(t.Context(), "test-context", 1)
				} else {
					err = impl(c, m).RollbackRadius(t.Context(), "test-context")
				}
				require.NoError(t, err)
				require.Equal(t, 1, rollbacks)
				c.requireOffTrust(t)
			})
		}
	}

	// A rollback retried after UCP did not finish moving to the staged ucp-cert waits for the rollout again, though the
	// ucp Deployment template already mounts ucp-cert, and rolls back only once every pod serves it.
	t.Run("retry with an incomplete rollout", func(t *testing.T) {
		expireUCPAPIServiceWait(t)
		c := newStageCluster(t, nil, true)
		c.onUCP(t)
		c.setUCPRolledOut(t, false)
		releases := []*releasev1.Release{
			{Version: 1, Config: map[string]any{}, Manifest: offManifest, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
			{Version: 2, Config: authzOnValues, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
		}
		mock := func(rollbacks int, beforeHelm func()) *MockHelmClient {
			m := NewMockHelmClient(gomock.NewController(t))
			m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(releases, nil)
			m.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, driver.ErrReleaseNotFound)
			m.EXPECT().RunHelmRollback(gomock.Any(), radiusReleaseName, 1, true).Times(rollbacks).DoAndReturn(func(*helm.Configuration, string, int, bool) error {
				beforeHelm()
				return nil
			})
			return m
		}

		for range 2 {
			err := impl(c, mock(0, func() {})).RollbackRadius(t.Context(), "test-context")
			require.ErrorContains(t, err, "UCP does not serve the certificate in Secret ucp-cert")
			require.Equal(t, []string{"radius-apiserver-ca", "ucp-ca"}, apiServiceCABundle(t, c.dynamic), "the API server must trust both CAs while pods move")
		}

		c.setUCPRolledOut(t, true)
		rollbacks := 0
		writes := len(c.clusterWrites())
		require.NoError(t, impl(c, mock(1, func() {
			rollbacks++
			require.Len(t, c.clusterWrites(), writes+1, "a completed-stage retry only acquires its operation Lease before Helm")
			c.helmAppliesOff(t, offSecret, offCABundle)
		})).RollbackRadius(t.Context(), "test-context"))
		require.Equal(t, 1, rollbacks)
		c.requireOffTrust(t)
	})

	// The issuer guard refuses the target before rad stages anything.
	t.Run("refused target stages nothing", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		c := newStageCluster(t, nil, true)
		require.NoError(t, c.dynamic.Tracker().Add(c.offAPIService(t)))
		legacy := manifest + "\n---\n" + `apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ucp-mtls
  namespace: radius-system
spec:
  secretName: ucp-mtls
  issuerRef:
    name: radius-mtls-issuer
    kind: Issuer
`
		m := NewMockHelmClient(gomock.NewController(t))
		m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history(legacy), nil)

		err := impl(c, m).RollbackRadiusToRevision(t.Context(), "test-context", 1)
		require.ErrorContains(t, err, "Radius cannot roll back to revision 1")
		for _, action := range c.dynamic.Actions() {
			require.Equal(t, "get", action.GetVerb(), "a refused rollback must not mutate the cluster")
		}
		requireNoTrustSecret(t, c.kube)
	})
}
