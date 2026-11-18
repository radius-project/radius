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
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	initialPolicyName  = "radius-certificate-issuance"
	initialPolicyKind  = "ValidatingAdmissionPolicy"
	initialBindingKind = "ValidatingAdmissionPolicyBinding"
	initialProbeName   = "radius-bootstrap-admission-check"
)

var initialAdmissionResources = map[string]schema.GroupVersionResource{
	initialPolicyKind:  {Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingadmissionpolicies"},
	initialBindingKind: {Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingadmissionpolicybindings"},
}

// Only this public, signed CSR is retained. There is no corresponding key to
// store, and the request is submitted exclusively with DryRunAll.
const initialProbeCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIHlMIGMAgEAMCoxKDAmBgNVBAMMH3JhZC1ib290c3RyYXAtYWRtaXNzaW9uLmlu
dmFsaWQwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQImy9GztdKItr4VmTU0zpH
hgjdHOq+KRzlnA5BHWLxUNy/sdrG1nIZcjUnNCuXoYpvFHj0G0sJbMxz1ayVf3FK
oAAwCgYIKoZIzj0EAwIDSAAwRQIhAN0CWj66aUsRx/ueWOYV/ephCECzlxWFrZxT
efZFhbmnAiA63GZL4JmO1odQ082vWhn7P/EIuVdrhK2GKSw0fbv7pg==
-----END CERTIFICATE REQUEST-----
`

func initialAdmissionChart(source *chart.Chart) *chart.Chart {
	selected := certificateHelperChart(source)
	for _, file := range source.Templates {
		if file.Name == "templates/global/certificate-request-policy.yaml" {
			selected.Templates = append(selected.Templates, file)
		}
	}
	return selected
}

func (b *initialCertificateInstaller) stageInitialAdmission(ctx context.Context) error {
	manifest, err := b.impl.renderBootstrapChart(ctx, initialAdmissionChart(b.selected), b.options, b.values, b.kubeContext)
	if err != nil {
		return err
	}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	objects := map[string]*unstructured.Unstructured{}
	for {
		var obj unstructured.Unstructured
		if err := decoder.Decode(&obj); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		resource, ok := initialAdmissionResources[obj.GetKind()]
		if !ok || obj.GetAPIVersion() != resource.GroupVersion().String() || obj.GetNamespace() != "" ||
			obj.GetName() != initialPolicyName || objects[obj.GetKind()] != nil {
			return errors.New("unexpected or duplicate object in selected requester policy template")
		}
		objects[obj.GetKind()] = &obj
	}
	for _, kind := range []string{initialPolicyKind, initialBindingKind} {
		if objects[kind] == nil {
			return fmt.Errorf("selected chart does not render %s %s", kind, initialPolicyName)
		}
	}
	// Spell out the Kubernetes v1 defaults so exact spec comparison does not
	// ignore unexpected selectors, exclusions or match conditions.
	match, found, err := unstructured.NestedMap(objects[initialPolicyKind].Object, "spec", "matchConstraints")
	if err != nil || !found {
		return errors.New("selected requester restriction lacks match constraints")
	}
	if _, ok := match["matchPolicy"]; !ok {
		match["matchPolicy"] = "Equivalent"
	}
	if _, ok := match["objectSelector"]; !ok {
		match["objectSelector"] = map[string]any{}
	}
	rules, _, err := unstructured.NestedSlice(match, "resourceRules")
	if err != nil || len(rules) == 0 {
		return errors.New("selected requester restriction lacks resource rules")
	}
	for _, rule := range rules {
		r, ok := rule.(map[string]any)
		if !ok {
			return errors.New("invalid requester restriction resource rule")
		}
		if _, ok := r["scope"]; !ok {
			r["scope"] = "*"
		}
	}
	match["resourceRules"] = rules
	if err := unstructured.SetNestedMap(objects[initialPolicyKind].Object, match, "spec", "matchConstraints"); err != nil {
		return err
	}
	for _, kind := range []string{initialPolicyKind, initialBindingKind} {
		if err := b.ensureInitialAdmissionObject(ctx, objects[kind]); err != nil {
			return err
		}
	}
	return b.probeInitialAdmission(ctx)
}

func (b *initialCertificateInstaller) ownsInitialAdmission(obj *unstructured.Unstructured) bool {
	intent := obj.GetAnnotations()[initialCertificateIntent]
	return rotationOwned(obj, b.store.namespace, b.store.release) && obj.GetLabels()[helmManagedByLabel] == helmManagedByValue &&
		len(obj.GetOwnerReferences()) == 0 && (intent == "" || intent == string(b.journal.UID))
}

func initialAdmissionUIDAnnotation(kind string) string {
	return "radius.io/initial-" + initialAdmissionResources[kind].Resource + "-uid"
}

func (b *initialCertificateInstaller) ensureInitialAdmissionObject(ctx context.Context, desired *unstructured.Unstructured) error {
	kind := desired.GetKind()
	client := b.store.dynamic.Resource(initialAdmissionResources[kind])
	if err := b.checkMaterial(ctx); err != nil {
		return err
	}
	obj, err := client.Get(ctx, initialPolicyName, metav1.GetOptions{})
	previousUID := b.journal.Annotations[initialAdmissionUIDAnnotation(kind)]
	if apierrors.IsNotFound(err) {
		if previousUID != "" {
			return fmt.Errorf("previously staged %s is missing", kind)
		}
		meta := metav1.ObjectMeta{Labels: desired.GetLabels(), Annotations: desired.GetAnnotations()}
		setHelmOwnership(&meta, b.store.release, b.store.namespace)
		meta.Annotations[initialCertificateIntent] = string(b.journal.UID)
		desired.SetLabels(meta.Labels)
		desired.SetAnnotations(meta.Annotations)
		if err := b.checkMaterial(ctx); err != nil {
			return err
		}
		obj, err = client.Create(ctx, desired, metav1.CreateOptions{FieldManager: FieldManager})
	}
	if err != nil {
		return err
	}
	if !b.ownsInitialAdmission(obj) || !reflect.DeepEqual(desired.Object["spec"], obj.Object["spec"]) ||
		(previousUID != "" && previousUID != string(obj.GetUID())) {
		return fmt.Errorf("preserving foreign or mismatched %s %s", kind, initialPolicyName)
	}
	pin, err := bootstrapResourcePin(obj)
	if err != nil {
		return err
	}
	if b.admission == nil {
		b.admission = map[string]aggregationBootstrapPin{}
	}
	b.admission[kind] = pin
	if previousUID != "" {
		return nil
	}
	return b.save(ctx)
}

func (b *initialCertificateInstaller) checkInitialAdmissionPins(ctx context.Context) error {
	for kind, expected := range b.admission {
		obj, err := b.store.dynamic.Resource(initialAdmissionResources[kind]).Get(ctx, initialPolicyName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		pin, err := bootstrapResourcePin(obj)
		if err != nil || pin.UID != expected.UID || pin.SpecSHA256 != expected.SpecSHA256 || !b.ownsInitialAdmission(obj) {
			return fmt.Errorf("%s %s changed during bootstrap", kind, initialPolicyName)
		}
	}
	return nil
}

func initialDenialPrefix(namespace string) string {
	return fmt.Sprintf("ValidatingAdmissionPolicy '%s' with binding '%s' denied request: CertificateRequestDenied: reserved-issuer: only the cert-manager controller may request certificates from Issuer %s in namespace %s, not ", initialPolicyName, initialPolicyName, RadiusBootstrapIssuerName, namespace)
}

func initialAdmissionDenied(err error, namespace string) bool {
	var status *apierrors.StatusError
	if !errors.As(err, &status) || status.ErrStatus.Code != http.StatusForbidden || status.ErrStatus.Reason != metav1.StatusReasonForbidden {
		return false
	}
	d := status.ErrStatus.Details
	if d == nil || d.Group != "cert-manager.io" || d.Kind != "certificaterequests" || d.Name != initialProbeName || len(d.Causes) != 1 {
		return false
	}
	cause := d.Causes[0].Message
	user, ok := strings.CutPrefix(cause, initialDenialPrefix(namespace))
	return ok && user != "" && status.ErrStatus.Message == fmt.Sprintf("certificaterequests.cert-manager.io %q is forbidden: %s", initialProbeName, cause)
}

func (b *initialCertificateInstaller) probeInitialAdmission(ctx context.Context) error {
	request := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1", "kind": "CertificateRequest",
		"metadata": map[string]any{"name": initialProbeName, "namespace": b.store.namespace},
		"spec": map[string]any{"request": base64.StdEncoding.EncodeToString([]byte(initialProbeCSR)),
			"issuerRef": map[string]any{"name": RadiusBootstrapIssuerName, "kind": "Issuer", "group": "cert-manager.io"}},
	}}
	client := b.store.dynamic.Resource(rotationIssuerResource.GroupVersion().WithResource("certificaterequests")).Namespace(b.store.namespace)
	// Informers can lag a successful read. This witnesses the responding API
	// server's decision in this operation, not convergence across HA replicas.
	admitted := false
	err := wait.PollUntilContextCancel(ctx, 200*time.Millisecond, true, func(ctx context.Context) (bool, error) {
		if err := b.checkMaterial(ctx); err != nil {
			return false, err
		}
		_, err := client.Create(ctx, request, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}, FieldValidation: metav1.FieldValidationStrict})
		if initialAdmissionDenied(err, b.store.namespace) {
			return true, b.checkMaterial(ctx)
		}
		if err == nil {
			admitted = true
		}
		return false, err
	})
	if admitted && errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("the server admitted the dry run without the required denial; verify the installer is not an allowed cert-manager requester: %w", err)
	}
	return err
}
