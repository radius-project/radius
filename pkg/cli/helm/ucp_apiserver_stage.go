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
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/cli/output"
)

var (
	certManagerCertificatesResource = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	certManagerIssuersResource      = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
)

const (
	// radiusBootstrapIssuerName is the chart's self-signed Issuer, which signs the Radius CAs.
	radiusBootstrapIssuerName = "radius-selfsigned-bootstrap"
	// radiusAPIServerIssuerName is the chart's Issuer that signs UCP's aggregated API server certificate from
	// radius-apiserver-ca.
	radiusAPIServerIssuerName = "radius-apiserver-issuer"
)

// ucpAPIServerCAStageObjects are the objects of the chart that cert-manager needs to issue radius-apiserver-ca, in
// the order rad applies them: the self-signed Issuer, the CA Certificate, and the Issuer that signs ucp-cert from it.
var ucpAPIServerCAStageObjects = []stageInventoryObject{
	{kind: "Issuer", name: radiusBootstrapIssuerName, resource: certManagerIssuersResource},
	{kind: "Certificate", name: ucpAPIServerCASecretName, resource: certManagerCertificatesResource},
	{kind: "Issuer", name: radiusAPIServerIssuerName, resource: certManagerIssuersResource},
}

// stageInventoryObject is an object that rad may stage before Helm adopts it into the Radius release.
type stageInventoryObject struct {
	kind     string
	name     string
	resource schema.GroupVersionResource
}

// renderRadiusManifest returns the manifest of the Radius release that the next Helm call applies. ok is false when
// that call does not change the release, so there is nothing to stage.
type renderRadiusManifest func(ctx context.Context) (manifest string, ok bool, err error)

// renderOnce returns a renderRadiusManifest that calls render at most once and then returns its first result.
func renderOnce(render renderRadiusManifest) renderRadiusManifest {
	var done bool
	var manifest string
	var ok bool
	var err error
	return func(ctx context.Context) (string, bool, error) {
		if !done {
			manifest, ok, err = render(ctx)
			done = true
		}
		return manifest, ok, err
	}
}

// stageUCPOffCertificate moves UCP to the certificate that Helm applies when authorization turns off, before Helm
// applies it, while the Kubernetes API server trusts both the CA UCP serves now and the CA of that certificate.
//
// With authorization off, the chart serves UCP from Secret ucp-cert, signed by the Helm-generated ucp-ca, and sets the
// APIService caBundle to only ucp-ca. Helm applies that caBundle at once, but UCP serves a radius-apiserver-ca
// certificate until its pods roll out, so the API server would reject UCP in between. A rollback renders the chart of
// the target revision, which may predate this release, so the chart cannot avoid this by rendering both CAs. Instead,
// rad renders what Helm applies (see renderRadiusRollback), applies the rendered ucp-cert with the metadata Helm needs
// to adopt it, adds its CA to the caBundle, and mounts it in the ucp Deployment, then waits until every UCP pod serves
// it. Helm then keeps that Secret, because the chart it renders, for an upgrade or a rollback, reuses a live ucp-cert
// and its ucp-ca, so the caBundle Helm applies trusts what UCP serves.
//
// Like stageUCPAPIServerCA, it refuses an APIService that the Radius release does not own before it writes anything.
//
// It does nothing when the ucp Deployment or the APIService does not exist, the Deployment mounts no named certificate
// volume, or manifest has no ucp-cert Secret. If UCP does not serve the certificate in time, it returns an error before
// Helm changes anything, with both CAs trusted; running the command again checks the same stage and waits again, and
// writes nothing that already holds.
func (i *Impl) stageUCPOffCertificate(ctx context.Context, kubeContext, radiusNamespace, releaseName string, render renderRadiusManifest) error {
	ctx = withUCPMutationGuard(ctx, i.ucpMutationGuard)
	kubeClient, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}
	deployments := kubeClient.AppsV1().Deployments(radiusNamespace)
	deployment, err := deployments.Get(ctx, "ucp", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read Deployment %s/ucp: %w", radiusNamespace, err)
	}
	// When an earlier run already mounted ucp-cert, its pods may still be rolling out with the radius-apiserver-ca
	// certificate, so rad checks the stage and waits for them again before Helm drops that CA.
	mounted := ucpCertificateSecret(deployment)
	if mounted == "" {
		return nil
	}
	if !helmOwned(deployment, radiusNamespace, releaseName) {
		return foreignUCPDeploymentError(radiusNamespace, releaseName)
	}
	apiService, err := releaseUCPAPIService(ctx, kubeContext, dynamicClient, radiusNamespace, releaseName)
	if err != nil || apiService == nil {
		return err
	}
	live, err := apiServiceCACertificates(apiService)
	if err != nil || len(live) == 0 {
		return err
	}

	// The chart copies a live ucp-cert into the Secret it renders, so its CA would go into the caBundle below. rad
	// uses only one the release owns, and checks it again after rendering so the chart did not read another.
	before, err := releaseUCPOffCertificate(ctx, kubeContext, kubeClient, radiusNamespace, releaseName)
	if err != nil {
		return err
	}
	manifest, ok, err := render(ctx)
	if err != nil {
		return fmt.Errorf("failed to render the Radius chart to stage Secret %s: %w", ucpOffCertificateSecretName, err)
	}
	if !ok {
		return nil
	}
	after, err := releaseUCPOffCertificate(ctx, kubeContext, kubeClient, radiusNamespace, releaseName)
	if err != nil {
		return err
	}
	if (before == nil) != (after == nil) || before != nil && (before.UID != after.UID || before.ResourceVersion != after.ResourceVersion) {
		return fmt.Errorf("Secret %s/%s changed while rad rendered the Radius chart. Helm has not changed Radius; run the command again",
			radiusNamespace, ucpOffCertificateSecretName)
	}
	rendered, err := renderedSecret(manifest, ucpOffCertificateSecretName)
	if err != nil {
		return fmt.Errorf("failed to read the rendered Radius chart: %w", err)
	}
	if rendered == nil {
		return nil
	}
	target, err := caCertificates(rendered.Data[caCertificateKey])
	if err != nil {
		return fmt.Errorf("invalid rendered Secret %s ca.crt: %w", ucpOffCertificateSecretName, err)
	}
	if len(target) == 0 {
		return fmt.Errorf("the rendered Secret %s has no CA certificate in %s", ucpOffCertificateSecretName, caCertificateKey)
	}
	// Helm looks up each key of a live ucp-cert separately, so ca.crt may not be the CA that issued tls.crt. rad
	// checks the pair and finds its issuer in ca.crt before it writes anything.
	issuer, err := ucpOffCertificateIssuer(rendered, target, radiusNamespace)
	if err != nil {
		return fmt.Errorf("%w. Helm has not changed Radius; fix or delete Secret %s, then run the command again", err, ucpOffCertificateSecretName)
	}

	// cainjector may replace the caBundle with ca.crt of the Secret the APIService names rather than merge into it, so
	// both CAs go into the owned trust Secret and the APIService names it, before UCP moves. A trust Secret that rad
	// does not own is refused here, before anything changes.
	_, trust, err := readUCPAPIServerTrust(ctx, kubeClient, radiusNamespace)
	if err != nil {
		return err
	}
	if trust != nil && !ownsUCPAPIServerTrust(trust, radiusNamespace) {
		return overlapTrustError(trust, kubeContext, radiusNamespace, ucpOffCertificateSecretName)
	}
	overlap := unionCertificates(live, target)
	injected := false
	for _, annotation := range ucpInjectAnnotations {
		if _, ok := apiService.GetAnnotations()[annotation]; ok {
			injected = true
		}
	}

	output.LogInfo("Moving UCP to the certificate in Secret %s before authorization turns off...", ucpOffCertificateSecretName)
	// Without an inject annotation cainjector leaves the caBundle alone, so one that already trusts the CA of ucp-cert
	// needs no change; this is the state once authorization is off.
	if injected || !containsCertificates(live, target) {
		if _, err := writeUCPAPIServerTrust(ctx, kubeClient, radiusNamespace, overlap); err != nil {
			return err
		}
		if err := trustUCPAPIServerCAs(ctx, kubeContext, dynamicClient, apiService.GetUID(), radiusNamespace, releaseName, overlap,
			"both the current CA of UCP and the CA in Secret "+ucpOffCertificateSecretName); err != nil {
			return err
		}
	} else if err := settleStagedUCPTrust(ctx, kubeContext, dynamicClient, apiService.GetUID(), radiusNamespace, releaseName, overlap, "the Off CA", false); err != nil {
		return err
	}
	if err := applyHelmOwnedSecret(ctx, kubeContext, kubeClient, rendered, after, releaseName, radiusNamespace); err != nil {
		return err
	}
	// UCP reads its certificate only when it starts, and the rollout counters of a Deployment whose template already
	// mounts ucp-cert say nothing about which ucp-cert its pods read. A pod-template annotation with the hash of the
	// certificate rad applied makes the Deployment roll out again whenever that certificate changes, so the wait below
	// passes only once every pod started with it.
	certificateHash := ucpCertificateHash(rendered)
	if mounted != ucpOffCertificateSecretName || deployment.Spec.Template.Annotations[ucpCertificateHashAnnotation] != certificateHash {
		// The waits above let the Deployment's status change, so rad reads it again and patches only the Deployment it
		// inspected, at the resourceVersion it checked.
		current, err := deployments.Get(ctx, "ucp", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to read Deployment %s/ucp: %w", radiusNamespace, err)
		}
		if current.UID != deployment.UID || !helmOwned(current, radiusNamespace, releaseName) {
			return foreignUCPDeploymentError(radiusNamespace, releaseName)
		}
		patch, err := json.Marshal(map[string]any{
			"metadata": map[string]any{"uid": current.UID, "resourceVersion": current.ResourceVersion},
			"spec": map[string]any{"template": map[string]any{
				"metadata": map[string]any{"annotations": map[string]any{ucpCertificateHashAnnotation: certificateHash}},
				"spec": map[string]any{
					"volumes": []any{map[string]any{"name": ucpCertificateVolumeName, "secret": map[string]any{"secretName": ucpOffCertificateSecretName}}},
				},
			}},
		})
		if err != nil {
			return err
		}
		if _, err := checkUCPMutation(ctx, "patch", "Deployment", current); err != nil {
			return err
		}
		if _, err := deployments.Patch(ctx, "ucp", types.StrategicMergePatchType, patch, metav1.PatchOptions{FieldManager: FieldManager}); err != nil {
			return fmt.Errorf("failed to mount Secret %s in Deployment %s/ucp: %w", ucpOffCertificateSecretName, radiusNamespace, err)
		}
	}

	problem, err := pollUCPAPIServer(ctx, func(ctx context.Context) (string, error) {
		_, problem, err := ucpServingCertificate(ctx, kubeClient, radiusNamespace, ucpOffCertificateSecretName, "the CA in its "+caCertificateKey, issuer)
		return problem, err
	})
	if err != nil {
		if wait.Interrupted(err) {
			return fmt.Errorf("UCP does not serve the certificate in Secret %s after %s: %s. Helm has not changed the Radius release and the API server trusts both CAs; "+
				"run the command again once %s succeeds",
				ucpOffCertificateSecretName, ucpAPIServiceTimeout, problem, kubectlCommand(kubeContext, radiusNamespace, "rollout status deployment/ucp"))
		}
		return err
	}
	return nil
}

// ucpCertificateHash returns the SHA-256 of the certificate chain in secret, which rad sets as annotation
// ucpCertificateHashAnnotation on the pod template of the ucp Deployment.
func ucpCertificateHash(secret *corev1.Secret) string {
	sum := sha256.Sum256(secret.Data[corev1.TLSCertKey])
	return hex.EncodeToString(sum[:])
}

// releaseUCPOffCertificate returns the live Secret ucp-cert, or nil if it does not exist, or an error if it does not
// belong to Helm release releaseName, as applyHelmOwnedSecret requires, so rad never trusts the CA of another owner.
func releaseUCPOffCertificate(ctx context.Context, kubeContext string, client kubernetes.Interface, radiusNamespace, releaseName string) (*corev1.Secret, error) {
	secret, err := client.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpOffCertificateSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpOffCertificateSecretName, err)
	}
	if !helmOwned(secret, radiusNamespace, releaseName) {
		return nil, foreignUCPOffCertificateError(kubeContext, radiusNamespace, releaseName)
	}
	return secret, nil
}

// foreignUCPOffCertificateError reports that Secret ucp-cert is not part of Helm release releaseName, so rad does not
// trust its CA.
func foreignUCPOffCertificateError(kubeContext, radiusNamespace, releaseName string) error {
	return fmt.Errorf("Secret %s/%s exists but is not part of Helm release %s/%s, so rad will not trust its CA. Helm has not changed Radius. "+
		"Delete it with %s if Radius should manage it, then run the command again",
		radiusNamespace, ucpOffCertificateSecretName, radiusNamespace, releaseName,
		kubectlCommand(kubeContext, radiusNamespace, "delete secret "+ucpOffCertificateSecretName))
}

// ucpOffCertificateIssuer returns the CA of cas that the certificate in rendered, the Secret ucp-cert that Helm applies,
// chains to for UCP's Service name, through the intermediates after it in tls.crt, or an error if its tls.key does not match it or no CA of cas issued it.
func ucpOffCertificateIssuer(rendered *corev1.Secret, cas []*x509.Certificate, radiusNamespace string) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair(rendered.Data[corev1.TLSCertKey], rendered.Data[corev1.TLSPrivateKeyKey])
	if err != nil {
		return nil, fmt.Errorf("the rendered Secret %s does not hold a valid key pair: %w", ucpOffCertificateSecretName, err)
	}
	chain := make([]*x509.Certificate, 0, len(pair.Certificate))
	for _, der := range pair.Certificate {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("the rendered Secret %s does not hold a valid certificate: %w", ucpOffCertificateSecretName, err)
		}
		chain = append(chain, cert)
	}
	for _, ca := range cas {
		if verifiesUCPCertificate(chain[0], chain[1:], ca, radiusNamespace) {
			return ca, nil
		}
	}
	return nil, fmt.Errorf("no CA in %s of the rendered Secret %s issued its certificate for ucp.%s.svc", caCertificateKey, ucpOffCertificateSecretName, radiusNamespace)
}

// foreignUCPDeploymentError reports that the ucp Deployment is not the one the Radius release owns, so rad refuses to
// mount a Secret in it.
func foreignUCPDeploymentError(radiusNamespace, releaseName string) error {
	return fmt.Errorf("Deployment %s/ucp does not belong to Helm release %q, or was replaced after rad read it, so rad did not mount Secret %s in it. "+
		"Helm has not changed Radius; check the Deployment, then run the command again", radiusNamespace, releaseName, ucpOffCertificateSecretName)
}

// apiServiceCACertificates returns the CA certificates in the caBundle of apiService.
func apiServiceCACertificates(apiService *unstructured.Unstructured) ([]*x509.Certificate, error) {
	encoded, _, err := unstructured.NestedString(apiService.Object, "spec", "caBundle")
	if err != nil {
		return nil, fmt.Errorf("invalid APIService caBundle: %w", err)
	}
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode the caBundle of APIService %s: %w", ucpAPIServiceName, err)
	}
	cas, err := caCertificates(bundle)
	if err != nil {
		return nil, fmt.Errorf("invalid APIService trust bundle: %w", err)
	}
	return cas, nil
}

// releaseUCPAPIService returns the UCP APIService when the Radius release releaseName in radiusNamespace owns it and it
// routes to Service ucp in radiusNamespace, or nil when it does not exist. The APIService is cluster-scoped, so another
// Radius release, in another namespace, may own it; rad refuses to change it before Helm in that case rather than
// redirect the trust of another installation.
func releaseUCPAPIService(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, radiusNamespace, releaseName string) (*unstructured.Unstructured, error) {
	apiService, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read APIService %s: %w", ucpAPIServiceName, err)
	}
	if problem := foreignUCPAPIServiceProblem(apiService, radiusNamespace, releaseName); problem != "" {
		return nil, fmt.Errorf("APIService %s %s, so rad will not change it. Helm has not changed Radius. "+
			"Check which installation owns it with %s",
			ucpAPIServiceName, problem, kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml"))
	}
	return apiService, nil
}

// foreignUCPAPIServiceProblem describes why apiService does not belong to the Radius release releaseName in
// radiusNamespace, or returns "" when it does.
func foreignUCPAPIServiceProblem(apiService *unstructured.Unstructured, radiusNamespace, releaseName string) string {
	if !helmOwned(apiService, radiusNamespace, releaseName) {
		annotations := apiService.GetAnnotations()
		return fmt.Sprintf("is not part of Helm release %s/%s (it names release %q in namespace %q)",
			radiusNamespace, releaseName, annotations[helmReleaseNameAnnotation], annotations[helmReleaseNamespaceAnnotation])
	}
	namespace, _, _ := unstructured.NestedString(apiService.Object, "spec", "service", "namespace")
	name, _, _ := unstructured.NestedString(apiService.Object, "spec", "service", "name")
	if namespace != radiusNamespace || name != "ucp" {
		return fmt.Sprintf("routes to Service %s/%s rather than %s/%s", namespace, name, radiusNamespace, "ucp")
	}
	port, found, err := unstructured.NestedInt64(apiService.Object, "spec", "service", "port")
	if err != nil || (found && port != 443) {
		return "does not route to the ucp Service on port 443"
	}
	return ""
}

// trustUCPAPIServerCAs points the inject-ca-from-secret annotation of the UCP APIService at the radius-apiserver-trust
// Secret in radiusNamespace and sets its caBundle to cas in one patch, then waits until both have held for
// ucpAPIServerCASettleChecks polls in a row, patching again if cainjector writes back a caBundle it read earlier. trusts
// describes cas in the timeout error. uid is the UID of the APIService that releaseUCPAPIService checked. On every
// read, rad stops if the APIService was replaced, no longer belongs to Helm release releaseName in radiusNamespace, or
// no longer routes to Service ucp in radiusNamespace. Each patch carries the uid and resourceVersion rad read, so the
// API server rejects it if the APIService changed in between, and rad reads it again.
func trustUCPAPIServerCAs(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, uid types.UID, radiusNamespace, releaseName string, cas []*x509.Certificate, trusts string) error {
	return settleStagedUCPTrust(ctx, kubeContext, dynamicClient, uid, radiusNamespace, releaseName, cas, trusts, true)
}

func settleStagedUCPTrust(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, uid types.UID, radiusNamespace, releaseName string, cas []*x509.Certificate, trusts string, inject bool) error {
	source := ucpAPIServerTrustSource(radiusNamespace)
	caBundle := base64.StdEncoding.EncodeToString(encodeCertificates(cas))
	check := kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml")
	held := 0
	err := wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		apiService, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to read APIService %s: %w", ucpAPIServiceName, err)
		}
		if apiService.GetUID() != uid {
			return false, fmt.Errorf("APIService %s was replaced while rad updated its caBundle. Helm has not changed the Radius release; check it with %s, then run the command again",
				ucpAPIServiceName, check)
		}
		if problem := foreignUCPAPIServiceProblem(apiService, radiusNamespace, releaseName); problem != "" {
			return false, fmt.Errorf("APIService %s %s, so rad stopped updating its caBundle. Helm has not changed the Radius release; "+
				"check which installation owns it with %s, then run the command again", ucpAPIServiceName, problem, check)
		}
		live, err := apiServiceCACertificates(apiService)
		if err != nil {
			return false, err
		}
		wantSource := source
		if !inject {
			wantSource = ""
		}
		if apiService.GetAnnotations()[injectCAFromSecretAnnotation] == wantSource && apiService.GetAnnotations()["cert-manager.io/inject-ca-from"] == "" && apiService.GetAnnotations()["cert-manager.io/inject-apiserver-ca"] == "" && containsCertificates(live, cas) {
			held++
			return held >= ucpAPIServerCASettleChecks, nil
		}
		held = 0
		if !inject {
			return false, fmt.Errorf("APIService %s trust changed while staging the Off certificate; check it with %s, then run the command again", ucpAPIServiceName, check)
		}
		patch, err := json.Marshal(map[string]any{
			"metadata": map[string]any{
				"uid":             uid,
				"resourceVersion": apiService.GetResourceVersion(),
				"annotations":     map[string]any{injectCAFromSecretAnnotation: source, "cert-manager.io/inject-ca-from": nil, "cert-manager.io/inject-apiserver-ca": nil},
			},
			"spec": map[string]any{"caBundle": caBundle},
		})
		if err != nil {
			return false, err
		}
		if _, err := checkUCPMutation(ctx, "patch", "APIService", apiService); err != nil {
			return false, err
		}
		_, err = dynamicClient.Resource(apiServicesResource).Patch(ctx, ucpAPIServiceName, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: FieldManager})
		// The APIService changed since rad read it: read and check it again rather than overwrite that change.
		if apierrors.IsConflict(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to update the caBundle of APIService %s: %w", ucpAPIServiceName, err)
		}
		return false, nil
	})
	if wait.Interrupted(err) {
		return fmt.Errorf("the caBundle of APIService %s does not trust %s through Secret %s after %s. Helm has not changed the Radius release; check it with %s, then run the command again",
			ucpAPIServiceName, trusts, source, ucpAPIServiceTimeout, kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml"))
	}
	return err
}

// renderedSecret returns the named Secret in manifest, or nil when absent.
func renderedSecret(manifest, name string) (*corev1.Secret, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var document map[string]any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		object := &unstructured.Unstructured{Object: document}
		if document == nil || object.GetAPIVersion() != "v1" || object.GetKind() != "Secret" || object.GetName() != name {
			continue
		}
		secret := &corev1.Secret{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(document, secret); err != nil {
			return nil, err
		}
		return secret, nil
	}
}

// applyHelmOwnedSecret is applyHelmOwnedObject for a Secret, whose content is in data rather than spec. It writes over
// existing, the Secret as rad read it before rendering, or creates the Secret when existing is nil, so it fails rather
// than replace a Secret that changed since, such as a newer key pair of the same release.
func applyHelmOwnedSecret(ctx context.Context, kubeContext string, client kubernetes.Interface, rendered, existing *corev1.Secret, releaseName, releaseNamespace string) error {
	secrets := client.CoreV1().Secrets(releaseNamespace)
	changed := func() error {
		return fmt.Errorf("Secret %s/%s changed after rad rendered the Radius chart. Helm has not changed Radius; run the command again",
			releaseNamespace, rendered.Name)
	}
	if existing == nil {
		desired := rendered.DeepCopy()
		desired.Namespace = releaseNamespace
		desired.ResourceVersion = ""
		setHelmOwnership(&desired.ObjectMeta, releaseName, releaseNamespace)
		if _, err := checkUCPMutation(ctx, "create", "Secret", desired); err != nil {
			return err
		}
		_, err := secrets.Create(ctx, desired, metav1.CreateOptions{FieldManager: FieldManager})
		if apierrors.IsAlreadyExists(err) {
			return changed()
		}
		if err != nil {
			return fmt.Errorf("failed to create Secret %s/%s: %w", releaseNamespace, rendered.Name, err)
		}
		return ctx.Err()
	}
	if existing.Labels[helmManagedByLabel] != helmManagedByValue || existing.Annotations[helmReleaseNameAnnotation] != releaseName ||
		existing.Annotations[helmReleaseNamespaceAnnotation] != releaseNamespace {
		return fmt.Errorf("Secret %s/%s exists but is not part of Helm release %s/%s, so Helm cannot adopt it. Delete it with %s if Radius should manage it, then run the command again",
			releaseNamespace, rendered.Name, releaseNamespace, releaseName,
			kubectlCommand(kubeContext, releaseNamespace, "delete secret "+rendered.Name))
	}
	updated := existing.DeepCopy()
	updated.Data = rendered.Data
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	maps.Copy(updated.Labels, rendered.Labels)
	setHelmOwnership(&updated.ObjectMeta, releaseName, releaseNamespace)
	if equality.Semantic.DeepEqual(existing, updated) {
		// The pair is unchanged since rad read it; a later change would have failed the update below, so check it now.
		current, err := secrets.Get(ctx, rendered.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to read Secret %s/%s: %w", releaseNamespace, rendered.Name, err)
		}
		if current.UID != existing.UID || current.ResourceVersion != existing.ResourceVersion {
			return changed()
		}
		return nil
	}
	if _, err := checkUCPMutation(ctx, "update", "Secret", existing); err != nil {
		return err
	}
	_, err := secrets.Update(ctx, updated, metav1.UpdateOptions{FieldManager: FieldManager})
	if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
		return changed()
	}
	if err != nil {
		return fmt.Errorf("failed to update Secret %s/%s: %w", releaseNamespace, rendered.Name, err)
	}
	return ctx.Err()
}

// setHelmOwnership sets the metadata Helm checks before it adopts an existing object into release releaseName.
func setHelmOwnership(meta *metav1.ObjectMeta, releaseName, releaseNamespace string) {
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	meta.Labels[helmManagedByLabel] = helmManagedByValue
	if meta.Annotations == nil {
		meta.Annotations = map[string]string{}
	}
	meta.Annotations[helmReleaseNameAnnotation] = releaseName
	meta.Annotations[helmReleaseNamespaceAnnotation] = releaseNamespace
}

type stagedObject struct {
	resource schema.GroupVersionResource
	object   *unstructured.Unstructured
}

// ucpAPIServerCAStage returns the objects of ucpAPIServerCAStageObjects in manifest, in order, or none when manifest
// has no radius-apiserver-ca Certificate.
func ucpAPIServerCAStage(manifest string) ([]stagedObject, error) {
	found := map[string]*unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var document map[string]any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if document == nil {
			continue
		}
		object := &unstructured.Unstructured{Object: document}
		if object.GroupVersionKind().Group != "cert-manager.io" {
			continue
		}
		found[object.GetKind()+"/"+object.GetName()] = object
	}
	if found["Certificate/"+ucpAPIServerCASecretName] == nil {
		return nil, nil
	}

	objects := []stagedObject{}
	for _, staged := range ucpAPIServerCAStageObjects {
		if object := found[staged.kind+"/"+staged.name]; object != nil {
			objects = append(objects, stagedObject{resource: staged.resource, object: object})
		}
	}
	return objects, nil
}

// applyHelmOwnedObject creates rendered in releaseNamespace, or updates the object that an earlier stage created, with
// the metadata Helm checks before it adopts an existing object into release releaseName. It refuses an object that
// another owner manages, because Helm would refuse to adopt it too.
func applyHelmOwnedObject(ctx context.Context, kubeContext string, client dynamic.Interface, resource schema.GroupVersionResource, rendered *unstructured.Unstructured, releaseName, releaseNamespace string) error {
	desired := rendered.DeepCopy()
	desired.SetNamespace(releaseNamespace)
	labels := maps.Clone(desired.GetLabels())
	if labels == nil {
		labels = map[string]string{}
	}
	labels[helmManagedByLabel] = helmManagedByValue
	desired.SetLabels(labels)
	annotations := maps.Clone(desired.GetAnnotations())
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[helmReleaseNameAnnotation] = releaseName
	annotations[helmReleaseNamespaceAnnotation] = releaseNamespace
	desired.SetAnnotations(annotations)

	objects := client.Resource(resource).Namespace(releaseNamespace)
	existing, err := objects.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := checkUCPMutation(ctx, "create", rendered.GetKind(), desired); err != nil {
			return err
		}
		if _, err := objects.Create(ctx, desired, metav1.CreateOptions{FieldManager: FieldManager}); err != nil {
			return fmt.Errorf("failed to create %s %s/%s: %w", desired.GetKind(), releaseNamespace, desired.GetName(), err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read %s %s/%s: %w", desired.GetKind(), releaseNamespace, desired.GetName(), err)
	}

	if existing.GetLabels()[helmManagedByLabel] != helmManagedByValue || existing.GetAnnotations()[helmReleaseNameAnnotation] != releaseName ||
		existing.GetAnnotations()[helmReleaseNamespaceAnnotation] != releaseNamespace {
		return fmt.Errorf("%s %s/%s exists but is not part of Helm release %s/%s, so Helm cannot adopt it. Delete it with %s if Radius should manage it, then run the command again",
			desired.GetKind(), releaseNamespace, desired.GetName(), releaseNamespace, releaseName,
			kubectlCommand(kubeContext, releaseNamespace, "delete "+strings.ToLower(desired.GetKind())+" "+desired.GetName()))
	}
	updated := existing.DeepCopy()
	updated.Object["spec"] = desired.Object["spec"]
	mergedLabels := updated.GetLabels()
	maps.Copy(mergedLabels, desired.GetLabels())
	updated.SetLabels(mergedLabels)
	mergedAnnotations := updated.GetAnnotations()
	maps.Copy(mergedAnnotations, desired.GetAnnotations())
	updated.SetAnnotations(mergedAnnotations)
	if _, err := checkUCPMutation(ctx, "update", rendered.GetKind(), existing); err != nil {
		return err
	}
	if _, err := objects.Update(ctx, updated, metav1.UpdateOptions{FieldManager: FieldManager}); err != nil {
		return fmt.Errorf("failed to update %s %s/%s: %w", desired.GetKind(), releaseNamespace, desired.GetName(), err)
	}
	return nil
}

// containsCertificates reports whether certs holds every certificate in want.
func containsCertificates(certs, want []*x509.Certificate) bool {
	for _, w := range want {
		found := false
		for _, c := range certs {
			if c.Equal(w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// renderRadiusUpgrade renders the Radius upgrade that RunHelmUpgrade applies with values and reuseValues, as a
// server-side dry run against a copy of the release history, so lookups and capabilities match the real upgrade.
func (i *Impl) renderRadiusUpgrade(kubeContext string, helmConf *helm.Configuration, helmChart *chart.Chart, values map[string]any, releaseName, namespace string, reuseValues bool) renderRadiusManifest {
	return func(ctx context.Context) (string, bool, error) {
		history, err := i.Helm.RunHelmHistory(helmConf, releaseName)
		if err != nil {
			return "", false, err
		}
		manifest, err := i.dryRunRadiusUpgrade(ctx, kubeContext, history, helmChart, values, releaseName, namespace, reuseValues, false)
		return manifest, err == nil, err
	}
}

// dryRunRadiusUpgrade returns the manifest of an upgrade of releaseName, with history, to helmChart and values, as a
// server-side dry run with the configuration that RenderConfiguration returns: it reads the capabilities of the
// cluster, its lookups read the live objects, and it changes neither the cluster nor history.
func (i *Impl) dryRunRadiusUpgrade(ctx context.Context, kubeContext string, history []*releasev1.Release, helmChart *chart.Chart, values map[string]any, releaseName, namespace string, reuseValues, disableHooks bool) (string, error) {
	configuration, err := i.cluster().RenderConfiguration(kubeContext, namespace)
	if err != nil {
		return "", err
	}
	named := make([]*releasev1.Release, 0, len(history))
	for _, revision := range history {
		if revision == nil {
			continue
		}
		copied := *revision
		if copied.Name == "" {
			copied.Name = releaseName
		}
		if copied.Namespace == "" {
			copied.Namespace = namespace
		}
		named = append(named, &copied)
	}
	releases, err := releaseHistoryStorage(named)
	if err != nil {
		return "", err
	}
	configuration.Releases = releases
	client := newUpgradeClient(configuration, namespace, false, reuseValues, 0, usesServerSideApply(configuration, releaseName))
	client.DisableHooks = disableHooks
	client.DryRunStrategy = helm.DryRunServer
	upgraded, err := client.RunWithContext(ctx, releaseName, cloneChart(helmChart), copyValues(values))
	if err != nil {
		return "", err
	}
	rendered, err := asRelease(upgraded)
	if err != nil {
		return "", err
	}
	return releaseActionManifest(rendered, !disableHooks), nil
}

// renderRadiusInstall renders what ApplyHelmChart applies: a fresh install, or with Reinstall an upgrade that reuses
// the stored values. An installed release without Reinstall is left unchanged, so nothing is rendered.
func (i *Impl) renderRadiusInstall(kubeContext string, helmAction HelmAction, helmConf *helm.Configuration, helmChart *chart.Chart, values map[string]any, options ChartOptions) renderRadiusManifest {
	return func(ctx context.Context) (string, bool, error) {
		installed, _, err := helmAction.QueryRelease(kubeContext, options.ReleaseName, options.Namespace)
		if err != nil {
			return "", false, err
		}
		if installed {
			if !options.Reinstall {
				return "", false, nil
			}
			return i.renderRadiusUpgrade(kubeContext, helmConf, helmChart, values, options.ReleaseName, options.Namespace, true)(ctx)
		}
		configuration, err := i.cluster().RenderConfiguration(kubeContext, options.Namespace)
		if err != nil {
			return "", false, err
		}
		install := helm.NewInstall(configuration)
		install.DryRunStrategy = helm.DryRunServer
		install.ReleaseName = options.ReleaseName
		install.Namespace = options.Namespace
		installedRelease, err := install.RunWithContext(ctx, cloneChart(helmChart), copyValues(values))
		if err != nil {
			return "", false, err
		}
		rendered, err := asRelease(installedRelease)
		if err != nil {
			return "", false, err
		}
		return releaseActionManifest(rendered, true), true, nil
	}
}

// renderRadiusRollback renders what RunHelmRollback applies to roll Radius back to revision of history: not the
// stored manifest of revision, but an upgrade to its chart and stored values, without hooks, whose lookups read the
// live cluster. A chart that keeps a live Secret, such as the ucp-cert of authorization off, renders the live key
// rather than the one revision stored.
func (i *Impl) renderRadiusRollback(kubeContext, namespace string, history []*releasev1.Release, revision int) renderRadiusManifest {
	return func(ctx context.Context) (string, bool, error) {
		target := releaseRevision(history, revision)
		if target == nil || target.Chart == nil {
			return "", false, nil
		}
		values := target.Config
		if values == nil {
			values = map[string]any{}
		}
		manifest, err := i.dryRunRadiusUpgrade(ctx, kubeContext, history, target.Chart, values, radiusReleaseName, namespace, false, true)
		return manifest, err == nil, err
	}
}

// releaseRevision returns revision of history, or nil when history does not have it.
func releaseRevision(history []*releasev1.Release, revision int) *releasev1.Release {
	for _, rel := range history {
		if rel != nil && rel.Version == revision {
			return rel
		}
	}
	return nil
}

type renderRadiusRelease func(context.Context) (*releasev1.Release, error)

func (render renderRadiusRelease) manifest(includeWorkloadHooks bool) renderRadiusManifest {
	return func(ctx context.Context) (string, bool, error) {
		rel, err := render(ctx)
		if err != nil || rel == nil {
			return "", false, err
		}
		return releaseActionManifest(rel, includeWorkloadHooks), true, nil
	}
}

func releaseActionManifest(rel *releasev1.Release, includeWorkloadHooks bool) string {
	if !includeWorkloadHooks {
		return rel.Manifest
	}
	// Helm stores hook Pods and Jobs separately from the workload manifest.
	pre, post := releasev1.HookPreUpgrade, releasev1.HookPostUpgrade
	if rel.Version == 1 {
		pre, post = releasev1.HookPreInstall, releasev1.HookPostInstall
	}
	documents := []string{rel.Manifest}
	for _, hook := range rel.Hooks {
		if slices.Contains(hook.Events, pre) || slices.Contains(hook.Events, post) {
			documents = append(documents, hook.Manifest)
		}
	}
	return strings.Join(documents, "\n---\n")
}
