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
	"encoding/json"
	"fmt"
	"strings"

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

// ucpOffLeftoverSecrets are the Secrets that only authorization on uses, with the cert-manager Certificate that
// issues each. With authorization off, Helm removes those Certificates, but cert-manager keeps their Secrets.
var ucpOffLeftoverSecrets = []struct{ secret, certificate string }{
	{secret: ucpAPIServerTrustSecretName},
	{secret: ucpAPIServerCASecretName, certificate: ucpAPIServerCASecretName},
	{secret: ucpAPIServerTLSSecretName, certificate: ucpAPIServerCertificateName},
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
// It does nothing when the APIService or Secret ucp-cert does not exist. If UCP does not serve the certificate in
// ucp-cert in time, it returns an error and leaves the caBundle trusting both CAs; running the command again resumes.
func (i *Impl) ReconcileUCPOffAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string) error {
	kubeClient, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}
	apiService, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read APIService %s: %w", ucpAPIServiceName, err)
	}
	offSecret, err := kubeClient.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpOffCertificateSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpOffCertificateSecretName, err)
	}
	offCAs := caCertificates(offSecret.Data[caCertificateKey])
	if len(offCAs) == 0 {
		return fmt.Errorf("Secret %s/%s has no CA certificate in %s", radiusNamespace, ucpOffCertificateSecretName, caCertificateKey)
	}
	leftovers, err := ucpOffLeftovers(ctx, kubeClient, dynamicClient, radiusNamespace)
	if err != nil {
		return err
	}
	clean, err := ucpOffTrustClean(apiService, offCAs)
	if err != nil {
		return err
	}
	if clean && len(leftovers) == 0 {
		return nil
	}

	output.LogInfo("Removing the trust in %s from APIService %s now that authorization is off...", ucpAPIServerCASecretName, ucpAPIServiceName)
	if err := waitForUCPOffCertificate(ctx, kubeContext, kubeClient, radiusNamespace, offCAs); err != nil {
		return err
	}
	if !clean {
		if err := settleUCPOffTrust(ctx, kubeContext, dynamicClient, offCAs); err != nil {
			return err
		}
	}

	leftovers, err = ucpOffLeftovers(ctx, kubeClient, dynamicClient, radiusNamespace)
	if err != nil {
		return err
	}
	for _, name := range leftovers {
		err := kubeClient.CoreV1().Secrets(radiusNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Secret %s/%s: %w", radiusNamespace, name, err)
		}
	}
	return nil
}

// ucpOffTrustClean reports whether apiService has none of the cert-manager inject annotations and its caBundle holds
// exactly the CA certificates offCAs.
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
	live := caCertificates(bundle)
	return containsCertificates(live, offCAs) && containsCertificates(offCAs, live), nil
}

// ucpOffLeftovers returns the Secrets of ucpOffLeftoverSecrets that exist while their Certificate does not.
func ucpOffLeftovers(ctx context.Context, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, radiusNamespace string) ([]string, error) {
	leftovers := []string{}
	for _, leftover := range ucpOffLeftoverSecrets {
		_, err := kubeClient.CoreV1().Secrets(radiusNamespace).Get(ctx, leftover.secret, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, leftover.secret, err)
		}
		if leftover.certificate != "" {
			// NotFound also covers a cluster without the cert-manager CRDs.
			_, err := dynamicClient.Resource(certManagerCertificatesResource).Namespace(radiusNamespace).Get(ctx, leftover.certificate, metav1.GetOptions{})
			if err == nil {
				continue
			}
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to read Certificate %s/%s: %w", radiusNamespace, leftover.certificate, err)
			}
		}
		leftovers = append(leftovers, leftover.secret)
	}
	return leftovers, nil
}

// waitForUCPOffCertificate waits until every UCP pod serves the certificate in Secret ucp-cert, issued by one of
// offCAs, so removing the other CAs from the caBundle cannot make the API server reject UCP.
func waitForUCPOffCertificate(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, radiusNamespace string, offCAs []*x509.Certificate) error {
	var problem string
	err := wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		for _, ca := range offCAs {
			leaf, p, err := ucpServingCertificate(ctx, kubeClient, radiusNamespace, ucpOffCertificateSecretName, "the CA in its "+caCertificateKey, ca)
			if err != nil || leaf != nil {
				return leaf != nil, err
			}
			problem = p
		}
		return false, nil
	})
	if wait.Interrupted(err) {
		return fmt.Errorf("UCP does not serve the certificate in Secret %s after %s: %s. The API server still trusts %s; run the command again once %s succeeds",
			ucpOffCertificateSecretName, ucpAPIServiceTimeout, problem, ucpAPIServerCASecretName,
			kubectlCommand(kubeContext, radiusNamespace, "rollout status deployment/ucp"))
	}
	return err
}

// settleUCPOffTrust removes the cert-manager inject annotations from the UCP APIService and sets its caBundle to
// offCAs, until both hold for ucpAPIServerCASettleChecks polls in a row.
func settleUCPOffTrust(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, offCAs []*x509.Certificate) error {
	held := 0
	err := wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		apiService, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to read APIService %s: %w", ucpAPIServiceName, err)
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
