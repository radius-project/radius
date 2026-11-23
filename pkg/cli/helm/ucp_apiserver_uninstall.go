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
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// helmResourcePolicyAnnotation set to keep makes Helm leave an object when it uninstalls the release.
const helmResourcePolicyAnnotation = "helm.sh/resource-policy"

// ucpOffCertificateStageObject is Secret ucp-cert, which stageUCPOffCertificate applies with the release's Helm metadata
// before the Helm upgrade to authorization off adopts it. A release with authorization on renders a Certificate of
// that name instead, so Helm does not delete the Secret when it uninstalls that release.
var ucpOffCertificateStageObject = stageInventoryObject{kind: "Secret", name: ucpOffCertificateSecretName, resource: secretsResource}

var secretsResource = corev1.SchemeGroupVersion.WithResource("secrets")

// ucpUninstallLeftoverSecrets are the Secrets that cert-manager issues for Certificates of the Radius release. They have
// no owner reference, so neither Helm nor Kubernetes deletes them with the release.
var ucpUninstallLeftoverSecrets = []ucpOffLeftoverSecret{
	{secret: ucpAPIServerCASecretName, certificate: ucpAPIServerCASecretName},
	{secret: ucpAPIServerTLSSecretName, certificate: ucpAPIServerCertificateName},
}

// deleteUCPAPIServerLeftovers deletes, after Helm uninstalls Radius, the objects of UCP's API server that Helm leaves:
// the radius-apiserver-ca chain and Secret ucp-cert that an interrupted command staged but Helm never adopted, Secret
// radius-apiserver-trust, and the Secrets radius-apiserver-ca and ucp-apiserver-tls, which hold private keys, once
// their Certificates are gone. It deletes only what rad or cert-manager wrote for the release, each pinned to the uid
// and resourceVersion rad read, and warns about anything else with those names.
func (i *Impl) deleteUCPAPIServerLeftovers(ctx context.Context, kubeContext, radiusNamespace string) error {
	kubeClient, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}
	if err := i.deleteUnadoptedUCPAPIServerStage(ctx, kubeContext, dynamicClient, radiusNamespace, radiusReleaseName, nil, true); err != nil {
		return err
	}
	if err := i.deleteUnadoptedStagedObject(ctx, kubeContext, dynamicClient, ucpOffCertificateStageObject, radiusNamespace, radiusReleaseName, true); err != nil {
		return err
	}
	if err := i.deleteOwnedUCPAPIServerTrust(ctx, kubeContext, kubeClient, radiusNamespace); err != nil {
		return err
	}
	return i.deleteUCPOffLeftovers(ctx, kubeContext, kubeClient, dynamicClient, radiusNamespace, ucpUninstallLeftoverSecrets, true)
}

// UninstallLeftover is an object that a previous Radius uninstall left after Helm removed the release, as rad read it.
type UninstallLeftover struct {
	Kind, Namespace, Name string
	resource              schema.GroupVersionResource
	uid                   types.UID
	resourceVersion       string
}

func (l UninstallLeftover) String() string {
	return fmt.Sprintf("%s %s/%s", l.Kind, l.Namespace, l.Name)
}

// RadiusUninstallLeftovers lists the objects of UCP's API server that deleteUCPAPIServerLeftovers would delete. Helm
// uninstalls Radius before rad deletes them, so when that cleanup failed, the release is gone but they remain; a later
// uninstall finds no Radius, shows this list, and passes it to DeleteRadiusUninstallLeftovers. It includes the Secret
// that cert-manager issued for each listed Certificate, because deleting the Certificate leaves that Secret unused. It
// only reads the cluster.
func (i *Impl) RadiusUninstallLeftovers(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) ([]UninstallLeftover, error) {
	kubeClient, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return nil, err
	}
	radiusNamespace := clusterOptions.Radius.Namespace
	leftovers := []UninstallLeftover{}
	planned := func(kind, name string) bool {
		return slices.ContainsFunc(leftovers, func(l UninstallLeftover) bool { return l.Kind == kind && l.Name == name })
	}
	add := func(kind string, resource schema.GroupVersionResource, object metav1.Object) {
		leftovers = append(leftovers, UninstallLeftover{Kind: kind, Namespace: radiusNamespace, Name: object.GetName(), resource: resource,
			uid: object.GetUID(), resourceVersion: object.GetResourceVersion()})
	}
	for index := len(ucpAPIServerCAStageObjects) - 1; index >= -1; index-- {
		staged := ucpOffCertificateStageObject
		if index >= 0 {
			staged = ucpAPIServerCAStageObjects[index]
		}
		object, err := dynamicClient.Resource(staged.resource).Namespace(radiusNamespace).Get(ctx, staged.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read %s %s/%s: %w", staged.kind, radiusNamespace, staged.name, err)
		}
		if object.GetDeletionTimestamp() == nil && stagedForRelease(object, radiusNamespace, radiusReleaseName) {
			add(staged.kind, staged.resource, object)
		}
	}
	_, trust, err := readUCPAPIServerTrust(ctx, kubeClient, radiusNamespace)
	if err != nil {
		return nil, err
	}
	if trust != nil && ownsUCPAPIServerTrust(trust) {
		add("Secret", secretsResource, trust)
	}
	for _, leftover := range ucpUninstallLeftoverSecrets {
		found, err := ucpOffLeftover(ctx, kubeClient, dynamicClient, radiusNamespace, leftover)
		if err != nil {
			return nil, err
		}
		// A Secret whose Certificate rad deletes with this list is a leftover once that delete succeeds.
		if found.object != nil && found.object.DeletionTimestamp == nil && (found.state == ucpOffSecretLeftover ||
			(found.state == ucpOffSecretKeep && planned("Certificate", leftover.certificate) && ownsUCPOffSecret(found.object, leftover))) {
			add("Secret", secretsResource, found.object)
		}
	}
	return leftovers, nil
}

// DeleteRadiusUninstallLeftovers resumes the cleanup that UninstallRadius runs after Helm uninstalls Radius, for a
// cluster where the Radius release is already gone. It deletes only leftovers, which RadiusUninstallLeftovers listed
// and the user confirmed, each pinned to the uid and resourceVersion rad read for the list, so an object that changed
// or was replaced since survives. It stops at the first object that changed and returns an error, because a listed Secret
// can depend on a Certificate listed before it, and the cleanup is then incomplete, so the caller keeps cert-manager.
func (i *Impl) DeleteRadiusUninstallLeftovers(ctx context.Context, kubeContext string, leftovers []UninstallLeftover) error {
	kubeClient, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}
	for _, leftover := range leftovers {
		options := metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &leftover.uid, ResourceVersion: &leftover.resourceVersion}}
		if leftover.Kind == "Secret" {
			err = kubeClient.CoreV1().Secrets(leftover.Namespace).Delete(ctx, leftover.Name, options)
		} else {
			err = dynamicClient.Resource(leftover.resource).Namespace(leftover.Namespace).Delete(ctx, leftover.Name, options)
		}
		switch {
		case err == nil, apierrors.IsNotFound(err), meta.IsNoMatchError(err):
		case apierrors.IsConflict(err):
			return fmt.Errorf("%s changed after rad listed it, so rad stopped before deleting the remaining objects. Run the command again to review them", leftover)
		default:
			return fmt.Errorf("failed to delete %s: %w", leftover, err)
		}
	}
	return nil
}

// deleteUnadoptedUCPAPIServerStage deletes, after Helm uninstalls Radius or applies a release, each object of ucpAPIServerCAStageObjects
// that rad staged for release releaseName but Helm never adopted, because the command stopped before the Helm upgrade.
// Helm deletes the objects it adopted itself, so any that remain with the release's Helm ownership metadata were
// staged. Each delete carries the uid and resourceVersion rad read, so an object that changed or was recreated since
// survives. rad leaves an object with other ownership, or with helm.sh/resource-policy: keep, and warns how to delete it.
// It keeps each object that declared, the objects of the release Helm applied, holds; declared is empty after Helm
// uninstalls the release. When an object changed before rad deleted it, it warns, or, when incompleteErr is set, stops
// with an error, so that uninstall keeps cert-manager for a retry.
func (i *Impl) deleteUnadoptedUCPAPIServerStage(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, radiusNamespace, releaseName string, declared manifestObjectSet, incompleteErr bool) error {
	for index := len(ucpAPIServerCAStageObjects) - 1; index >= 0; index-- {
		staged := ucpAPIServerCAStageObjects[index]
		if declared.has(staged) {
			continue
		}
		if err := i.deleteUnadoptedStagedObject(ctx, kubeContext, dynamicClient, staged, radiusNamespace, releaseName, incompleteErr); err != nil {
			return err
		}
	}
	return nil
}

func (i *Impl) deleteUnadoptedStagedObject(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, staged stageInventoryObject, radiusNamespace, releaseName string, incompleteErr bool) error {
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
	if incompleteErr {
		return fmt.Errorf("rad did not delete %s %s, which changed after rad read it. Run the command again to delete it", staged.kind, path)
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
func helmOwned(object metav1.Object, releaseNamespace, releaseName string) bool {
	annotations := object.GetAnnotations()
	return object.GetLabels()[helmManagedByLabel] == helmManagedByValue &&
		annotations[helmReleaseNameAnnotation] == releaseName &&
		annotations[helmReleaseNamespaceAnnotation] == releaseNamespace
}
