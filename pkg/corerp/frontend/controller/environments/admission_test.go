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

package environments

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	}{
		{name: "off", opts: authz.Options{ControlPlaneNamespace: "default"}},
		{name: "dryRun", opts: authz.Options{Mode: authz.ModeDryRun, ControlPlaneNamespace: "default"}, wantWouldDeny: true},
		{name: "enforce", opts: authz.Options{Mode: authz.ModeEnforce, ControlPlaneNamespace: "default"}, wantCode: v1.CodeAdmissionPolicyDenied},
		{name: "enforce other namespace", opts: authz.Options{Mode: authz.ModeEnforce, ControlPlaneNamespace: "radius-system"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mctrl := gomock.NewController(t)
			databaseClient := database.NewMockClient(mctrl)

			envInput, envDataModel, _ := getTestModels20231001preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(t.Context(), http.MethodPut, testHeaderfile, envInput)
			require.NoError(t, err)
			core, logs := observer.New(zap.DebugLevel)
			ctx := logr.NewContext(rpctest.NewARMRequestContext(req), zapr.NewLogger(zap.New(core)))

			databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return nil, &database.ErrNotFound{ID: id}
				})
			if tt.wantCode == "" {
				databaseClient.EXPECT().Query(gomock.Any(), gomock.Any()).Return(&database.ObjectQueryResult{}, nil)
				databaseClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, _ ...database.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = envDataModel
						return nil
					})
			}

			kubeClient := k8sutil.NewFakeKubeClient(nil)
			ctl, err := NewCreateOrUpdateEnvironment(ctrl.Options{DatabaseClient: databaseClient, KubeClient: kubeClient, Authorization: tt.opts})
			require.NoError(t, err)

			_, err = ctl.Run(ctx, w, req)
			nsErr := kubeClient.Get(ctx, client.ObjectKey{Name: "default"}, &corev1.Namespace{})
			if tt.wantCode != "" {
				denied, ok := errors.AsType[*authz.DeniedError](err)
				require.True(t, ok, "expected a denial, got %v", err)
				require.Equal(t, tt.wantCode, denied.Code)
				require.Equal(t, authz.RuleControlPlaneNamespace, denied.Rule)
				require.True(t, apierrors.IsNotFound(nsErr), "the namespace must not be created")
				return
			}
			require.NoError(t, err)
			require.NoError(t, nsErr)

			wouldDeny := logs.FilterField(zap.String(authz.LogFieldRule, authz.RuleControlPlaneNamespace)).FilterField(zap.Bool(authz.LogFieldWouldDeny, true))
			require.Equal(t, tt.wantWouldDeny, wouldDeny.Len() == 1)
		})
	}
}
