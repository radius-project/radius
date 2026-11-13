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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

func initialCertificatePolicyChart(selected *chart.Chart) *chart.Chart {
	result := certificateHelperChart(selected)
	result.Templates = append(result.Templates, &common.File{Name: "templates/initial-certificate-policy.yaml",
		Data: []byte(`{{ include "radius.authz.certificates.validate" . }}`)})
	return result
}

func initialRootResource(key string) (schema.GroupVersionResource, string, error) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 || parts[0] != "cert-manager.io" {
		return schema.GroupVersionResource{}, "", errors.New("invalid initial root resource")
	}
	resource := rotationCertificateResource
	if parts[1] == "issuers" && parts[2] == "radius-selfsigned-bootstrap" {
		resource = rotationIssuerResource
	} else if parts[1] != "certificates" || parts[2] != "radius-ca" && parts[2] != "radius-apiserver-ca" {
		return schema.GroupVersionResource{}, "", errors.New("bootstrap resource is not a selected root")
	}
	return resource, parts[2], nil
}

func (b *initialCertificateInstaller) initialRoots(ctx context.Context) ([]*unstructured.Unstructured, error) {
	selected := cloneChart(b.selected)
	selected.Templates = nil
	for _, file := range b.selected.Templates {
		if strings.HasPrefix(file.Name, "templates/_") {
			selected.Templates = append(selected.Templates, &common.File{Name: file.Name, Data: file.Data})
		}
		selected.Templates = append(selected.Templates, &common.File{Name: "templates/initial-roots.yaml", Data: []byte(`
{{ include "radius.authz.certificates.validate" . }}
{{ include "radius.authz.certificates.bootstrapIssuer" . }}
{{ if eq (include "radius.authz.tls.provider" .) "radius" }}
---
{{ include "radius.authz.certificates.root" (dict "root" . "name" "radius-ca") }}
{{ end }}
---
{{ include "radius.authz.certificates.root" (dict "root" . "name" "radius-apiserver-ca") }}
`)})
	}
	manifest, err := b.impl.renderBootstrapChart(ctx, selected, b.options, b.values, b.kubeContext)
	if err != nil {
		return nil, err
	}
	names := []string{"radius-selfsigned-bootstrap", "radius-apiserver-ca"}
	if b.state.Provider == "radius" {
		names = append(names, "radius-ca")
	}
	objects := map[string]*unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		object := &unstructured.Unstructured{}
		if err := decoder.Decode(object); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		for _, name := range names {
			if object.GetName() != name {
				continue
			}
			if objects[name] != nil || object.GetNamespace() != b.store.namespace || object.GetAPIVersion() != "cert-manager.io/v1" {
				return nil, errors.New("selected chart has ambiguous initial root resources")
			}
			if name == "radius-selfsigned-bootstrap" {
				spec, _, err := unstructured.NestedMap(object.Object, "spec")
				if err != nil || object.GetKind() != "Issuer" || len(spec) != 1 || spec["selfSigned"] == nil {
					return nil, errors.New("selected chart bootstrap issuer is not selfSigned")
				}
			} else {
				isCA, _, _ := unstructured.NestedBool(object.Object, "spec", "isCA")
				secret, _, _ := unstructured.NestedString(object.Object, "spec", "secretName")
				issuer, _, _ := unstructured.NestedString(object.Object, "spec", "issuerRef", "name")
				if object.GetKind() != "Certificate" || !isCA || secret != name || issuer != "radius-selfsigned-bootstrap" {
					return nil, errors.New("selected chart root is not the expected namespace-local CA")
				}
			}
			objects[name] = object
		}
	}
	var result []*unstructured.Unstructured
	for _, name := range names {
		if objects[name] == nil {
			return nil, fmt.Errorf("selected chart is missing initial root %s", name)
		}
		result = append(result, objects[name])
	}
	return result, nil
}

func (b *initialCertificateInstaller) ensureRoot(ctx context.Context, desired *unstructured.Unstructured) error {
	if err := b.checkMaterial(ctx); err != nil {
		return err
	}
	resource := rotationCertificateResource
	if desired.GetKind() == "Issuer" {
		resource = rotationIssuerResource
	}
	key := bootstrapResourceKey(resource, desired.GetName())
	if _, _, err := initialRootResource(key); err != nil {
		return err
	}
	expected := desired.DeepCopy()
	labels, annotations := expected.GetLabels(), expected.GetAnnotations()
	if labels == nil {
		labels = map[string]string{}
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	labels[helmManagedByLabel] = helmManagedByValue
	annotations[helmReleaseNameAnnotation], annotations[helmReleaseNamespaceAnnotation] = b.store.release, b.store.namespace
	annotations[initialCertificateIntent] = string(b.journal.UID)
	expected.SetLabels(labels)
	expected.SetAnnotations(annotations)
	client := b.store.dynamic.Resource(resource).Namespace(b.store.namespace)
	object, err := client.Get(ctx, expected.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if err := b.check(ctx); err != nil {
			return err
		}
		object, err = client.Create(ctx, expected, metav1.CreateOptions{FieldManager: FieldManager})
	}
	if err != nil {
		return err
	}
	pin, err := bootstrapResourcePin(object)
	if err != nil {
		return err
	}
	// Admission may add cert-manager defaults. Pin the entire admitted spec,
	// while requiring every explicitly selected field to remain unchanged.
	if !apiequality.Semantic.DeepDerivative(expected.Object["spec"], object.Object["spec"]) ||
		!rotationOwned(object, b.store.namespace, b.store.release) ||
		object.GetAnnotations()[initialCertificateIntent] != string(b.journal.UID) || len(object.GetOwnerReferences()) != 0 {
		return errors.New("initial root cannot be adopted or changed by bootstrap")
	}
	if previous, found := b.state.Roots[key]; found {
		if previous.UID != pin.UID || previous.SpecSHA256 != pin.SpecSHA256 {
			return errors.New("initial root was replaced")
		}
		return nil
	}
	b.state.Roots[key] = pin
	return b.save(ctx)
}

func (b *initialCertificateInstaller) rootIssued(ctx context.Context, name string, secret *corev1.Secret) (bool, error) {
	pin, found := b.state.Roots[bootstrapResourceKey(rotationCertificateResource, name)]
	if !found {
		return false, errors.New("bootstrap signer has no pinned Certificate")
	}
	root, err := b.store.dynamic.Resource(rotationCertificateResource).Namespace(b.store.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if root.GetUID() != pin.UID || root.GetDeletionTimestamp() != nil ||
		secret.Annotations["cert-manager.io/issuer-name"] != "radius-selfsigned-bootstrap" ||
		secret.Annotations["cert-manager.io/issuer-kind"] != "Issuer" ||
		secret.Annotations["cert-manager.io/issuer-group"] != "cert-manager.io" ||
		secret.CreationTimestamp.Time.Before(root.GetCreationTimestamp().Time) {
		return false, errors.New("bootstrap signer is not bound to the selected root Certificate")
	}
	for _, owner := range secret.OwnerReferences {
		if owner.UID != root.GetUID() || owner.Kind != "Certificate" || owner.Name != name ||
			owner.APIVersion != "cert-manager.io/v1" {
			return false, errors.New("bootstrap signer has a foreign owner")
		}
	}
	conditions, _, err := unstructured.NestedSlice(root.Object, "status", "conditions")
	if err != nil {
		return false, err
	}
	for _, entry := range conditions {
		condition, ok := entry.(map[string]any)
		if !ok {
			return false, errors.New("bootstrap Certificate has invalid status conditions")
		}
		if condition["type"] == "Ready" && condition["status"] == "True" && condition["observedGeneration"] == root.GetGeneration() {
			return true, nil
		}
	}
	return false, nil
}

type initialCertificateGetter struct {
	genericclioptions.RESTClientGetter
	ctx   context.Context
	check func(context.Context) error
}

func (g initialCertificateGetter) ToRESTConfig() (*rest.Config, error) {
	config, err := g.RESTClientGetter.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	config = rest.CopyConfig(config)
	config.Wrap(func(next http.RoundTripper) http.RoundTripper {
		return initialCertificateTransport{next: next, ctx: g.ctx, check: g.check}
	})
	return config, nil
}

type initialCertificateTransport struct {
	next  http.RoundTripper
	ctx   context.Context
	check func(context.Context) error
}

func (t initialCertificateTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		if err := t.check(t.ctx); err != nil {
			return nil, err
		}
	}
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(request.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	response, err := t.next.RoundTrip(request.WithContext(ctx))
	cleanup := func() { stop(); cancel() }
	if err != nil || response == nil || response.Body == nil {
		cleanup()
		return response, err
	}
	response.Body = initialCertificateResponseBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}

type initialCertificateResponseBody struct {
	io.ReadCloser
	cleanup func()
}

func (b initialCertificateResponseBody) Close() error {
	defer b.cleanup()
	return b.ReadCloser.Close()
}

func (b *initialCertificateInstaller) applyChart(ctx context.Context) error {
	if b.helm == nil || b.helm.RESTClientGetter == nil {
		return errors.New("initial installation requires a Kubernetes Helm configuration")
	}
	configuration := &helm.Configuration{}
	base, ok := b.helm.RESTClientGetter.(genericclioptions.RESTClientGetter)
	if !ok {
		return errors.New("initial installation requires a Helm Kubernetes configuration loader")
	}
	getter := initialCertificateGetter{RESTClientGetter: base, ctx: ctx, check: b.checkMaterial}
	if err := configuration.Init(getter, b.store.namespace, helmDriverSecret); err != nil {
		return err
	}
	recorded, err := configuration.Releases.Last(b.store.release)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		install := newInstallClient(configuration, b.store.release, b.store.namespace, b.options.Radius.Wait, b.options.Radius.Timeout)
		install.CreateNamespace = false
		install.Labels = map[string]string{initialCertificateIntent: b.state.ID}
		_, err = install.RunWithContext(ctx, cloneChart(b.selected), copyValues(b.values))
		return err
	}
	if err != nil {
		return err
	}
	previous, ok := recorded.(*releasev1.Release)
	if !ok {
		return errors.New("initial bootstrap requires a supported Helm release record")
	}
	if previous.Labels[initialCertificateIntent] != b.state.ID {
		return errors.New("initial bootstrap cannot upgrade an unrelated Helm installation")
	}
	chartDigest, err := bootstrapDigest(previous.Chart)
	if err != nil || chartDigest != b.state.ChartSHA256 {
		return errors.New("recorded bootstrap Helm chart changed")
	}
	digest, err := bootstrapDigest(previous.Config)
	if err != nil || digest != b.state.ValuesSHA256 {
		return errors.New("recorded bootstrap Helm values changed")
	}
	upgrade := newUpgradeClient(configuration, b.store.namespace, b.options.Radius.Wait, false, b.options.Radius.Timeout,
		usesServerSideApply(configuration, b.store.release))
	upgrade.Labels = map[string]string{initialCertificateIntent: b.state.ID}
	_, err = upgrade.RunWithContext(ctx, b.store.release, cloneChart(b.selected), copyValues(b.values))
	return err
}
