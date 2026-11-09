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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func TestIdentityOperationNativeExclusion(t *testing.T) {
	s, _, _, _ := newRotationTestStore(t)
	ctx, leases := t.Context(), s.kube.CoordinationV1().Leases(s.namespace)
	var firstUID types.UID
	require.NoError(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
		firstUID = operation.lease.UID
		require.NoError(t, operation.checkProviderJournal(ctx, s.release, nil))
		require.ErrorContains(t, withIdentityOperation(ctx, s.kube, s.namespace, func(*identityOperation) error {
			t.Fatal("competing provider/CA holder must not execute")
			return nil
		}), "cannot acquire")
		require.NoError(t, operation.check(ctx))
		return nil
	}))
	_, err := leases.Get(ctx, identityOperationLeaseName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
	callbackErr := errors.New("phase failed")
	require.ErrorIs(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
		require.NotEqual(t, firstUID, operation.lease.UID)
		return callbackErr
	}), callbackErr)
	_, err = leases.Get(ctx, identityOperationLeaseName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err), "handled phase failure releases the lease")

	_, err = leases.Create(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: identityOperationLeaseName, Namespace: s.namespace},
		Spec: coordinationv1.LeaseSpec{HolderIdentity: ptr.To("crashed"), LeaseDurationSeconds: ptr.To(int32(1)),
			AcquireTime: &metav1.MicroTime{Time: time.Now().Add(-time.Hour)}}}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, withIdentityOperation(ctx, s.kube, s.namespace, func(*identityOperation) error {
		t.Fatal("expired lease cannot be stolen")
		return nil
	}), "do not steal")
	require.NoError(t, leases.Delete(ctx, identityOperationLeaseName, metav1.DeleteOptions{}))

	for _, change := range []string{"resourceVersion", "replacement", "cancellation"} {
		t.Run(change, func(t *testing.T) {
			phaseCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			err := withIdentityOperation(phaseCtx, s.kube, s.namespace, func(operation *identityOperation) error {
				if change == "cancellation" {
					cancel()
					return phaseCtx.Err()
				}
				if change == "replacement" {
					require.NoError(t, leases.Delete(ctx, operation.lease.Name, metav1.DeleteOptions{}))
					copy := operation.lease.DeepCopy()
					copy.UID, copy.ResourceVersion = "", ""
					copy.CreationTimestamp = metav1.Time{}
					_, err := leases.Create(ctx, copy, metav1.CreateOptions{})
					require.NoError(t, err)
				} else {
					copy := operation.lease.DeepCopy()
					copy.Annotations = map[string]string{"changed": "true"}
					_, err := leases.Update(ctx, copy, metav1.UpdateOptions{})
					require.NoError(t, err)
				}
				require.ErrorContains(t, operation.check(ctx), "lock changed")
				return callbackErr
			})
			if change == "cancellation" {
				require.ErrorIs(t, err, context.Canceled)
				_, err = leases.Get(ctx, identityOperationLeaseName, metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err), "cleanup is bounded independently of cancelled phase")
				return
			}
			require.ErrorIs(t, err, callbackErr)
			require.ErrorContains(t, err, "cannot release")
			_, err = leases.Get(ctx, identityOperationLeaseName, metav1.GetOptions{})
			require.NoError(t, err, "stale release cannot delete a replaced or changed lease")
			require.NoError(t, leases.Delete(ctx, identityOperationLeaseName, metav1.DeleteOptions{}))
		})
	}
}

func TestIdentityOperationNativeProviderJournalGuard(t *testing.T) {
	s, _, _, _ := newRotationTestStore(t)
	ctx, maps := t.Context(), s.kube.CoreV1().ConfigMaps(s.namespace)
	complete := `{"version":1,"plan":{"testIdentity":"full validation delegated to A11"},"phase":"complete"}`
	for _, scenario := range []string{"complete", "stopping", "staging", "starting", "verifying", "unknown phase", "unknown version",
		"unknown field", "duplicate phase", "nested duplicate", "null", "trailing", "oversize", "foreign owner", "foreign Helm",
		"missing validator", "validator refusal", "journal mutation", "lease mutation"} {
		t.Run(scenario, func(t *testing.T) {
			raw := complete
			switch scenario {
			case "stopping", "staging", "starting", "verifying":
				raw = strings.ReplaceAll(raw, `"complete"`, `"`+scenario+`"`)
			case "unknown phase":
				raw = strings.ReplaceAll(raw, `"complete"`, `"done"`)
			case "unknown version":
				raw = strings.ReplaceAll(raw, `"version":1`, `"version":2`)
			case "unknown field":
				raw = strings.Replace(raw, "{", `{"unknown":true,`, 1)
			case "duplicate phase":
				raw = strings.Replace(raw, "{", `{"phase":"staging",`, 1)
			case "nested duplicate":
				raw = strings.Replace(raw, `"plan":{`, `"plan":{"testIdentity":"duplicate",`, 1)
			case "null":
				raw = strings.Replace(raw, `"plan":{`, `"plan":{"null":null,`, 1)
			case "trailing":
				raw += "{}"
			case "oversize":
				raw += strings.Repeat(" ", maxIdentityProviderBytes)
			}
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: identityProviderJournal, Namespace: s.namespace,
				Labels: map[string]string{identityProviderMarker: s.release}}, Data: map[string]string{identityProviderStateKey: raw}}
			if scenario == "foreign owner" {
				cm.Labels[identityProviderMarker] = "someone-else"
			}
			if scenario == "foreign Helm" {
				cm.Annotations = map[string]string{"meta.helm.sh/release-name": s.release}
			}
			_, err := maps.Create(ctx, cm, metav1.CreateOptions{})
			require.NoError(t, err)
			defer func() { require.NoError(t, maps.Delete(ctx, cm.Name, metav1.DeleteOptions{})) }()
			called := false
			err = withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
				validator := func(observed *corev1.ConfigMap) error {
					called = true
					if scenario == "validator refusal" {
						return errors.New("A11 rejected plan identity")
					}
					if scenario == "journal mutation" {
						observed.Annotations = map[string]string{"changed": "true"}
						_, err := maps.Update(ctx, observed, metav1.UpdateOptions{})
						require.NoError(t, err)
					}
					if scenario == "lease mutation" {
						copy := operation.lease.DeepCopy()
						copy.Annotations = map[string]string{"changed": "true"}
						_, err := s.kube.CoordinationV1().Leases(s.namespace).Update(ctx, copy, metav1.UpdateOptions{})
						require.NoError(t, err)
					}
					return nil
				}
				if scenario == "missing validator" {
					validator = nil
				}
				return operation.checkProviderJournal(ctx, s.release, validator)
			})
			if scenario == "complete" {
				require.NoError(t, err)
				require.True(t, called)
			} else {
				require.Error(t, err)
				require.Equal(t, scenario == "validator refusal" || scenario == "journal mutation" || scenario == "lease mutation", called)
			}
			if scenario == "lease mutation" {
				require.NoError(t, s.kube.CoordinationV1().Leases(s.namespace).Delete(ctx, identityOperationLeaseName, metav1.DeleteOptions{}))
			}
			if scenario == "staging" {
				require.Error(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
					return operation.checkProviderJournal(ctx, s.release, func(*corev1.ConfigMap) error {
						t.Fatal("inter-phase provider intent remains authoritative after lease release")
						return nil
					})
				}))
			}
		})
	}
}
