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

package helm

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestProviderJournalNativePublicEntries(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated API-server tests.")
	}
	path, kube := startTestAPIServer(t)
	t.Setenv("KUBECONFIG", path)
	ctx := t.Context()
	_, err := kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	held, err := kube.CoordinationV1().Leases(RadiusSystemNamespace).Create(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: identityOperationLeaseName},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: ptr.To("another-operation")},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	for _, raw := range []string{`{"version":1,"phase":"stopping"}`, `{"version":1,"phase":"complete"}`, `{"version":2,"phase":"complete"}`, "{"} {
		cm := providerJournalFixture(t)
		cm.UID, cm.ResourceVersion = "", ""
		cm.Data[identityProviderStateKey] = raw
		cm, err = kube.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{})
		require.NoError(t, err)
		for name, call := range providerEntryCalls(&Impl{}) {
			t.Run(name+"/"+raw, func(t *testing.T) { require.Error(t, call(t.Context())) })
		}
		retained, err := kube.CoreV1().ConfigMaps(cm.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, cm, retained)
		lock, err := kube.CoordinationV1().Leases(held.Namespace).Get(ctx, held.Name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, held, lock)
		require.NoError(t, kube.CoreV1().ConfigMaps(cm.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}))
	}
}
