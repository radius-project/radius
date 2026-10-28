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
	"os"
	"testing"
	"time"

	fluxmeta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
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
}

func TestFluxSourcePolicySelection(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	require.NoError(t, sourcev1.AddToScheme(scheme))
	source := readyFluxSource()
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).Build()
	selection := hostoptions.FluxSourceSelection{Namespace: source.Namespace, Name: source.Name, UID: string(source.UID)}
	for _, tt := range []struct {
		name    string
		options hostoptions.FluxOptions
		key     client.ObjectKey
		reason  string
	}{
		{"disabled", hostoptions.FluxOptions{}, client.ObjectKeyFromObject(source), "SourceNotSelected"},
		{"empty", hostoptions.FluxOptions{Enabled: true}, client.ObjectKeyFromObject(source), "SourceNotSelected"},
		{"selected", hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{selection}}, client.ObjectKeyFromObject(source), ""},
		{"different namespace", hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{selection}}, client.ObjectKey{Namespace: "other", Name: source.Name}, "SourceNotSelected"},
		{"recreated", hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{{Namespace: source.Namespace, Name: source.Name, UID: "previous-uid"}}}, client.ObjectKeyFromObject(source), "SourceIdentityChanged"},
		{"deleted", hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{{Namespace: source.Namespace, Name: "missing", UID: "missing-uid"}}}, client.ObjectKey{Namespace: source.Namespace, Name: "missing"}, "SourceIdentityChanged"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			policy, err := NewFluxSourcePolicy(tt.options, c)
			require.NoError(t, err)
			got, hold, err := policy.source(t.Context(), tt.key)
			require.NoError(t, err)
			if tt.reason != "" {
				require.Nil(t, got)
				require.NotNil(t, hold)
				require.Equal(t, tt.reason, hold.reason)
			} else {
				require.Nil(t, hold)
				require.Equal(t, source.UID, got.UID)
			}
		})
	}
}

func TestFluxSourcePolicyCopiesSelection(t *testing.T) {
	t.Parallel()
	options := hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{
		{Namespace: "flux-system", Name: "apps", UID: "source-uid"},
	}}
	policy, err := NewFluxSourcePolicy(options, nil)
	require.NoError(t, err)
	options.Sources[0].UID = "changed"
	require.EqualValues(t, "source-uid", policy.sources[client.ObjectKey{Namespace: "flux-system", Name: "apps"}])
	_, _, err = policy.source(t.Context(), client.ObjectKey{Namespace: "flux-system", Name: "apps"})
	require.ErrorContains(t, err, "requires a Kubernetes reader")
}

func readyFluxSource() *sourcev1.GitRepository {
	return &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Namespace: "flux-system", Name: "apps", UID: "source-uid", Generation: 1},
		Status: sourcev1.GitRepositoryStatus{
			ObservedGeneration: 1,
			Artifact:           &fluxmeta.Artifact{Revision: "main@sha1:example", Digest: "sha256:example"},
			Conditions: []metav1.Condition{{
				Type: fluxmeta.ReadyCondition, Status: metav1.ConditionTrue, ObservedGeneration: 1,
				LastTransitionTime: metav1.Now(), Reason: "Succeeded", Message: "Artifact ready",
			}},
		},
	}

}

func newFluxPolicyTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, sourcev1.AddToScheme(s))
	require.NoError(t, radappiov1alpha3.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, apiextensionsv1.AddToScheme(s))
	return s
}

func TestFluxSourcePolicyReadiness(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		change func(*sourcev1.GitRepository)
	}{
		{"suspended", func(s *sourcev1.GitRepository) { s.Spec.Suspend = true }},
		{"stale generation", func(s *sourcev1.GitRepository) { s.Generation++ }},
		{"stale condition", func(s *sourcev1.GitRepository) { s.Status.Conditions[0].ObservedGeneration = 0 }},
		{"not ready", func(s *sourcev1.GitRepository) { s.Status.Conditions[0].Status = metav1.ConditionFalse }},
		{"no conditions", func(s *sourcev1.GitRepository) { s.Status.Conditions = nil }},
		{"no artifact", func(s *sourcev1.GitRepository) { s.Status.Artifact = nil }},
		{"deleting", func(s *sourcev1.GitRepository) {
			s.DeletionTimestamp = new(metav1.Now())
			s.Finalizers = []string{"test"}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			source := readyFluxSource()
			tt.change(source)
			s := runtime.NewScheme()
			require.NoError(t, sourcev1.AddToScheme(s))
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(source).Build()
			policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{{
				Namespace: source.Namespace, Name: source.Name, UID: string(source.UID),
			}}}, c)
			require.NoError(t, err)
			got, hold, err := policy.source(t.Context(), client.ObjectKeyFromObject(source))
			require.NoError(t, err)
			require.Nil(t, got)
			require.NotNil(t, hold)
			require.Equal(t, "SourceNotReady", hold.reason)
		})
	}
}

func TestFluxSourcePolicyTemplateSchema(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../deploy/Chart/crds/radius/radapp.io_deploymenttemplates.yaml")
	require.NoError(t, err)
	var current apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(data, &current))
	for _, old := range []bool{false, true} {
		name := "current"
		if old {
			name = "previous schema"
		}
		t.Run(name, func(t *testing.T) {
			crd := current.DeepCopy()
			if old {
				delete(crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties, "sourceRef")
			}
			s := runtime.NewScheme()
			require.NoError(t, apiextensionsv1.AddToScheme(s))
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(crd).Build()
			policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, c)
			require.NoError(t, err)
			err = policy.templateSchema(t.Context())
			if old {
				require.ErrorContains(t, err, "apply the matching chart CRD")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
