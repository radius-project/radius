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

package v20250801preview

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/test/k8sutil"
)

// The test environment uses namespace "default", so the tests configure it as the control-plane
// namespace.
func TestCreateOrUpdateEnvironment_ControlPlaneNamespace(t *testing.T) {
	tests := []struct {
		name          string
		opts          authz.Options
		wantCode      string
		wantWouldDeny bool
		targetUID     types.UID
		wantError     string
	}{
		{name: "off", opts: authz.Options{ControlPlaneNamespace: "default"}},
		{name: "dryRun", opts: authz.Options{Mode: authz.ModeDryRun, ControlPlaneNamespace: "default"}, wantWouldDeny: true},
		{name: "enforce", opts: authz.Options{Mode: authz.ModeEnforce, ControlPlaneNamespace: "default"}, wantCode: v1.CodeAdmissionPolicyDenied},
		{name: "enforce other namespace", opts: authz.Options{Mode: authz.ModeEnforce, ControlPlaneNamespace: "radius-system"}},
		{name: "off distinct cluster", opts: authz.Options{ControlPlaneNamespace: "default"}, targetUID: "external"},
		{name: "dryRun distinct cluster", opts: authz.Options{Mode: authz.ModeDryRun, ControlPlaneNamespace: "default"}, targetUID: "external"},
		{name: "enforce distinct cluster", opts: authz.Options{Mode: authz.ModeEnforce, ControlPlaneNamespace: "default"}, targetUID: "external"},
		{name: "enforce same-cluster alias", opts: authz.Options{Mode: authz.ModeEnforce, ControlPlaneNamespace: "default"}, targetUID: "control-plane", wantCode: v1.CodeAdmissionPolicyDenied},
		{name: "dryRun same-cluster alias", opts: authz.Options{Mode: authz.ModeDryRun, ControlPlaneNamespace: "default"}, targetUID: "control-plane", wantWouldDeny: true},
		{name: "enforce target identity error", opts: authz.Options{Mode: authz.ModeEnforce, ControlPlaneNamespace: "default"}, targetUID: "unreadable", wantError: "failed to identify deployment-target cluster"},
		{name: "dryRun target identity error", opts: authz.Options{Mode: authz.ModeDryRun, ControlPlaneNamespace: "default"}, targetUID: "unreadable", wantError: "failed to identify deployment-target cluster"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mctrl := gomock.NewController(t)
			databaseClient := database.NewMockClient(mctrl)

			envInput, envDataModel, _ := getTestModelsv20250801preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(t.Context(), http.MethodPut, testHeaderfilev20250801preview, envInput)
			require.NoError(t, err)
			core, logs := observer.New(zap.DebugLevel)
			ctx := logr.NewContext(rpctest.NewARMRequestContext(req), zapr.NewLogger(zap.New(core)))

			databaseClient.EXPECT().Get(gomock.Any(), testRecipePackID).Return(testRecipePackObject(), nil).AnyTimes()
			databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return nil, &database.ErrNotFound{ID: id}
				}).AnyTimes()
			if tt.wantCode == "" && tt.wantError == "" {
				databaseClient.EXPECT().Query(gomock.Any(), gomock.Any()).Return(&database.ObjectQueryResult{}, nil).AnyTimes()
				databaseClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, _ ...database.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = envDataModel
						return nil
					})
			}

			t.Setenv(kubeutil.TargetKubeconfigEnvVar, "")
			kubeClient := k8sutil.NewFakeKubeClient(nil, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
			var controlPlaneClient client.Client = kubeClient
			namespaceClient := kubeClient
			if tt.targetUID != "" {
				require.NoError(t, kubeClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: "control-plane"}}))
				controlPlaneServer := k8sutil.NewNamespaceAPIServer(t, kubeClient)
				controlPlaneClient, err = kubeutil.NewRuntimeClient(&rest.Config{Host: controlPlaneServer.URL})
				require.NoError(t, err)
				if tt.targetUID != "control-plane" {
					namespaceClient = k8sutil.NewFakeKubeClient(nil, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
					if tt.targetUID != "unreadable" {
						require.NoError(t, namespaceClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: tt.targetUID}}))
					}
				}
				targetServer := k8sutil.NewNamespaceAPIServer(t, namespaceClient)
				require.NotEqual(t, controlPlaneServer.URL, targetServer.URL)
				path := filepath.Join(t.TempDir(), "target.kubeconfig")
				k8sutil.WriteKubeconfig(t, path, "target", map[string]string{"target": targetServer.URL})
				t.Setenv(kubeutil.TargetKubeconfigEnvVar, path)
			}
			ctl, err := NewCreateOrUpdateEnvironmentv20250801preview(ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     controlPlaneClient,
				Authorization:  tt.opts,
			})
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			if tt.wantCode != "" {
				denied, ok := errors.AsType[*authz.DeniedError](err)
				require.True(t, ok, "expected a denial, got %v", err)
				require.Equal(t, tt.wantCode, denied.Code)
				require.Equal(t, authz.RuleControlPlaneNamespace, denied.Rule)
				return
			}
			require.NoError(t, err)
			require.NoError(t, resp.Apply(ctx, w, req))
			require.Equal(t, http.StatusOK, w.Result().StatusCode)
			if tt.targetUID == "external" {
				ns := &corev1.Namespace{}
				require.NoError(t, namespaceClient.Get(ctx, client.ObjectKey{Name: "default"}, ns))
				require.Equal(t, tt.opts.PodSecurityLabels("application"), ns.Labels)
				require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Name: "default"}, ns))
				require.Empty(t, ns.Labels, "the control-plane namespace must not be relabeled")
			}

			wouldDeny := logs.FilterField(zap.String(authz.LogFieldRule, authz.RuleControlPlaneNamespace)).FilterField(zap.Bool(authz.LogFieldWouldDeny, true))
			require.Equal(t, tt.wantWouldDeny, wouldDeny.Len() == 1)
		})
	}
}
