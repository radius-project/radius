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

package authz

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	radiuscore "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/test/rp"
)

const (
	admissionPolicyName = "radius-control-plane-isolation"
	applicationsRPUser  = "system:serviceaccount:" + radiusNamespace + ":applications-rp"
)

// Test_AdmissionPolicy_Rules sends server-side dry-run Deployments to the API server, mostly as
// the applications-rp identity, and checks which admission rule each one triggers.
func Test_AdmissionPolicy_Rules(t *testing.T) {
	options := rp.NewRPTestOptions(t)
	ctx := t.Context()
	mode := expectedInstalledMode(t)
	skipWithoutAdmissionPolicyAPI(t, options.K8sClient)

	_, err := options.K8sClient.AdmissionregistrationV1().ValidatingAdmissionPolicies().Get(ctx, admissionPolicyName, metav1.GetOptions{})
	if mode == authz.ModeOff {
		require.True(t, apierrors.IsNotFound(err), "expected no admission policy in off mode, got %v", err)
		return
	}
	require.NoError(t, err)

	tests := []struct {
		name      string
		namespace string
		user      string
		unlabeled bool
		spec      corev1.PodSpec
		rule      string
	}{
		{name: "control-plane service account", namespace: radiusNamespace, user: applicationsRPUser, spec: corev1.PodSpec{ServiceAccountName: "ucp"}, rule: "control-plane-service-account"},
		{name: "deprecated serviceAccount field", namespace: radiusNamespace, user: applicationsRPUser, spec: corev1.PodSpec{DeprecatedServiceAccount: "dynamic-rp"}, rule: "control-plane-service-account"},
		{name: "control-plane secret volume", namespace: radiusNamespace, user: applicationsRPUser, spec: secretVolumeSpec("database-secret"), rule: "control-plane-secret"},
		{name: "control-plane secret env", namespace: radiusNamespace, user: applicationsRPUser, spec: secretEnvSpec("radius-encryption-key"), rule: "control-plane-secret"},
		{name: "other workload in control-plane namespace", namespace: radiusNamespace, user: applicationsRPUser, rule: "control-plane-namespace"},
		{name: "application namespace", namespace: "default", user: applicationsRPUser, spec: secretVolumeSpec("database-secret")},
		{name: "object not rendered by Radius", namespace: radiusNamespace, user: applicationsRPUser, unlabeled: true, spec: corev1.PodSpec{ServiceAccountName: "ucp"}},
		{name: "other identity", namespace: radiusNamespace, spec: corev1.PodSpec{ServiceAccountName: "ucp"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := &warningRecorder{}
			cfg := rest.CopyConfig(options.K8sConfig)
			cfg.WarningHandler = warnings
			cfg.Impersonate = rest.ImpersonationConfig{UserName: tt.user}
			client, err := k8s.NewForConfig(cfg)
			require.NoError(t, err)

			deployment := admissionTestDeployment(tt.spec, !tt.unlabeled)
			_, err = client.AppsV1().Deployments(tt.namespace).Create(ctx, deployment, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
			admissionWarnings := strings.Join(warnings.matching("AdmissionPolicyDenied"), "\n")

			switch {
			case tt.rule == "":
				require.NoError(t, err)
				require.Empty(t, admissionWarnings)
			case mode == authz.ModeDryRun:
				require.NoError(t, err, "dryRun must not reject")
				require.Contains(t, admissionWarnings, "AdmissionPolicyDenied: "+tt.rule)
			default:
				require.True(t, apierrors.IsForbidden(err), "expected Forbidden, got %v", err)
				require.Contains(t, err.Error(), "AdmissionPolicyDenied: "+tt.rule)
			}
		})
	}
}

// Expected: an environment or application whose Kubernetes namespace is the Radius control-plane
// namespace is rejected with AdmissionPolicyDenied (403) naming rule control-plane-namespace.
//
// Runs only in enforce. In dryRun the requests succeed and log a would-deny, which the CI
// would-deny check treats as a failure, and in off mode they create resources in the control-plane
// namespace. The applications-rp provider check for control-plane Secret references runs after
// these checks, so it is covered by unit tests and manual testing.
func Test_Boundary_ControlPlaneNamespaceDenied(t *testing.T) {
	options := rp.NewRPTestOptions(t)
	ctx := t.Context()
	if mode := expectedInstalledMode(t); mode != authz.ModeEnforce {
		t.Skipf("requires enforce mode, installed mode is %s", mode)
	}

	name := fmt.Sprintf("authz-adm-%d", time.Now().UnixNano())
	scope := strings.TrimPrefix(options.Workspace.Scope, "/")
	connection, err := options.Workspace.Connect(ctx)
	require.NoError(t, err)
	clientOptions := sdk.NewClientOptions(connection)
	envClient, err := corerp.NewEnvironmentsClient(&tokencredentials.AnonymousCredential{}, clientOptions)
	require.NoError(t, err)
	radiusEnvClient, err := radiuscore.NewEnvironmentsClient(&tokencredentials.AnonymousCredential{}, clientOptions)
	require.NoError(t, err)
	appClient, err := corerp.NewApplicationsClient(&tokencredentials.AnonymousCredential{}, clientOptions)
	require.NoError(t, err)

	// Never delete the control-plane namespace here; only resources this test created.
	t.Cleanup(func() {
		ctx := context.Background() //nolint:usetesting // t.Context() is cancelled before cleanup runs.
		_, _ = appClient.Delete(ctx, scope, name, nil)
		_, _ = envClient.Delete(ctx, scope, name, nil)
		_, _ = envClient.Delete(ctx, scope, name+"-cp", nil)
		_, _ = radiusEnvClient.Delete(ctx, scope, name+"-cp", nil)
		_ = options.K8sClient.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	})

	requireNamespaceDenied := func(t *testing.T, err error) {
		t.Helper()
		var respErr *azcore.ResponseError
		require.ErrorAs(t, err, &respErr)
		require.Equal(t, http.StatusForbidden, respErr.StatusCode)
		require.Equal(t, v1.CodeAdmissionPolicyDenied, respErr.ErrorCode)
		require.Contains(t, err.Error(), "admission rule '"+authz.RuleControlPlaneNamespace+"'")
	}

	t.Run("Applications.Core environment", func(t *testing.T) {
		_, err := envClient.CreateOrUpdate(ctx, scope, name+"-cp", corerp.EnvironmentResource{
			Location:   new("global"),
			Properties: &corerp.EnvironmentProperties{Compute: &corerp.KubernetesCompute{Kind: new("kubernetes"), Namespace: new(radiusNamespace)}},
		}, nil)
		requireNamespaceDenied(t, err)
	})

	t.Run("Radius.Core environment", func(t *testing.T) {
		_, err := radiusEnvClient.CreateOrUpdate(ctx, scope, name+"-cp", radiuscore.EnvironmentResource{
			Location:   new("global"),
			Properties: &radiuscore.EnvironmentProperties{Providers: &radiuscore.Providers{Kubernetes: &radiuscore.ProvidersKubernetes{Namespace: new(radiusNamespace)}}},
		}, nil)
		requireNamespaceDenied(t, err)
	})

	t.Run("application namespace", func(t *testing.T) {
		env, err := envClient.CreateOrUpdate(ctx, scope, name, corerp.EnvironmentResource{
			Location:   new("global"),
			Properties: &corerp.EnvironmentProperties{Compute: &corerp.KubernetesCompute{Kind: new("kubernetes"), Namespace: new(name)}},
		}, nil)
		require.NoError(t, err)
		_, err = appClient.CreateOrUpdate(ctx, scope, name, corerp.ApplicationResource{
			Location: new("global"),
			Properties: &corerp.ApplicationProperties{
				Environment: env.ID,
				Extensions:  []corerp.ExtensionClassification{&corerp.KubernetesNamespaceExtension{Kind: new("kubernetesNamespace"), Namespace: new(radiusNamespace)}},
			},
		}, nil)
		requireNamespaceDenied(t, err)
	})
}

func skipWithoutAdmissionPolicyAPI(t *testing.T, client k8s.Interface) {
	t.Helper()

	resources, err := client.Discovery().ServerResourcesForGroupVersion("admissionregistration.k8s.io/v1")
	require.NoError(t, err)
	for _, resource := range resources.APIResources {
		if resource.Name == "validatingadmissionpolicies" {
			return
		}
	}
	t.Skip("cluster does not serve ValidatingAdmissionPolicy (requires Kubernetes 1.30+)")
}

// admissionTestDeployment returns a Deployment with spec as its pod spec, labeled as rendered by
// applications-rp when labeled is true.
func admissionTestDeployment(spec corev1.PodSpec, labeled bool) *appsv1.Deployment {
	labels := map[string]string{"app": "authz-admission"}
	metadataLabels := map[string]string{}
	if labeled {
		metadataLabels["radapp.io/resource-type"] = "applications.core-containers"
	}
	spec.Containers = []corev1.Container{{Name: "app", Image: "busybox"}}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "authz-admission", Labels: metadataLabels},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: spec},
		},
	}
}

func secretVolumeSpec(secret string) corev1.PodSpec {
	return corev1.PodSpec{Volumes: []corev1.Volume{{
		Name:         "secret",
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: secret}},
	}}}
}

func secretEnvSpec(secret string) corev1.PodSpec {
	return corev1.PodSpec{InitContainers: []corev1.Container{{
		Name:  "init",
		Image: "busybox",
		Env: []corev1.EnvVar{{Name: "VALUE", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secret}, Key: "value"},
		}}},
	}}}
}
