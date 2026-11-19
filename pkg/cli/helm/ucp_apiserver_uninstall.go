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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// helmResourcePolicyAnnotation set to keep makes Helm leave an object when it uninstalls the release.
const helmResourcePolicyAnnotation = "helm.sh/resource-policy"

// deleteUnadoptedUCPAPIServerStage deletes, after Helm uninstalls Radius, each object of ucpAPIServerCAStageObjects
// that rad staged for release releaseName but Helm never adopted, because the command stopped before the Helm upgrade.
// Helm deletes the objects it adopted itself, so any that remain with the release's Helm ownership metadata were
// staged. Each delete carries the uid and resourceVersion rad read, so an object that changed or was recreated since
// survives. rad leaves an object with other ownership, or with helm.sh/resource-policy: keep, and warns how to delete it.
func (i *Impl) deleteUnadoptedUCPAPIServerStage(ctx context.Context, kubeContext string, dynamicClient dynamic.Interface, radiusNamespace, releaseName string) error {
	for index := len(ucpAPIServerCAStageObjects) - 1; index >= 0; index-- {
		if err := i.deleteUnadoptedStagedObject(ctx, kubeContext, dynamicClient, ucpAPIServerCAStageObjects[index], radiusNamespace, releaseName); err != nil {
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
