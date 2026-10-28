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
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
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
	}{
		{name: "off", mode: authz.ModeOff},
		{name: "dryRun", mode: authz.ModeDryRun, wantWouldDeny: true},
		{name: "enforce", mode: authz.ModeEnforce, wantCode: v1.CodeAdmissionPolicyDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tCtx := rpctest.NewControllerContext(t)
			kubeClient := k8sutil.NewFakeKubeClient(nil)
			opts := ctrl.Options{
				DatabaseClient: tCtx.MockSC,
				KubeClient:     kubeClient,
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
			nsErr := kubeClient.Get(ctx, client.ObjectKey{Name: "radius-system"}, &corev1.Namespace{})
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

			wouldDeny := logs.FilterField(zap.String(authz.LogFieldRule, authz.RuleControlPlaneNamespace)).FilterField(zap.Bool(authz.LogFieldWouldDeny, true))
			require.Equal(t, tt.wantWouldDeny, wouldDeny.Len() == 1)
		})
	}
}
