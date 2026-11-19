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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func expectControllerStrategyMigration(client *MockHelmClient, reuseValues bool) *gomock.Call {
	return client.EXPECT().MigrateControllerStrategy(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "radius", "radius-system", reuseValues).Return(nil).Call
}

func TestMigrateControllerDeployment(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"migrate", "already migrated", "missing", "foreign release", "foreign namespace", "foreign manager", "get forbidden", "patch forbidden", "concurrent edit", "concurrent replacement"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "controller", Namespace: "custom-namespace", UID: "original", ResourceVersion: "1",
					Labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"},
					Annotations: map[string]string{
						"meta.helm.sh/release-name":      "custom-release",
						"meta.helm.sh/release-namespace": "custom-namespace",
					},
				},
				Spec: appsv1.DeploymentSpec{Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: &appsv1.RollingUpdateDeployment{},
				}},
			}
			switch name {
			case "already migrated":
				deployment.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
			case "foreign release":
				deployment.Annotations["meta.helm.sh/release-name"] = "another-release"
			case "foreign namespace":
				deployment.Annotations["meta.helm.sh/release-namespace"] = "another-namespace"
			case "foreign manager":
				deployment.Labels["app.kubernetes.io/managed-by"] = "another-manager"
			}
			client := fake.NewClientset()
			if name != "missing" {
				require.NoError(t, client.Tracker().Add(deployment))
			}
			forbidden := apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, deployment.Name, errors.New("denied"))
			if name == "get forbidden" {
				client.PrependReactor("get", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, forbidden
				})
			}
			patches := 0
			client.PrependReactor("patch", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
				patches++
				if name == "patch forbidden" {
					return true, nil, forbidden
				}
				if name == "concurrent edit" || name == "concurrent replacement" {
					changed := deployment.DeepCopy()
					if name == "concurrent edit" {
						changed.ResourceVersion = "2"
					} else {
						changed.UID = "replacement"
					}
					require.NoError(t, client.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), changed, changed.Namespace))
				}
				return false, nil, nil
			})

			err := migrateControllerDeployment(t.Context(), client.AppsV1().Deployments(deployment.Namespace), "custom-release", deployment.Namespace)
			switch name {
			case "migrate":
				require.NoError(t, err)
				require.Equal(t, 1, patches)
				after, err := client.AppsV1().Deployments(deployment.Namespace).Get(t.Context(), deployment.Name, metav1.GetOptions{})
				require.NoError(t, err)
				expected := deployment.DeepCopy()
				expected.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
				require.Equal(t, expected, after)
			case "missing", "already migrated":
				require.NoError(t, err)
				require.Zero(t, patches)
			case "foreign release", "foreign namespace", "foreign manager":
				require.ErrorContains(t, err, "not owned by Helm release")
				require.Zero(t, patches)
			case "get forbidden", "patch forbidden":
				require.ErrorIs(t, err, forbidden)
			default:
				require.ErrorContains(t, err, "failed to migrate controller")
				after, getErr := client.AppsV1().Deployments(deployment.Namespace).Get(t.Context(), deployment.Name, metav1.GetOptions{})
				require.NoError(t, getErr)
				require.Equal(t, deployment.Spec.Strategy, after.Spec.Strategy)
			}
		})
	}
}

func TestRadiusControllerMigrationFailureStopsUpgrade(t *testing.T) {
	for _, reinstall := range []bool{false, true} {
		t.Run(map[bool]string{false: "upgrade", true: "reinstall"}[reinstall], func(t *testing.T) {
			client := NewMockHelmClient(gomock.NewController(t))
			options := NewDefaultClusterOptions()
			options.Contour.Disabled = true
			options.Radius.Reinstall = reinstall
			expectChartPull(t, client, radiusChartRef(options), newTestContourChart())

			failure := errors.New("controller patch denied")
			client.EXPECT().MigrateControllerStrategy(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "radius", "radius-system", true).Return(failure)
			impl := withoutUCPAPIService(&Impl{Helm: client})
			var err error
			if reinstall {
				expectNoRadiusHistory(client)
				err = impl.InstallRadius(t.Context(), options, "test-context")
			} else {
				expectRadiusBaseline(t, client, options)
				err = impl.UpgradeRadius(t.Context(), options, "test-context")
			}
			require.ErrorIs(t, err, failure)
		})
	}
}
