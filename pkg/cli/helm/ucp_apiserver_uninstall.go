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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// helmResourcePolicyAnnotation set to keep makes Helm leave an object when it uninstalls the release.
const helmResourcePolicyAnnotation = "helm.sh/resource-policy"

// ucpOffCertificateStageObject is Secret ucp-cert, which stageUCPOffCertificate applies with the release's Helm metadata
// before the Helm upgrade to authorization off adopts it. A release with authorization on renders a Certificate of
// that name instead, so Helm does not delete the Secret when it uninstalls that release.
var ucpOffCertificateStageObject = stageInventoryObject{kind: "Secret", name: ucpOffCertificateSecretName, resource: corev1.SchemeGroupVersion.WithResource("secrets")}

// deleteUCPAPIServerLeftovers deletes, after Helm uninstalls Radius, the objects of UCP's API server that Helm leaves:
// the radius-apiserver-ca chain and Secret ucp-cert that an interrupted command staged but Helm never adopted, Secret
// radius-apiserver-trust, and the Secrets radius-apiserver-ca and ucp-apiserver-tls, which hold private keys, once
// their Certificates are gone. It deletes only what rad or cert-manager wrote for the release, each pinned to the uid
// and resourceVersion rad read, and warns about anything else with those names.
func (i *Impl) deleteUCPAPIServerLeftovers(ctx context.Context, kubeContext string, store *rotationStore) error {
	radiusNamespace, dynamicClient := store.namespace, store.dynamic
	if err := i.deleteUnadoptedUCPAPIServerStage(ctx, kubeContext, dynamicClient, radiusNamespace, radiusReleaseName, nil); err != nil {
		return err
	}
	if err := i.deleteUnadoptedStagedObject(ctx, kubeContext, dynamicClient, ucpOffCertificateStageObject, radiusNamespace, radiusReleaseName); err != nil {
		return err
	}
	return i.deleteUCPOffLeftovers(ctx, kubeContext, store.kube, dynamicClient, radiusNamespace, ucpOffLeftoverSecrets)
}

// RadiusUninstallLeftovers lists the objects of UCP's API server that deleteUCPAPIServerLeftovers would delete. Helm
// uninstalls Radius before rad deletes them, so when that cleanup failed, the release is gone but they remain; a later
// uninstall finds no Radius and resumes the cleanup with DeleteRadiusUninstallLeftovers. It only reads the cluster.
func (i *Impl) RadiusUninstallLeftovers(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) ([]string, error) {
	radiusNamespace := clusterOptions.Radius.Namespace
	if err := i.guardCertificateRotation(ctx, kubeContext, radiusNamespace, "uninstall", nil); err != nil {
		return nil, err
	}
	store, err := i.rotationStoreForContext(kubeContext, radiusNamespace)
	if err != nil {
		return nil, err
	}
	journal, state, err := readInitialCertificateJournal(ctx, store)
	if err == nil {
		b := &initialCertificateInstaller{store: store, state: state}
		return issuedUninstallInventory(ctx, store, b.bootstrapOrphans(), []string{bootstrapUninstallEntry(journal)})
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err := store.excludeCertificateOffTransition(ctx); err != nil {
		return nil, err
	}
	dynamicClient := store.dynamic
	leftovers := []string{}
	for _, staged := range append(append([]stageInventoryObject{}, ucpAPIServerCAStageObjects...), ucpOffCertificateStageObject) {
		object, err := dynamicClient.Resource(staged.resource).Namespace(radiusNamespace).Get(ctx, staged.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read %s %s/%s: %w", staged.kind, radiusNamespace, staged.name, err)
		}
		if object.GetUID() != "" && object.GetResourceVersion() != "" && object.GetDeletionTimestamp() == nil && stagedForRelease(object, radiusNamespace, radiusReleaseName) {
			leftovers = append(leftovers, uninstallInventoryEntry(staged.kind, object))
		}
	}
	return issuedUninstallInventory(ctx, store, ucpOffLeftoverSecrets, leftovers)
}

func issuedUninstallInventory(ctx context.Context, store *rotationStore, entries []ucpOffLeftoverSecret, leftovers []string) ([]string, error) {
	found, err := ucpOffLeftovers(ctx, store.kube, store.dynamic, store.namespace, entries)
	if err != nil {
		return nil, err
	}
	for _, f := range found {
		if f.state == ucpOffSecretLeftover {
			leftovers = append(leftovers, uninstallInventoryEntry("Secret", f.object))
		}
	}
	return leftovers, nil
}

func uninstallInventoryEntry(kind string, obj metav1.Object) string {
	return fmt.Sprintf("%s %s/%s (UID %s, version %s)", kind, obj.GetNamespace(), obj.GetName(), obj.GetUID(), obj.GetResourceVersion())
}

func bootstrapUninstallEntry(journal metav1.Object) string {
	return uninstallInventoryEntry("ConfigMap", journal) + " and its pinned bootstrap roots, public sources, namespace intent"
}

// DeleteRadiusUninstallLeftovers resumes the cleanup that UninstallRadius runs after Helm uninstalls Radius, for a
// cluster where the Radius release is already gone. Like UninstallRadius, it deletes only what rad or cert-manager
// wrote for the release.
func (i *Impl) DeleteRadiusUninstallLeftovers(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error {
	if len(clusterOptions.ConfirmedLeftovers) == 0 {
		return fmt.Errorf("uninstall cleanup requires confirmed inventory")
	}
	if err := i.guardCertificateRotation(ctx, kubeContext, clusterOptions.Radius.Namespace, "uninstall", nil); err != nil {
		return err
	}
	var removed bool
	return i.uninstallRadiusRelease(ctx, clusterOptions.Radius.Namespace, kubeContext, clusterOptions.ConfirmedLeftovers, &removed)
}

// deleteUnadoptedUCPAPIServerStage deletes, after Helm uninstalls Radius or applies a release, each object of ucpAPIServerCAStageObjects
// that rad staged for release releaseName but Helm never adopted, because the command stopped before the Helm upgrade.
// Helm deletes the objects it adopted itself, so any that remain with the release's Helm ownership metadata were
// staged. Each delete carries the uid and resourceVersion rad read, so an object that changed or was recreated since
// survives. rad leaves an object with other ownership, or with helm.sh/resource-policy: keep, and warns how to delete it.
// It keeps each object that declared, the objects of the release Helm applied, holds; declared is empty after Helm
// uninstalls the release.
func (i *Impl) deleteUnadoptedUCPAPIServerStage(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, radiusNamespace, releaseName string, declared manifestObjectSet) error {
	for index := len(ucpAPIServerCAStageObjects) - 1; index >= 0; index-- {
		staged := ucpAPIServerCAStageObjects[index]
		if declared.has(staged) {
			continue
		}
		if err := i.deleteUnadoptedStagedObject(ctx, kubeContext, dynamicClient, staged, radiusNamespace, releaseName); err != nil {
			return err
		}
	}
	return nil
}

func (i *Impl) deleteUnadoptedStagedObject(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, staged stageInventoryObject, radiusNamespace, releaseName string) error {
	namespace, path := radiusNamespace, radiusNamespace+"/"+staged.name
	objects := dynamicClient.Resource(staged.resource).Namespace(namespace)
	read := func() (*unstructured.Unstructured, error) {
		object, err := objects.Get(ctx, staged.name, metav1.GetOptions{})
		// cert-manager's CRDs may already be gone.
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read %s %s: %w", staged.kind, path, err)
		}
		if object.GetDeletionTimestamp() != nil {
			return nil, nil
		}
		return object, nil
	}
	warnForeign := func() {
		i.logInfo("Warning: rad did not delete %s %s, which is not an object rad staged for the Radius release. If nothing else uses it, delete it with %s",
			staged.kind, path, kubectlCommand(kubeContext, namespace, "delete "+strings.ToLower(staged.kind)+" "+staged.name))
	}
	object, err := read()
	if err != nil || object == nil {
		return err
	}
	if !stagedForRelease(object, radiusNamespace, releaseName) {
		warnForeign()
		return nil
	}
	uid, resourceVersion := object.GetUID(), object.GetResourceVersion()
	if uid == "" || resourceVersion == "" {
		i.logInfo("Warning: retaining %s %s without a UID and resourceVersion; check its ownership before retrying.", staged.kind, path)
		return nil
	}
	retain, err := checkUCPMutation(ctx, "delete", staged.kind, object)
	if err != nil || retain {
		return err
	}
	err = objects.Delete(ctx, staged.name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &resourceVersion}})
	if err == nil {
		return nil
	}
	if !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete %s %s: %w", staged.kind, path, err)
	}
	// The object changed or was deleted since rad read it. Decide again from what exists now, but never delete an
	// object rad has not inspected.
	object, err = read()
	if err != nil || object == nil {
		return err
	}
	if !stagedForRelease(object, radiusNamespace, releaseName) {
		warnForeign()
		return nil
	}
	i.logInfo("Warning: rad did not delete %s %s, which changed after rad read it. Run the command again to delete it", staged.kind, path)
	return nil
}

// stagedForRelease reports whether object has the Helm ownership metadata rad sets when it stages an object for
// release releaseName in releaseNamespace, and Helm would delete it with the release.
func stagedForRelease(object *unstructured.Unstructured, releaseNamespace, releaseName string) bool {
	return helmOwned(object, releaseNamespace, releaseName) && object.GetAnnotations()[helmResourcePolicyAnnotation] != "keep"
}

// helmOwned reports whether object has the Helm ownership metadata of release releaseName in releaseNamespace, which
// Helm uninstall requires before it deletes an object of the release.
func helmOwned(object *unstructured.Unstructured, releaseNamespace, releaseName string) bool {
	annotations := object.GetAnnotations()
	return object.GetLabels()[helmManagedByLabel] == helmManagedByValue &&
		annotations[helmReleaseNameAnnotation] == releaseName &&
		annotations[helmReleaseNamespaceAnnotation] == releaseNamespace
}
