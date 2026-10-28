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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/test/rp"
)

const (
	admissionPolicyName = "radius-control-plane-isolation"
	applicationsRPUser  = "system:serviceaccount:" + radiusNamespace + ":applications-rp"
)

// Test_AdmissionPolicy_Rules sends server-side dry-run Pods and Deployments to the API server, mostly as
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
			for _, kind := range []string{"Pod", "Deployment"} {
				t.Run(kind, func(t *testing.T) {
					warnings := &warningRecorder{}
					cfg := rest.CopyConfig(options.K8sConfig)
					cfg.WarningHandler = warnings
					cfg.Impersonate = rest.ImpersonationConfig{UserName: tt.user}
					client, err := k8s.NewForConfig(cfg)
					require.NoError(t, err)

					deployment := admissionTestDeployment(tt.spec, !tt.unlabeled)
					createOptions := metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}
					switch kind {
					case "Pod":
						pod := &corev1.Pod{ObjectMeta: deployment.ObjectMeta, Spec: deployment.Spec.Template.Spec}
						_, err = client.CoreV1().Pods(tt.namespace).Create(ctx, pod, createOptions)
					case "Deployment":
						_, err = client.AppsV1().Deployments(tt.namespace).Create(ctx, deployment, createOptions)
					}
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
		})
	}
}

// Expected: an application workload that references a control-plane Secret is rejected
// by admission with AdmissionPolicyDenied (403).
//
// The container's base manifest mounts a control-plane Secret, and its application deploys
// into the control-plane namespace. Runs only in enforce and uses zero replicas so an
// admission regression cannot start a Pod with access to the Secret.
func Test_Boundary_ControlPlaneSecretReferenceDenied(t *testing.T) {
	options := rp.NewRPTestOptions(t)
	ctx := t.Context()
	if mode := expectedInstalledMode(t); mode != authz.ModeEnforce {
		t.Skipf("requires enforce mode, installed mode is %s", mode)
	}
	skipWithoutAdmissionPolicyAPI(t, options.K8sClient)

	name := fmt.Sprintf("authz-adm-%d", time.Now().UnixNano())
	scope := strings.TrimPrefix(options.Workspace.Scope, "/")
	connection, err := options.Workspace.Connect(ctx)
	require.NoError(t, err)
	clientOptions := sdk.NewClientOptions(connection)
	envClient, err := corerp.NewEnvironmentsClient(&tokencredentials.AnonymousCredential{}, clientOptions)
	require.NoError(t, err)
	appClient, err := corerp.NewApplicationsClient(&tokencredentials.AnonymousCredential{}, clientOptions)
	require.NoError(t, err)
	containerClient, err := corerp.NewContainersClient(&tokencredentials.AnonymousCredential{}, clientOptions)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx := context.Background() //nolint:usetesting // t.Context() is cancelled before cleanup runs.
		if poller, err := containerClient.BeginDelete(ctx, scope, name, nil); err == nil {
			_, _ = poller.PollUntilDone(ctx, nil)
		}
		_, _ = appClient.Delete(ctx, scope, name, nil)
		_, _ = envClient.Delete(ctx, scope, name, nil)
		_ = options.K8sClient.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
		_ = options.K8sClient.CoreV1().ServiceAccounts(radiusNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		// Failed deployments may have no output inventory, so remove partially applied resources explicitly.
		err := options.K8sClient.RbacV1().RoleBindings(radiusNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		assert.True(t, err == nil || apierrors.IsNotFound(err), "delete generated RoleBinding: %v", err)
		err = options.K8sClient.RbacV1().Roles(radiusNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		assert.True(t, err == nil || apierrors.IsNotFound(err), "delete generated Role: %v", err)
		err = options.K8sClient.AppsV1().Deployments(radiusNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		assert.True(t, err == nil || apierrors.IsNotFound(err), "delete generated Deployment: %v", err)
	})

	env, err := envClient.CreateOrUpdate(ctx, scope, name, corerp.EnvironmentResource{
		Location:   new("global"),
		Properties: &corerp.EnvironmentProperties{Compute: &corerp.KubernetesCompute{Kind: new("kubernetes"), Namespace: new(name)}},
	}, nil)
	require.NoError(t, err)
	app, err := appClient.CreateOrUpdate(ctx, scope, name, corerp.ApplicationResource{
		Location: new("global"),
		Properties: &corerp.ApplicationProperties{
			Environment: env.ID,
			Extensions:  []corerp.ExtensionClassification{&corerp.KubernetesNamespaceExtension{Kind: new("kubernetesNamespace"), Namespace: new(radiusNamespace)}},
		},
	}, nil)
	require.NoError(t, err)

	base := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
spec:
  replicas: 0
  template:
    spec:
      volumes:
      - name: control-plane-secret
        secret:
          secretName: database-secret
`, name)
	poller, err := containerClient.BeginCreateOrUpdate(ctx, scope, name, corerp.ContainerResource{
		Location: new("global"),
		Properties: &corerp.ContainerProperties{
			Application: app.ID,
			Container:   &corerp.Container{Image: new("busybox")},
			Runtimes:    &corerp.RuntimesProperties{Kubernetes: &corerp.KubernetesRuntimeProperties{Base: new(base)}},
		},
	}, nil)
	if err == nil {
		_, err = poller.PollUntilDone(ctx, nil)
	}
	require.Error(t, err, "expected admission to reject the container")
	require.Contains(t, err.Error(), "AdmissionPolicyDenied: control-plane-secret")
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
