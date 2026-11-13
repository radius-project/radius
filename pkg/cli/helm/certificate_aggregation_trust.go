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
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/authz"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// readUCPAPIServerCA consumes only the permanent public source, never a signer or
// a leaf's returned ca.crt. Overlap is resolved by the lifecycle, not this helper.
func readUCPAPIServerCA(ctx context.Context, client kubernetes.Interface, namespace string) (*x509.Certificate, []byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, "", err
	}
	source, err := client.CoreV1().Secrets(namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot read permanent aggregation trust: %w", err)
	}
	if err := publicAggregationSource(source, namespace); err != nil {
		return nil, nil, "", err
	}
	cert, err := validatePublicCA(source.Data[caCertificateKey], time.Now())
	if err != nil {
		return nil, nil, "", fmt.Errorf("aggregation overlap requires an authorized lifecycle snapshot: %w", err)
	}
	return cert, source.Data[caCertificateKey], "", nil
}

func publicAggregationSource(source *corev1.Secret, namespace string) error {
	if source.UID == "" || source.ResourceVersion == "" || source.Namespace != namespace ||
		source.DeletionTimestamp != nil || len(source.OwnerReferences) != 0 ||
		source.Type != corev1.SecretTypeOpaque || len(source.Data) != 1 || len(source.StringData) != 0 ||
		source.Annotations[allowDirectInjectionAnnotation] != "true" ||
		source.Labels[helmManagedByLabel] != helmManagedByValue ||
		source.Annotations[helmReleaseNameAnnotation] != radiusReleaseName ||
		source.Annotations[helmReleaseNamespaceAnnotation] != namespace {
		return errors.New("permanent aggregation trust has foreign ownership, missing identity, or non-public data")
	}
	return nil
}

func (i *Impl) reconcileUCPAPIServerTrust(ctx context.Context, kubeContext, namespace string) error {
	kube, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}
	before, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// No Secret deletion or signer-source retargeting. Unknown overlap requires
	// the lifecycle's serving and loaded-trust barriers before retirement.
	if err := settleUCPAPIServerCA(ctx, kubeContext, kube, dynamicClient, namespace); err != nil {
		return err
	}
	if err := observeAggregationServing(ctx, kubeContext, namespace, kube); err != nil {
		return err
	}
	after, err := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if after.GetUID() != before.GetUID() || after.GetDeletionTimestamp() != nil ||
		!reflect.DeepEqual(before.Object["spec"], after.Object["spec"]) ||
		!reflect.DeepEqual(before.GetAnnotations(), after.GetAnnotations()) {
		return errors.New("APIService changed during exact-pod serving observation")
	}
	return nil
}

// settleUCPAPIServerCA observes an exact set, including absence of retired and
// expired extras. It does not claim to fence future shared-controller writes.
func settleUCPAPIServerCA(ctx context.Context, kubeContext string, kube kubernetes.Interface, client dynamic.Interface, namespace string) error {
	_, selected, _, err := readUCPAPIServerCA(ctx, kube, namespace)
	if err != nil {
		return err
	}
	source, err := kube.CoreV1().Secrets(namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	clean := 0
	var apiUID types.UID
	return wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		live, err := kube.CoreV1().Secrets(namespace).Get(ctx, source.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if err := publicAggregationSource(live, namespace); err != nil {
			return false, err
		}
		if live.UID != source.UID || live.ResourceVersion != source.ResourceVersion ||
			exactCertificateSet(live.Data[caCertificateKey], selected, time.Now()) != nil {
			return false, errors.New("aggregation public source changed during convergence")
		}
		api, err := client.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if api.GetUID() == "" || api.GetDeletionTimestamp() != nil ||
			api.GetLabels()[helmManagedByLabel] != helmManagedByValue ||
			api.GetAnnotations()[helmReleaseNameAnnotation] != radiusReleaseName ||
			api.GetAnnotations()[helmReleaseNamespaceAnnotation] != namespace {
			return false, errors.New("APIService has no live Radius ownership")
		}
		if apiUID != "" && apiUID != api.GetUID() {
			return false, errors.New("APIService was replaced during convergence")
		}
		apiUID = api.GetUID()
		service, _, err := unstructured.NestedMap(api.Object, "spec", "service")
		if err != nil || service["name"] != "ucp" || service["namespace"] != namespace || service["port"] != int64(443) {
			return false, errors.New("APIService does not select the canonical aggregation Service")
		}
		insecure, _, err := unstructured.NestedBool(api.Object, "spec", "insecureSkipTLSVerify")
		if err != nil || insecure {
			return false, errors.New("APIService does not verify aggregation TLS")
		}
		if api.GetAnnotations()[injectCAFromSecretAnnotation] != ucpAPIServerCASource(namespace) ||
			api.GetAnnotations()["cert-manager.io/inject-ca-from"] != "" ||
			api.GetAnnotations()["cert-manager.io/inject-apiserver-ca"] != "" {
			return false, errors.New("APIService does not select the permanent public aggregation source")
		}
		value, _, err := unstructured.NestedString(api.Object, "spec", "caBundle")
		if err != nil {
			return false, err
		}
		bundle, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return false, err
		}
		if err := exactCertificateSet(bundle, selected, time.Now()); err != nil {
			// A merging injector can preserve old roots indefinitely. Do not
			// silently race it with unconditional patches.
			return false, fmt.Errorf("APIService requires fenced exact-set convergence, not an ordinary Helm cleanup: %w", err)
		}
		clean++
		return clean >= ucpAPIServerCASettleChecks, nil
	})
}

func (i *Impl) stageUCPAPIServerCA(ctx context.Context, kubeContext, namespace, release string, mode authz.Mode, render renderRadiusManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if mode == authz.ModeOff {
		return i.clearAggregationInjection(ctx, kubeContext, namespace, release)
	}
	manifest, ok, err := render(ctx)
	if err != nil || !ok {
		return err
	}
	selected, err := renderedAggregationTrust(manifest, namespace)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return fmt.Errorf("selfSigned aggregation requires CA issuance and a second connected render before APIService readiness; automatic bootstrap is unavailable: %w", errCertificateObserverUnavailable)
	}
	kube, _, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return err
	}
	_, current, _, err := readUCPAPIServerCA(ctx, kube, namespace)
	if err != nil {
		return err
	}
	if err := exactCertificateSet(current, selected, time.Now()); err != nil {
		return fmt.Errorf("%w: %w", errCertificateObserverUnavailable, err)
	}
	return nil
}

func renderedAggregationTrust(manifest, namespace string) ([]byte, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var bundle []byte
	for {
		var object corev1.Secret
		err := decoder.Decode(&object)
		if errors.Is(err, io.EOF) {
			return bundle, nil
		}
		if err != nil {
			return nil, err
		}
		if object.Kind != "Secret" || object.Name != ucpAPIServerTrustSecretName {
			continue
		}
		if bundle != nil || object.Namespace != namespace || len(object.Data) != 1 || len(object.StringData) != 0 ||
			object.Type != corev1.SecretTypeOpaque || object.Annotations[allowDirectInjectionAnnotation] != "true" {
			return nil, errors.New("rendered aggregation trust is ambiguous or not public-only")
		}
		if _, err := validatePublicCA(object.Data[caCertificateKey], time.Now()); err != nil {
			return nil, err
		}
		bundle = object.Data[caCertificateKey]
	}
}

func (i *Impl) clearAggregationInjection(ctx context.Context, kubeContext, namespace, release string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store, err := i.rotationStoreForContext(kubeContext, namespace)
	if err != nil {
		return err
	}
	kube, client := store.kube, store.dynamic
	api, err := client.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if api.GetAnnotations()[injectCAFromSecretAnnotation] == "" &&
		api.GetAnnotations()["cert-manager.io/inject-ca-from"] == "" &&
		api.GetAnnotations()["cert-manager.io/inject-apiserver-ca"] == "" {
		return nil
	}
	if api.GetUID() == "" || api.GetResourceVersion() == "" || api.GetDeletionTimestamp() != nil ||
		api.GetLabels()[helmManagedByLabel] != helmManagedByValue ||
		api.GetAnnotations()[helmReleaseNameAnnotation] != release ||
		api.GetAnnotations()[helmReleaseNamespaceAnnotation] != namespace {
		return errors.New("cannot clear CA injection on an APIService not owned by this Radius release")
	}
	return withIdentityOperation(ctx, kube, namespace, func(operation *identityOperation) error {
		if err := operation.checkProviderJournal(ctx, release, i.validateCompletedProviderJournal); err != nil {
			return err
		}
		if err := i.guardCertificateRotation(ctx, kubeContext, namespace, "authorization Off", nil); err != nil {
			return err
		}
		patch := []map[string]any{
			{"op": "test", "path": "/metadata/uid", "value": api.GetUID()},
			{"op": "test", "path": "/metadata/resourceVersion", "value": api.GetResourceVersion()},
		}
		for _, annotation := range []string{injectCAFromSecretAnnotation, "cert-manager.io/inject-ca-from", "cert-manager.io/inject-apiserver-ca"} {
			if _, found := api.GetAnnotations()[annotation]; found {
				patch = append(patch, map[string]any{"op": "remove", "path": "/metadata/annotations/" + strings.ReplaceAll(annotation, "/", "~1")})
			}
		}
		data, err := json.Marshal(patch)
		if err != nil {
			return err
		}
		if err := operation.check(ctx); err != nil {
			return err
		}
		_, err = client.Resource(apiServicesResource).Patch(ctx, api.GetName(), types.JSONPatchType, data, metav1.PatchOptions{FieldManager: FieldManager})
		return err
	})
}
