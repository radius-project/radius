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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCertificatePendingFinalReleaseRecovery(t *testing.T) {
	for _, scenario := range []string{"recover", "foreign", "changed record", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			// A completed trust fixture exercises real Helm storage encoding.
			// It is not evidence that workloads adopted or served the new trust.
			w := completedLifecycleFixture(t)
			selected, err := loader.Load("../../../deploy/Chart")
			require.NoError(t, err)
			values := map[string]any{"fixture": "target"}
			w.state.Execution.ChartSHA256, err = bootstrapDigest(selected)
			require.NoError(t, err)
			w.state.Execution.TargetValuesSHA256, err = bootstrapDigest(values)
			require.NoError(t, err)
			w.state.Execution.FinalReleaseVersion = 0
			data, err := json.Marshal(w.state)
			require.NoError(t, err)
			w.journal.Data[rotationJournalKey] = string(data)
			w.journal, err = w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Update(t.Context(), w.journal, metav1.UpdateOptions{})
			require.NoError(t, err)
			history := storage.Init(driver.NewSecrets(w.store.kube.CoreV1().Secrets(w.store.namespace)))
			revision := &releasev1.Release{Name: w.store.release, Namespace: w.store.namespace, Version: 2,
				Chart: selected, Config: values, Labels: map[string]string{certificateExecutionOwner: w.state.ID},
				Info: &releasev1.Info{Status: releasecommon.StatusPendingUpgrade}}
			require.NoError(t, history.Create(revision))
			if scenario == "foreign" {
				revision.Labels[certificateExecutionOwner] = "foreign"
			} else if scenario == "changed record" {
				changed := *revision
				info := *revision.Info
				info.Description = "other writer"
				changed.Info = &info
				require.NoError(t, history.Update(&changed))
			}
			require.NoError(t, withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
				w.operation = operation
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if scenario == "canceled" {
					cancel()
				}
				err := w.recoverPendingFinalRelease(ctx, revision)
				if scenario == "recover" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				recorded, err := history.Get(w.store.release, 2)
				require.NoError(t, err)
				actual := recorded.(*releasev1.Release)
				if scenario == "recover" {
					require.Equal(t, releasecommon.StatusFailed, actual.Info.Status)
				} else {
					require.Equal(t, releasecommon.StatusPendingUpgrade, actual.Info.Status)
				}
				require.Zero(t, w.state.Execution.FinalReleaseVersion, "recovering history is not successful reconciliation")
				return nil
			}))
		})
	}
}
