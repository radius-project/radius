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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func injectorFixture(name string, args ...string) (*appsv1.Deployment, *appsv1.ReplicaSet, *corev1.Pod) {
	controller := true
	replicas := int32(1)
	spec := corev1.PodSpec{ServiceAccountName: name, Containers: []corev1.Container{{
		Name: "cainjector", Image: "quay.io/jetstack/cert-manager-cainjector:v1.21.2", Args: args,
	}}}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "security", UID: types.UID(name), Generation: 1},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: spec}},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1},
	}
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: name + "-rs", Namespace: "security", UID: types.UID(name + "-rs"),
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: name, UID: d.UID, Controller: &controller}},
	}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-pod", Namespace: "security", UID: types.UID(name + "-pod"),
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: rs.Name, UID: rs.UID, Controller: &controller}}},
		Spec:   *spec.DeepCopy(),
		Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	return d, rs, pod
}

func TestInspectCAInjectorPrerequisites(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		change func(*appsv1.Deployment, *corev1.Pod)
		extra  bool
		err    string
	}{
		{name: "default merging enabled"},
		{name: "split namespace flag", args: []string{"--namespace", "radius-system"}},
		{name: "explicit nonmerging", args: []string{"--feature-gates=CAInjectorMerging=false"}},
		{name: "ignores radius", args: []string{"--ignore-namespaces=other,radius-system"}, err: "0 eligible"},
		{name: "quoted CSV ignored namespace", args: []string{`--ignore-namespaces="radius-system"`}, err: "0 eligible"},
		{name: "different scope", args: []string{"--namespace=other"}, err: "0 eligible"},
		{name: "APIService controller disabled", args: []string{"--enable-apiservices-injectable=false"}, err: "0 eligible"},
		{name: "multiple eligible controllers", extra: true, err: "2 eligible"},
		{name: "unknown flag", args: []string{"--future-mode=true"}, err: "unknown cainjector flag"},
		{name: "malformed bool", args: []string{"--enable-apiservices-injectable=maybe"}, err: "invalid boolean"},
		{name: "duplicate flag", args: []string{"--namespace=other", "--namespace=radius-system"}, err: "duplicate"},
		{name: "unknown feature gate", args: []string{"--feature-gates=NewBehavior=true"}, err: "unknown or malformed"},
		{name: "duplicate feature gate", args: []string{"--feature-gates=CAInjectorMerging=false,CAInjectorMerging=true"}, err: "unknown or malformed"},
		{name: "unreadable image file", args: []string{"--config=/etc/unknown.yaml"}, err: "not an inspectable"},
		{name: "different cluster", args: []string{"--kubeconfig=/etc/kubeconfig"}, err: "target cluster unverifiable"},
		{name: "environment override", args: []string{"--namespace=$(WATCH_NAMESPACE)"}, err: "unverifiable environment"},
		{name: "stale deployment", change: func(d *appsv1.Deployment, p *corev1.Pod) { d.Generation++ }, err: "current Ready rollout"},
		{name: "stale ready pod", change: func(d *appsv1.Deployment, p *corev1.Pod) {
			p.Spec.Containers[0].Args = []string{"--enable-apiservices-injectable=false"}
		}, err: "pinned template"},
		{name: "orphan pod", change: func(d *appsv1.Deployment, p *corev1.Pod) { p.OwnerReferences = nil }, err: "Pod inventory"},
		{name: "unready pod", change: func(d *appsv1.Deployment, p *corev1.Pod) { p.Status.Conditions = nil }, err: "Running/Ready"},
		{name: "wrapper command", change: func(d *appsv1.Deployment, p *corev1.Pod) {
			d.Spec.Template.Spec.Containers[0].Command = []string{"sh", "-c", "cainjector"}
		}, err: "unsupported image, command"},
		{name: "renamed controller detected by image", change: func(d *appsv1.Deployment, p *corev1.Pod) { d.Name = "renamed" }},
		{name: "default injected API token volume", change: func(d *appsv1.Deployment, p *corev1.Pod) {
			p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{Name: "kube-api-access-test", VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{
					{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Path: "token"}},
					{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"}}},
					{DownwardAPI: &corev1.DownwardAPIProjection{}},
				}},
			}})
			p.Spec.Containers[0].VolumeMounts = append(p.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
				Name: "kube-api-access-test", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true,
			})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, rs, pod := injectorFixture("cm-cainjector", tt.args...)
			if tt.change != nil {
				tt.change(d, pod)
			}
			objects := []runtime.Object{d, rs, pod}
			if tt.extra {
				d2, rs2, p2 := injectorFixture("second-cainjector")
				objects = append(objects, d2, rs2, p2)
			}
			client := fake.NewClientset(objects...)
			instances, err := inspectCAInjectorPrerequisites(t.Context(), client, "radius-system")
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Len(t, instances, 1)
				require.True(t, instances[0].Eligible)
				require.Len(t, instances[0].ConfigurationSHA256, 64)
				require.Equal(t, tt.name != "explicit nonmerging", instances[0].MergeCAs)
			}
			for _, action := range client.Actions() {
				require.Contains(t, []string{"get", "list"}, action.GetVerb())
			}
		})
	}
}

func TestInspectCAInjectorConfig(t *testing.T) {
	for _, tt := range []struct {
		name, config string
		mutable      bool
		secret       bool
		override     bool
		err          string
	}{
		{name: "immutable ConfigMap", config: "namespace: radius-system"},
		{name: "immutable Secret", secret: true, config: "namespace: radius-system"},
		{name: "flags override config", config: "namespace: other\nenableInjectableConfig:\n  apiServices: false", override: true},
		{name: "disabled in config", config: "enableInjectableConfig:\n  apiServices: false", err: "0 eligible"},
		{name: "namespace from config", config: "namespace: other", err: "0 eligible"},
		{name: "ignored namespace in config", config: "ignoreNamespaces: [radius-system]", err: "0 eligible"},
		{name: "unknown field", config: "futureField: true", err: "unsupported --config"},
		{name: "duplicate YAML key", config: "namespace: other\nnamespace: radius-system", err: "unsupported --config"},
		{name: "multiple YAML documents", config: "namespace: radius-system\n---\nnamespace: other", err: "exactly one configuration document"},
		{name: "mutable config cannot prove startup", mutable: true, err: "must be immutable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, rs, pod := injectorFixture("cm-cainjector", "--config=/config/injector.yaml")
			immutable := !tt.mutable
			text := "apiVersion: cainjector.config.cert-manager.io/v1alpha1\nkind: CAInjectorConfiguration\n" + tt.config + "\n"
			volume := corev1.Volume{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "injector-config"}}}}
			var source runtime.Object = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "injector-config", Namespace: "security", UID: "config-uid", ResourceVersion: "1"}, Immutable: &immutable, Data: map[string]string{"injector.yaml": text}}
			if tt.secret {
				volume.VolumeSource = corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "injector-config"}}
				source = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "injector-config", Namespace: "security", UID: "config-uid", ResourceVersion: "1"}, Immutable: &immutable, Data: map[string][]byte{"injector.yaml": []byte(text)}}
			}
			d.Spec.Template.Spec.Volumes = []corev1.Volume{volume}
			d.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "config", MountPath: "/config", ReadOnly: true}}
			if tt.override {
				d.Spec.Template.Spec.Containers[0].Args = append(d.Spec.Template.Spec.Containers[0].Args, "--namespace=radius-system", "--enable-apiservices-injectable=true")
			}
			pod.Spec = *d.Spec.Template.Spec.DeepCopy()
			client := fake.NewClientset(d, rs, pod, source)
			instances, err := inspectCAInjectorPrerequisites(t.Context(), client, "radius-system")
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "config-uid", string(instances[0].Config.UID))
			require.Len(t, instances[0].Config.SHA256, 64)
		})
	}
	t.Run("forbidden cluster inventory", func(t *testing.T) {
		client := fake.NewClientset()
		client.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("forbidden inventory")
		})
		_, err := inspectCAInjectorPrerequisites(t.Context(), client, "radius-system")
		require.ErrorContains(t, err, "forbidden inventory")
	})
	t.Run("no controller", func(t *testing.T) {
		_, err := inspectCAInjectorPrerequisites(t.Context(), fake.NewClientset(), "radius-system")
		require.ErrorContains(t, err, "0 eligible")
	})
	t.Run("excluded controller does not conflict", func(t *testing.T) {
		d, rs, pod := injectorFixture("main-cainjector")
		other, otherRS, otherPod := injectorFixture("other-cainjector", "--namespace=other")
		instances, err := inspectCAInjectorPrerequisites(t.Context(), fake.NewClientset(d, rs, pod, other, otherRS, otherPod), "radius-system")
		require.NoError(t, err)
		require.Len(t, instances, 2)
		require.NotEmpty(t, instances[1].Exclusion)
	})
}
