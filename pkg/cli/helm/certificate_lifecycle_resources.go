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
	"strings"

	"helm.sh/helm/v4/pkg/chart/common"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/engine"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func lifecycleRootName(state certificateLifecycle, plane string) string {
	return "radius-" + plane + "-ca-" + strings.ReplaceAll(state.ID, "-", "")[:16]
}

// planCertificateResources renders the actual selected chart's root and issuer
// helpers. Names and specs are intent; UID/RV fields are populated only by reads.
func (s *rotationStore) planCertificateResources(ctx context.Context, selected *chart.Chart, target *certificateTarget,
	state certificateLifecycle) ([]certificateResourceIntent, error) {
	if state.Execution == nil || state.Phase != certificatePrepare || state.Mode != target.Mode || state.Provider != target.Provider {
		return nil, errors.New("resource planning requires a matching prepared certificate target")
	}
	if err := state.validate(s.now()); err != nil {
		return nil, err
	}
	hash, err := bootstrapDigest(selected)
	if err != nil || hash != state.Execution.ChartSHA256 {
		return nil, errors.New("resource planning chart differs from its pinned intent")
	}
	if target.Mode == "issuerRef" {
		return nil, nil
	}
	var result []certificateResourceIntent
	seedPlanned := false
	for _, name := range []string{"internal", "aggregation"} {
		plane, found := state.Planes[name]
		if !found || !plane.Affected {
			continue
		}
		if plane.Issuer.Group != "cert-manager.io" || plane.Issuer.Kind != "Issuer" {
			return nil, errors.New("local target requires a namespaced cert-manager issuer")
		}
		signingSecret := target.SigningSecret
		if target.Mode == "selfSigned" {
			signingSecret = lifecycleRootName(state, name)
			if !seedPlanned {
				seed, err := s.selectedCertificateResource(ctx, selected, target.Values, name, `{{ include "radius.authz.certificates.bootstrapIssuer" . }}`)
				if err != nil {
					return nil, err
				}
				result, seedPlanned = append(result, seed), true
			}
			root, err := s.selectedCertificateResource(ctx, selected, target.Values, name,
				fmt.Sprintf(`{{ include "radius.authz.certificates.root" (dict "root" . "name" %q) }}`, signingSecret))
			if err != nil {
				return nil, err
			}
			result = append(result, root)
		}
		if err := validateCASecretName(signingSecret); err != nil {
			return nil, err
		}
		issuer, err := s.selectedCertificateResource(ctx, selected, target.Values, name,
			fmt.Sprintf(`{{ include "radius.authz.certificates.signingIssuer" (dict "root" . "name" %q "secretName" %q) }}`,
				plane.Issuer.Name, signingSecret))
		if err != nil {
			return nil, err
		}
		if issuer.Before != nil && (plane.Previous.Issuer.Reference != plane.Issuer ||
			issuer.Before.UID != plane.Previous.Issuer.UID || issuer.Before.SpecSHA256 != plane.Previous.Issuer.SpecSHA256 ||
			issuer.Before.ResourceVersion != plane.Previous.Issuer.ResourceVersion) {
			return nil, errors.New("selected issuer is not the previous owned configuration")
		}
		result = append(result, issuer)
	}
	return result, ctx.Err()
}

func (s *rotationStore) selectedCertificateResource(ctx context.Context, selected *chart.Chart, values map[string]any, plane, template string) (certificateResourceIntent, error) {
	var intent certificateResourceIntent
	if err := ctx.Err(); err != nil {
		return intent, err
	}
	c := certificateHelperChart(selected)
	const filename = "templates/lifecycle-resource.yaml"
	c.Templates = append(c.Templates, &common.File{Name: filename, Data: []byte(template)})
	effective, err := valuesutil.CoalesceValues(c, values)
	if err != nil {
		return intent, err
	}
	rendered, err := engine.Render(c, common.Values{"Values": effective, "Capabilities": common.DefaultCapabilities,
		"Release": map[string]any{"Name": s.release, "Namespace": s.namespace, "Service": "Helm"}})
	if err != nil {
		return intent, err
	}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(rendered[c.Name()+"/"+filename]), 4096)
	object := &unstructured.Unstructured{}
	if err := decoder.Decode(object); err != nil {
		return intent, err
	}
	if err := decoder.Decode(&unstructured.Unstructured{}); err != io.EOF {
		return intent, errors.New("selected certificate helper must render exactly one resource")
	}
	if object.GetAPIVersion() != "cert-manager.io/v1" || object.GetNamespace() != s.namespace {
		return intent, errors.New("selected certificate helper changed its API or namespace")
	}
	spec, found, err := unstructured.NestedMap(object.Object, "spec")
	if err != nil || !found {
		return intent, errors.New("selected certificate helper has no specification")
	}
	intent = certificateResourceIntent{Kind: object.GetKind(), Name: object.GetName(), Plane: plane, Spec: spec}
	if err := intent.validateSpec(); err != nil {
		return intent, err
	}
	existing, err := s.dynamic.Resource(certificateIntentResource(intent.Kind)).Namespace(s.namespace).Get(ctx, intent.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return intent, ctx.Err()
	}
	if err != nil {
		return intent, err
	}
	if intent.Kind == "Certificate" || !rotationOwned(existing, s.namespace, s.release) || len(existing.GetOwnerReferences()) != 0 {
		return intent, errors.New("selected certificate resource already exists outside its creation intent")
	}
	pin, err := bootstrapResourcePin(existing)
	if err != nil {
		return intent, err
	}
	if intent.Spec["selfSigned"] != nil {
		digest, err := bootstrapDigest(intent.Spec)
		if err != nil || digest != pin.SpecSHA256 {
			return intent, errors.New("existing bootstrap issuer differs from the selected selfSigned configuration")
		}
	}
	intent.Before = &pin
	return intent, ctx.Err()
}
