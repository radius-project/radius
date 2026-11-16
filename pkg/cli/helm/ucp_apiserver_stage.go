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
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/cli/output"
)

var (
	certManagerCertificatesResource = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	certManagerIssuersResource      = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
)

// ucpAPIServerCAStageObjects are the objects of the chart that cert-manager needs to issue radius-apiserver-ca, in
// the order rad applies them: the self-signed Issuer, the CA Certificate, and the Issuer that signs ucp-cert from it.
var ucpAPIServerCAStageObjects = []struct {
	kind     string
	name     string
	resource schema.GroupVersionResource
}{
	{kind: "Issuer", name: RadiusBootstrapIssuerName, resource: certManagerIssuersResource},
	{kind: "Certificate", name: ucpAPIServerCASecretName, resource: certManagerCertificatesResource},
	{kind: "Issuer", name: RadiusAPIServerIssuerName, resource: certManagerIssuersResource},
}

// renderRadiusManifest returns the manifest of the Radius release that the next Helm call applies. ok is false when
// that call does not change the release, so there is nothing to stage.
type renderRadiusManifest func(ctx context.Context) (manifest string, ok bool, err error)

// stageUCPAPIServerCA makes the Kubernetes API server trust both the CA that the running UCP pods serve, such as the
// Helm-generated ucp-ca, and radius-apiserver-ca before Helm reissues ucp-cert from radius-apiserver-issuer.
//
// Helm creates radius-apiserver-ca and reissues ucp-cert in the same call, and UCP reloads its certificate within
// seconds, so the API server would reject UCP until rad trusted the new CA after Helm returned. Instead, rad applies
// the radius-apiserver-ca chain from manifest first, with the metadata Helm needs to adopt it into the release, waits
// for cert-manager to issue the CA, and writes the earlier CAs and the new one to the trust Secret and the APIService
// caBundle. The chart then renders the APIService from the trust Secret, and reconcileUCPAPIServerTrust removes the
// earlier CAs after Helm once UCP serves the new certificate.
//
// It does nothing when authorization will be off, the APIService does not exist, as on a fresh install, or the
// APIService caBundle already holds only the key of radius-apiserver-ca. When manifest has no radius-apiserver-ca
// Certificate, as in charts before it, it only records the CAs of the live caBundle in the trust Secret. If cert-manager
// does not issue the CA in time, it returns an error before Helm changes anything; running the command again resumes.
func (i *Impl) stageLegacyUCPAPIServerCA(ctx context.Context, kubeContext, radiusNamespace, releaseName string, mode authz.Mode, render renderRadiusManifest) error {
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
	encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("failed to decode the caBundle of APIService %s: %w", ucpAPIServiceName, err)
	}
	live, err := caCertificates(bundle)
	if err != nil {
		return fmt.Errorf("invalid APIService %s caBundle: %w", ucpAPIServiceName, err)
	}
	if len(live) == 0 {
		return nil
	}
	// Decide from what the API server trusts, not from the inject-ca-from-secret annotation: Helm's switch to
	// authorization off keeps the annotation rad set, while UCP serves a ucp-ca certificate again.
	current, _, _, err := readUCPAPIServerCA(ctx, kubeClient, radiusNamespace)
	if err != nil {
		return err
	}
	if current != nil && onlyKey(live, current) {
		return nil
	}

	manifest, ok, err := render(ctx)
	if err != nil {
		return fmt.Errorf("failed to render the Radius chart to stage %s: %w", ucpAPIServerCASecretName, err)
	}
	if !ok {
		return nil
	}
	objects, err := ucpAPIServerCAStage(manifest)
	if err != nil {
		return fmt.Errorf("failed to read the rendered Radius chart: %w", err)
	}

	trusted, _, err := readUCPAPIServerTrust(ctx, kubeClient, radiusNamespace)
	if err != nil {
		return err
	}
	trusted = unionCertificates(trusted, live)
	if _, err := writeUCPAPIServerTrust(ctx, kubeClient, radiusNamespace, trusted); err != nil {
		return err
	}
	if len(objects) == 0 {
		return nil
	}

	output.LogInfo("Creating %s before UCP moves to it...", ucpAPIServerCASecretName)
	for _, object := range objects {
		if err := applyHelmOwnedObject(ctx, kubeContext, dynamicClient, object.resource, object.object, releaseName, radiusNamespace); err != nil {
			return err
		}
	}

	var ca *x509.Certificate
	var problem string
	err = wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		ca, _, problem, err = readUCPAPIServerCA(ctx, kubeClient, radiusNamespace)
		return ca != nil, err
	})
	if err != nil {
		if wait.Interrupted(err) {
			return fmt.Errorf("Secret %s/%s is not usable after %s: %s. Helm has not changed Radius and UCP still serves its current certificate. "+
				"Check the CA certificate with %s, then run the command again",
				radiusNamespace, ucpAPIServerCASecretName, ucpAPIServiceTimeout, problem,
				kubectlCommand(kubeContext, radiusNamespace, "describe certificate "+ucpAPIServerCASecretName))
		}
		return err
	}

	trusted = unionCertificates(trusted, []*x509.Certificate{ca})
	if err := applyUCPAPIServerTrust(ctx, kubeClient, dynamicClient, radiusNamespace, trusted); err != nil {
		return err
	}
	err = wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		apiService, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to read APIService %s: %w", ucpAPIServiceName, err)
		}
		encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
		bundle, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return false, fmt.Errorf("failed to decode APIService %s caBundle: %w", ucpAPIServiceName, err)
		}
		cas, err := caCertificates(bundle)
		if err != nil {
			return false, fmt.Errorf("invalid APIService %s caBundle: %w", ucpAPIServiceName, err)
		}
		return containsCertificates(cas, trusted), nil
	})
	if err != nil {
		if wait.Interrupted(err) {
			return fmt.Errorf("the caBundle of APIService %s does not trust both the current CA of UCP and %s after %s. Helm has not changed Radius; check it with %s, then run the command again",
				ucpAPIServiceName, ucpAPIServerCASecretName, ucpAPIServiceTimeout, kubectlCommand(kubeContext, "", "get apiservice "+ucpAPIServiceName+" -o yaml"))
		}
		return err
	}
	return nil
}

// onlyKey reports whether every CA in certs uses the key of ca, as when the caBundle holds only radius-apiserver-ca,
// including copies that cert-manager reissued with the same key.
func onlyKey(certs []*x509.Certificate, ca *x509.Certificate) bool {
	for _, cert := range certs {
		if !bytes.Equal(cert.RawSubjectPublicKeyInfo, ca.RawSubjectPublicKeyInfo) {
			return false
		}
	}
	return true
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
		configuration, err := i.cluster().RenderConfiguration(kubeContext, namespace)
		if err != nil {
			return "", false, err
		}
		releases, err := releaseHistoryStorage(history)
		if err != nil {
			return "", false, err
		}
		configuration.Releases = releases
		client := newUpgradeClient(configuration, namespace, false, reuseValues, 0, usesServerSideApply(configuration, releaseName))
		client.DryRunStrategy = helm.DryRunServer
		upgraded, err := client.RunWithContext(ctx, releaseName, cloneChart(helmChart), copyValues(values))
		if err != nil {
			return "", false, err
		}
		rendered, err := asRelease(upgraded)
		if err != nil {
			return "", false, err
		}
		return rendered.Manifest, true, nil
	}
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
		return rendered.Manifest, true, nil
	}
}

// renderRadiusRollback returns the manifest of revision in history, which Helm's rollback applies.
func renderRadiusRollback(history []*releasev1.Release, revision int) renderRadiusManifest {
	return func(context.Context) (string, bool, error) {
		for _, rel := range history {
			if rel != nil && rel.Version == revision {
				return rel.Manifest, true, nil
			}
		}
		return "", false, nil
	}
}
