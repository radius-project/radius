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

package reconciler

import (
	"context"
	"fmt"

	fluxmeta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FluxSourcePolicy is an immutable, startup-loaded selection of trusted sources.
// Its reader must bypass the manager cache when used by a running controller.
type FluxSourcePolicy struct {
	enabled bool
	sources map[client.ObjectKey]types.UID
	reader  client.Reader
}

// NewFluxSourcePolicy validates and copies source settings.
func NewFluxSourcePolicy(options hostoptions.FluxOptions, reader client.Reader) (*FluxSourcePolicy, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	policy := &FluxSourcePolicy{
		enabled: options.Enabled,
		sources: make(map[client.ObjectKey]types.UID, len(options.Sources)),
		reader:  reader,
	}
	for _, source := range options.Sources {
		policy.sources[client.ObjectKey{Namespace: source.Namespace, Name: source.Name}] = types.UID(source.UID)
	}
	return policy, nil
}

// Enabled reports whether any sources can be processed. A nil policy selects none.
func (p *FluxSourcePolicy) Enabled() bool {
	return p != nil && p.enabled && len(p.sources) != 0
}

type sourceHold struct {
	reason  string
	message string
}

func (h *sourceHold) Error() string {
	return h.reason + ": " + h.message
}

func sourceKey(ref *radappiov1alpha3.DeploymentTemplateSourceReference) string {
	if ref == nil || ref.Namespace == "" || ref.Name == "" || ref.UID == "" {
		return ""
	}
	return ref.Namespace + "/" + ref.Name + "/" + string(ref.UID)
}

func sameSource(a, b *radappiov1alpha3.DeploymentTemplateSourceReference) bool {
	return sourceKey(a) != "" && b != nil &&
		a.Namespace == b.Namespace && a.Name == b.Name && a.UID == b.UID
}

func checkTemplateOwner(template *radappiov1alpha3.DeploymentTemplate, ref *radappiov1alpha3.DeploymentTemplateSourceReference) error {
	if !sameSource(template.Spec.SourceRef, ref) || template.Spec.Repository != ref.Name {
		return &sourceHold{"TemplateSourceMismatch", "DeploymentTemplate requires an explicit association with this source before it can be updated or pruned"}
	}
	return nil
}

func checkTemplateWritable(template *radappiov1alpha3.DeploymentTemplate, ref *radappiov1alpha3.DeploymentTemplateSourceReference) error {
	if err := checkTemplateOwner(template, ref); err != nil {
		return err
	}
	if !template.DeletionTimestamp.IsZero() {
		return &sourceHold{"TemplateDeleting", "DeploymentTemplate deletion must finish before it can be updated"}
	}
	return nil
}

func (p *FluxSourcePolicy) checkArtifact(ctx context.Context, ref *radappiov1alpha3.DeploymentTemplateSourceReference) error {
	if sourceKey(ref) == "" {
		return &sourceHold{"TemplateSourceRequired", "A complete source reference is required"}
	}
	source, hold, err := p.source(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name})
	if err != nil {
		return err
	}
	if hold != nil {
		return hold
	}
	if source.UID != ref.UID || source.Generation != ref.Generation ||
		source.Status.Artifact.Revision != ref.Revision || source.Status.Artifact.Digest != ref.Digest {
		return &sourceHold{"SourceIdentityChanged", "Source or artifact changed during reconciliation; waiting for a fresh reconciliation"}
	}
	return nil
}

func (p *FluxSourcePolicy) checkTemplate(ctx context.Context, template *radappiov1alpha3.DeploymentTemplate) error {
	ref := template.Spec.SourceRef
	if ref == nil {
		if template.Spec.Repository != "" {
			return &sourceHold{"TemplateSourceRequired", "Legacy source-managed template requires a reviewed source association"}
		}
		// Direct manifests rely on the installation's Kubernetes write permissions.
		return nil
	}
	if sourceKey(ref) == "" || ref.Generation < 1 || template.Spec.Repository != ref.Name {
		return &sourceHold{"TemplateSourceRequired", "DeploymentTemplate source attribution is incomplete or inconsistent"}
	}
	source, hold, err := p.source(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name})
	if err != nil {
		return err
	}
	if hold != nil {
		return hold
	}
	if source.UID != ref.UID || source.Generation != ref.Generation {
		return &sourceHold{"SourceIdentityChanged", "DeploymentTemplate must be reconciled from the current source identity and generation"}
	}
	return nil
}

func (p *FluxSourcePolicy) source(ctx context.Context, key client.ObjectKey) (*sourcev1.GitRepository, *sourceHold, error) {
	if !p.Enabled() {
		return nil, &sourceHold{"SourceNotSelected", "Flux source processing is disabled or no sources are selected"}, nil
	}
	uid, selected := p.sources[key]
	if !selected {
		return nil, &sourceHold{"SourceNotSelected", "GitRepository is not selected in controller.flux.sources"}, nil
	}
	if p.reader == nil {
		return nil, nil, fmt.Errorf("flux source policy requires a Kubernetes reader")
	}
	source := &sourcev1.GitRepository{}
	if err := p.reader.Get(ctx, key, source); apierrors.IsNotFound(err) {
		return nil, &sourceHold{"SourceIdentityChanged", "Selected GitRepository no longer exists"}, nil
	} else if err != nil {
		return nil, nil, fmt.Errorf("reading selected GitRepository: %w", err)
	}
	if source.UID != uid {
		return nil, &sourceHold{"SourceIdentityChanged", "GitRepository UID does not match the selected identity"}, nil
	}
	ready := apimeta.FindStatusCondition(source.Status.Conditions, fluxmeta.ReadyCondition)
	if !source.DeletionTimestamp.IsZero() || source.Spec.Suspend || source.Generation < 1 ||
		source.Status.ObservedGeneration != source.Generation || ready == nil ||
		ready.Status != metav1.ConditionTrue || ready.ObservedGeneration != source.Generation ||
		source.Status.Artifact == nil {
		return nil, &sourceHold{"SourceNotReady", "GitRepository must be active and Ready for its current generation with an artifact"}, nil
	}
	return source, nil, nil
}

func (p *FluxSourcePolicy) templateSchema(ctx context.Context) error {
	if p == nil || p.reader == nil {
		return fmt.Errorf("flux source policy requires a Kubernetes reader")
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := p.reader.Get(ctx, client.ObjectKey{Name: "deploymenttemplates.radapp.io"}, &crd); err != nil {
		return fmt.Errorf("reading DeploymentTemplate schema: %w", err)
	}
	for _, version := range crd.Spec.Versions {
		if version.Name != radappiov1alpha3.GroupVersion.Version || !version.Served ||
			version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
			continue
		}
		ref := version.Schema.OpenAPIV3Schema.Properties["spec"].Properties["sourceRef"]
		if ref.Type != "object" {
			break
		}
		for _, name := range []string{"namespace", "name", "uid", "revision", "digest"} {
			if ref.Properties[name].Type != "string" {
				return fmt.Errorf("deployment template schema is missing sourceRef.%s; apply the matching chart CRD before enabling Flux", name)
			}
		}
		if ref.Properties["generation"].Type == "integer" {
			return nil
		}
	}
	return fmt.Errorf("deployment template schema does not support sourceRef; apply the matching chart CRD before enabling Flux")
}

func sourceReference(source *sourcev1.GitRepository) *radappiov1alpha3.DeploymentTemplateSourceReference {
	return &radappiov1alpha3.DeploymentTemplateSourceReference{
		Namespace: source.Namespace, Name: source.Name, UID: source.UID, Generation: source.Generation,
		Revision: source.Status.Artifact.Revision, Digest: source.Status.Artifact.Digest,
	}
}

func reportSourceHold(ctx context.Context, object client.Object, recorder record.EventRecorder, hold *sourceHold) ctrl.Result {
	ucplog.FromContextOrDiscard(ctx).Info("Reconciliation held", "reason", hold.reason,
		"namespace", object.GetNamespace(), "name", object.GetName())
	if recorder != nil {
		recorder.Event(object, corev1.EventTypeWarning, hold.reason, hold.message)
	}
	return ctrl.Result{RequeueAfter: DeleteRetryDelay}
}
