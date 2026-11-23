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

// Steady selfSigned trust follows the generated signer's public ca.crt. BYO and
// journal-bound rotation keep their independent exact public-source contract.
func readUCPAPIServerCA(ctx context.Context, client kubernetes.Interface, namespace string) (*x509.Certificate, []byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, "", err
	}
	source, err := aggregationPublicSource(ctx, client, namespace)
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot read permanent aggregation trust: %w", err)
	}
	if source.Name == ucpAPIServerCASecretName {
		cert, bundle, problem, err := readLegacyUCPAPIServerCA(ctx, client, namespace)
		if err != nil {
			return cert, bundle, problem, err
		}
		if cert == nil {
			return nil, nil, problem, fmt.Errorf("selfSigned aggregation signer is not usable: %s", problem)
		}
		store := &rotationStore{kube: client, namespace: namespace, release: radiusReleaseName, now: time.Now}
		_, state, err := readInitialCertificateJournal(ctx, store)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, nil, "", err
		}
		if err == nil {
			if state.Mode != "selfSigned" {
				return nil, nil, "", errors.New("bootstrap mode does not authorize selfSigned renewal")
			}
			anchor := state.Anchors["aggregation"]
			if state.Phase == "complete" {
				if err := validateBootstrapRenewal(source, anchor, store.now()); err != nil {
					return nil, nil, "", err
				}
			} else if err := store.checkCertificateSignerSnapshot(ctx, anchor.Signer, rotationPublish); err != nil {
				return nil, nil, "", err
			}
		}
		return cert, bundle, problem, nil
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

func aggregationPublicSource(ctx context.Context, client kubernetes.Interface, namespace string) (*corev1.Secret, error) {
	journal, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, rotationJournalName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		state, err := decodeCertificateLifecycle(journal, namespace, radiusReleaseName, time.Now())
		if err != nil {
			return nil, err
		}
		if state.Phase != rotationComplete {
			return nil, errors.New("active rotation requires its exact guarded projection")
		}
		source, err := client.CoreV1().Secrets(namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		plane := state.Planes["aggregation"]
		if source.UID != plane.Source.UID || !bytes.Equal(source.Data[caCertificateKey], plane.NewCA) {
			return nil, errors.New("completed rotation source differs from its journal")
		}
		return source, nil
	}
	source, err := client.CoreV1().Secrets(namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if err == nil && source.Annotations["radius.io/certificate-mode"] != "selfSigned" {
		return source, nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	return client.CoreV1().Secrets(namespace).Get(ctx, ucpAPIServerCASecretName, metav1.GetOptions{})
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
	before, err := appliedUCPAPIService(ctx, kubeContext, dynamicClient, namespace, "")
	if err != nil {
		return err
	}
	if before == nil {
		return errors.New("Radius APIService is missing")
	}
	source, err := aggregationPublicSource(ctx, kube, namespace)
	if err != nil {
		return err
	}
	if source.Name == ucpAPIServerCASecretName {
		ca, _, _, err := readUCPAPIServerCA(ctx, kube, namespace)
		if err != nil {
			return err
		}
		if err := validateUCPAPIServerAPIAnchors(ca, before); err != nil {
			return err
		}
	}
	// No Secret deletion or signer-source retargeting. Unknown overlap requires
	// the lifecycle's serving and loaded-trust barriers before retirement.
	if err := settleUCPAPIServerCA(ctx, kubeContext, kube, dynamicClient, namespace, before.GetUID()); err != nil {
		return err
	}
	if err := observeAggregationServing(ctx, kubeContext, namespace, kube); err != nil {
		return err
	}
	after, err := appliedUCPAPIService(ctx, kubeContext, dynamicClient, namespace, before.GetUID())
	if err != nil {
		return err
	}
	if after == nil || after.GetUID() != before.GetUID() || after.GetDeletionTimestamp() != nil ||
		!reflect.DeepEqual(before.Object["spec"], after.Object["spec"]) ||
		!reflect.DeepEqual(before.GetAnnotations(), after.GetAnnotations()) {
		return errors.New("APIService changed during exact-pod serving observation")
	}
	return nil
}

// settleUCPAPIServerCA observes an exact set, including absence of retired and
// expired extras. It does not claim to fence future shared-controller writes.
func settleUCPAPIServerCA(ctx context.Context, kubeContext string, kube kubernetes.Interface, client dynamic.Interface, namespace string, apiUID types.UID) error {
	_, selected, _, err := readUCPAPIServerCA(ctx, kube, namespace)
	if err != nil {
		return err
	}
	source, err := aggregationPublicSource(ctx, kube, namespace)
	if err != nil {
		return err
	}
	clean := 0
	return wait.PollUntilContextTimeout(ctx, ucpAPIServicePollInterval, ucpAPIServiceTimeout, true, func(ctx context.Context) (bool, error) {
		live, err := kube.CoreV1().Secrets(namespace).Get(ctx, source.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if source.Name == ucpAPIServerCASecretName {
			if err := validateUCPAPIServerCAOwnership(live); err != nil {
				return false, err
			}
		} else {
			if err := publicAggregationSource(live, namespace); err != nil {
				return false, err
			}
		}
		if live.UID != source.UID || live.ResourceVersion != source.ResourceVersion ||
			exactCertificateSet(live.Data[caCertificateKey], selected, time.Now()) != nil {
			return false, errors.New("aggregation public source changed during convergence")
		}
		api, err := appliedUCPAPIService(ctx, kubeContext, client, namespace, apiUID)
		if err != nil {
			return false, err
		}
		if api == nil || api.GetUID() == "" || api.GetDeletionTimestamp() != nil ||
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
		if api.GetAnnotations()[injectCAFromSecretAnnotation] != namespace+"/"+source.Name ||
			api.GetAnnotations()["cert-manager.io/inject-ca-from"] != "" ||
			api.GetAnnotations()["cert-manager.io/inject-apiserver-ca"] != "" {
			return false, errors.New("APIService does not select the permanent public aggregation source")
		}
		value, _, err := unstructured.NestedString(api.Object, "spec", "caBundle")
		if err != nil {
			return false, err
		}
		if value == "" {
			clean = 0
			return false, nil
		}
		bundle, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return false, err
		}
		if source.Name == ucpAPIServerCASecretName {
			current, err := validatePublicCA(selected, time.Now())
			if err != nil {
				return false, err
			}
			if err := validateUCPAPIServerAPIAnchors(current, api); err != nil {
				return false, err
			}
			return renewedCertificateSet(bundle, current)
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

func (i *Impl) stageUCPAPIServerCA(ctx context.Context, kubeContext, namespace, release string, deployed, mode authz.Mode, render renderRadiusManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if mode == authz.ModeOff {
		if err := i.clearAggregationInjection(ctx, kubeContext, namespace, release); err != nil {
			return err
		}
		if deployed == authz.ModeOff {
			return nil
		}
		if err := i.stageUCPOffCertificate(ctx, kubeContext, namespace, release, render); err != nil {
			return err
		}
		if operation, ok := ctx.Value(certificateOffOperationKey{}).(*certificateOffOperation); ok {
			if err := operation.observe(ctx); err != nil {
				return err
			}
			return operation.check(ctx)
		}
		return nil
	}
	if deployed == authz.ModeOff {
		return errCertificateObserverUnavailable
	}
	kube, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return fmt.Errorf("%w: %w", errCertificateObserverUnavailable, err)
	}
	if render == nil {
		if _, err := retainedUCPAPIServerTrust(ctx, kube, namespace); err != nil {
			return err
		}
		return fmt.Errorf("selfSigned aggregation requires the selected release render: %w", errCertificateObserverUnavailable)
	}
	if err := checkCurrentAggregationSource(ctx, kube, dynamicClient, namespace); err != nil {
		return err
	}
	manifest, ok, err := render(ctx)
	if err != nil || !ok {
		return err
	}
	selected, err := renderedAggregationTrust(manifest, namespace)
	if err != nil {
		return fmt.Errorf("%w: %w", errCertificateObserverUnavailable, err)
	}
	if len(selected) == 0 {
		return fmt.Errorf("selfSigned aggregation requires CA issuance and a second connected render before APIService readiness; automatic bootstrap is unavailable: %w", errCertificateObserverUnavailable)
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

func checkCurrentAggregationSource(ctx context.Context, kube kubernetes.Interface, dynamicClient dynamic.Interface, namespace string) error {
	api, apiErr := dynamicClient.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if apiErr != nil && !apierrors.IsNotFound(apiErr) {
		return apiErr
	}
	if apiErr == nil {
		source, err := aggregationPublicSource(ctx, kube, namespace)
		if err != nil {
			return fmt.Errorf("%w: %w", errCertificateObserverUnavailable, err)
		}
		if source.Name == ucpAPIServerCASecretName {
			ca, _, _, err := readUCPAPIServerCA(ctx, kube, namespace)
			if err != nil {
				return err
			}
			if err := validateUCPAPIServerAPIAnchors(ca, api); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderedAggregationTrust(manifest, namespace string) ([]byte, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var bundle []byte
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if errors.Is(err, io.EOF) {
			return bundle, nil
		}
		if err != nil {
			return nil, err
		}
		var metadata metav1.PartialObjectMetadata
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, err
		}
		if metadata.Kind == "APIService" && metadata.Name == ucpAPIServiceName {
			var api unstructured.Unstructured
			if err := json.Unmarshal(raw, &api); err != nil {
				return nil, err
			}
			if api.GetAnnotations()[injectCAFromSecretAnnotation] == namespace+"/"+ucpAPIServerCASecretName {
				value, _, err := unstructured.NestedString(api.Object, "spec", "caBundle")
				if err != nil {
					return nil, err
				}
				if value == "" {
					return nil, errors.New("rendered aggregation APIService has no CA bundle")
				}
				if bundle != nil {
					return nil, errors.New("rendered aggregation trust has multiple sources")
				}
				bundle, err = base64.StdEncoding.DecodeString(value)
				if err != nil {
					return nil, err
				}
				if _, err := validatePublicCA(bundle, time.Now()); err != nil {
					return nil, err
				}
			}
			continue
		}
		if metadata.Kind != "Secret" || metadata.Name != ucpAPIServerTrustSecretName {
			continue
		}
		var object corev1.Secret
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, err
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
	store, err := i.certificateOffStore(kubeContext, namespace)
	if err != nil {
		return err
	}
	if store.release != release {
		return errors.New("authorization Off release identity changed")
	}
	// This is exclusion only. Injection removal belongs to the inherited
	// post-Helm writer, after UCP serves the Off certificate.
	return store.excludeCertificateOffTransition(ctx)
}
