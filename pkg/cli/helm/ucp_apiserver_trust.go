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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"

	"github.com/radius-project/radius/pkg/cli/output"
)

const (
	// ucpAPIServerTrustSecretName is the Secret whose ca.crt holds the CA certificates the Kubernetes API server
	// trusts for UCP while UCP moves from an earlier CA, such as the Helm-generated ucp-ca, to radius-apiserver-ca.
	// Before the Helm call that turns authorization on, rad stages radius-apiserver-ca and writes both CAs into it, so
	// the API server trusts the new CA before Helm reissues ucp-cert. rad deletes it once every UCP pod serves a
	// certificate from radius-apiserver-ca; Helm would revert the overlap if the chart owned it.
	ucpAPIServerTrustSecretName = "radius-apiserver-trust"

	// ucpAPIServerCASecretName is the Secret of the CA that issues UCP's aggregated API certificate. Outside the
	// upgrade overlap, cert-manager's cainjector copies its ca.crt, the self-signed CA certificate, into the
	// APIService caBundle, so the API server follows each renewal of the CA without rad running.
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

	// ucpCertificateHashAnnotation on the pod template of the ucp Deployment holds the hash of the certificate that UCP
	// must serve before the API server stops trusting an earlier CA: the ucp-cert certificate that rad applied before
	// authorization turns off, or the ucp-apiserver-tls certificate when authorization is on. The Deployment rolls out
	// again when that certificate changes.
	ucpCertificateHashAnnotation = "radapp.io/ucp-cert-sha256"

	// allowDirectInjectionAnnotation lets cert-manager's cainjector copy ca.crt of a Secret into an APIService.
	allowDirectInjectionAnnotation = "cert-manager.io/allow-direct-injection"

	caCertificateKey = "ca.crt"

	// ucpAPIServerCASettleChecks is how many polls in a row must find the APIService caBundle holding only
	// radius-apiserver-ca before rad stops correcting a stale write from cainjector.
	ucpAPIServerCASettleChecks = 3
)

// reconcileUCPAPIServerTrust makes the Kubernetes API server trust radius-apiserver-ca for UCP through
// cert-manager's cainjector. If rad staged the trust Secret with an earlier CA, such as ucp-ca, it first trusts both
// CAs and waits until every UCP pod serves a certificate from radius-apiserver-ca; if UCP does not move in time it
// returns an error and leaves both CAs trusted. It then points the APIService at Secret radius-apiserver-ca and
// deletes the trust Secret, which removes the earlier CA.
//
// It verifies only the current trust: rad writes the caBundle itself, so a cainjector that cannot update the
// APIService passes. Such an injector leaves the earlier radius-apiserver-ca certificate in the caBundle after
// cert-manager renews it, which keeps working, because the renewal keeps the key, until that certificate expires.
//
// fresh reports that the release is at its first revision. A trust Secret then is left from an earlier install rather
// than staged for this one, so rad trusts none of its CAs and only deletes it.
func (i *Impl) reconcileUCPAPIServerTrust(ctx context.Context, kubeContext, radiusNamespace string, fresh bool) error {
	kubeClient, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}

	output.LogInfo("Waiting for the CA of UCP's API server certificate...")
	var ca *x509.Certificate
	var caBundle []byte
	problem, err := pollUCPAPIServer(ctx, func(ctx context.Context) (string, error) {
		var problem string
		var err error
		ca, caBundle, problem, err = readUCPAPIServerCA(ctx, kubeClient, radiusNamespace)
		return problem, err
	})
	if err != nil {
		if ucpWaitTimedOut(ctx, err) {
			return fmt.Errorf("Secret %s/%s is not usable after %s: %s. Check the CA certificate with %s",
				radiusNamespace, ucpAPIServerCASecretName, ucpAPIServiceTimeout, problem,
				kubectlCommand(kubeContext, radiusNamespace, "describe certificate "+ucpAPIServerCASecretName))
		}
		return err
	}

	// The APIService is cluster-scoped, so rad checks that the Radius release still owns it, and pins each later write to
	// the APIService it checked here.
	apiService, err := appliedUCPAPIService(ctx, kubeContext, dynamicClient, radiusNamespace, "")
	if err != nil || apiService == nil {
		// The wait that follows reports a missing APIService.
		return err
	}
	uid := apiService.GetUID()

	trusted, trust, err := readUCPAPIServerTrust(ctx, kubeClient, radiusNamespace)
	if err != nil {
		return err
	}
	// rad neither changes nor deletes a Secret of that name that it does not own, and does not trust its CAs.
	overlap := trust != nil && ownsUCPAPIServerTrust(trust, radiusNamespace)
	if trust != nil && !overlap {
		i.warnForeignUCPAPIServerTrust(kubeContext, radiusNamespace)
	}
	if overlap && !fresh {
		trusted = unionCertificates(trusted, []*x509.Certificate{ca})
		if err := applyUCPAPIServerTrust(ctx, kubeContext, kubeClient, dynamicClient, radiusNamespace, uid, trusted); err != nil {
			return err
		}
		if len(trusted) > 1 {
			output.LogInfo("Waiting for UCP to serve a certificate from %s before the API server stops trusting earlier CAs...", ucpAPIServerCASecretName)
			problem, err := pollUCPAPIServer(ctx, func(ctx context.Context) (string, error) {
				_, problem, err := ucpServingCertificate(ctx, kubeClient, radiusNamespace, ucpAPIServerTLSSecretName, ucpAPIServerCASecretName, ca)
				return problem, err
			})
			if err != nil {
				if ucpWaitTimedOut(ctx, err) {
					return fmt.Errorf("UCP does not serve a certificate from %s after %s: %s. The API server still trusts the earlier CA too; run the command again once %s and %s succeed",
						ucpAPIServerCASecretName, ucpAPIServiceTimeout, problem,
						kubectlCommand(kubeContext, radiusNamespace, "rollout status deployment/ucp"),
						kubectlCommand(kubeContext, radiusNamespace, "describe certificate "+ucpAPIServerCertificateName))
				}
				return err
			}
			if err := rollUCPOnServingCertificate(ctx, kubeContext, kubeClient, radiusNamespace, ca); err != nil {
				return err
			}
		}
	}

	// Point the APIService at radius-apiserver-ca before deleting the trust Secret, so an interrupted run leaves the
	// API server trusting UCP either way.
	if err := injectUCPAPIServerCA(ctx, kubeContext, dynamicClient, radiusNamespace, uid, caBundle); err != nil {
		return err
	}
	if overlap {
		if err := i.deleteOwnedUCPAPIServerTrust(ctx, kubeContext, kubeClient, radiusNamespace); err != nil {
			return err
		}
	}
	if err := settleUCPAPIServerCA(ctx, kubeContext, kubeClient, dynamicClient, radiusNamespace, uid); err != nil {
		return err
	}
	if overlap && !fresh && len(trusted) > 1 {
		output.LogInfo("Removed %d earlier CA certificate(s) from APIService %s, which now trusts only %s/%s",
			len(trusted)-1, ucpAPIServiceName, radiusNamespace, ucpAPIServerCASecretName)
	}
	// A chart whose Certificate ucp-cert writes another Secret, such as ucp-mtls, leaves ucp-apiserver-tls and its
	// private key unused, so rad deletes it as authorization off does. It keeps a Secret that rad does not own, and
	// keeps it while UCP may still mount it, because a replacement pod could not start without it.
	moved, err := ucpServesFromOtherSecret(ctx, kubeClient, radiusNamespace, ucpAPIServerTLSSecretName)
	if err != nil || !moved {
		return err
	}
	_, err = i.deleteLeftoverSecrets(ctx, kubeClient, dynamicClient, radiusNamespace,
		[]ucpOffLeftoverSecret{{secret: ucpAPIServerTLSSecretName, certificate: ucpAPIServerCertificateName}}, false)
	return err
}

// ucpServesFromOtherSecret reports whether the Radius release's ucp Deployment has finished rolling out with its API
// server certificate volume mounting a Secret other than secretName, so no UCP pod, current or replacement, needs it.
func ucpServesFromOtherSecret(ctx context.Context, client kubernetes.Interface, radiusNamespace, secretName string) (bool, error) {
	deployment, err := client.AppsV1().Deployments(radiusNamespace).Get(ctx, "ucp", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read Deployment %s/ucp: %w", radiusNamespace, err)
	}
	mounted := ucpCertificateSecret(deployment)
	return helmOwned(deployment, radiusNamespace, radiusReleaseName) && ucpRolledOut(deployment) &&
		mounted != "" && mounted != secretName, nil
}

// ucpRolledOut reports whether every pod of deployment runs its current template.
func ucpRolledOut(deployment *appsv1.Deployment) bool {
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	// Replicas counts old and new pods, so equal counts mean no pod with the earlier template is left.
	return deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.Replicas == replicas &&
		deployment.Status.UpdatedReplicas == replicas && deployment.Status.AvailableReplicas == replicas
}

// ucpAPIServerCASource is the inject-ca-from-secret value that makes cainjector copy radius-apiserver-ca.
func ucpAPIServerCASource(radiusNamespace string) string {
	return radiusNamespace + "/" + ucpAPIServerCASecretName
}

// ucpAPIServerTrustSource is the inject-ca-from-secret value that makes cainjector copy the trust Secret.
func ucpAPIServerTrustSource(radiusNamespace string) string {
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
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
		_, err = dynamicClient.Resource(apiServicesResource).Patch(ctx, ucpAPIServiceName, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: FieldManager})
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	})
}

// settleUCPAPIServerCA waits until the APIService caBundle holds ca.crt of radius-apiserver-ca and no other
// certificate, other than expired ones, for ucpAPIServerCASettleChecks polls in a row. cainjector merges
// ca.crt into the caBundle it reads from its cache and only drops expired certificates, so a reconcile that read the
// APIService before rad switched it can write an earlier CA, such as ucp-ca, back and keep it trusted. rad sets the
// caBundle to ca.crt again whenever that happens.
//
// It refuses an APIService that no longer has uid or that the Radius release no longer owns, as appliedUCPAPIService
// describes.
func settleUCPAPIServerCA(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, radiusNamespace string, uid types.UID) error {
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
		apiService, err := appliedUCPAPIService(ctx, kubeContext, dynamicClient, radiusNamespace, uid)
		if err != nil {
			return false, err
		}
		if apiService == nil {
			// The wait for the APIService reports it.
			return true, nil
		}
		encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
		bundle, decodeErr := base64.StdEncoding.DecodeString(encoded)
		extra := extraCertificates(bundle, caBundle)
		missing := !containsCertificates(parseCertificates(bundle), parseCertificates(caBundle))
		if decodeErr == nil && extra == 0 && !missing && apiService.GetAnnotations()[injectCAFromSecretAnnotation] == ucpAPIServerCASource(radiusNamespace) {
			clean++
			return clean >= ucpAPIServerCASettleChecks, nil
		}
		clean, problem = 0, fmt.Sprintf("its caBundle trusts %d certificate(s) outside Secret %s/%s", extra, radiusNamespace, ucpAPIServerCASecretName)
		if missing {
			problem = fmt.Sprintf("its caBundle does not hold the CA in Secret %s/%s", radiusNamespace, ucpAPIServerCASecretName)
		}
		return false, injectUCPAPIServerCA(ctx, kubeContext, dynamicClient, radiusNamespace, uid, caBundle)
	})
	if ucpWaitTimedOut(ctx, err) {
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
	return i.deleteOwnedUCPAPIServerTrust(ctx, kubeContext, kubeClient, radiusNamespace)
}

// deleteOwnedUCPAPIServerTrust deletes the trust Secret if rad owns it, as ownsUCPAPIServerTrust decides, pinned to
// the uid and resourceVersion rad read. It keeps a Secret that rad does not own, or that changed or was recreated
// after rad read it, and warns about it.
func (i *Impl) deleteOwnedUCPAPIServerTrust(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, radiusNamespace string) error {
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
	if !ownsUCPAPIServerTrust(secret, radiusNamespace) {
		i.warnForeignUCPAPIServerTrust(kubeContext, radiusNamespace)
		return nil
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
	if !ownsUCPAPIServerTrust(secret, radiusNamespace) {
		i.warnForeignUCPAPIServerTrust(kubeContext, radiusNamespace)
		return nil
	}
	i.logInfo("Warning: rad did not delete Secret %s/%s, which changed after rad read it. Run the command again to delete it",
		radiusNamespace, ucpAPIServerTrustSecretName)
	return nil
}

// ownsUCPAPIServerTrust reports whether secret, the trust Secret, has the shape rad writes and the chart README's
// plain-Helm steps create: only a non-empty ca.crt, with annotation cert-manager.io/allow-direct-injection=true and the
// release annotations that name the Radius release in radiusNamespace. rad writes, trusts and deletes only such a
// Secret; the chart renders the APIService from it only then too.
func ownsUCPAPIServerTrust(secret *corev1.Secret, radiusNamespace string) bool {
	return len(secret.Data[caCertificateKey]) > 0 && len(secret.Data) == 1 && secret.Annotations[allowDirectInjectionAnnotation] == "true" &&
		secret.Annotations[releaseNameAnnotation] == radiusReleaseName && secret.Annotations[releaseNamespaceAnnotation] == radiusNamespace
}

// foreignUCPAPIServerTrustProblem describes a trust Secret that rad does not own.
func foreignUCPAPIServerTrustProblem(radiusNamespace string) string {
	return fmt.Sprintf("Secret %s/%s does not hold only a non-empty %s with annotations %s=true, %s=%s and %s=%s as rad writes it",
		radiusNamespace, ucpAPIServerTrustSecretName, caCertificateKey, allowDirectInjectionAnnotation,
		releaseNameAnnotation, radiusReleaseName, releaseNamespaceAnnotation, radiusNamespace)
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
// into the APIService, or describes why they are not usable. The Secret must carry the release markers that the
// Certificate's secretTemplate sets, as ownsUCPAPIServerCA describes; ca.crt must hold only CA certificates, including
// the one in tls.crt, and the Secret must allow cainjector to copy it.
func readUCPAPIServerCA(ctx context.Context, client kubernetes.Interface, radiusNamespace string) (*x509.Certificate, []byte, string, error) {
	secret, err := client.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpAPIServerCASecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil, "it does not exist", nil
	}
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpAPIServerCASecretName, err)
	}
	if !ownsUCPAPIServerCA(secret, radiusNamespace) {
		return nil, nil, foreignUCPAPIServerCAProblem(radiusNamespace), nil
	}
	cert, err := firstCertificate(secret.Data[corev1.TLSCertKey])
	if err != nil {
		return nil, nil, "its tls.crt has no certificate: " + err.Error(), nil
	}
	if !cert.BasicConstraintsValid || !cert.IsCA {
		return nil, nil, "its tls.crt is not a CA certificate", nil
	}
	// A CA Secret kept from an earlier install can hold an expired CA until cert-manager renews it, so rad waits for
	// the renewal rather than trust it.
	if now := time.Now(); now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return nil, nil, "its tls.crt CA is expired or not yet valid", nil
	}
	bundle := secret.Data[caCertificateKey]
	cas := caCertificates(bundle)
	if len(cas) == 0 || len(cas) != countCertificates(bundle) || !onlyCertificateBlocks(bundle) {
		return nil, nil, "its ca.crt does not hold only CA certificates", nil
	}
	if len(unionCertificates(cas, []*x509.Certificate{cert})) != len(cas) {
		return nil, nil, "its ca.crt does not hold the CA certificate in tls.crt", nil
	}
	if secret.Annotations[allowDirectInjectionAnnotation] != "true" {
		return nil, nil, fmt.Sprintf("it does not have annotation %s: \"true\", which the Certificate's secretTemplate sets", allowDirectInjectionAnnotation), nil
	}
	// cainjector copies ca.crt as it is, so it was checked above to hold nothing else; rad writes only the certificates.
	return cert, encodeCertificates(cas), "", nil
}

// onlyCertificateBlocks reports whether bundle holds only PEM CERTIFICATE blocks and whitespace. pem.Decode skips
// text between blocks, so the check also requires that nothing but whitespace surrounds them.
func onlyCertificateBlocks(bundle []byte) bool {
	rest := bytes.TrimSpace(bundle)
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 || !bytes.HasPrefix(rest, []byte("-----BEGIN")) {
			return false
		}
		rest = bytes.TrimSpace(next)
	}
	return true
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
	if !ucpRolledOut(deployment) {
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
	chain, err := certificateChain(secret.Data[corev1.TLSCertKey])
	if err != nil {
		return nil, "Secret " + secretName + " has no certificate", nil
	}
	leaf := chain[0]
	if !verifiesUCPCertificate(leaf, chain[1:], ca, radiusNamespace) {
		return nil, "the certificate in Secret " + secretName + " is not issued by " + caName, nil
	}
	return leaf, "", nil
}

// rollUCPOnServingCertificate makes every ucp pod start after Secret ucp-apiserver-tls holds a certificate from ca,
// which the wait before it found. UCP reloads its certificate when the kubelet updates the mounted Secret, which takes
// up to about a minute after cert-manager reissues it, so the rollout counters of a Deployment that already mounted
// ucp-apiserver-tls do not tell whether its pods serve the reissued certificate yet. rad sets
// ucpCertificateHashAnnotation to the hash of that certificate, which rolls the Deployment out again, and waits for
// the rollout. A Deployment whose template already holds the hash rolled out after the Secret held this certificate.
func rollUCPOnServingCertificate(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, radiusNamespace string, ca *x509.Certificate) error {
	secret, err := kubeClient.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpAPIServerTLSSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpAPIServerTLSSecretName, err)
	}
	certificateHash := ucpCertificateHash(secret)
	deployments := kubeClient.AppsV1().Deployments(radiusNamespace)
	deployment, err := deployments.Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to read Deployment %s/ucp: %w", radiusNamespace, err)
	}
	if deployment.Spec.Template.Annotations[ucpCertificateHashAnnotation] == certificateHash {
		return nil
	}
	if !helmOwned(deployment, radiusNamespace, radiusReleaseName) {
		return fmt.Errorf("Deployment %s/ucp does not belong to Helm release %q, so rad did not restart it. The API server still trusts the earlier CA too; "+
			"check the Deployment, then run the command again", radiusNamespace, radiusReleaseName)
	}
	// The patch carries the uid and resourceVersion that rad checked, so it fails if the Deployment changed since.
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"uid": deployment.UID, "resourceVersion": deployment.ResourceVersion},
		"spec": map[string]any{"template": map[string]any{
			"metadata": map[string]any{"annotations": map[string]any{ucpCertificateHashAnnotation: certificateHash}},
		}},
	})
	if err != nil {
		return err
	}
	output.LogInfo("Restarting UCP so that every pod serves the certificate in Secret %s...", ucpAPIServerTLSSecretName)
	if _, err := deployments.Patch(ctx, "ucp", types.StrategicMergePatchType, patch, metav1.PatchOptions{FieldManager: FieldManager}); err != nil {
		return fmt.Errorf("failed to restart Deployment %s/ucp: %w", radiusNamespace, err)
	}
	problem, err := pollUCPAPIServer(ctx, func(ctx context.Context) (string, error) {
		_, problem, err := ucpServingCertificate(ctx, kubeClient, radiusNamespace, ucpAPIServerTLSSecretName, ucpAPIServerCASecretName, ca)
		return problem, err
	})
	if err != nil {
		if ucpWaitTimedOut(ctx, err) {
			return fmt.Errorf("UCP does not serve a certificate from %s after %s: %s. The API server still trusts the earlier CA too; run the command again once %s succeeds",
				ucpAPIServerCASecretName, ucpAPIServiceTimeout, problem, kubectlCommand(kubeContext, radiusNamespace, "rollout status deployment/ucp"))
		}
		return err
	}
	return nil
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

// verifiesUCPCertificate reports whether leaf chains to ca, through intermediates if any, for UCP's Service name and is
// valid now. intermediates are the certificates after the leaf in tls.crt, which UCP serves with it; only ca is trusted.
func verifiesUCPCertificate(leaf *x509.Certificate, intermediates []*x509.Certificate, ca *x509.Certificate, radiusNamespace string) bool {
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	pool := x509.NewCertPool()
	for _, cert := range intermediates {
		pool.AddCert(cert)
	}
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: pool,
		DNSName:       "ucp." + radiusNamespace + ".svc",
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err == nil
}

// readUCPAPIServerTrust returns the trust Secret, or nil if it does not exist, and, if rad owns it, the CA
// certificates in it.
func readUCPAPIServerTrust(ctx context.Context, client kubernetes.Interface, radiusNamespace string) ([]*x509.Certificate, *corev1.Secret, error) {
	secret, err := client.CoreV1().Secrets(radiusNamespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read Secret %s/%s: %w", radiusNamespace, ucpAPIServerTrustSecretName, err)
	}
	if !ownsUCPAPIServerTrust(secret, radiusNamespace) {
		return nil, secret, nil
	}
	return caCertificates(secret.Data[caCertificateKey]), secret, nil
}

// writeUCPAPIServerTrust stores the CA certificates in ca.crt of the trust Secret, which holds no other data. It
// returns the PEM bundle. It returns an error, and changes nothing, if a trust Secret that rad does not own exists.
func writeUCPAPIServerTrust(ctx context.Context, client kubernetes.Interface, radiusNamespace string, cas []*x509.Certificate) ([]byte, error) {
	bundle := encodeCertificates(cas)
	secrets := client.CoreV1().Secrets(radiusNamespace)
	var foreign bool
	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
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
		_, err = secrets.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ucpAPIServerTrustSecretName,
				Namespace: radiusNamespace,
				Labels:    map[string]string{"app.kubernetes.io/name": "ucp", "app.kubernetes.io/part-of": "radius"},
				Annotations: map[string]string{
					allowDirectInjectionAnnotation: "true",
					releaseNameAnnotation:          radiusReleaseName,
					releaseNamespaceAnnotation:     radiusNamespace,
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{caCertificateKey: bundle},
		}, metav1.CreateOptions{})
		return false, err
	}
	if err != nil {
		return false, err
	}
	if !ownsUCPAPIServerTrust(secret, radiusNamespace) {
		return true, nil
	}
	if bytes.Equal(secret.Data[caCertificateKey], bundle) {
		return false, nil
	}
	secret.Data = map[string][]byte{caCertificateKey: bundle}
	_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	return false, err
}

// applyUCPAPIServerTrust writes cas to the trust Secret and to the caBundle of the APIService with uid. cert-manager's cainjector
// would copy the Secret too, but setting the caBundle directly does not depend on when it runs.
func applyUCPAPIServerTrust(ctx context.Context, kubeContext string, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, radiusNamespace string, uid types.UID, cas []*x509.Certificate) error {
	bundle, err := writeUCPAPIServerTrust(ctx, kubeClient, radiusNamespace, cas)
	if err != nil {
		return err
	}
	if err := patchAppliedUCPAPIService(ctx, kubeContext, dynamicClient, radiusNamespace, uid, nil, bundle); err != nil {
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

// certificateChain returns every certificate in bundle, the leaf first, or an error if it has none or one that does
// not parse.
func certificateChain(bundle []byte) ([]*x509.Certificate, error) {
	var chain []*x509.Certificate
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		chain = append(chain, cert)
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("no PEM certificate")
	}
	return chain, nil
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

// ownsUCPAPIServerCA reports whether secret, Secret radius-apiserver-ca, carries the annotations that the
// secretTemplate of the Radius release's Certificate sets. rad trusts the CA of no other Secret of that name.
func ownsUCPAPIServerCA(secret *corev1.Secret, radiusNamespace string) bool {
	return secret.Annotations[releaseNameAnnotation] == radiusReleaseName &&
		secret.Annotations[releaseNamespaceAnnotation] == radiusNamespace
}

func foreignUCPAPIServerCAProblem(radiusNamespace string) string {
	return fmt.Sprintf("it does not have annotations %s: %q and %s: %q, which the Certificate's secretTemplate sets for the Radius release",
		releaseNameAnnotation, radiusReleaseName, releaseNamespaceAnnotation, radiusNamespace)
}
