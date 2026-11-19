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
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/release"
	"helm.sh/helm/v4/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// stagedUCPAPIServerObject returns object name of kind as rad stages it for the Radius release, or, when release is
// empty, without Helm ownership metadata.
func stagedUCPAPIServerObject(kind, name, release string, annotations map[string]string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{}
	object.SetAPIVersion("cert-manager.io/v1")
	object.SetKind(kind)
	object.SetNamespace(RadiusSystemNamespace)
	object.SetName(name)
	object.SetResourceVersion("1")
	object.SetUID(types.UID("uid-" + name))
	if release != "" {
		object.SetLabels(map[string]string{helmManagedByLabel: helmManagedByValue})
		annotations = mergeAnnotations(annotations, map[string]string{
			helmReleaseNameAnnotation:      release,
			helmReleaseNamespaceAnnotation: RadiusSystemNamespace,
		})
	}
	object.SetAnnotations(annotations)
	return object
}

func mergeAnnotations(a, b map[string]string) map[string]string {
	out := maps.Clone(a)
	if out == nil {
		out = map[string]string{}
	}
	maps.Copy(out, b)
	return out
}

func stageObjectExists(t *testing.T, client dynamic.Interface, resource schema.GroupVersionResource, name string) bool {
	t.Helper()
	_, err := client.Resource(resource).Namespace(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

func countDeletes(client *dynamicfake.FakeDynamicClient) int {
	deletes := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == "delete" {
			deletes++
		}
	}
	return deletes
}

// An upgrade that turns authorization on stages the radius-apiserver-ca chain before Helm adopts it. If it stops
// before Helm runs, the chain is outside the release, so Helm's uninstall leaves it; rad deletes it afterwards.
func Test_UninstallRadius_DeletesUnadoptedUCPAPIServerStage(t *testing.T) {
	var objects []runtime.Object
	for _, staged := range ucpAPIServerCAStageObjects {
		objects = append(objects, stagedUCPAPIServerObject(staged.kind, staged.name, radiusReleaseName, nil))
	}
	dynamicClient := fakeCertManagerDynamicClient(nil, objects...)

	mockHelmClient := NewMockHelmClient(gomock.NewController(t))
	options := NewDefaultClusterOptions()
	for _, c := range []struct{ releaseName, ns string }{
		{options.Radius.ReleaseName, options.Radius.Namespace},
		{options.Contour.ReleaseName, options.Contour.Namespace},
	} {
		mockHelmClient.EXPECT().RunHelmUninstall(gomock.AssignableToTypeOf(&helm.Configuration{}), c.releaseName, c.ns, true).Return(&release.UninstallReleaseResponse{}, nil)
	}
	for _, name := range []string{certManagerReleaseName} {
		mockHelmClient.EXPECT().RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), name).Return(nil, driver.ErrReleaseNotFound)
	}
	impl := &Impl{
		Helm:                        mockHelmClient,
		removeDefaultContourGateway: (&defaultContourGatewayCalls{}).remove,
		newUCPAPIServerTrustClient:  uninstallTrustClient,
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return dynamicClient, nil },
	}
	require.NoError(t, impl.UninstallRadius(t.Context(), options, "test-context"))
	for _, staged := range ucpAPIServerCAStageObjects {
		require.False(t, stageObjectExists(t, dynamicClient, staged.resource, staged.name), "uninstall must delete the staged %s %s", staged.kind, staged.name)
	}
}

func Test_deleteUnadoptedUCPAPIServerStage(t *testing.T) {
	certificate := ucpAPIServerCAStageObjects[1]
	require.Equal(t, "Certificate", certificate.kind)

	tests := []struct {
		name string
		// object replaces the staged Certificate; nil leaves it as rad stages it.
		object func() *unstructured.Unstructured
		// changeOnDelete makes the Certificate change between rad's read and its delete.
		changeOnDelete func(*unstructured.Unstructured)
		wantKept       bool
		wantWarning    string
		// wantRerunDeletes is true when a rerun reads the changed Certificate again, still finds it staged, and
		// deletes it.
		wantRerunDeletes bool
	}{
		{name: "staged for the release"},
		{
			name: "without Helm ownership",
			object: func() *unstructured.Unstructured {
				return stagedUCPAPIServerObject("Certificate", certificate.name, "", nil)
			},
			wantKept:    true,
			wantWarning: `which is not an object rad staged for the Radius release. If nothing else uses it, delete it with kubectl --context "test-context" -n radius-system delete certificate radius-apiserver-ca`,
		},
		{
			name: "of another Helm release",
			object: func() *unstructured.Unstructured {
				return stagedUCPAPIServerObject("Certificate", certificate.name, "other", nil)
			},
			wantKept:    true,
			wantWarning: "which is not an object rad staged for the Radius release",
		},
		{
			name: "kept by helm.sh/resource-policy",
			object: func() *unstructured.Unstructured {
				return stagedUCPAPIServerObject("Certificate", certificate.name, radiusReleaseName, map[string]string{helmResourcePolicyAnnotation: "keep"})
			},
			wantKept:    true,
			wantWarning: "which is not an object rad staged for the Radius release",
		},
		{
			name:             "changed after rad read it",
			changeOnDelete:   func(object *unstructured.Unstructured) { object.SetResourceVersion("2") },
			wantKept:         true,
			wantWarning:      "which changed after rad read it. Run the command again to delete it",
			wantRerunDeletes: true,
		},
		{
			name: "recreated by another owner after rad read it",
			changeOnDelete: func(object *unstructured.Unstructured) {
				object.SetUID("recreated")
				object.SetResourceVersion("2")
				object.SetLabels(nil)
				object.SetAnnotations(nil)
			},
			wantKept:    true,
			wantWarning: "which is not an object rad staged for the Radius release",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []runtime.Object
			for _, staged := range ucpAPIServerCAStageObjects {
				object := stagedUCPAPIServerObject(staged.kind, staged.name, radiusReleaseName, nil)
				if staged.name == certificate.name && staged.kind == certificate.kind && tt.object != nil {
					object = tt.object()
				}
				objects = append(objects, object)
			}
			dynamicClient := fakeCertManagerDynamicClient(nil, objects...)
			if tt.changeOnDelete != nil {
				changed := false
				dynamicClient.PrependReactor("delete", certificate.resource.Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
					if changed || action.(k8stesting.DeleteAction).GetName() != certificate.name {
						return false, nil, nil
					}
					changed = true
					live, err := dynamicClient.Tracker().Get(certificate.resource, RadiusSystemNamespace, certificate.name)
					require.NoError(t, err)
					object := live.(*unstructured.Unstructured).DeepCopy()
					tt.changeOnDelete(object)
					require.NoError(t, dynamicClient.Tracker().Update(certificate.resource, object, RadiusSystemNamespace))
					return true, nil, apierrors.NewConflict(certificate.resource.GroupResource(), certificate.name, fmt.Errorf("precondition failed"))
				})
			}
			var logs []string
			impl := &Impl{logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }}

			require.NoError(t, impl.deleteUnadoptedUCPAPIServerStage(t.Context(), "test-context", dynamicClient, RadiusSystemNamespace, radiusReleaseName))
			for _, staged := range ucpAPIServerCAStageObjects {
				want := tt.wantKept && staged.name == certificate.name && staged.kind == certificate.kind
				require.Equal(t, want, stageObjectExists(t, dynamicClient, staged.resource, staged.name), "%s %s", staged.kind, staged.name)
			}
			if tt.wantWarning == "" {
				require.Empty(t, logs)
			} else {
				require.Len(t, logs, 1)
				require.Contains(t, logs[0], "Warning: rad did not delete Certificate radius-system/radius-apiserver-ca")
				require.Contains(t, logs[0], tt.wantWarning)
			}

			deletes := countDeletes(dynamicClient)
			require.NoError(t, impl.deleteUnadoptedUCPAPIServerStage(t.Context(), "test-context", dynamicClient, RadiusSystemNamespace, radiusReleaseName))
			if tt.wantRerunDeletes {
				require.Equal(t, deletes+1, countDeletes(dynamicClient), "a rerun deletes the Certificate it read again")
				require.False(t, stageObjectExists(t, dynamicClient, certificate.resource, certificate.name))
				return
			}
			require.Equal(t, deletes, countDeletes(dynamicClient), "a rerun must not delete anything")
		})
	}

	t.Run("cert-manager CRDs already removed", func(t *testing.T) {
		dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		require.NoError(t, (&Impl{}).deleteUnadoptedUCPAPIServerStage(t.Context(), "test-context", dynamicClient, RadiusSystemNamespace, radiusReleaseName))
	})
}
