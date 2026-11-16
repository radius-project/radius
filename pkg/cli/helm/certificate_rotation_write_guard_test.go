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
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestRotationImportChecksOperation(t *testing.T) {
	t.Parallel()
	for _, existingNamespace := range []bool{false, true} {
		t.Run(map[bool]string{false: "namespace creation", true: "Secret creation"}[existingNamespace], func(t *testing.T) {
			t.Parallel()
			kube := kubefake.NewClientset()
			if existingNamespace {
				require.NoError(t, kube.Tracker().Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace}}))
			}
			blocked := errors.New("operation lock changed")
			plan := &certificatePlan{client: kube, namespace: RadiusSystemNamespace,
				secret:      &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "new-ca", Namespace: RadiusSystemNamespace}},
				beforeWrite: func(context.Context) error { return blocked }}
			require.ErrorIs(t, (&Impl{}).applyCertificatePlan(t.Context(), plan), blocked)
			for _, action := range kube.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}
