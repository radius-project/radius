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

package container

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/radius-project/radius/pkg/corerp/handlers"
	"github.com/radius-project/radius/pkg/kubernetes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

const (
	secretHashTestNamespace = "app-ns"
	secretHashTestName      = "frontend"
)

func secretHashTestDeployment(podAnnotations map[string]string, annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: secretHashTestName, Namespace: secretHashTestNamespace, Annotations: annotations},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: podAnnotations}},
		},
	}
}

func secretHashTestSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: secretHashTestNamespace}, Data: data}
}

func Test_SecretHashTransformer(t *testing.T) {
	t.Parallel()

	data := map[string][]byte{"CONNECTION_REDIS_PASSWORD": []byte("password")}
	changed := map[string][]byte{"CONNECTION_REDIS_PASSWORD": []byte("rotated")}
	current, legacy := kubernetes.HashSecretData(data), kubernetes.LegacyHashSecretData(data)
	staleLegacy := kubernetes.LegacyHashSecretData(changed)

	for _, tt := range []struct {
		name         string
		live         []client.Object
		dependencies map[string]map[string]string
		expected     string
	}{
		{name: "new deployment", live: []client.Object{secretHashTestSecret(secretHashTestName, data)}, expected: current},
		{
			name:     "current hash",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: current}, nil), secretHashTestSecret(secretHashTestName, data)},
			expected: current,
		},
		{
			name:     "current hash of other data",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: kubernetes.HashSecretData(changed)}, nil), secretHashTestSecret(secretHashTestName, data)},
			expected: current,
		},
		{
			name:     "legacy hash of unchanged data",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, nil), secretHashTestSecret(secretHashTestName, data)},
			expected: legacy,
		},
		{
			name:     "legacy hash of changed data",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: staleLegacy}, nil), secretHashTestSecret(secretHashTestName, data)},
			expected: current,
		},
		{
			name: "legacy hash and secret with data Radius does not manage",
			live: []client.Object{
				secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: kubernetes.LegacyHashSecretData(map[string][]byte{"CONNECTION_REDIS_PASSWORD": []byte("password"), "EXTRA": []byte("x")})}, nil),
				secretHashTestSecret(secretHashTestName, map[string][]byte{"CONNECTION_REDIS_PASSWORD": []byte("password"), "EXTRA": []byte("x")}),
			},
			expected: current,
		},
		{
			name:     "legacy hash without secret",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, nil)},
			expected: current,
		},
		{
			name:     "legacy hash with recorded current hash",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, map[string]string{kubernetes.AnnotationSecretHashSHA256: kubernetes.RecordSecretHash(legacy, current)})},
			expected: legacy,
		},
		{
			// The recorded hash is authoritative, so a later release can drop the legacy hash function.
			name:     "legacy hash with recorded hash of other data",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, map[string]string{kubernetes.AnnotationSecretHashSHA256: kubernetes.RecordSecretHash(legacy, kubernetes.HashSecretData(changed))}), secretHashTestSecret(secretHashTestName, data)},
			expected: current,
		},
		{
			// A rollback deployed the changed data under its legacy hash and kept the record of the original data. When
			// the original data is deployed again, the pods must restart to use it.
			name:     "record of another legacy hash after a rollback",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: staleLegacy}, map[string]string{kubernetes.AnnotationSecretHashSHA256: kubernetes.RecordSecretHash(legacy, current)}), secretHashTestSecret(secretHashTestName, data)},
			expected: current,
		},
		{
			name:     "record of another legacy hash of unchanged data",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, map[string]string{kubernetes.AnnotationSecretHashSHA256: kubernetes.RecordSecretHash(staleLegacy, kubernetes.HashSecretData(changed))}), secretHashTestSecret(secretHashTestName, data)},
			expected: legacy,
		},
		{
			name:     "record without its legacy hash",
			live:     []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, map[string]string{kubernetes.AnnotationSecretHashSHA256: kubernetes.HashSecretData(changed)}), secretHashTestSecret(secretHashTestName, data)},
			expected: legacy,
		},
		{
			name:         "secret named by its output resource",
			live:         []client.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, nil), secretHashTestSecret("frontend-secrets", data)},
			dependencies: map[string]map[string]string{rpv1.LocalIDSecret: {handlers.ResourceName: "frontend-secrets", handlers.KubernetesNamespaceKey: secretHashTestNamespace}},
			expected:     legacy,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reader := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(tt.live...).Build()
			desired := secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: current}, nil)
			options := &handlers.PutOptions{
				Resource:             &rpv1.OutputResource{LocalID: rpv1.LocalIDDeployment, CreateResource: &rpv1.Resource{Data: desired}},
				DependencyProperties: tt.dependencies,
			}

			require.NoError(t, NewSecretHashTransformer(reader)(t.Context(), options))
			require.Equal(t, tt.expected, desired.Spec.Template.Annotations[kubernetes.AnnotationSecretHash])
		})
	}
}

func Test_SecretHashTransformer_Skips(t *testing.T) {
	t.Parallel()

	failing := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return errors.New("the transformer must not read the cluster")
		},
	}).Build()

	for name, data := range map[string]any{
		"not a deployment":               secretHashTestSecret(secretHashTestName, nil),
		"deployment without hash":        secretHashTestDeployment(map[string]string{}, nil),
		"deployment without annotations": secretHashTestDeployment(nil, nil),
	} {
		options := &handlers.PutOptions{Resource: &rpv1.OutputResource{CreateResource: &rpv1.Resource{Data: data}}}
		require.NoError(t, NewSecretHashTransformer(failing)(t.Context(), options), name)
	}

	// Without a reader, as in tests that do not apply resources, the current hash is used.
	desired := secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: "sha256:value"}, nil)
	options := &handlers.PutOptions{Resource: &rpv1.OutputResource{CreateResource: &rpv1.Resource{Data: desired}}}
	require.NoError(t, NewSecretHashTransformer(nil)(t.Context(), options))
	require.Equal(t, "sha256:value", desired.Spec.Template.Annotations[kubernetes.AnnotationSecretHash])
}

func Test_SecretHashTransformer_ReadErrors(t *testing.T) {
	t.Parallel()

	data := map[string][]byte{"KEY": []byte("value")}
	legacy := kubernetes.LegacyHashSecretData(data)
	for _, kind := range []string{"Deployment", "Secret"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			reader := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).
				WithObjects(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, nil)).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*corev1.Secret); ok == (kind == "Secret") {
							return errors.New("unavailable")
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).Build()
			desired := secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: kubernetes.HashSecretData(data)}, nil)
			options := &handlers.PutOptions{Resource: &rpv1.OutputResource{CreateResource: &rpv1.Resource{Data: desired}}}
			err := NewSecretHashTransformer(reader)(t.Context(), options)
			require.ErrorContains(t, err, "failed to read "+kind)
		})
	}
}

func Test_SecretHashTransformer_Forbidden(t *testing.T) {
	t.Parallel()

	data := map[string][]byte{"KEY": []byte("value")}
	current := kubernetes.HashSecretData(data)
	for _, kind := range []string{"Deployment", "Secret"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			reader := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).
				WithObjects(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: kubernetes.LegacyHashSecretData(data)}, nil), secretHashTestSecret(secretHashTestName, data)).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*corev1.Secret); ok == (kind == "Secret") {
							return apierrors.NewForbidden(schema.GroupResource{Resource: strings.ToLower(kind) + "s"}, key.Name, errors.New("denied"))
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).Build()
			desired := secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: current}, nil)
			options := &handlers.PutOptions{Resource: &rpv1.OutputResource{CreateResource: &rpv1.Resource{Data: desired}}}

			// Without read access the legacy hash cannot be verified, so the deployment proceeds with the current hash.
			require.NoError(t, NewSecretHashTransformer(reader)(t.Context(), options))
			require.Equal(t, current, desired.Spec.Template.Annotations[kubernetes.AnnotationSecretHash])
		})
	}
}

func secretHashTestLabeled(deployment *appsv1.Deployment) *appsv1.Deployment {
	deployment.Labels = map[string]string{kubernetes.LabelRadiusResource: secretHashTestName}
	return deployment
}

func Test_RecordSecretHashes(t *testing.T) {
	t.Parallel()

	data := map[string][]byte{"CONNECTION_REDIS_PASSWORD": []byte("password")}
	changed := map[string][]byte{"CONNECTION_REDIS_PASSWORD": []byte("rotated")}
	current, legacy := kubernetes.HashSecretData(data), kubernetes.LegacyHashSecretData(data)
	record := kubernetes.RecordSecretHash(legacy, current)
	staleRecord := kubernetes.RecordSecretHash(kubernetes.LegacyHashSecretData(changed), kubernetes.HashSecretData(changed))

	for _, tt := range []struct {
		name     string
		objects  []runtime.Object
		expected hashmigration.Result
		recorded string
	}{
		{
			name:     "legacy hash of unchanged secret",
			objects:  []runtime.Object{secretHashTestLabeled(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, nil)), secretHashTestSecret(secretHashTestName, data)},
			expected: hashmigration.Result{Migrated: 1},
			recorded: record,
		},
		{
			// A rollback deployed other data and kept the record of the data deployed before it.
			name:     "record of another legacy hash",
			objects:  []runtime.Object{secretHashTestLabeled(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, map[string]string{kubernetes.AnnotationSecretHashSHA256: staleRecord})), secretHashTestSecret(secretHashTestName, data)},
			expected: hashmigration.Result{Migrated: 1},
			recorded: record,
		},
		{
			name:     "legacy hash of changed secret",
			objects:  []runtime.Object{secretHashTestLabeled(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, nil)), secretHashTestSecret(secretHashTestName, changed)},
			expected: hashmigration.Result{Skipped: 1},
		},
		{
			name:     "legacy hash without secret",
			objects:  []runtime.Object{secretHashTestLabeled(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, nil))},
			expected: hashmigration.Result{Skipped: 1},
		},
		{
			name:    "current hash",
			objects: []runtime.Object{secretHashTestLabeled(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: current}, nil)), secretHashTestSecret(secretHashTestName, data)},
		},
		{
			name:     "already recorded",
			objects:  []runtime.Object{secretHashTestLabeled(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, map[string]string{kubernetes.AnnotationSecretHashSHA256: record}))},
			recorded: record,
		},
		{
			name:    "deployment Radius does not manage",
			objects: []runtime.Object{secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: legacy}, nil), secretHashTestSecret(secretHashTestName, data)},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clientset := k8sfake.NewClientset(tt.objects...)
			result, err := RecordSecretHashes(t.Context(), clientset)
			require.NoError(t, err)
			result.Message = ""
			require.Equal(t, tt.expected, result)

			deployment, err := clientset.AppsV1().Deployments(secretHashTestNamespace).Get(t.Context(), secretHashTestName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tt.recorded, deployment.Annotations[kubernetes.AnnotationSecretHashSHA256])
			if tt.expected.Migrated > 0 {
				require.Equal(t, legacy, deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash], "recording must not change the Pod template")
			}
		})
	}
}

func Test_RecordSecretHashes_Retries(t *testing.T) {
	t.Parallel()

	data := map[string][]byte{"KEY": []byte("value")}
	objects := []runtime.Object{
		secretHashTestLabeled(secretHashTestDeployment(map[string]string{kubernetes.AnnotationSecretHash: kubernetes.LegacyHashSecretData(data)}, nil)),
		secretHashTestSecret(secretHashTestName, data),
	}

	for _, tt := range []struct {
		verb     string
		resource string
	}{
		{verb: "get", resource: "secrets"},
		{verb: "patch", resource: "deployments"},
	} {
		t.Run(tt.verb+" "+tt.resource, func(t *testing.T) {
			t.Parallel()
			clientset := k8sfake.NewClientset(objects...)
			clientset.PrependReactor(tt.verb, tt.resource, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: tt.resource}, secretHashTestName, errors.New("changed"))
			})

			result, err := RecordSecretHashes(t.Context(), clientset)
			require.NoError(t, err)
			require.Equal(t, 1, result.Remaining)
			require.Zero(t, result.Migrated)
		})
	}

	t.Run("list fails", func(t *testing.T) {
		t.Parallel()
		clientset := k8sfake.NewClientset()
		clientset.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("unavailable")
		})
		_, err := RecordSecretHashes(t.Context(), clientset)
		require.ErrorContains(t, err, "failed to list Deployments")
	})
}

func Test_NewSecretHashMigrationStep(t *testing.T) {
	t.Parallel()

	step := NewSecretHashMigrationStep(func(context.Context) (k8s.Interface, error) { return k8sfake.NewClientset(), nil })
	require.Equal(t, SecretHashMigrationStepName, step.Name())
	result, err := step.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{}, result)

	step = NewSecretHashMigrationStep(func(context.Context) (k8s.Interface, error) { return nil, errors.New("no cluster") })
	_, err = step.Run(t.Context())
	require.ErrorContains(t, err, "no cluster")
}
