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
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/ptr"
)

func TestIdentityOperationNativeCAEntry(t *testing.T) {
	for _, provider := range []string{"radius", "external"} {
		t.Run(provider, func(t *testing.T) {
			testIdentityOperationNativeCAEntry(t, provider)
		})
	}
}

func testIdentityOperationNativeCAEntry(t *testing.T, provider string) {
	t.Helper()
	s, config, state := newPlannedRotationTest(t)
	ctx := t.Context()
	if provider == "external" {
		state = aggregationOnlyState(t, state)
		state.Plan.ServingSecret = "ucp-cert-aggregation-" + strings.ReplaceAll(state.ID, "-", "")[:16]
		journal, _, err := s.read(ctx)
		require.NoError(t, err)
		data, err := json.Marshal(state)
		require.NoError(t, err)
		journal.Data[rotationJournalKey] = string(data)
		_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Update(ctx, journal, metav1.UpdateOptions{})
		require.NoError(t, err)
	}
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, clientcmd.WriteToFile(clientcmdapi.Config{
		Clusters:  map[string]*clientcmdapi.Cluster{"test": {Server: config.Host, CertificateAuthorityData: config.CAData}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"test": {ClientCertificateData: config.CertData, ClientKeyData: config.KeyData}},
		Contexts:  map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}}, CurrentContext: "test",
	}, path))
	t.Setenv("KUBECONFIG", path)
	release := &releasev1.Release{Name: s.release, Namespace: s.namespace,
		Chart: &chart.Chart{Metadata: &chart.Metadata{Annotations: map[string]string{"radius.io/ca-rotation-protocol": "1"}},
			Values: map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}},
		Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}
	if provider == "external" {
		release.Config = externalProviderValues()
	}
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmGet(gomock.Any(), s.release).Return(release, nil).Times(6)
	impl := &Impl{Helm: client}
	options := NewDefaultClusterOptions()
	options.Certificates.RotationPhase = string(rotationPublish)
	leases := s.kube.CoordinationV1().Leases(s.namespace)
	held, err := leases.Create(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: identityOperationLeaseName, Namespace: s.namespace},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: ptr.To("another-invocation")},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, impl.runCertificateRotation(ctx, options, "test", false), "cannot acquire identity operation")
	require.ErrorContains(t, impl.runCertificateRotation(ctx, options, "test", true), "planned CA signing Secret")
	unchanged, err := leases.Get(ctx, held.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, held, unchanged, "read-only preflight must not acquire or release a Lease")
	require.NoError(t, leases.Delete(ctx, held.Name, metav1.DeleteOptions{}))
	// The fixture deliberately has no signer: a handled phase failure must
	// release the lease without changing the persistent transaction.
	journal, _, err := s.read(ctx)
	require.NoError(t, err)
	require.ErrorContains(t, impl.runCertificateRotation(ctx, options, "test", false), "planned CA signing Secret")
	_, err = leases.Get(ctx, held.Name, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
	_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: identityProviderJournal, Namespace: s.namespace,
			Labels: map[string]string{identityProviderMarker: s.release}},
		Data: map[string]string{identityProviderStateKey: `{"version":1,"plan":{},"phase":"starting"}`},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, impl.runCertificateRotation(ctx, options, "test", false), "invalid provider transition")
	_, err = leases.Get(ctx, held.Name, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
	retained, _, err := s.read(ctx)
	require.NoError(t, err)
	require.Equal(t, journal, retained)
	require.NoError(t, s.kube.CoreV1().ConfigMaps(s.namespace).Delete(ctx, identityProviderJournal, metav1.DeleteOptions{}))
	target, key, _ := testSigningCA(t, nil, nil, nil, nil)
	signing, err := s.kube.CoreV1().Secrets(s.namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: state.Plan.SigningSecret, Namespace: s.namespace},
		Type:       corev1.SecretTypeTLS, Data: map[string][]byte{"tls.crt": target, "tls.key": key},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	digest, _, err := mtls.TrustBundleDigest(target, time.Now())
	require.NoError(t, err)
	state.NewCA, state.Plan.SigningUID = target, signing.UID
	state.Plan.TargetTrustSHA256, state.Plan.TargetTrustMap = digest, "radius-mtls-ca-"+digest[:32]
	data, err := json.Marshal(state)
	require.NoError(t, err)
	retained.Data[rotationJournalKey] = string(data)
	_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Update(ctx, retained, metav1.UpdateOptions{})
	require.NoError(t, err)
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(api.Object, base64.StdEncoding.EncodeToString(state.OldCA), "spec", "caBundle"))
	_, err = s.dynamic.Resource(apiServiceResource).Update(ctx, api, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, impl.runCertificateRotation(ctx, options, "test", false))
	published, phase, err := s.read(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationAdopt, phase.Phase)
	require.NoError(t, impl.runCertificateRotation(ctx, options, "test", true))
	afterPreflight, _, err := s.read(ctx)
	require.NoError(t, err)
	require.Equal(t, published, afterPreflight)
	_, err = leases.Get(ctx, held.Name, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

func TestIdentityOperationNativeCAWriterIntegration(t *testing.T) {
	s, _, old, target := newRotationTestStore(t)
	ctx := t.Context()
	s.guardWrite = nil
	_, err := s.begin(ctx, uuid.NewString(), target)
	require.ErrorContains(t, err, "shared identity operation")
	var transaction string
	require.NoError(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
		s.guardWrite = func(ctx context.Context) error { return operation.checkProviderJournal(ctx, s.release, nil) }
		state, err := s.begin(ctx, uuid.NewString(), target)
		require.NoError(t, err)
		transaction = state.ID
		_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: identityProviderJournal, Namespace: s.namespace,
				Labels: map[string]string{identityProviderMarker: s.release}},
			Data: map[string]string{identityProviderStateKey: `{"version":1,"plan":{},"phase":"staging"}`},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
		require.ErrorContains(t, s.reconcileTrust(ctx, state.ID), "finish the provider transition")
		return nil
	}))
	require.NoError(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
		s.guardWrite = func(ctx context.Context) error { return operation.checkProviderJournal(ctx, s.release, nil) }
		require.ErrorContains(t, s.reconcileTrust(ctx, transaction), "finish the provider transition")
		return nil
	}))
	require.Error(t, s.reconcileTrust(ctx, transaction), "a released operation cannot authorize a later write")
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	bundle, err := apiServiceBundle(api)
	require.NoError(t, err)
	require.Equal(t, old, bundle, "provider interference must not publish the target CA")
}

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
