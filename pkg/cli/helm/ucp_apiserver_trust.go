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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/cli/output"
)

const (
	// ucpAPIServerTrustSecretName is the Secret whose ca.crt holds the CA certificates the Kubernetes API server
	// trusts for UCP while UCP moves from an earlier CA, such as the Helm-generated ucp-ca, to radius-apiserver-ca.
	// rad creates it before Helm changes the APIService and deletes it once every UCP pod serves a certificate from
	// radius-apiserver-ca; Helm would revert the overlap if the chart owned it.
	ucpAPIServerTrustSecretName = "radius-apiserver-trust"

	// ucpAPIServerCASecretName is the Secret of the CA that issues UCP's aggregated API certificate. Outside the
	// upgrade overlap, cert-manager's cainjector copies its ca.crt, the self-signed CA certificate, into the
	// APIService caBundle, so the API server follows each renewal of the CA without rad running.
	ucpAPIServerCASecretName = "radius-apiserver-ca"

	// injectCAFromSecretAnnotation names the Secret whose ca.crt cert-manager's cainjector copies into the APIService.
	injectCAFromSecretAnnotation = "cert-manager.io/inject-ca-from-secret"

	// ucpAPIServerTLSSecretName is the Secret of UCP's aggregated API certificate when authorization is on.
	ucpAPIServerTLSSecretName = "ucp-apiserver-tls"

	// allowDirectInjectionAnnotation lets cert-manager's cainjector copy ca.crt of a Secret into an APIService.
	allowDirectInjectionAnnotation = "cert-manager.io/allow-direct-injection"

	caCertificateKey = "ca.crt"

	// ucpAPIServerCASettleChecks is how many polls in a row must find the APIService caBundle holding only
	// radius-apiserver-ca before rad stops correcting a stale write from cainjector.
	ucpAPIServerCASettleChecks = 3
)

// seedUCPAPIServerTrust adds the CA certificates of the live UCP APIService caBundle to the trust Secret before
// Helm changes the APIService. When Radius moves from the Helm-generated ucp-ca to cert-manager, the chart then
// renders the APIService with a caBundle that still trusts the running UCP pods, so the API server never loses UCP
// while they roll. It does nothing when authorization will be off, the APIService does not exist, or the APIService
// already trusts radius-apiserver-ca through cainjector.
func (i *Impl) seedUCPAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string, mode authz.Mode) error {
	if mode == authz.ModeOff {
		return nil
	}
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
	if apiService.GetAnnotations()[injectCAFromSecretAnnotation] == ucpAPIServerCASource(radiusNamespace) {
		return nil
	}
	encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("failed to decode the caBundle of APIService %s: %w", ucpAPIServiceName, err)
	}
	live := caCertificates(bundle)
	if len(live) == 0 {
		return nil
	}

	trusted, _, err := readUCPAPIServerTrust(ctx, kubeClient, radiusNamespace)
	if err != nil {
		return err
	}
	_, err = writeUCPAPIServerTrust(ctx, kubeClient, radiusNamespace, unionCertificates(trusted, live))
	return err
}

// reconcileUCPAPIServerTrust makes the Kubernetes API server trust radius-apiserver-ca for UCP through
// cert-manager's cainjector. If rad seeded the trust Secret with an earlier CA, such as ucp-ca, it first trusts both
// CAs and waits until every UCP pod serves a certificate from radius-apiserver-ca; if UCP does not move in time it
// returns an error and leaves both CAs trusted. It then points the APIService at Secret radius-apiserver-ca and
// deletes the trust Secret, which removes the earlier CA.
func (i *Impl) reconcileUCPAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string) error {
	kubeClient, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}

	output.LogInfo("Waiting for the CA of UCP's API server certificate...")
	var ca *x509.Certificate
	var caBundle []byte
	var problem string
	err = wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		ca, caBundle, problem, err = readUCPAPIServerCA(ctx, kubeClient, radiusNamespace)
		return ca != nil, err
	})
	if err != nil {
		if wait.Interrupted(err) {
			return fmt.Errorf("Secret %s/%s is not usable after %s: %s. Check the CA certificate with %s",
				radiusNamespace, ucpAPIServerCASecretName, ucpAPIServiceTimeout, problem,
				kubectlCommand(kubeContext, radiusNamespace, "describe certificate "+ucpAPIServerCASecretName))
		}
		return err
	}

	trusted, overlap, err := readUCPAPIServerTrust(ctx, kubeClient, radiusNamespace)
	if err != nil {
		return err
	}
	if overlap {
		trusted = unionCertificates(trusted, []*x509.Certificate{ca})
		if err := applyUCPAPIServerTrust(ctx, kubeClient, dynamicClient, radiusNamespace, trusted); err != nil {
			return err
		}
		if len(trusted) > 1 {
			output.LogInfo("Waiting for UCP to serve a certificate from %s before the API server stops trusting earlier CAs...", ucpAPIServerCASecretName)
			err = wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
				var leaf *x509.Certificate
				leaf, problem, err = ucpServingCertificate(ctx, kubeClient, radiusNamespace, ca)
				return leaf != nil, err
			})
			if err != nil {
				if wait.Interrupted(err) {
					return fmt.Errorf("UCP does not serve a certificate from %s after %s: %s. The API server still trusts the earlier CA too; run the command again once %s and %s succeed",
						ucpAPIServerCASecretName, ucpAPIServiceTimeout, problem,
						kubectlCommand(kubeContext, radiusNamespace, "rollout status deployment/ucp"),
						kubectlCommand(kubeContext, radiusNamespace, "describe certificate "+ucpAPIServerCertificateName))
				}
				return err
			}
		}
	}

	// Point the APIService at radius-apiserver-ca before deleting the trust Secret, so an interrupted run leaves the
	// API server trusting UCP either way.
	if err := injectUCPAPIServerCA(ctx, dynamicClient, radiusNamespace, caBundle); err != nil {
		return err
	}
	if overlap {
		if err := kubeClient.CoreV1().Secrets(radiusNamespace).Delete(ctx, ucpAPIServerTrustSecretName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Secret %s/%s: %w", radiusNamespace, ucpAPIServerTrustSecretName, err)
		}
	}
	if err := settleUCPAPIServerCA(ctx, kubeContext, kubeClient, dynamicClient, radiusNamespace); err != nil {
		return err
	}
	if overlap && len(trusted) > 1 {
		output.LogInfo("Removed %d earlier CA certificate(s) from APIService %s, which now trusts %s/%s through cert-manager",
			len(trusted)-1, ucpAPIServiceName, radiusNamespace, ucpAPIServerCASecretName)
	}
	return nil
}

// ucpAPIServerCASource is the inject-ca-from-secret value that makes cainjector copy radius-apiserver-ca.
func ucpAPIServerCASource(radiusNamespace string) string {
	return radiusNamespace + "/" + ucpAPIServerCASecretName
}

// injectUCPAPIServerCA makes cainjector copy ca.crt of radius-apiserver-ca into the UCP APIService, and sets the
// caBundle to it now so the API server does not wait for cainjector.
func injectUCPAPIServerCA(ctx context.Context, dynamicClient dynamic.Interface, radiusNamespace string, caBundle []byte) error {
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{injectCAFromSecretAnnotation: ucpAPIServerCASource(radiusNamespace)}},
		"spec":     map[string]any{"caBundle": base64.StdEncoding.EncodeToString(caBundle)},
	})
	if err != nil {
		return err
	}
	// A missing APIService is reported by the wait that follows.
	_, err = dynamicClient.Resource(apiServicesResource).Patch(ctx, ucpAPIServiceName, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to point APIService %s at Secret %s: %w", ucpAPIServiceName, ucpAPIServerCASource(radiusNamespace), err)
	}
	return nil
}

// settleUCPAPIServerCA waits until the APIService caBundle holds no certificate outside ca.crt of
// radius-apiserver-ca, other than expired ones, for ucpAPIServerCASettleChecks polls in a row. cainjector merges
// ca.crt into the caBundle it reads from its cache and only drops expired certificates, so a reconcile that read the
// APIService before rad switched it can write an earlier CA, such as ucp-ca, back and keep it trusted. rad sets the
// caBundle to ca.crt again whenever that happens.
func settleUCPAPIServerCA(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, radiusNamespace string) error {
	clean := 0
	problem := "it has not been checked"
	err := wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		_, caBundle, caProblem, err := readUCPAPIServerCA(ctx, kubeClient, radiusNamespace)
		if err != nil {
			return false, err
		}
		if caBundle == nil {
			clean, problem = 0, fmt.Sprintf("Secret %s/%s %s", radiusNamespace, ucpAPIServerCASecretName, caProblem)
			return false, nil
		}
		apiService, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// The wait for the APIService reports it.
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to read APIService %s: %w", ucpAPIServiceName, err)
		}
		encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
		bundle, decodeErr := base64.StdEncoding.DecodeString(encoded)
		extra := extraCertificates(bundle, caBundle)
		if decodeErr == nil && extra == 0 && apiService.GetAnnotations()[injectCAFromSecretAnnotation] == ucpAPIServerCASource(radiusNamespace) {
			clean++
			return clean >= ucpAPIServerCASettleChecks, nil
		}
		clean, problem = 0, fmt.Sprintf("its caBundle trusts %d certificate(s) outside Secret %s/%s", extra, radiusNamespace, ucpAPIServerCASecretName)
		return false, injectUCPAPIServerCA(ctx, dynamicClient, radiusNamespace, caBundle)
	})
	if wait.Interrupted(err) {
		return fmt.Errorf("APIService %s does not trust only Secret %s/%s after %s: %s. Check its caBundle and annotations with %s, and the logs of cert-manager's cainjector",
			ucpAPIServiceName, radiusNamespace, ucpAPIServerCASecretName, ucpAPIServiceTimeout, problem,
			kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml"))
	}
	return err
}

// extraCertificates counts the unexpired certificates in bundle that allowed does not hold.
func extraCertificates(bundle, allowed []byte) int {
	known := map[string]bool{}
	for _, cert := range parseCertificates(allowed) {
		known[string(cert.Raw)] = true
	}
	now := time.Now()
	extra := 0
	for _, cert := range parseCertificates(bundle) {
		if !known[string(cert.Raw)] && !now.After(cert.NotAfter) {
			extra++
		}
	}
	return extra
}

func parseCertificates(bundle []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			return certs
		}
		if cert, err := x509.ParseCertificate(block.Bytes); block.Type == "CERTIFICATE" && err == nil {
			certs = append(certs, cert)
		}
	}
}

// deleteUCPAPIServerTrust deletes a trust Secret left by an interrupted upgrade, after Helm uninstalls Radius.
func (i *Impl) deleteUCPAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string) error {
	kubeClient, err := i.ucpAPIServerTrustKubeClient(kubeContext)
	if err != nil {
		return err
	}
	err = kubeClient.CoreV1().Secrets(radiusNamespace).Delete(ctx, ucpAPIServerTrustSecretName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Secret %s/%s: %w", radiusNamespace, ucpAPIServerTrustSecretName, err)
	}
	return nil
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
func readUCPAPIServerCA(ctx context.Context, client kubernetes.Interface, radiusNamespace string) (*x509.Certificate, []byte, string, error) {
	secret, err := client.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpAPIServerCASecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil, "it does not exist", nil
	}
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpAPIServerCASecretName, err)
	}
	cert, err := firstCertificate(secret.Data[corev1.TLSCertKey])
	if err != nil {
		return nil, nil, "its tls.crt has no certificate: " + err.Error(), nil
	}
	if !cert.BasicConstraintsValid || !cert.IsCA {
		return nil, nil, "its tls.crt is not a CA certificate", nil
	}
	bundle := secret.Data[caCertificateKey]
	cas := caCertificates(bundle)
	if len(cas) == 0 || len(cas) != countCertificates(bundle) {
		return nil, nil, "its ca.crt does not hold only CA certificates", nil
	}
	if len(unionCertificates(cas, []*x509.Certificate{cert})) != len(cas) {
		return nil, nil, "its ca.crt does not hold the CA certificate in tls.crt", nil
	}
	if secret.Annotations[allowDirectInjectionAnnotation] != "true" {
		return nil, nil, fmt.Sprintf("it does not have annotation %s: \"true\", which the Certificate's secretTemplate sets", allowDirectInjectionAnnotation), nil
	}
	return cert, bundle, "", nil
}

// ucpServingCertificate returns UCP's API server certificate once the ucp Deployment has rolled out and that
// certificate is issued by ca, or describes why it is not yet.
func ucpServingCertificate(ctx context.Context, client kubernetes.Interface, radiusNamespace string, ca *x509.Certificate) (*x509.Certificate, string, error) {
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

	secret, err := client.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpAPIServerTLSSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, "Secret " + ucpAPIServerTLSSecretName + " does not exist", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpAPIServerTLSSecretName, err)
	}
	leaf, err := firstCertificate(secret.Data[corev1.TLSCertKey])
	if err != nil {
		return nil, "Secret " + ucpAPIServerTLSSecretName + " has no certificate", nil
	}
	if !verifiesUCPCertificate(leaf, ca, radiusNamespace) {
		return nil, "the certificate in Secret " + ucpAPIServerTLSSecretName + " is not issued by " + ucpAPIServerCASecretName, nil
	}
	return leaf, "", nil
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

// readUCPAPIServerTrust returns the CA certificates in the trust Secret and whether the Secret exists.
func readUCPAPIServerTrust(ctx context.Context, client kubernetes.Interface, radiusNamespace string) ([]*x509.Certificate, bool, error) {
	secret, err := client.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpAPIServerTrustSecretName, err)
	}
	return caCertificates(secret.Data[caCertificateKey]), true, nil
}

// writeUCPAPIServerTrust stores the CA certificates in ca.crt of the trust Secret, which holds no other data. It
// returns the PEM bundle.
func writeUCPAPIServerTrust(ctx context.Context, client kubernetes.Interface, radiusNamespace string, cas []*x509.Certificate) ([]byte, error) {
	bundle := encodeCertificates(cas)
	secrets := client.CoreV1().Secrets(radiusNamespace)
	secret, err := secrets.Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = secrets.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        ucpAPIServerTrustSecretName,
				Namespace:   radiusNamespace,
				Labels:      map[string]string{"app.kubernetes.io/name": "ucp", "app.kubernetes.io/part-of": "radius"},
				Annotations: map[string]string{allowDirectInjectionAnnotation: "true"},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{caCertificateKey: bundle},
		}, metav1.CreateOptions{})
	} else if err == nil {
		if secret.Annotations[allowDirectInjectionAnnotation] == "true" && len(secret.Data) == 1 && bytes.Equal(secret.Data[caCertificateKey], bundle) {
			return bundle, nil
		}
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[allowDirectInjectionAnnotation] = "true"
		secret.Data = map[string][]byte{caCertificateKey: bundle}
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to write Secret %s/%s: %w", radiusNamespace, ucpAPIServerTrustSecretName, err)
	}
	return bundle, nil
}

// applyUCPAPIServerTrust writes cas to the trust Secret and to the APIService caBundle. cert-manager's cainjector
// would copy the Secret too, but setting the caBundle directly does not depend on when it runs.
func applyUCPAPIServerTrust(ctx context.Context, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, radiusNamespace string, cas []*x509.Certificate) error {
	bundle, err := writeUCPAPIServerTrust(ctx, kubeClient, radiusNamespace, cas)
	if err != nil {
		return err
	}
	patch, err := json.Marshal(map[string]any{"spec": map[string]any{"caBundle": base64.StdEncoding.EncodeToString(bundle)}})
	if err != nil {
		return err
	}
	// A missing APIService is reported by the wait that follows.
	_, err = dynamicClient.Resource(apiServicesResource).Patch(ctx, ucpAPIServiceName, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to set the caBundle of APIService %s: %w", ucpAPIServiceName, err)
	}
	return nil
}

// caCertificates returns the CA certificates in a PEM bundle and drops anything else, such as leaf certificates.
func caCertificates(bundle []byte) []*x509.Certificate {
	var cas []*x509.Certificate
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			return cas
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.BasicConstraintsValid || !cert.IsCA {
			continue
		}
		cas = unionCertificates(cas, []*x509.Certificate{cert})
	}
}

func countCertificates(bundle []byte) int {
	count := 0
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			return count
		}
		if block.Type == "CERTIFICATE" {
			count++
		}
	}
}

func firstCertificate(bundle []byte) (*x509.Certificate, error) {
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			return nil, fmt.Errorf("no PEM certificate")
		}
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
	}
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
