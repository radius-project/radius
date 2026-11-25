//go:build !windows

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
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestFluxSourcePolicySchemaRoundTrip(t *testing.T) {
	SkipWithoutEnvironment(t)
	current := fluxTemplateCRD(t)
	previous := current.DeepCopy()
	delete(previous.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties, "sourceRef")
	environment := &envtest.Environment{CRDs: []*apiextensionsv1.CustomResourceDefinition{previous}}
	cfg, err := environment.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, environment.Stop()) })
	c, err := client.New(cfg, client.Options{Scheme: newFluxPolicyTestScheme(t)})
	require.NoError(t, err)
	policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, c)
	require.NoError(t, err)
	require.ErrorContains(t, policy.templateSchema(t.Context()), "apply the matching chart CRD")
	require.NoError(t, c.Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "schema-roundtrip"}}))
	ref := sourceReference(readyFluxSource())
	dt := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Namespace: "schema-roundtrip", Name: "legacy"},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", Repository: ref.Name, SourceRef: ref},
	}
	require.NoError(t, c.Create(t.Context(), dt))
	key := client.ObjectKeyFromObject(dt)
	require.NoError(t, c.Get(t.Context(), key, dt))
	require.Nil(t, dt.Spec.SourceRef, "previous schema prunes unknown attribution")
	require.ErrorContains(t, policy.checkTemplate(t.Context(), dt), "TemplateSourceRequired")
	uid := dt.UID
	installed := &apiextensionsv1.CustomResourceDefinition{}
	require.NoError(t, c.Get(t.Context(), client.ObjectKey{Name: current.Name}, installed))
	installed.Spec = current.Spec
	require.NoError(t, c.Update(t.Context(), installed))
	require.Eventually(t, func() bool {
		if err := c.Get(t.Context(), key, dt); err != nil {
			return false
		}
		dt.Spec.SourceRef = ref.DeepCopy()
		if err := c.Update(t.Context(), dt); err != nil {
			return false
		}
		if err := c.Get(t.Context(), key, dt); err != nil {
			return false
		}
		return dt.Spec.SourceRef != nil
	}, 10*time.Second, 100*time.Millisecond)
	require.Equal(t, uid, dt.UID)
	require.Equal(t, "{}", dt.Spec.Template)
	require.Equal(t, ref, dt.Spec.SourceRef)
	require.NoError(t, policy.templateSchema(t.Context()))

	defaulted := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Namespace: "schema-roundtrip", Name: "defaulted-kind"},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", Repository: ref.Name, SourceRef: ref.DeepCopy()},
	}
	defaulted.Spec.SourceRef.Kind = ""
	require.NoError(t, c.Create(t.Context(), defaulted))
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(defaulted), defaulted))
	require.Equal(t, sourcev1.GitRepositoryKind, defaulted.Spec.SourceRef.Kind)
	unsupported := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Namespace: "schema-roundtrip", Name: "unsupported-kind"},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", Repository: ref.Name, SourceRef: ref.DeepCopy()},
	}
	unsupported.Spec.SourceRef.Kind = "OCIRepository"
	require.Error(t, c.Create(t.Context(), unsupported))
}
