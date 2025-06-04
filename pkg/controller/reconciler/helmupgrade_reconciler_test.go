/*
Copyright 2023 The Radius Authors.

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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/radius-project/radius/pkg/upgrade/preflight"
	"github.com/radius-project/radius/pkg/cli/output"
)

func TestHelmUpgradeReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name           string
		secret         *corev1.Secret
		expectReconcile bool
		expectError    bool
	}{
		{
			name: "non-radius-system namespace should be ignored",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.radius.v1",
					Namespace: "default",
				},
				Type: "helm.sh/release.v1",
			},
			expectReconcile: false,
			expectError:     false,
		},
		{
			name: "non-helm secret should be ignored",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "radius-system",
				},
				Type: corev1.SecretTypeOpaque,
			},
			expectReconcile: false,
			expectError:     false,
		},
		{
			name: "non-radius helm release should be ignored",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.contour.v1",
					Namespace: "radius-system",
				},
				Type: "helm.sh/release.v1",
			},
			expectReconcile: false,
			expectError:     false,
		},
		{
			name: "radius helm release should be processed",
			secret: createValidRadiusHelmSecret("radius", "radius-system", 1, "0.45.0"),
			expectReconcile: true,
			expectError:     false,
		},
		{
			name: "already processed revision should be skipped",
			secret: createValidRadiusHelmSecretWithAnnotation("radius", "radius-system", 1, "0.45.0", "1"),
			expectReconcile: false,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with the secret
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.secret).
				Build()

			// Create mock preflight registry
			mockOutput := &output.MockOutput{}
			registry := preflight.NewRegistry(mockOutput)

			// Create reconciler
			reconciler := &HelmUpgradeReconciler{
				Client:            fakeClient,
				Scheme:            scheme,
				EventRecorder:     &record.FakeRecorder{},
				PreflightRegistry: registry,
				DelayInterval:     time.Second,
			}

			// Create reconcile request
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.secret.Name,
					Namespace: tt.secret.Namespace,
				},
			}

			// Reconcile
			result, err := reconciler.Reconcile(context.Background(), req)

			// Check results
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify result
			require.Equal(t, ctrl.Result{}, result)

			if tt.expectReconcile && !tt.expectError {
				// Verify that the annotation was added
				updatedSecret := &corev1.Secret{}
				err = fakeClient.Get(context.Background(), req.NamespacedName, updatedSecret)
				require.NoError(t, err)

				expectedRevision := "1"
				actualRevision, exists := updatedSecret.Annotations[RadiusUpgradeAnnotation]
				require.True(t, exists, "Expected annotation to be added")
				require.Equal(t, expectedRevision, actualRevision)
			}
		})
	}
}

func TestHelmUpgradeReconciler_isRadiusHelmRelease(t *testing.T) {
	reconciler := &HelmUpgradeReconciler{}

	tests := []struct {
		name     string
		secret   *corev1.Secret
		expected bool
	}{
		{
			name: "valid radius helm release",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sh.helm.release.v1.radius.v1",
				},
				Type: "helm.sh/release.v1",
			},
			expected: true,
		},
		{
			name: "valid radius helm release with different version",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sh.helm.release.v1.radius.v5",
				},
				Type: "helm.sh/release.v1",
			},
			expected: true,
		},
		{
			name: "non-helm secret type",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sh.helm.release.v1.radius.v1",
				},
				Type: corev1.SecretTypeOpaque,
			},
			expected: false,
		},
		{
			name: "helm secret but not radius",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sh.helm.release.v1.contour.v1",
				},
				Type: "helm.sh/release.v1",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconciler.isRadiusHelmRelease(tt.secret)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestHelmUpgradeReconciler_parseHelmRelease(t *testing.T) {
	reconciler := &HelmUpgradeReconciler{}

	t.Run("valid helm release data", func(t *testing.T) {
		secret := createValidRadiusHelmSecret("radius", "radius-system", 2, "0.46.0")
		
		release, err := reconciler.parseHelmRelease(secret)
		require.NoError(t, err)
		require.NotNil(t, release)
		require.Equal(t, "radius", release.Name)
		require.Equal(t, "radius-system", release.Namespace)
		require.Equal(t, 2, release.Version)
		require.Equal(t, "0.46.0", release.Chart.Metadata.Version)
	})

	t.Run("secret without release data", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.radius.v1",
				Namespace: "radius-system",
			},
			Type: "helm.sh/release.v1",
			Data: map[string][]byte{},
		}
		
		_, err := reconciler.parseHelmRelease(secret)
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not contain release data")
	})

	t.Run("invalid base64 data", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.radius.v1",
				Namespace: "radius-system",
			},
			Type: "helm.sh/release.v1",
			Data: map[string][]byte{
				"release": []byte("invalid-base64!!!"),
			},
		}
		
		_, err := reconciler.parseHelmRelease(secret)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to decode release data")
	})
}

// Helper function to create a valid Radius Helm secret for testing
func createValidRadiusHelmSecret(name, namespace string, version int, chartVersion string) *corev1.Secret {
	release := HelmRelease{
		Name:      name,
		Namespace: namespace,
		Version:   version,
		Info: HelmReleaseInfo{
			FirstDeployed: metav1.Now(),
			LastDeployed:  metav1.Now(),
			Status:        "deployed",
			Description:   "Upgrade complete",
		},
		Chart: HelmChart{
			Metadata: HelmChartMetadata{
				Name:    "radius",
				Version: chartVersion,
			},
		},
		Config: map[string]interface{}{},
	}

	releaseJSON, _ := json.Marshal(release)
	releaseData := base64.StdEncoding.EncodeToString(releaseJSON)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1." + name + ".v" + fmt.Sprintf("%d", version),
			Namespace: namespace,
		},
		Type: "helm.sh/release.v1",
		Data: map[string][]byte{
			"release": []byte(releaseData),
		},
	}
}

// Helper function to create a valid Radius Helm secret with annotation for testing
func createValidRadiusHelmSecretWithAnnotation(name, namespace string, version int, chartVersion, lastChecked string) *corev1.Secret {
	secret := createValidRadiusHelmSecret(name, namespace, version, chartVersion)
	secret.Annotations = map[string]string{
		RadiusUpgradeAnnotation: lastChecked,
	}
	return secret
}