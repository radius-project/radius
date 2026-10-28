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

package deployment

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const controlPlaneSecret = "database-secret"

func deploymentWith(namespace string, mutate func(*corev1.PodSpec)) *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: namespace},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "web", Image: "nginx"}},
		}}},
	}
	if mutate != nil {
		mutate(&d.Spec.Template.Spec)
	}
	return d
}

func Test_admitOutputResources(t *testing.T) {
	id := resources.MustParse("/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/web")
	secretVolume := func(s *corev1.PodSpec) {
		s.Volumes = []corev1.Volume{{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: controlPlaneSecret}}}}
	}
	projected := func(s *corev1.PodSpec) {
		s.Volumes = []corev1.Volume{{Name: "v", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{
			Sources: []corev1.VolumeProjection{{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: controlPlaneSecret}}}},
		}}}}
	}
	envFrom := func(s *corev1.PodSpec) {
		s.Containers[0].EnvFrom = []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: controlPlaneSecret}}}}
	}
	initSecretKeyRef := func(s *corev1.PodSpec) {
		s.InitContainers = []corev1.Container{{Name: "init", Env: []corev1.EnvVar{{Name: "E", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: controlPlaneSecret}, Key: "k"},
		}}}}}
	}
	imagePull := func(s *corev1.PodSpec) {
		s.ImagePullSecrets = []corev1.LocalObjectReference{{Name: controlPlaneSecret}}
	}
	appSecret := func(s *corev1.PodSpec) {
		s.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "app-secret"}}
	}

	outputs := map[string]rpv1.OutputResource{
		"secret volume":     kubeOutputFor(deploymentWith("radius-system", secretVolume)),
		"projected secret":  kubeOutputFor(deploymentWith("radius-system", projected)),
		"envFrom secretRef": kubeOutputFor(deploymentWith("radius-system", envFrom)),
		"init secretKeyRef": kubeOutputFor(deploymentWith("radius-system", initSecretKeyRef)),
		"imagePullSecrets":  kubeOutputFor(deploymentWith("radius-system", imagePull)),
		"pod secret volume": kubeOutputFor(&corev1.Pod{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "radius-system"},
			Spec:       deploymentWith("", secretVolume).Spec.Template.Spec,
		}),
		"control-plane Secret object": kubeOutputFor(&corev1.Secret{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{Name: controlPlaneSecret, Namespace: "radius-system"},
		}),
		"app secret in control-plane namespace": kubeOutputFor(deploymentWith("radius-system", appSecret)),
		"plain deployment in control-plane ns":  kubeOutputFor(deploymentWith("radius-system", nil)),
		"secret volume in app namespace":        kubeOutputFor(deploymentWith("app", secretVolume)),
		"non-kubernetes provider": {
			LocalID: "azure",
			CreateResource: &rpv1.Resource{
				ResourceType: resourcemodel.ResourceType{Type: "Microsoft.Storage/storageAccounts", Provider: resourcemodel.ProviderAzure},
				Data:         deploymentWith("radius-system", secretVolume),
			},
		},
		"no create resource": {LocalID: "existing"},
	}

	tests := []struct {
		output string
		rule   string
	}{
		{output: "secret volume", rule: authz.RuleControlPlaneSecret},
		{output: "projected secret", rule: authz.RuleControlPlaneSecret},
		{output: "envFrom secretRef", rule: authz.RuleControlPlaneSecret},
		{output: "init secretKeyRef", rule: authz.RuleControlPlaneSecret},
		{output: "imagePullSecrets", rule: authz.RuleControlPlaneSecret},
		{output: "pod secret volume", rule: authz.RuleControlPlaneSecret},
		{output: "control-plane Secret object", rule: authz.RuleControlPlaneSecret},
		{output: "app secret in control-plane namespace", rule: authz.RuleControlPlaneNamespace},
		{output: "plain deployment in control-plane ns", rule: authz.RuleControlPlaneNamespace},
		{output: "secret volume in app namespace"},
		{output: "non-kubernetes provider"},
		{output: "no create resource"},
	}

	modes := []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce}
	for _, tt := range tests {
		for _, mode := range modes {
			t.Run(tt.output+"/"+string(mode), func(t *testing.T) {
				opts := authz.Options{Mode: mode, ControlPlaneNamespace: "radius-system", ControlPlaneSecrets: []string{controlPlaneSecret, "ucp-cert"}}
				core, logs := observer.New(zap.DebugLevel)
				ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))

				err := admitOutputResources(ctx, opts, id, []rpv1.OutputResource{outputs[tt.output]})

				denies := tt.rule != "" && mode != authz.ModeOff
				if denies && mode == authz.ModeEnforce {
					denied, ok := errors.AsType[*authz.DeniedError](err)
					require.True(t, ok, "expected a denial, got %v", err)
					require.Equal(t, v1.CodeAdmissionPolicyDenied, denied.Code)
					require.Equal(t, tt.rule, denied.Rule)
					require.Equal(t, id.String(), denied.Target)
					return
				}

				require.NoError(t, err)
				wouldDeny := logs.FilterField(zap.Bool(authz.LogFieldWouldDeny, true))
				if denies {
					require.Equal(t, 1, wouldDeny.FilterField(zap.String(authz.LogFieldRule, tt.rule)).Len())
				} else {
					require.Zero(t, wouldDeny.Len())
				}
			})
		}
	}
}

func Test_admitOutputResources_ReportsMostSpecificRule(t *testing.T) {
	id := resources.MustParse("/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/web")
	outputs := []rpv1.OutputResource{
		kubeOutputFor(deploymentWith("app", nil)),
		kubeOutputFor(deploymentWith("radius-system", nil)),
		kubeOutputFor(&corev1.Secret{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{Name: controlPlaneSecret, Namespace: "radius-system"},
		}),
	}

	t.Run("enforce", func(t *testing.T) {
		opts := authz.Options{Mode: authz.ModeEnforce, ControlPlaneSecrets: []string{controlPlaneSecret}}
		err := admitOutputResources(t.Context(), opts, id, outputs)
		denied, ok := errors.AsType[*authz.DeniedError](err)
		require.True(t, ok, "expected a denial, got %v", err)
		require.Equal(t, authz.RuleControlPlaneSecret, denied.Rule)
	})

	t.Run("dryRun logs one decision", func(t *testing.T) {
		opts := authz.Options{Mode: authz.ModeDryRun, ControlPlaneSecrets: []string{controlPlaneSecret}}
		core, logs := observer.New(zap.DebugLevel)
		ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))
		require.NoError(t, admitOutputResources(ctx, opts, id, outputs))
		wouldDeny := logs.FilterField(zap.Bool(authz.LogFieldWouldDeny, true))
		require.Equal(t, 1, wouldDeny.Len())
		require.Equal(t, 1, wouldDeny.FilterField(zap.String(authz.LogFieldRule, authz.RuleControlPlaneSecret)).Len())
	})
}

func kubeOutputFor(obj runtime.Object) rpv1.OutputResource {
	meta := obj.(metav1.Object)
	return rpv1.NewKubernetesOutputResource("local", obj, metav1.ObjectMeta{Name: meta.GetName(), Namespace: meta.GetNamespace()})
}
