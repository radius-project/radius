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

package applications

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/kubeutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/k8sutil"
)

func TestCreateAppScopedNamespace_ControlPlaneNamespace(t *testing.T) {
	tests := []struct {
		name          string
		mode          authz.Mode
		wantCode      string
		wantWouldDeny bool
		targetUID     types.UID
		wantError     string
	}{
		{name: "off", mode: authz.ModeOff},
		{name: "dryRun", mode: authz.ModeDryRun, wantWouldDeny: true},
		{name: "enforce", mode: authz.ModeEnforce, wantCode: v1.CodeAdmissionPolicyDenied},
		{name: "off distinct cluster", mode: authz.ModeOff, targetUID: "external"},
		{name: "dryRun distinct cluster", mode: authz.ModeDryRun, targetUID: "external"},
		{name: "enforce distinct cluster", mode: authz.ModeEnforce, targetUID: "external"},
		{name: "enforce same-cluster alias", mode: authz.ModeEnforce, targetUID: "control-plane", wantCode: v1.CodeAdmissionPolicyDenied},
		{name: "dryRun same-cluster alias", mode: authz.ModeDryRun, targetUID: "control-plane", wantWouldDeny: true},
		{name: "enforce target identity error", mode: authz.ModeEnforce, targetUID: "unreadable", wantError: "failed to identify deployment-target cluster"},
		{name: "dryRun target identity error", mode: authz.ModeDryRun, targetUID: "unreadable", wantError: "failed to identify deployment-target cluster"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tCtx := rpctest.NewControllerContext(t)
			kubeClient := k8sutil.NewFakeKubeClient(nil)
			var controlPlaneClient client.Client = kubeClient
			t.Setenv(kubeutil.TargetKubeconfigEnvVar, "")
			namespaceClient := kubeClient
			if tt.targetUID != "" {
				require.NoError(t, kubeClient.Create(tCtx.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: "control-plane"}}))
				controlPlaneServer := k8sutil.NewNamespaceAPIServer(t, kubeClient)
				var err error
				controlPlaneClient, err = kubeutil.NewRuntimeClient(&rest.Config{Host: controlPlaneServer.URL})
				require.NoError(t, err)
				if tt.targetUID != "control-plane" {
					namespaceClient = k8sutil.NewFakeKubeClient(nil)
					if tt.targetUID != "unreadable" {
						require.NoError(t, namespaceClient.Create(tCtx.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: tt.targetUID}}))
					}
				}
				targetServer := k8sutil.NewNamespaceAPIServer(t, namespaceClient)
				require.NotEqual(t, controlPlaneServer.URL, targetServer.URL)
				path := filepath.Join(t.TempDir(), "target.kubeconfig")
				k8sutil.WriteKubeconfig(t, path, "target", map[string]string{"target": targetServer.URL})
				t.Setenv(kubeutil.TargetKubeconfigEnvVar, path)
			}
			opts := ctrl.Options{
				DatabaseClient: tCtx.MockSC,
				KubeClient:     controlPlaneClient,
				Authorization:  authz.Options{Mode: tt.mode, ControlPlaneNamespace: "radius-system"},
			}

			envdm := &datamodel.Environment{
				Properties: datamodel.EnvironmentProperties{
					Compute: rpv1.EnvironmentCompute{
						Kind:              rpv1.KubernetesComputeKind,
						KubernetesCompute: rpv1.KubernetesComputeProperties{Namespace: "default"},
					},
				},
			}
			tCtx.MockSC.EXPECT().Get(gomock.Any(), gomock.Any()).Return(rpctest.FakeStoreObject(envdm), nil)
			tCtx.MockSC.EXPECT().Query(gomock.Any(), gomock.Any()).Return(&database.ObjectQueryResult{}, nil).Times(2)

			newResource := &datamodel.Application{
				Properties: datamodel.ApplicationProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{Environment: testEnvID},
					Extensions: []datamodel.Extension{{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "radius-system"},
					}},
				},
			}

			id, err := resources.ParseResource(testAppID)
			require.NoError(t, err)
			core, logs := observer.New(zap.DebugLevel)
			ctx := logr.NewContext(v1.WithARMRequestContext(tCtx.Ctx, &v1.ARMRequestContext{ResourceID: id}), zapr.NewLogger(zap.New(core)))

			resp, err := CreateAppScopedNamespace(ctx, newResource, nil, &opts)
			ns := &corev1.Namespace{}
			nsErr := namespaceClient.Get(ctx, client.ObjectKey{Name: "radius-system"}, ns)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				require.True(t, apierrors.IsNotFound(nsErr))
				return
			}
			if tt.wantCode != "" {
				denied, ok := errors.AsType[*authz.DeniedError](err)
				require.True(t, ok, "expected a denial, got %v", err)
				require.Equal(t, tt.wantCode, denied.Code)
				require.Equal(t, authz.RuleControlPlaneNamespace, denied.Rule)
				require.True(t, apierrors.IsNotFound(nsErr), "the namespace must not be created")
				return
			}
			require.NoError(t, err)
			require.Nil(t, resp)
			require.NoError(t, nsErr)
			if tt.targetUID == "external" {
				require.True(t, apierrors.IsNotFound(kubeClient.Get(ctx, client.ObjectKey{Name: "radius-system"}, &corev1.Namespace{})))
				require.Equal(t, (authz.Options{Mode: tt.mode}).PodSecurityLabels("application"), ns.Labels)
			}

			wouldDeny := logs.FilterField(zap.String(authz.LogFieldRule, authz.RuleControlPlaneNamespace)).FilterField(zap.Bool(authz.LogFieldWouldDeny, true))
			require.Equal(t, tt.wantWouldDeny, wouldDeny.Len() == 1)
		})
	}
}
