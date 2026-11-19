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
	"github.com/stretchr/testify/require"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"testing"
)

func TestInternalServingInventoryDoesNotUseLegacyDriverEvidence(t *testing.T) {
	state, _ := lifecycleFixture(t)
	state.Phase = rotationIssue
	agg := state.Planes["aggregation"]
	agg.Affected = false
	state.Planes["aggregation"] = agg
	kube := kubefake.NewSimpleClientset()
	_, err := readLifecycleServingInventory(t.Context(), kube, state, &certificateTarget{Provider: state.Provider})
	require.ErrorIs(t, err, errCertificateObserverUnavailable)
	require.Empty(t, kube.Actions())
}
