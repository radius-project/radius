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

package secretstores

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/radius-project/radius/test/testutil"
)

const (
	// sentinelSecretValue is stored in every test Secret. It must never appear in errors, logs or
	// denial responses.
	sentinelSecretValue = "sentinel-secret-value"

	testControlPlaneSecret = "database-secret"
	testSecretUID          = "test-secret-uid"
)

func admissionContext(t *testing.T) (context.Context, *observer.ObservedLogs) {
	t.Helper()
	id, err := resources.ParseResource(testSecretID)
	require.NoError(t, err)
	core, logs := observer.New(zap.DebugLevel)
	ctx := v1.WithARMRequestContext(t.Context(), &v1.ARMRequestContext{ResourceID: id})
	return logr.NewContext(ctx, zapr.NewLogger(zap.New(core))), logs
}

func existingSecret(ns, name string, labels map[string]string) *corev1.Secret {
	return &corev1.Secret{
		Name:      name,
		Namespace: ns,
		UID:       testSecretUID,
		Labels:    labels,
		Data: map[string][]byte{
			"tls.crt": []byte(sentinelSecretValue),
			"tls.key": []byte(sentinelSecretValue),
		},
	}
}

func authzOptions(mode authz.Mode) authz.Options {
	return authz.Options{Mode: mode, ControlPlaneSecrets: []string{testControlPlaneSecret, "ucp-cert"}}
}

// requireNoSecretValue fails if the sentinel Secret value appears in err, its ARM denial response,
// or any log entry.
func requireNoSecretValue(t *testing.T, err error, logs *observer.ObservedLogs) {
	t.Helper()
	if err != nil {
		require.NotContains(t, err.Error(), sentinelSecretValue)
		if denied, ok := errors.AsType[*authz.DeniedError](err); ok {
			resp, rerr := denied.Response()
			require.NoError(t, rerr)
			w := httptest.NewRecorder()
			require.NoError(t, resp.Apply(t.Context(), w, httptest.NewRequest("PUT", "/", nil)))
			require.Equal(t, 403, w.Code)
			require.NotContains(t, w.Body.String(), sentinelSecretValue)
			require.Contains(t, w.Body.String(), v1.CodeAdmissionPolicyDenied)
		}
	}
	for _, entry := range logs.All() {
		require.NotContains(t, entry.Message, sentinelSecretValue)
		require.NotContains(t, fmt.Sprint(entry.ContextMap()), sentinelSecretValue)
	}
}

func requireAdmission(t *testing.T, err error, logs *observer.ObservedLogs, mode authz.Mode, wantRule string) {
	t.Helper()
	requireNoSecretValue(t, err, logs)
	if wantRule != "" && mode == authz.ModeEnforce {
		denied, ok := errors.AsType[*authz.DeniedError](err)
		require.True(t, ok, "expected a denial, got %v", err)
		require.Equal(t, v1.CodeAdmissionPolicyDenied, denied.Code)
		require.Equal(t, wantRule, denied.Rule)
		require.Equal(t, testSecretID, denied.Target)
		require.Equal(t, 1, logs.FilterField(zap.Bool(authz.LogFieldDenied, true)).FilterField(zap.String(authz.LogFieldRule, wantRule)).Len())
		return
	}

	require.NoError(t, err)
	if mode == "" || mode == authz.ModeOff {
		for _, entry := range logs.All() {
			require.NotContains(t, entry.ContextMap(), authz.LogFieldMode)
		}
	}
	require.Zero(t, logs.FilterField(zap.Bool(authz.LogFieldDenied, true)).Len())
	wouldDeny := logs.FilterField(zap.Bool(authz.LogFieldWouldDeny, true))
	if wantRule != "" && mode == authz.ModeDryRun {
		require.Equal(t, 1, wouldDeny.FilterField(zap.String(authz.LogFieldRule, wantRule)).Len())
	} else {
		require.Zero(t, wouldDeny.Len())
	}
}

func TestUpsertSecret_ControlPlaneSecretReference(t *testing.T) {
	tests := []struct {
		name     string
		mode     authz.Mode
		ref      string
		cpNS     string
		wantRule string
	}{
		{name: "unset mode protected secret", ref: "radius-system/" + testControlPlaneSecret},
		{name: "off protected secret", mode: authz.ModeOff, ref: "radius-system/" + testControlPlaneSecret},
		{name: "unset mode application secret", ref: "default/app-secret"},
		{name: "off application secret", mode: authz.ModeOff, ref: "default/app-secret"},
		{name: "dryRun application secret", mode: authz.ModeDryRun, ref: "default/app-secret"},
		{name: "dryRun protected secret", mode: authz.ModeDryRun, ref: "radius-system/" + testControlPlaneSecret, wantRule: authz.RuleControlPlaneSecret},
		{name: "enforce protected secret", mode: authz.ModeEnforce, ref: "radius-system/" + testControlPlaneSecret, wantRule: authz.RuleControlPlaneSecret},
		{name: "dryRun other secret in control-plane namespace", mode: authz.ModeDryRun, ref: "radius-system/app-secret", wantRule: authz.RuleControlPlaneNamespace},
		{name: "enforce other secret in control-plane namespace", mode: authz.ModeEnforce, ref: "radius-system/app-secret", wantRule: authz.RuleControlPlaneNamespace},
		{name: "enforce protected name outside control-plane namespace", mode: authz.ModeEnforce, ref: "default/" + testControlPlaneSecret},
		{name: "enforce application secret", mode: authz.ModeEnforce, ref: "default/app-secret"},
		{name: "enforce configured control-plane namespace", mode: authz.ModeEnforce, ref: "radius/" + testControlPlaneSecret, cpNS: "radius", wantRule: authz.RuleControlPlaneSecret},
		{name: "enforce default namespace not control plane when configured", mode: authz.ModeEnforce, ref: "radius-system/" + testControlPlaneSecret, cpNS: "radius"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(kubeutil.TargetKubeconfigEnvVar, "")
			ns, name, err := fromResourceID(tt.ref)
			require.NoError(t, err)
			newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
			newResource.Properties.Resource = tt.ref
			newResource.Properties.Status.OutputResources = nil
			opts := authzOptions(tt.mode)
			opts.ControlPlaneNamespace = tt.cpNS
			opt := &controller.Options{
				KubeClient:    k8sutil.NewFakeKubeClient(nil, existingSecret(ns, name, nil)),
				Authorization: opts,
			}

			ctx, logs := admissionContext(t)
			resp, err := UpsertSecret(ctx, newResource, nil, opt)
			requireAdmission(t, err, logs, tt.mode, tt.wantRule)
			if err != nil {
				require.Empty(t, newResource.Properties.Status.OutputResources, "a denied secret store must not record the Secret")
				return
			}
			require.Nil(t, resp)
			require.Equal(t, tt.ref, newResource.Properties.Resource)
			require.Len(t, newResource.Properties.Status.OutputResources, 1)
		})
	}
}

// TestUpsertSecret_ControlPlaneNamespaceFromScope covers a secret store without
// $.properties.resource whose application resolves to the control-plane namespace. The
// Namespace and Secret must not be created in enforce mode.
func TestUpsertSecret_ControlPlaneNamespaceFromScope(t *testing.T) {
	for _, mode := range []authz.Mode{"", authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Setenv(kubeutil.TargetKubeconfigEnvVar, "")
			sc := database.NewMockClient(gomock.NewController(t))
			app := &datamodel.Application{}
			app.Properties.Status.Compute = &rpv1.EnvironmentCompute{
				Kind:              rpv1.KubernetesComputeKind,
				KubernetesCompute: rpv1.KubernetesComputeProperties{Namespace: "radius-system"},
			}
			sc.EXPECT().Get(gomock.Any(), testAppID, gomock.Any()).Return(rpctest.FakeStoreObject(app), nil)

			newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValue)
			newResource.Properties.Resource = ""
			kubeClient := newSecretClient()
			opt := &controller.Options{DatabaseClient: sc, KubeClient: kubeClient, Authorization: authzOptions(mode)}

			ctx, logs := admissionContext(t)
			_, err := ValidateAndMutateRequest(ctx, newResource, nil, opt)
			require.NoError(t, err)
			_, err = UpsertSecret(ctx, newResource, nil, opt)
			requireAdmission(t, err, logs, mode, authz.RuleControlPlaneNamespace)
			for _, v := range newResource.Properties.Data {
				if v.Value != nil {
					require.NotContains(t, fmt.Sprint(logs.All()), *v.Value)
					if err != nil {
						require.NotContains(t, err.Error(), *v.Value)
					}
				}
			}

			nsErr := kubeClient.Get(ctx, runtimeclient.ObjectKey{Name: "radius-system"}, &corev1.Namespace{})
			secretErr := kubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: "radius-system", Name: "secret0"}, &corev1.Secret{})
			if mode == authz.ModeEnforce {
				require.True(t, apierrors.IsNotFound(nsErr), "the namespace must not be created")
				require.True(t, apierrors.IsNotFound(secretErr), "the Secret must not be created")
				return
			}
			require.NoError(t, nsErr)
			require.NoError(t, secretErr)
		})
	}
}

// TestUpsertSecret_ControlPlaneReferenceIgnoresTarget verifies that a separate deployment target
// never exempts a secret store: secret stores use the control-plane client, so the reference is on
// the control-plane cluster whatever RADIUS_TARGET_KUBECONFIG names, and no target identity is
// resolved.
func TestUpsertSecret_ControlPlaneReferenceIgnoresTarget(t *testing.T) {
	ref := "radius-system/" + testControlPlaneSecret
	tests := []struct {
		name       string
		kubeconfig func(t *testing.T) string
		distinct   bool
	}{
		{
			name: "distinct remote target",
			kubeconfig: func(t *testing.T) string {
				target := k8sutil.NewFakeKubeClient(nil)
				require.NoError(t, target.Create(t.Context(), &corev1.Namespace{Name: "kube-system", UID: "external"}))
				server := k8sutil.NewNamespaceAPIServer(t, target)
				path := filepath.Join(t.TempDir(), "target.kubeconfig")
				k8sutil.WriteKubeconfig(t, path, "target", map[string]string{"target": server.URL})
				return path
			},
		},
		{
			name: "unreadable target kubeconfig",
			kubeconfig: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.kubeconfig")
			},
		},
		{name: "options already marked distinct", distinct: true},
	}

	for _, tt := range tests {
		for _, mode := range []authz.Mode{"", authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
			t.Run(tt.name+" "+string(mode), func(t *testing.T) {
				path := ""
				if tt.kubeconfig != nil {
					path = tt.kubeconfig(t)
				}
				t.Setenv(kubeutil.TargetKubeconfigEnvVar, path)

				newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
				newResource.Properties.Resource = ref
				opts := authzOptions(mode)
				opts.TargetClusterIsDistinct = tt.distinct
				opt := &controller.Options{
					KubeClient:    k8sutil.NewFakeKubeClient(nil, existingSecret("radius-system", testControlPlaneSecret, nil)),
					Authorization: opts,
				}

				ctx, logs := admissionContext(t)
				_, err := UpsertSecret(ctx, newResource, nil, opt)
				requireAdmission(t, err, logs, mode, authz.RuleControlPlaneSecret)
			})
		}
	}
}

func TestUpsertSecret_BackwardCompatibleUpdate(t *testing.T) {
	for _, mode := range []authz.Mode{"", authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		for _, ns := range []string{"default", "radius-system"} {
			t.Run(string(mode)+"/"+ns, func(t *testing.T) {
				t.Setenv(kubeutil.TargetKubeconfigEnvVar, filepath.Join(t.TempDir(), "missing.kubeconfig"))
				kubeClient := k8sutil.NewFakeKubeClient(nil, existingSecret(ns, testControlPlaneSecret,
					map[string]string{kubernetes.LabelRadiusResourceType: "applications.core-secretstores"}))
				oldResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
				oldResource.Properties.Resource = ns + "/" + testControlPlaneSecret
				ctx, _ := admissionContext(t)
				resp, err := UpsertSecret(ctx, oldResource, nil, &controller.Options{KubeClient: kubeClient})
				require.NoError(t, err)
				require.Nil(t, resp)

				newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValue)
				newResource.Properties.Resource = ""
				newResource.Properties.Status.OutputResources = nil
				opts := &controller.Options{KubeClient: kubeClient, Authorization: authzOptions(mode)}
				ctx, logs := admissionContext(t)
				resp, err = UpsertSecret(ctx, newResource, oldResource, opts)
				wantRule := ""
				if ns == "radius-system" {
					wantRule = authz.RuleControlPlaneSecret
				}
				requireAdmission(t, err, logs, mode, wantRule)
				require.Nil(t, resp)
				stored := &corev1.Secret{}
				require.NoError(t, kubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: testControlPlaneSecret}, stored))
				if mode == authz.ModeEnforce && wantRule != "" {
					require.Equal(t, []byte(sentinelSecretValue), stored.Data["tls.crt"])
				} else {
					expected := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValue)
					for key, value := range expected.Properties.Data {
						require.Equal(t, []byte(*value.Value), stored.Data[key])
					}
					require.Equal(t, oldResource.Properties.Resource, newResource.Properties.Resource)
					require.Equal(t, oldResource.Properties.Status.OutputResources, newResource.Properties.Status.OutputResources)
				}
			})
		}
	}
}

// TestUpsertSecret_LegacyControlPlaneReferenceLifecycle covers a secret store that referenced a
// control-plane Secret before enforcement: updates and deletion are denied, while ordinary
// owned Secrets can still be cleaned up.
func TestUpsertSecret_LegacyControlPlaneReferenceLifecycle(t *testing.T) {
	t.Setenv(kubeutil.TargetKubeconfigEnvVar, "")
	ref := "radius-system/" + testControlPlaneSecret
	kubeClient := k8sutil.NewFakeKubeClient(nil, existingSecret("radius-system", testControlPlaneSecret, nil))

	oldResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
	oldResource.Properties.Resource = ref
	ctx, logs := admissionContext(t)
	_, err := UpsertSecret(ctx, oldResource, nil, &controller.Options{KubeClient: kubeClient, Authorization: authzOptions(authz.ModeOff)})
	require.NoError(t, err)

	newResource := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
	newResource.Properties.Resource = ""
	newResource.Properties.Status.OutputResources = nil
	opt := &controller.Options{KubeClient: kubeClient, Authorization: authzOptions(authz.ModeEnforce)}
	_, err = UpsertSecret(ctx, newResource, oldResource, opt)
	requireAdmission(t, err, logs, authz.ModeEnforce, authz.RuleControlPlaneSecret)

	ctx, logs = admissionContext(t)
	_, err = DeleteRadiusSecret(ctx, oldResource, opt)
	requireAdmission(t, err, logs, authz.ModeEnforce, authz.RuleControlPlaneSecret)
	require.NoError(t, kubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: "radius-system", Name: testControlPlaneSecret}, &corev1.Secret{}),
		"delete must not remove a control-plane Secret")

	managed := existingSecret("default", "secret0", map[string]string{kubernetes.LabelRadiusResourceType: "applications.core-secretstores"})
	require.NoError(t, kubeClient.Create(ctx, managed))
	current := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
	current.Properties.Status.OutputResources = []rpv1.OutputResource{{
		LocalID:              rpv1.LocalIDSecret,
		ID:                   resources_kubernetes.IDFromParts(resources_kubernetes.PlaneNameTODO, "", resources_kubernetes.KindSecret, "default", "secret0"),
		RadiusManaged:        new(true),
		AdditionalProperties: map[string]string{secretUIDProperty: testSecretUID},
	}}
	_, err = DeleteRadiusSecret(ctx, current, opt)
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(kubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: "default", Name: "secret0"}, &corev1.Secret{})))
}

func TestListSecrets_ControlPlaneSecretReference(t *testing.T) {
	tests := []struct {
		name     string
		mode     authz.Mode
		ns       string
		secret   string
		wantRule string
	}{
		{name: "unset mode protected secret", ns: "radius-system", secret: testControlPlaneSecret},
		{name: "off protected secret", mode: authz.ModeOff, ns: "radius-system", secret: testControlPlaneSecret},
		{name: "unset mode application secret", ns: "default", secret: "app-secret"},
		{name: "off application secret", mode: authz.ModeOff, ns: "default", secret: "app-secret"},
		{name: "dryRun application secret", mode: authz.ModeDryRun, ns: "default", secret: "app-secret"},
		{name: "dryRun protected secret", mode: authz.ModeDryRun, ns: "radius-system", secret: testControlPlaneSecret, wantRule: authz.RuleControlPlaneSecret},
		{name: "enforce protected secret", mode: authz.ModeEnforce, ns: "radius-system", secret: testControlPlaneSecret, wantRule: authz.RuleControlPlaneSecret},
		{name: "enforce other secret in control-plane namespace", mode: authz.ModeEnforce, ns: "radius-system", secret: "app-secret", wantRule: authz.RuleControlPlaneNamespace},
		{name: "enforce application secret", mode: authz.ModeEnforce, ns: "default", secret: "app-secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(kubeutil.TargetKubeconfigEnvVar, "")
			secretdm := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
			secretdm.Properties.Resource = tt.ns + "/" + tt.secret
			secretdm.Properties.Status.OutputResources = []rpv1.OutputResource{{
				LocalID: rpv1.LocalIDSecret,
				ID:      resources_kubernetes.IDFromParts(resources_kubernetes.PlaneNameTODO, "", resources_kubernetes.KindSecret, tt.ns, tt.secret),
			}}
			databaseClient := database.NewMockClient(gomock.NewController(t))
			databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&database.Object{Data: secretdm}, nil)

			req, err := rpctest.NewHTTPRequestWithContent(t.Context(), v1.OperationPost.HTTPMethod(),
				"http://localhost:8080"+testSecretID+"/listsecrets?api-version=2023-10-01-preview", nil)
			require.NoError(t, err)
			core, logs := observer.New(zap.DebugLevel)
			ctx := logr.NewContext(rpctest.NewARMRequestContext(req), zapr.NewLogger(zap.New(core)))

			secretReads := 0
			kubeClient := interceptor.NewClient(k8sutil.NewFakeKubeClient(nil, existingSecret(tt.ns, tt.secret, nil)), interceptor.Funcs{
				Get: func(ctx context.Context, client runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					if _, ok := obj.(*corev1.Secret); ok {
						secretReads++
					}
					return client.Get(ctx, key, obj, opts...)
				},
			})
			ctl, err := NewListSecrets(controller.Options{
				DatabaseClient: databaseClient,
				KubeClient:     kubeClient,
				Authorization:  authzOptions(tt.mode),
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			resp, err := ctl.Run(ctx, w, req)
			requireAdmission(t, err, logs, tt.mode, tt.wantRule)
			if err != nil {
				require.Nil(t, resp)
				require.Zero(t, secretReads, "admission must deny before reading Secret data")
				return
			}
			require.Equal(t, 1, secretReads)
			require.NoError(t, resp.Apply(ctx, w, req))
			require.Equal(t, 200, w.Code)
			require.True(t, strings.Contains(w.Body.String(), sentinelSecretValue), "an allowed request returns the Secret values")
		})
	}
}

func TestDeleteRadiusSecret_ControlPlaneSecretReference(t *testing.T) {
	references := []struct {
		name     string
		ns       string
		secret   string
		cpNS     string
		wantRule string
	}{
		{name: "protected secret", ns: "radius-system", secret: testControlPlaneSecret, wantRule: authz.RuleControlPlaneSecret},
		{name: "other control-plane secret", ns: "radius-system", secret: "other", wantRule: authz.RuleControlPlaneNamespace},
		{name: "application secret", ns: "default", secret: testControlPlaneSecret},
		{name: "custom control-plane namespace", ns: "custom-radius", cpNS: "custom-radius", secret: testControlPlaneSecret, wantRule: authz.RuleControlPlaneSecret},
	}
	labels := []struct {
		name   string
		values map[string]string
		owned  bool
	}{
		{name: "unlabeled"},
		{name: "secretstore label", values: map[string]string{kubernetes.LabelRadiusResourceType: "applications.core-secretstores"}},
		{name: "unrelated resource label", values: map[string]string{kubernetes.LabelRadiusResourceType: "unrelated"}},
		{name: "owned without label", owned: true},
		{name: "owned with label", values: map[string]string{kubernetes.LabelRadiusResourceType: "applications.core-secretstores"}, owned: true},
	}
	for _, mode := range []authz.Mode{"", authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		for _, ref := range references {
			for _, label := range labels {
				t.Run(string(mode)+"/"+ref.name+"/"+label.name, func(t *testing.T) {
					t.Setenv(kubeutil.TargetKubeconfigEnvVar, filepath.Join(t.TempDir(), "missing.kubeconfig"))
					store := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
					// Deletion must check the output reference it actually uses, not properties.resource.
					store.Properties.Resource = "default/unused"
					store.Properties.Status.OutputResources = []rpv1.OutputResource{{
						LocalID: rpv1.LocalIDSecret,
						ID:      resources_kubernetes.IDFromParts(resources_kubernetes.PlaneNameTODO, "", resources_kubernetes.KindSecret, ref.ns, ref.secret),
					}}
					if label.owned {
						store.Properties.Status.OutputResources[0].RadiusManaged = new(true)
						store.Properties.Status.OutputResources[0].AdditionalProperties = map[string]string{secretUIDProperty: testSecretUID}
					}
					backingClient := k8sutil.NewFakeKubeClient(nil, existingSecret(ref.ns, ref.secret, label.values))
					secretReads, secretDeletes := 0, 0
					client := interceptor.NewClient(backingClient, interceptor.Funcs{
						Get: func(ctx context.Context, client runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
							if _, ok := obj.(*corev1.Secret); ok {
								secretReads++
							}
							return client.Get(ctx, key, obj, opts...)
						},
						Delete: func(ctx context.Context, client runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
							if _, ok := obj.(*corev1.Secret); ok {
								secretDeletes++
							}
							return client.Delete(ctx, obj, opts...)
						},
					})
					opts := authzOptions(mode)
					opts.ControlPlaneNamespace = ref.cpNS
					opts.TargetClusterIsDistinct = true
					ctx, logs := admissionContext(t)
					resp, err := DeleteRadiusSecret(ctx, store, &controller.Options{KubeClient: client, Authorization: opts})
					require.Nil(t, resp)
					requireAdmission(t, err, logs, mode, ref.wantRule)

					denied := mode == authz.ModeEnforce && ref.wantRule != ""
					if denied {
						require.Zero(t, secretReads, "denial must precede reading Secret data")
						require.Zero(t, secretDeletes)
						denial, ok := errors.AsType[*authz.DeniedError](err)
						require.True(t, ok)
						require.Equal(t, ResourceTypeName+"/delete", denial.Action)
					} else if label.owned {
						require.Equal(t, 1, secretReads)
					} else {
						require.Zero(t, secretReads, "unowned references need no Secret access for cleanup")
					}
					stored := &corev1.Secret{}
					getErr := backingClient.Get(ctx, runtimeclient.ObjectKey{Namespace: ref.ns, Name: ref.secret}, stored)
					if !denied && label.owned {
						require.Equal(t, 1, secretDeletes)
						require.True(t, apierrors.IsNotFound(getErr))
					} else {
						require.Zero(t, secretDeletes)
						require.NoError(t, getErr)
						require.Equal(t, []byte(sentinelSecretValue), stored.Data["tls.crt"])
					}
				})
			}
		}
	}
}
