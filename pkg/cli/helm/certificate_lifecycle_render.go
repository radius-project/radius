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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// The validated projection is render-only and never stored in Helm history.
// Direct Helm retains the original helper and rejects unfinished journals.
func (w *certificateExecutionWriter) renderSelectedPhase(ctx context.Context, impl *Impl, options ClusterOptions,
	selected *chart.Chart, target *certificateTarget, history []*releasev1.Release, kubeContext string) ([]*unstructured.Unstructured, error) {
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	if !slices.Contains([]rotationPhase{rotationAdopt, rotationIssue}, w.state.Phase) || target == nil {
		return nil, errors.New("workload rendering requires an active adopt or issue target")
	}
	for _, entry := range []struct {
		value any
		hash  string
	}{{selected, w.state.Execution.ChartSHA256}, {target.Values, w.state.Execution.TargetValuesSHA256},
		{options.RadiusReleaseValues, w.state.Execution.BaselineValuesSHA256}} {
		hash, err := bootstrapDigest(entry.value)
		if err != nil || hash != entry.hash {
			return nil, errors.New("phase render changed its pinned chart or release values")
		}
	}
	baseline, err := certificatePolicyBaseline(options, history)
	if err != nil {
		return nil, err
	}
	if baseline.Version != w.state.Execution.BaselineVersion {
		return nil, errors.New("phase render changed its installed release revision")
	}
	projection := map[string]any{}
	for name, plane := range w.state.Planes {
		issuer := plane.Issuer
		signingSecret := plane.TargetIssuer.SigningSecret
		if w.state.Phase == rotationAdopt {
			issuer, signingSecret = plane.Previous.Issuer.Reference, plane.Previous.Issuer.SigningSecret
		}
		ref := map[string]any{"group": issuer.Group, "kind": issuer.Kind, "name": issuer.Name}
		issuerJSON, err := json.Marshal(ref)
		if err != nil {
			return nil, err
		}
		source := map[string]any{"kind": plane.Source.Kind, "name": plane.Source.Name, "key": caCertificateKey,
			"bundle": string(plane.Source.Bundle), "uid": plane.Source.UID, "resourceVersion": plane.Source.ResourceVersion,
			"contentDigest": plane.Source.SHA256, "issuer": ref, "signingSecret": signingSecret,
			"issuerDigest": fmt.Sprintf("%x", sha256.Sum256(issuerJSON)),
			"digest":       fmt.Sprintf("%x", sha256.Sum256(append(append([]byte(string(plane.Source.Bundle)), '\n'), issuerJSON...))),
			"journalUID":   plane.Source.JournalUID, "revision": plane.Source.Revision}
		if name == "aggregation" && w.state.Execution.ServingSecret != "" {
			source["servingSecret"] = w.state.Execution.PreviousServingSecret
			if w.state.Phase == rotationIssue {
				source["servingSecret"] = w.state.Execution.ServingSecret
			}
		}
		projection[name] = source
	}
	data, err := json.Marshal(projection)
	if err != nil {
		return nil, err
	}
	c := cloneChart(selected)
	resourceNames := map[string]bool{}
	for _, intent := range w.state.Execution.Resources {
		resourceNames[intent.Kind+"/"+intent.Name] = true
	}
	names, err := json.Marshal(resourceNames)
	if err != nil {
		return nil, err
	}
	c.Templates = slices.Clone(c.Templates)
	found := false
	for index, file := range c.Templates {
		if file.Name == "templates/_rotation.tpl" {
			c.Templates[index] = &common.File{Name: file.Name, Data: []byte(fmt.Sprintf(
				`{{- define "radius.lifecycleSnapshot" -}}{{ %q }}{{- end -}}
{{- define "radius.lifecycleResourceNames" -}}{{ %q }}{{- end -}}
{{- define "radius.lifecycleResources" -}}{{- end -}}`, string(data), string(names)))}
			found = true
		}
	}
	if !found {
		return nil, errors.New("selected chart does not support validated certificate lifecycle projections")
	}
	configuration, err := impl.cluster().RenderConfiguration(kubeContext, w.store.namespace)
	if err != nil {
		return nil, err
	}
	configuration.Releases, err = releaseHistoryStorage(history)
	if err != nil {
		return nil, err
	}
	upgrade := newUpgradeClient(configuration, w.store.namespace, false, false, options.Radius.Timeout,
		usesServerSideApply(configuration, w.store.release))
	upgrade.DryRunStrategy = helm.DryRunServer
	rendered, err := upgrade.RunWithContext(ctx, w.store.release, c, copyValues(target.Values))
	if err != nil {
		return nil, fmt.Errorf("selected-chart staged policy/render failed: %w", err)
	}
	revision, ok := rendered.(*releasev1.Release)
	if !ok {
		return nil, errors.New("phase render returned an unsupported Helm release")
	}
	objects, err := w.phaseObjects(revision.Manifest, target)
	if err != nil {
		return nil, err
	}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	return objects, nil
}

func (w *certificateExecutionWriter) phaseObjects(manifest string, target *certificateTarget) ([]*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var objects []*unstructured.Unstructured
	for {
		object := &unstructured.Unstructured{}
		if err := decoder.Decode(object); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		include := false
		switch object.GetKind() {
		case "Certificate":
			include = w.state.Phase == rotationIssue && w.state.Planes["aggregation"].Affected && object.GetName() == "ucp-cert"
		case "Deployment":
			include = w.state.Planes["internal"].Affected &&
				(slices.Contains(target.Receivers, object.GetName()) || object.GetName() == "controller") ||
				w.state.Planes["aggregation"].Affected && object.GetName() == "ucp"
			if include && w.state.Phase == rotationIssue {
				if err := unstructured.SetNestedField(object.Object, w.state.ID+"/issue", "spec", "template", "metadata", "annotations", certificateExecutionOwner); err != nil {
					return nil, err
				}
			}
		}
		if include {
			if object.GetNamespace() != w.store.namespace {
				return nil, errors.New("selected workload is outside the pinned namespace")
			}
			objects = append(objects, object)
		}
	}
	if len(objects) == 0 {
		return nil, errors.New("selected chart contains no affected certificate workloads")
	}
	slices.SortFunc(objects, func(a, b *unstructured.Unstructured) int {
		return strings.Compare(a.GetKind()+"/"+a.GetName(), b.GetKind()+"/"+b.GetName())
	})
	return objects, nil
}

// Obtain the admitted target specification before persisting its hash. Dry-run
// admission cannot stand in for issuance, rollout readiness or loaded trust.
func (w *certificateExecutionWriter) admitSelectedMutation(ctx context.Context, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	intent := certificateMutationIntent{Kind: desired.GetKind(), Name: desired.GetName(), Phase: w.state.Phase}
	resource, err := intent.resource()
	if err != nil {
		return nil, err
	}
	client := w.store.dynamic.Resource(resource).Namespace(w.store.namespace)
	current, err := client.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if !rotationOwned(current, w.store.namespace, w.store.release) || len(current.GetOwnerReferences()) != 0 {
		return nil, errors.New("selected workload is not Radius-owned")
	}
	spec, found, err := unstructured.NestedMap(desired.Object, "spec")
	if err != nil || !found {
		return nil, errors.New("selected workload has no valid specification")
	}
	if desired.GetKind() == "Deployment" {
		if replicas, found, err := unstructured.NestedInt64(current.Object, "spec", "replicas"); err != nil {
			return nil, err
		} else if found {
			spec["replicas"] = replicas
		}
	}
	current.Object["spec"] = spec
	annotations := maps.Clone(current.GetAnnotations())
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[certificateMutationOwner] = w.mutationMarker(intent)
	current.SetAnnotations(annotations)
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	admitted, err := client.Update(ctx, current, metav1.UpdateOptions{FieldManager: FieldManager, DryRun: []string{metav1.DryRunAll}})
	if err != nil {
		return nil, err
	}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	return admitted, nil
}
