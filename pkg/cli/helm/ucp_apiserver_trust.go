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
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
)

const (
	// ucpAPIServerTrustSecretName is the permanent public-only injection source.
	// Ordinary reconciliation must not delete it or discover trust from a signer.
	ucpAPIServerTrustSecretName = "radius-apiserver-trust"

	// ucpAPIServerCASecretName is the signing Secret, not an APIService injection source.
	ucpAPIServerCASecretName = "radius-apiserver-ca"

	// injectCAFromSecretAnnotation names the Secret whose ca.crt cert-manager's cainjector copies into the APIService.
	injectCAFromSecretAnnotation = "cert-manager.io/inject-ca-from-secret"

	// ucpAPIServerTLSSecretName is the Secret of UCP's aggregated API certificate when authorization is on.
	ucpAPIServerTLSSecretName = "ucp-apiserver-tls"

	// ucpOffCertificateSecretName is the Secret of UCP's aggregated API certificate that the chart generates from
	// ucp-ca when authorization is off.
	ucpOffCertificateSecretName = "ucp-cert"

	// ucpCertificateVolumeName is the volume of the ucp Deployment that mounts UCP's aggregated API certificate.
	ucpCertificateVolumeName = "cert"

	// allowDirectInjectionAnnotation lets cert-manager's cainjector copy ca.crt of a Secret into an APIService.
	allowDirectInjectionAnnotation = "cert-manager.io/allow-direct-injection"

	caCertificateKey = "ca.crt"

	// ucpAPIServerCASettleChecks is how many polls in a row must find the APIService caBundle holding only
	// radius-apiserver-ca before rad stops correcting a stale write from cainjector.
	ucpAPIServerCASettleChecks = 3
)

// ucpAPIServerCASource is the inject-ca-from-secret value that makes cainjector copy radius-apiserver-ca.
func ucpAPIServerCASource(radiusNamespace string) string {
	return radiusNamespace + "/" + ucpAPIServerTrustSecretName
}

// injectUCPAPIServerCA makes cainjector copy ca.crt of radius-apiserver-ca into the UCP APIService with uid, and sets
// the caBundle to it now so the API server does not wait for cainjector.
func injectUCPAPIServerCA(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, radiusNamespace string, uid types.UID, caBundle []byte) error {
	err := patchAppliedUCPAPIService(ctx, kubeContext, dynamicClient, radiusNamespace, uid,
		map[string]any{injectCAFromSecretAnnotation: ucpAPIServerCASource(radiusNamespace)}, caBundle)
	if err != nil {
		return fmt.Errorf("failed to point APIService %s at Secret %s: %w", ucpAPIServiceName, ucpAPIServerCASource(radiusNamespace), err)
	}
	return nil
}

// appliedUCPAPIService reads the UCP APIService after Helm applied the Radius release and checks, as
// releaseUCPAPIService does before Helm, that the release in radiusNamespace still owns it and that it routes to a
// Service in radiusNamespace. When uid is not empty it also refuses an APIService with another uid, which replaced
// the one rad checked earlier, even if it has the same Helm metadata. It returns nil when the APIService does not
// exist.
func appliedUCPAPIService(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, radiusNamespace string, uid types.UID) (*unstructured.Unstructured, error) {
	apiService, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read APIService %s: %w", ucpAPIServiceName, err)
	}
	check := kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml")
	if uid != "" && apiService.GetUID() != uid {
		return nil, fmt.Errorf("APIService %s was replaced after Helm applied the Radius release, so rad will not change it. "+
			"Check which installation owns it with %s, then run the command again", ucpAPIServiceName, check)
	}
	if problem := foreignUCPAPIServiceProblem(apiService, radiusNamespace, radiusReleaseName); problem != "" {
		return nil, fmt.Errorf("APIService %s %s, so rad will not change it. Helm has applied the Radius release; "+
			"check which installation owns the APIService with %s, then run the command again", ucpAPIServiceName, problem, check)
	}
	return apiService, nil
}

// patchAppliedUCPAPIService sets the caBundle of the UCP APIService with uid to caBundle, and sets annotations when not
// nil. It reads the APIService with appliedUCPAPIService first and sends the uid and resourceVersion it read with the
// patch, so the API server rejects the patch if the APIService changed in between; it retries such a conflict with a
// fresh read. It does nothing when the APIService does not exist; the wait that follows reports that.
func patchAppliedUCPAPIService(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, radiusNamespace string, uid types.UID, annotations map[string]any, caBundle []byte) error {
	var guardErr error
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		apiService, err := appliedUCPAPIService(ctx, kubeContext, dynamicClient, radiusNamespace, uid)
		if err != nil || apiService == nil {
			return err
		}
		metadata := map[string]any{"uid": apiService.GetUID(), "resourceVersion": apiService.GetResourceVersion()}
		if annotations != nil {
			metadata["annotations"] = annotations
		}
		patch, err := json.Marshal(map[string]any{
			"metadata": metadata,
			"spec":     map[string]any{"caBundle": base64.StdEncoding.EncodeToString(caBundle)},
		})
		if err != nil {
			return err
		}
		if _, guardErr = checkUCPMutation(ctx, "patch", "APIService", apiService); guardErr != nil {
			return nil
		}
		_, err = dynamicClient.Resource(apiServicesResource).Patch(ctx, ucpAPIServiceName, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: FieldManager})
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	})
	if guardErr != nil {
		return guardErr
	}
	return err
}

// settleUCPAPIServerCA waits until the APIService caBundle holds ca.crt of radius-apiserver-ca and no other
// certificate, other than expired ones, for ucpAPIServerCASettleChecks polls in a row. cainjector merges
// ca.crt into the caBundle it reads from its cache and only drops expired certificates, so a reconcile that read the
// APIService before rad switched it can write an earlier CA, such as ucp-ca, back and keep it trusted. rad sets the
// caBundle to ca.crt again whenever that happens.

// extraCertificates counts the unexpired certificates in bundle that allowed does not hold.
func extraCertificates(bundle, allowed []byte) (int, error) {
	anchors, err := parseCertificateBundlePEM(allowed)
	if err != nil {
		return 0, fmt.Errorf("invalid allowed bundle: %w", err)
	}
	if len(bundle) == 0 {
		return 0, nil
	}
	certs, err := parseCertificateBundlePEM(bundle)
	if err != nil {
		return 0, err
	}
	known := map[string]bool{}
	for _, cert := range anchors {
		known[string(cert.Raw)] = true
	}
	now := time.Now()
	extra := 0
	for _, cert := range certs {
		if !known[string(cert.Raw)] && !now.After(cert.NotAfter) {
			extra++
		}
	}
	return extra, nil
}

// deleteUCPAPIServerTrust deletes a trust Secret left by an interrupted upgrade, after Helm uninstalls Radius.
func (i *Impl) deleteUCPAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string) error {
	kubeClient, err := i.ucpAPIServerTrustKubeClient(kubeContext)
	if err != nil {
		return err
	}
	return i.deleteOwnedUCPAPIServerTrust(ctx, kubeContext, kubeClient, radiusNamespace)
}

// deleteOwnedUCPAPIServerTrust deletes the trust Secret if rad owns it, as ownsUCPAPIServerTrust decides, pinned to
// the uid and resourceVersion rad read. It keeps a Secret that rad does not own, or that changed or was recreated
// after rad read it, and warns about it.
func (i *Impl) deleteOwnedUCPAPIServerTrust(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, radiusNamespace string) error {
	ctx = withUCPMutationGuard(ctx, i.ucpMutationGuard)
	secrets := kubeClient.CoreV1().Secrets(radiusNamespace)
	read := func() (*corev1.Secret, error) {
		secret, err := secrets.Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpAPIServerTrustSecretName, err)
		}
		return secret, nil
	}
	secret, err := read()
	if err != nil || secret == nil {
		return err
	}
	if _, retained := secret.Annotations["radius.io/certificate-mode"]; retained {
		i.logInfo("Keeping certificate lifecycle public trust Secret %s/%s after uninstall or reconciliation; its Helm keep policy requires explicit lifecycle cleanup",
			radiusNamespace, ucpAPIServerTrustSecretName)
		return nil
	}
	if !ownsUCPAPIServerTrust(secret) {
		i.warnForeignUCPAPIServerTrust(kubeContext, radiusNamespace)
		return nil
	}
	retain, err := checkUCPMutation(ctx, "delete", "Secret", secret)
	if err != nil || retain {
		return err
	}
	err = secrets.Delete(ctx, ucpAPIServerTrustSecretName, metav1.DeleteOptions{Preconditions: secretPreconditions(secret)})
	if err == nil {
		return nil
	}
	if !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Secret %s/%s: %w", radiusNamespace, ucpAPIServerTrustSecretName, err)
	}
	// The Secret changed between the read and the delete. Decide again from what is there now.
	secret, err = read()
	if err != nil || secret == nil {
		return err
	}
	if _, retained := secret.Annotations["radius.io/certificate-mode"]; retained {
		i.logInfo("Keeping certificate lifecycle public trust Secret %s/%s, which changed after rad read it",
			radiusNamespace, ucpAPIServerTrustSecretName)
		return nil
	}
	if !ownsUCPAPIServerTrust(secret) {
		i.warnForeignUCPAPIServerTrust(kubeContext, radiusNamespace)
		return nil
	}
	i.logInfo("Warning: rad did not delete Secret %s/%s, which changed after rad read it. Run the command again to delete it",
		radiusNamespace, ucpAPIServerTrustSecretName)
	return nil
}

// ownsUCPAPIServerTrust reports whether secret, the trust Secret, has the shape rad writes and the chart README's
// plain-Helm steps create: only a non-empty ca.crt, with annotation cert-manager.io/allow-direct-injection=true. rad writes,
// trusts and deletes only such a Secret; the chart renders the APIService from it only then too.
func ownsUCPAPIServerTrust(secret *corev1.Secret) bool {
	_, pinned := secret.Annotations["radius.io/certificate-mode"]
	return !pinned && len(secret.Data[caCertificateKey]) > 0 && len(secret.Data) == 1 && secret.Annotations[allowDirectInjectionAnnotation] == "true"
}

// foreignUCPAPIServerTrustProblem describes a trust Secret that rad does not own.
func foreignUCPAPIServerTrustProblem(radiusNamespace string) string {
	return fmt.Sprintf("Secret %s/%s does not hold only a non-empty %s with annotation %s=true as rad writes it",
		radiusNamespace, ucpAPIServerTrustSecretName, caCertificateKey, allowDirectInjectionAnnotation)
}

// warnForeignUCPAPIServerTrust warns that rad left a trust Secret it does not own unchanged.
func (i *Impl) warnForeignUCPAPIServerTrust(kubeContext, radiusNamespace string) {
	i.logInfo("Warning: %s, so rad did not use, change or delete it. If nothing else uses it, delete it with %s",
		foreignUCPAPIServerTrustProblem(radiusNamespace), kubectlCommand(kubeContext, radiusNamespace, "delete secret "+ucpAPIServerTrustSecretName))
}

func (i *Impl) ucpAPIServerTrustKubeClient(kubeContext string) (kubernetes.Interface, error) {
	newClient := i.newUCPAPIServerTrustClient
	if newClient == nil {
		newClient = func(kubeContext string) (kubernetes.Interface, error) {
			kubeClient, _, err := newCertManagerClients(kubeContext)
			return kubeClient, err
		}
	}
	kubeClient, err := newClient(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	return kubeClient, nil
}

func (i *Impl) ucpAPIServerTrustClients(kubeContext string) (kubernetes.Interface, dynamic.Interface, error) {
	kubeClient, err := i.ucpAPIServerTrustKubeClient(kubeContext)
	if err != nil {
		return nil, nil, err
	}
	newDynamicClient := i.newCertManagerDynamicClient
	if newDynamicClient == nil {
		newDynamicClient = newCertManagerDynamicClient
	}
	dynamicClient, err := newDynamicClient(kubeContext)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	return kubeClient, dynamicClient, nil
}

// readUCPAPIServerCA returns the CA certificate in tls.crt of radius-apiserver-ca and the ca.crt cainjector copies
// into the APIService, or describes why they are not usable. ca.crt must hold only CA certificates, including the
// one in tls.crt, and the Secret must allow cainjector to copy it.
func readLegacyUCPAPIServerCA(ctx context.Context, client kubernetes.Interface, radiusNamespace string) (*x509.Certificate, []byte, string, error) {
	secret, err := client.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpAPIServerCASecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil, "it does not exist", nil
	}
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpAPIServerCASecretName, err)
	}
	if err := validateUCPAPIServerCAOwnership(secret); err != nil {
		return nil, nil, "", err
	}
	cert, err := firstCertificate(secret.Data[corev1.TLSCertKey])
	if err != nil {
		return nil, nil, "its tls.crt has no certificate: " + err.Error(), nil
	}
	if !cert.BasicConstraintsValid || !cert.IsCA {
		return nil, nil, "its tls.crt is not a CA certificate", nil
	}
	bundle := secret.Data[caCertificateKey]
	cas, err := parseCertificateBundlePEM(bundle)
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid Secret %s/%s ca.crt: %w", radiusNamespace, ucpAPIServerCASecretName, err)
	}
	for _, ca := range cas {
		if !ca.BasicConstraintsValid || !ca.IsCA {
			return nil, nil, "its ca.crt does not hold only CA certificates", nil
		}
	}
	if len(unionCertificates(cas, []*x509.Certificate{cert})) != len(cas) {
		return nil, nil, "its ca.crt does not hold the CA certificate in tls.crt", nil
	}
	if err := validateUCPAPIServerRenewal(cert, cas); err != nil {
		return nil, nil, "", err
	}
	retained, err := retainedUCPAPIServerTrust(ctx, client, radiusNamespace)
	if err != nil {
		return nil, nil, "", err
	}
	if retained != nil {
		pins, err := parseCertificateBundlePEM(retained.Data[caCertificateKey])
		if err != nil {
			return nil, nil, "", fmt.Errorf("invalid retained aggregation trust: %w", err)
		}
		if err := validateUCPAPIServerRenewal(cert, pins); err != nil {
			return nil, nil, "", err
		}
	}
	if secret.Annotations[allowDirectInjectionAnnotation] != "true" {
		return nil, nil, fmt.Sprintf("it does not have annotation %s: \"true\", which the Certificate's secretTemplate sets", allowDirectInjectionAnnotation), nil
	}
	return cert, bundle, "", nil
}

// ucpServingCertificate returns UCP's API server certificate in Secret secretName once the ucp Deployment has rolled
// out with that Secret mounted and caName's certificate ca issued it, or describes why it is not yet.
func ucpServingCertificate(ctx context.Context, client kubernetes.Interface, radiusNamespace, secretName, caName string, ca *x509.Certificate) (*x509.Certificate, string, error) {
	deployment, err := client.AppsV1().Deployments(radiusNamespace).Get(ctx, "ucp", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, "Deployment ucp does not exist", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to read Deployment %s/ucp: %w", radiusNamespace, err)
	}
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	// Replicas counts old and new pods, so equal counts mean no pod with the earlier certificate is left.
	if deployment.Status.ObservedGeneration < deployment.Generation || deployment.Status.Replicas != replicas ||
		deployment.Status.UpdatedReplicas != replicas || deployment.Status.AvailableReplicas != replicas {
		return nil, "Deployment ucp has not finished rolling out", nil
	}
	if mounted := ucpCertificateSecret(deployment); mounted != "" && mounted != secretName {
		return nil, fmt.Sprintf("Deployment ucp mounts Secret %q instead of %s", mounted, secretName), nil
	}

	secret, err := client.CoreV1().Secrets(radiusNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, "Secret " + secretName + " does not exist", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, secretName, err)
	}
	leaf, err := firstCertificate(secret.Data[corev1.TLSCertKey])
	if err != nil {
		return nil, "Secret " + secretName + " has no certificate", nil
	}
	if !verifiesUCPCertificate(leaf, ca, radiusNamespace) {
		return nil, "the certificate in Secret " + secretName + " is not issued by " + caName, nil
	}
	return leaf, "", nil
}

// ucpCertificateSecret returns the Secret that deployment mounts as UCP's API server certificate, or "" if it mounts
// none, as before the chart named the volume.
func ucpCertificateSecret(deployment *appsv1.Deployment) string {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == ucpCertificateVolumeName && volume.Secret != nil {
			return volume.Secret.SecretName
		}
	}
	return ""
}

// verifiesUCPCertificate reports whether ca issued leaf for UCP's Service name and it is valid now.
func verifiesUCPCertificate(leaf, ca *x509.Certificate, radiusNamespace string) bool {
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "ucp." + radiusNamespace + ".svc",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err == nil
}

// writeUCPAPIServerTrust stores the CA certificates in ca.crt of the trust Secret, which holds no other data. It
// returns the PEM bundle. It returns an error, and changes nothing, if a trust Secret that rad does not own exists.
func writeUCPAPIServerTrust(ctx context.Context, client kubernetes.Interface, radiusNamespace string, cas []*x509.Certificate) ([]byte, error) {
	bundle := encodeCertificates(cas)
	secrets := client.CoreV1().Secrets(radiusNamespace)
	var foreign bool
	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		var rejected *ucpMutationGuardError
		if errors.As(err, &rejected) {
			return false
		}
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		var err error
		foreign, err = writeUCPAPIServerTrustOnce(ctx, secrets, radiusNamespace, bundle)
		return err
	})
	if foreign {
		return nil, fmt.Errorf("%s, so rad does not change it", foreignUCPAPIServerTrustProblem(radiusNamespace))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to write Secret %s/%s: %w", radiusNamespace, ucpAPIServerTrustSecretName, err)
	}
	return bundle, nil
}

// writeUCPAPIServerTrustOnce writes bundle to the trust Secret, or reports that rad does not own the Secret there. An
// update carries the resourceVersion rad read, so it fails rather than overwrite a Secret that changed since.
func writeUCPAPIServerTrustOnce(ctx context.Context, secrets typedcorev1.SecretInterface, radiusNamespace string, bundle []byte) (bool, error) {
	secret, err := secrets.Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		created := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        ucpAPIServerTrustSecretName,
				Namespace:   radiusNamespace,
				Labels:      map[string]string{"app.kubernetes.io/name": "ucp", "app.kubernetes.io/part-of": "radius"},
				Annotations: map[string]string{allowDirectInjectionAnnotation: "true"},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{caCertificateKey: bundle},
		}
		if _, err := checkUCPMutation(ctx, "create", "Secret", created); err != nil {
			return false, err
		}
		_, err = secrets.Create(ctx, created, metav1.CreateOptions{})
		return false, err
	}
	if err != nil {
		return false, err
	}
	if _, retained := secret.Annotations["radius.io/certificate-mode"]; retained {
		return false, fmt.Errorf("preserve retained aggregation trust Secret %s/%s; use the certificate lifecycle instead of overwriting its public CA", radiusNamespace, ucpAPIServerTrustSecretName)
	}
	if !ownsUCPAPIServerTrust(secret) {
		return true, nil
	}
	if bytes.Equal(secret.Data[caCertificateKey], bundle) {
		return false, nil
	}
	secret.Data = map[string][]byte{caCertificateKey: bundle}
	if _, err := checkUCPMutation(ctx, "update", "Secret", secret); err != nil {
		return false, err
	}
	_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	return false, err
}

// caCertificates returns the CA certificates in a PEM bundle and drops anything else, such as leaf certificates.
func caCertificates(bundle []byte) ([]*x509.Certificate, error) {
	if len(bundle) == 0 {
		return nil, nil // An APIService may still be waiting for its first injection.
	}
	certs, err := parseCertificateBundlePEM(bundle)
	if err != nil {
		return nil, err
	}
	var cas []*x509.Certificate
	for _, cert := range certs {
		if !cert.BasicConstraintsValid || !cert.IsCA {
			continue
		}
		cas = unionCertificates(cas, []*x509.Certificate{cert})
	}
	return cas, nil
}

func firstCertificate(bundle []byte) (*x509.Certificate, error) {
	certs, err := parseCertificateBundlePEM(bundle)
	if err != nil {
		return nil, err
	}
	return certs[0], nil
}

// unionCertificates returns certs followed by the certificates of more that are not in certs.
func unionCertificates(certs, more []*x509.Certificate) []*x509.Certificate {
	result := append([]*x509.Certificate{}, certs...)
	for _, cert := range more {
		found := false
		for _, existing := range result {
			if existing.Equal(cert) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, cert)
		}
	}
	return result
}

func encodeCertificates(certs []*x509.Certificate) []byte {
	var buf bytes.Buffer
	for _, cert := range certs {
		_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	}
	return buf.Bytes()
}

func ucpAPIServerTrustSource(radiusNamespace string) string {
	return radiusNamespace + "/" + ucpAPIServerTrustSecretName
}
