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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/cli/output"
)

// ucpInjectAnnotations are the cert-manager annotations that make cainjector write the caBundle of the UCP APIService.
// The chart renders none of them with authorization off.
var ucpInjectAnnotations = []string{
	"cert-manager.io/inject-ca-from",
	injectCAFromSecretAnnotation,
	"cert-manager.io/inject-apiserver-ca",
}

// certificateNameAnnotation is the annotation cert-manager sets on each Secret it issues, naming its Certificate.
const certificateNameAnnotation = "cert-manager.io/certificate-name"

// The chart's UCP API server Certificates set these annotations through secretTemplate, so each Secret cert-manager
// issues for them names the Helm release that declared the Certificate.
const (
	releaseNameAnnotation      = "radapp.io/release-name"
	releaseNamespaceAnnotation = "radapp.io/release-namespace"
)

// ucpOffLeftoverSecret is a Secret that only authorization on uses. certificate names the cert-manager Certificate
// that issues it, or is empty for radius-apiserver-trust, which rad writes. With authorization off, Helm removes
// those Certificates, but cert-manager keeps their Secrets.
type ucpOffLeftoverSecret struct{ secret, certificate string }

var ucpOffLeftoverSecrets = []ucpOffLeftoverSecret{
	{secret: ucpAPIServerTrustSecretName},
	{secret: ucpAPIServerCASecretName, certificate: ucpAPIServerCASecretName},
	{secret: ucpAPIServerTLSSecretName, certificate: ucpAPIServerCertificateName},
}

// ucpOffSecretState is what ReconcileUCPOffAPIServerTrust does with a ucpOffLeftoverSecret.
type ucpOffSecretState int

const (
	// ucpOffSecretKeep: the Secret does not exist, or its Certificate does, so authorization on still uses it.
	ucpOffSecretKeep ucpOffSecretState = iota
	// ucpOffSecretForeign: the Secret exists, but neither cert-manager for the expected Certificate nor rad wrote it.
	ucpOffSecretForeign
	// ucpOffSecretLeftover: rad or cert-manager wrote the Secret for authorization on, and nothing uses it now.
	ucpOffSecretLeftover
	// ucpOffSecretUnchecked: cert-manager issued the Secret for the release, but rad may not read cert-manager
	// Certificates, so it cannot tell whether authorization on still uses it and keeps it.
	ucpOffSecretUnchecked
)

// ucpOffLeftoverFound is the state of a ucpOffLeftoverSecret, with the Secret rad read when it exists.
type ucpOffLeftoverFound struct {
	ucpOffLeftoverSecret
	state  ucpOffSecretState
	object *corev1.Secret
}

// ReconcileUCPOffAPIServerTrust makes the Kubernetes API server trust UCP only through the CA of Secret ucp-cert once
// Radius runs with authorization off. rad calls it after Helm installs, upgrades or rolls back to a release with
// authorization off; it changes nothing when the cluster already has that state, so it also finishes an earlier run
// that stopped after Helm.
//
// Helm cannot remove the inject-ca-from-secret annotation that rad set while authorization was on, because rad's
// field manager owns it, and cert-manager keeps Secret radius-apiserver-ca after Helm removes its Certificate. So
// cainjector would keep adding radius-apiserver-ca to the caBundle, although UCP no longer serves a certificate from
// it. Once every UCP pod serves the certificate in ucp-cert, this removes the cert-manager inject annotations and sets
// the caBundle to the CA certificates of ucp-cert in one JSON patch, guarded by the APIService's uid and
// resourceVersion so it never overwrites a write it has not read. It sets them again if cainjector writes back a
// caBundle it read earlier, until they hold for ucpAPIServerCASettleChecks polls in a row. Only then does it delete
// radius-apiserver-trust, and radius-apiserver-ca and ucp-apiserver-tls when their Certificates no longer exist: the
// cainjector source and the private keys that nothing uses with authorization off. Turning authorization on again
// issues a new radius-apiserver-ca and stages it before Helm.
//
// It deletes only Secrets it owns: radius-apiserver-ca and ucp-apiserver-tls when cert-manager issued them for the
// expected Certificate, and radius-apiserver-trust when it has the shape rad and the chart README's plain-Helm steps
// write. Each delete carries the uid and resourceVersion that rad read, so a Secret that changed or was recreated
// since survives. rad leaves any other Secret with those names and warns how to delete it.
//
// It does nothing when the APIService or Secret ucp-cert does not exist. If UCP does not serve the certificate in
// ucp-cert in time, it returns an error and leaves the caBundle trusting both CAs; running the command again resumes.
//
// declared holds the objects of the release Helm applied. An object of the radius-apiserver-ca chain with the release's
// Helm metadata that the release does not declare was staged by an upgrade to authorization on that stopped before
// Helm, and this deletes it. It keeps a declared object, such as the radius-selfsigned-bootstrap Issuer that charts
// which serve UCP from ucp-cert also use to issue radius-ca.
func (i *Impl) ReconcileUCPOffAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string, declared manifestObjectSet) error {
	return i.reconcileUCPOffAPIServerTrust(ctx, kubeContext, radiusNamespace, declared, false)
}

// reconcileUCPOffAPIServerTrust is ReconcileUCPOffAPIServerTrust. fresh reports that Helm installed the first revision
// of the release, which no upgrade preceded, so rad never staged the radius-apiserver-ca chain for it and skips reading
// the cert-manager Issuers and Certificate, which a user who cannot read them may still install Radius without.
func (i *Impl) reconcileUCPOffAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string, declared manifestObjectSet, fresh bool) error {
	kubeClient, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}
	if !fresh {
		if err := i.deleteUnadoptedUCPAPIServerStage(ctx, kubeContext, dynamicClient, radiusNamespace, radiusReleaseName, declared, false); err != nil {
			return err
		}
	}
	apiService, err := appliedUCPAPIService(ctx, kubeContext, dynamicClient, radiusNamespace, "")
	if err != nil || apiService == nil {
		return err
	}
	offSecret, err := kubeClient.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpOffCertificateSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpOffCertificateSecretName, err)
	}
	// Helm applied ucp-cert with the release, so one without the release's metadata was replaced since; rad does not
	// make the API server trust its CA.
	if !helmOwned(offSecret, radiusNamespace, radiusReleaseName) {
		return fmt.Errorf("Secret %s/%s is not part of Helm release %s/%s, so rad did not make APIService %s trust its CA. "+
			"Check the Secret, then run the command again", radiusNamespace, ucpOffCertificateSecretName, radiusNamespace, radiusReleaseName, ucpAPIServiceName)
	}
	offCAs := caCertificates(offSecret.Data[caCertificateKey])
	if len(offCAs) == 0 {
		return fmt.Errorf("Secret %s/%s has no CA certificate in %s", radiusNamespace, ucpOffCertificateSecretName, caCertificateKey)
	}
	found, err := ucpOffLeftovers(ctx, kubeClient, dynamicClient, radiusNamespace, ucpOffLeftoverSecrets, true)
	if err != nil {
		return err
	}
	clean, err := ucpOffTrustClean(apiService, offCAs)
	if err != nil {
		return err
	}
	if clean && !hasUCPOffLeftover(found) {
		i.warnForeignUCPOffSecrets(kubeContext, radiusNamespace, found)
		return nil
	}

	output.LogInfo("Removing the trust in %s from APIService %s now that authorization is off...", ucpAPIServerCASecretName, ucpAPIServiceName)
	if err := waitForUCPOffCertificate(ctx, kubeContext, kubeClient, radiusNamespace, offCAs); err != nil {
		return err
	}
	// cainjector may have rewritten the caBundle while rad waited, so settle even when the first read was clean.
	if err := settleUCPOffTrust(ctx, kubeContext, dynamicClient, radiusNamespace, apiService.GetUID(), offCAs); err != nil {
		return err
	}

	return i.deleteUCPOffLeftovers(ctx, kubeContext, kubeClient, dynamicClient, radiusNamespace, ucpOffLeftoverSecrets, false)
}

// deleteUCPOffLeftovers deletes each Secret of leftovers that is a leftover, as ucpOffLeftover decides, pinned to the
// uid and resourceVersion rad read, and warns about each that rad does not own. When a leftover changed before rad
// deleted it, it warns, or, when incompleteErr is set, returns an error, so that uninstall keeps cert-manager for a retry.
// Without incompleteErr, it keeps an owned Secret whose Certificate rad may not read, as ucpOffLeftover describes.
func (i *Impl) deleteUCPOffLeftovers(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, radiusNamespace string, leftovers []ucpOffLeftoverSecret, incompleteErr bool) error {
	found, err := ucpOffLeftovers(ctx, kubeClient, dynamicClient, radiusNamespace, leftovers, !incompleteErr)
	if err != nil {
		return err
	}
	var changed []string
	for n, f := range found {
		if f.state != ucpOffSecretLeftover {
			continue
		}
		err := kubeClient.CoreV1().Secrets(radiusNamespace).Delete(ctx, f.secret, metav1.DeleteOptions{Preconditions: secretPreconditions(f.object)})
		if err == nil {
			found[n].state = ucpOffSecretKeep
			continue
		}
		if !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Secret %s/%s: %w", radiusNamespace, f.secret, err)
		}
		// The Secret changed or was deleted since rad read it: decide again from what exists now, but never delete
		// an object rad has not inspected.
		again, err := ucpOffLeftover(ctx, kubeClient, dynamicClient, radiusNamespace, f.ucpOffLeftoverSecret, !incompleteErr)
		if err != nil {
			return err
		}
		found[n] = again
		if again.state == ucpOffSecretLeftover {
			found[n].state = ucpOffSecretKeep
			if incompleteErr {
				changed = append(changed, radiusNamespace+"/"+f.secret)
				continue
			}
			i.logInfo("Warning: Secret %s/%s changed after rad read it, so rad left it. Run the command again to remove it.", radiusNamespace, f.secret)
		}
	}
	i.warnForeignUCPOffSecrets(kubeContext, radiusNamespace, found)
	if len(changed) > 0 {
		return fmt.Errorf("rad did not delete Secret %s, which changed after rad read it. Run the command again to delete it", strings.Join(changed, ", "))
	}
	return nil
}

// secretPreconditions returns delete preconditions that match only secret as rad read it.
func secretPreconditions(secret *corev1.Secret) *metav1.Preconditions {
	preconditions := &metav1.Preconditions{}
	if secret.UID != "" {
		preconditions.UID = &secret.UID
	}
	if secret.ResourceVersion != "" {
		preconditions.ResourceVersion = &secret.ResourceVersion
	}
	return preconditions
}

// hasUCPOffLeftover reports whether found has a Secret to delete.
func hasUCPOffLeftover(found []ucpOffLeftoverFound) bool {
	for _, f := range found {
		if f.state == ucpOffSecretLeftover {
			return true
		}
	}
	return false
}

// warnForeignUCPOffSecrets warns about each Secret of found that rad does not delete because it does not own it, or
// because it cannot read the Certificate that would show whether authorization on still uses it.
func (i *Impl) warnForeignUCPOffSecrets(kubeContext, radiusNamespace string, found []ucpOffLeftoverFound) {
	for _, f := range found {
		if f.state == ucpOffSecretUnchecked {
			i.logInfo("Warning: rad did not delete Secret %s/%s because it cannot read Certificate %s/%s. If %s finds no Certificate, authorization off does not use the Secret; delete it with %s",
				radiusNamespace, f.secret, radiusNamespace, f.certificate,
				kubectlCommand(kubeContext, radiusNamespace, "get certificate "+f.certificate),
				kubectlCommand(kubeContext, radiusNamespace, "delete secret "+f.secret))
			continue
		}
		if f.state != ucpOffSecretForeign {
			continue
		}
		owner := fmt.Sprintf("cert-manager did not issue for Certificate %s of Helm release %q, as its annotations %s and %s show",
			f.certificate, radiusReleaseName, releaseNameAnnotation, releaseNamespaceAnnotation)
		if f.certificate == "" {
			owner = fmt.Sprintf("does not hold only a non-empty %s with annotations %s=true, %s=%s and %s=%s as rad writes it", caCertificateKey,
				allowDirectInjectionAnnotation, releaseNameAnnotation, radiusReleaseName, releaseNamespaceAnnotation, radiusNamespace)
		}
		i.logInfo("Warning: rad did not delete Secret %s/%s, which %s. Authorization off does not use it; if nothing else does, delete it with %s",
			radiusNamespace, f.secret, owner, kubectlCommand(kubeContext, radiusNamespace, "delete secret "+f.secret))
	}
}

// ucpOffTrustClean reports whether apiService has none of the cert-manager inject annotations and its caBundle holds
// exactly the CA certificates offCAs and nothing else: a leaf certificate or any other PEM block makes it unclean.
func ucpOffTrustClean(apiService *unstructured.Unstructured, offCAs []*x509.Certificate) (bool, error) {
	for _, annotation := range ucpInjectAnnotations {
		if _, ok := apiService.GetAnnotations()[annotation]; ok {
			return false, nil
		}
	}
	encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false, fmt.Errorf("failed to decode the caBundle of APIService %s: %w", ucpAPIServiceName, err)
	}
	live, ok := exactCertificates(bundle)
	return ok && containsCertificates(live, offCAs) && containsCertificates(offCAs, live), nil
}

// exactCertificates returns every certificate in a PEM bundle and whether the bundle holds nothing else: each block
// is a parseable CERTIFICATE and no data follows the last block.
func exactCertificates(bundle []byte) ([]*x509.Certificate, bool) {
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			return certs, len(bytes.TrimSpace(bundle)) == 0
		}
		if block.Type != "CERTIFICATE" {
			return nil, false
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, false
		}
		certs = append(certs, cert)
	}
}

// ucpOffLeftovers returns the state of each Secret of leftovers, as ucpOffLeftover decides.
func ucpOffLeftovers(ctx context.Context, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, radiusNamespace string, leftovers []ucpOffLeftoverSecret, keepUnreadable bool) ([]ucpOffLeftoverFound, error) {
	found := []ucpOffLeftoverFound{}
	for _, leftover := range leftovers {
		f, err := ucpOffLeftover(ctx, kubeClient, dynamicClient, radiusNamespace, leftover, keepUnreadable)
		if err != nil {
			return nil, err
		}
		found = append(found, f)
	}
	return found, nil
}

// ucpOffLeftover reads the Secret of leftover and returns its state. A Secret is a leftover when its Certificate does
// not exist and rad owns it: cert-manager issued it for that Certificate, or, for radius-apiserver-trust, it holds
// only ca.crt, the allow-direct-injection annotation and the release annotations, which is what rad and the chart
// README's plain-Helm steps write.
//
// It checks the Secret's markers before it reads the Certificate, so a Secret rad does not own is foreign without any
// cert-manager read. When keepUnreadable is set, an owned Secret whose Certificate rad is Forbidden to read is
// unchecked and kept, so an upgrade by a user without cert-manager access still succeeds. Uninstall leaves
// keepUnreadable unset and fails, so it never removes cert-manager while such a Secret may remain.
func ucpOffLeftover(ctx context.Context, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, radiusNamespace string, leftover ucpOffLeftoverSecret, keepUnreadable bool) (ucpOffLeftoverFound, error) {
	found := ucpOffLeftoverFound{ucpOffLeftoverSecret: leftover}
	secret, err := kubeClient.CoreV1().Secrets(radiusNamespace).Get(ctx, leftover.secret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return found, nil
	}
	if err != nil {
		return found, fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, leftover.secret, err)
	}
	found.object = secret
	if !ownsUCPOffSecret(secret, leftover) {
		found.state = ucpOffSecretForeign
		return found, nil
	}
	if leftover.certificate != "" {
		// NotFound also covers a cluster without the cert-manager CRDs.
		_, err := dynamicClient.Resource(certManagerCertificatesResource).Namespace(radiusNamespace).Get(ctx, leftover.certificate, metav1.GetOptions{})
		if err == nil {
			return found, nil
		}
		if keepUnreadable && apierrors.IsForbidden(err) {
			found.state = ucpOffSecretUnchecked
			return found, nil
		}
		if !apierrors.IsNotFound(err) {
			return found, fmt.Errorf("failed to read Certificate %s/%s: %w", radiusNamespace, leftover.certificate, err)
		}
	}
	found.state = ucpOffSecretLeftover
	return found, nil
}

// ownsUCPOffSecret reports whether rad's contract owns secret, the Secret of leftover. cert-manager must have issued a
// Secret with a Certificate for leftover's Certificate of the Radius release, as the release annotations that the
// chart's secretTemplate sets show; the certificate-name annotation alone does not identify the release.
func ownsUCPOffSecret(secret *corev1.Secret, leftover ucpOffLeftoverSecret) bool {
	if leftover.certificate != "" {
		return secret.Annotations[certificateNameAnnotation] == leftover.certificate &&
			secret.Annotations[releaseNameAnnotation] == radiusReleaseName &&
			secret.Annotations[releaseNamespaceAnnotation] == secret.Namespace
	}
	return ownsUCPAPIServerTrust(secret, secret.Namespace)
}

// waitForUCPOffCertificate waits until every UCP pod serves the certificate in Secret ucp-cert, issued by one of
// offCAs, so removing the other CAs from the caBundle cannot make the API server reject UCP.
func waitForUCPOffCertificate(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, radiusNamespace string, offCAs []*x509.Certificate) error {
	problem, err := pollUCPAPIServer(ctx, func(ctx context.Context) (string, error) {
		problem := "no CA is given"
		for _, ca := range offCAs {
			_, p, err := ucpServingCertificate(ctx, kubeClient, radiusNamespace, ucpOffCertificateSecretName, "the CA in its "+caCertificateKey, ca)
			if err != nil || p == "" {
				return p, err
			}
			problem = p
		}
		return problem, nil
	})
	if wait.Interrupted(err) {
		return fmt.Errorf("UCP does not serve the certificate in Secret %s after %s: %s. The API server still trusts %s; run the command again once %s succeeds",
			ucpOffCertificateSecretName, ucpAPIServiceTimeout, problem, ucpAPIServerCASecretName,
			kubectlCommand(kubeContext, radiusNamespace, "rollout status deployment/ucp"))
	}
	return err
}

// settleUCPOffTrust removes the cert-manager inject annotations from the UCP APIService and sets its caBundle to
// offCAs, until both hold for ucpAPIServerCASettleChecks polls in a row. It refuses an APIService that no longer has
// uid or that the Radius release in radiusNamespace no longer owns, as appliedUCPAPIService describes.
func settleUCPOffTrust(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, radiusNamespace string, uid types.UID, offCAs []*x509.Certificate) error {
	held := 0
	err := wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		apiService, err := appliedUCPAPIService(ctx, kubeContext, dynamicClient, radiusNamespace, uid)
		if err != nil {
			return false, err
		}
		if apiService == nil {
			return true, nil
		}
		clean, err := ucpOffTrustClean(apiService, offCAs)
		if err != nil {
			return false, err
		}
		if clean {
			held++
			return held >= ucpAPIServerCASettleChecks, nil
		}
		held = 0
		patch, err := ucpOffTrustPatch(apiService, offCAs)
		if err != nil {
			return false, err
		}
		_, err = dynamicClient.Resource(apiServicesResource).Patch(ctx, ucpAPIServiceName, types.JSONPatchType, patch, metav1.PatchOptions{FieldManager: FieldManager})
		// A failed uid or resourceVersion test, or a concurrent write, means the APIService changed since rad read it.
		if apierrors.IsInvalid(err) || apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to update APIService %s: %w", ucpAPIServiceName, err)
		}
		return false, nil
	})
	if wait.Interrupted(err) {
		return fmt.Errorf("APIService %s still has a cert-manager inject annotation or trusts a CA other than the one in Secret %s after %s; check it with %s, then run the command again",
			ucpAPIServiceName, ucpOffCertificateSecretName, ucpAPIServiceTimeout, kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml"))
	}
	return err
}

// ucpOffTrustPatch returns a JSON patch that removes the cert-manager inject annotations of apiService and sets its
// caBundle to offCAs, and applies only while apiService has the uid and resourceVersion that rad read.
func ucpOffTrustPatch(apiService *unstructured.Unstructured, offCAs []*x509.Certificate) ([]byte, error) {
	operations := []map[string]any{}
	if uid := string(apiService.GetUID()); uid != "" {
		operations = append(operations, map[string]any{"op": "test", "path": "/metadata/uid", "value": uid})
	}
	if resourceVersion := apiService.GetResourceVersion(); resourceVersion != "" {
		operations = append(operations, map[string]any{"op": "test", "path": "/metadata/resourceVersion", "value": resourceVersion})
	}
	for _, annotation := range ucpInjectAnnotations {
		if _, ok := apiService.GetAnnotations()[annotation]; ok {
			operations = append(operations, map[string]any{"op": "remove", "path": "/metadata/annotations/" + jsonPointerEscape(annotation)})
		}
	}
	operations = append(operations, map[string]any{"op": "add", "path": "/spec/caBundle", "value": base64.StdEncoding.EncodeToString(encodeCertificates(offCAs))})
	return json.Marshal(operations)
}

// jsonPointerEscape escapes token for use in a JSON pointer (RFC 6901).
func jsonPointerEscape(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}
