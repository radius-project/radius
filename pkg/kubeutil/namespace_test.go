// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubeutil

import (
	"context"
	"testing"

	"github.com/project-radius/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_PatchNamespace(t *testing.T) {
	client := k8sutil.NewFakeKubeClient(scheme.Scheme)

	err := PatchNamespace(context.Background(), client, "test")
	require.NoError(t, err)

	ns := &unstructured.Unstructured{}
	ns.SetAPIVersion("v1")
	ns.SetKind("Namespace")

	err = client.Get(context.Background(), runtime_client.ObjectKey{Name: "test"}, ns)
	require.NoError(t, err)

	expected := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name":            "test",
				"resourceVersion": "1",
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},
		},
	}
	require.Equal(t, expected, ns)
}
