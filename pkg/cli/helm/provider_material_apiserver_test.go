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
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestProviderSecretRetirementAPIServer(t *testing.T) {
	s, _, _, _ := newRotationTestStore(t)
	installRotationTestCRDs(t, s)
	for _, scenario := range []string{"complete", "last identity changed", "resource version race", "interrupted"} {
		t.Run(scenario, func(t *testing.T) {
			_, plan, _, _ := providerRecordingFixture(t, "radius", "external")
			ctx, secrets := t.Context(), s.kube.CoreV1().Secrets(s.namespace)
			for index := range plan.SourceSecrets {
				pin := &plan.SourceSecrets[index]
				secret, err := secrets.Create(ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: pin.Name, Namespace: s.namespace},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
				pin.UID = secret.UID
				t.Cleanup(func() {
					cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					err := secrets.Delete(cleanup, secret.Name, metav1.DeleteOptions{})
					require.True(t, err == nil || apierrors.IsNotFound(err))
				})
			}
			if scenario == "last identity changed" {
				plan.SourceSecrets[len(plan.SourceSecrets)-1].UID = types.UID(uuid.NewString())
			}
			require.NoError(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
				calls := 0
				fence := func(ctx context.Context) error {
					calls++
					if scenario == "interrupted" && calls == 2 {
						return errors.New("injected retirement interruption")
					}
					if scenario == "resource version race" && calls == 1 {
						secret, err := secrets.Get(ctx, plan.SourceSecrets[0].Name, metav1.GetOptions{})
						require.NoError(t, err)
						secret.Data = map[string][]byte{"changed": []byte("public-test-data")}
						_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
						require.NoError(t, err)
					}
					return operation.check(ctx)
				}
				err := s.retireProviderSecrets(ctx, plan, fence)
				switch scenario {
				case "complete":
					require.NoError(t, err)
				case "last identity changed":
					require.ErrorContains(t, err, "replaced")
					require.Zero(t, calls, "inspect the entire credential inventory before any deletion")
					for _, pin := range plan.SourceSecrets {
						_, err := secrets.Get(ctx, pin.Name, metav1.GetOptions{})
						require.NoError(t, err)
					}
					return nil
				case "resource version race":
					require.True(t, apierrors.IsConflict(err), "stale resourceVersion must prevent deletion")
				case "interrupted":
					require.ErrorContains(t, err, "interruption")
				}
				require.NoError(t, s.retireProviderSecrets(ctx, plan, operation.check))
				for _, pin := range plan.SourceSecrets {
					_, err := secrets.Get(ctx, pin.Name, metav1.GetOptions{})
					require.True(t, apierrors.IsNotFound(err))
				}
				return nil
			}))
		})
	}
}
