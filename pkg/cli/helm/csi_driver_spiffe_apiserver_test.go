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
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TestCheckCertManagerDefaultApprover_APIServer checks approver detection against a real RBAC authorizer,
// which a fake clientset cannot evaluate.
func TestCheckCertManagerDefaultApprover_APIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}

	_, client := startTestAPIServer(t)
	ctx := t.Context()
	const namespace = "cert-manager"
	_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	require.NoError(t, err)
	createCertManagerController(t, client, namespace, "cert-manager")
	signer := radiusCAIssuerSignerName(RadiusSystemNamespace)

	tests := []struct {
		name          string
		resourceNames []string
		wantConflict  bool
	}{
		{name: "no approve permission"},
		{name: "approves only cluster issuers", resourceNames: []string{"clusterissuers.cert-manager.io/*"}},
		{name: "approves another namespace's issuers", resourceNames: []string{"issuers.cert-manager.io/other.*"}},
		{name: "chart default approves all issuers", resourceNames: []string{"issuers.cert-manager.io/*", "clusterissuers.cert-manager.io/*"}, wantConflict: true},
		{name: "approves the Radius issuer by name", resourceNames: []string{signer}, wantConflict: true},
		{name: "rad-installed approveSignerNames", resourceNames: certManagerApproveSignerNames(RadiusSystemNamespace)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bindApprover(t, client, namespace, "cert-manager", tt.resourceNames)

			// The RBAC authorizer reads roles from an informer, so wait for the change to apply.
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				err := checkCertManagerDefaultApprover(ctx, client, RadiusSystemNamespace, "test-context")
				if !tt.wantConflict {
					assert.NoError(c, err)
					return
				}
				var conflict *defaultApproverConflictError
				if assert.True(c, errors.As(err, &conflict), "got %v", err) {
					assert.Equal(c, "system:serviceaccount:cert-manager:cert-manager", conflict.User)
					assert.Equal(c, signer, conflict.SignerName)
				}
			}, 10*time.Second, 100*time.Millisecond)
		})
	}

	t.Run("rad-installed approveSignerNames approve the Radius bootstrap and API server issuers", func(t *testing.T) {
		bindApprover(t, client, namespace, "cert-manager", certManagerApproveSignerNames(RadiusSystemNamespace))

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			missing, err := missingApprovableRadiusSigners(ctx, client, RadiusSystemNamespace)
			assert.NoError(c, err)
			assert.Empty(c, missing)
		}, 10*time.Second, 100*time.Millisecond)
	})

	t.Run("chart default approver conflict is fatal on install", func(t *testing.T) {
		bindApprover(t, client, namespace, "cert-manager", []string{"issuers.cert-manager.io/*", "clusterissuers.cert-manager.io/*"})

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			err := checkCertManagerDefaultApprover(ctx, client, RadiusSystemNamespace, "test-context")
			if assert.Error(c, err) {
				assert.Contains(c, err.Error(), "approveSignerNames={")
				assert.Contains(c, err.Error(), "--kube-context")
			}
		}, 10*time.Second, 100*time.Millisecond)
	})

	t.Run("disabled default approver is ignored", func(t *testing.T) {
		bindApprover(t, client, namespace, "cert-manager", []string{"issuers.cert-manager.io/*"})
		deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, "cert-manager", metav1.GetOptions{})
		require.NoError(t, err)
		deployment.Spec.Template.Spec.Containers[0].Args = []string{"--controllers=*,-certificaterequests-approver"}
		_, err = client.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		require.NoError(t, err)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.NoError(c, checkCertManagerDefaultApprover(ctx, client, RadiusSystemNamespace, "test-context"))
		}, 10*time.Second, 100*time.Millisecond)
	})
}

func createCertManagerController(t *testing.T, client kubernetes.Interface, namespace string, serviceAccount string) {
	t.Helper()
	labels := map[string]string{"app.kubernetes.io/name": "cert-manager", "app.kubernetes.io/component": "controller"}
	_, err := client.AppsV1().Deployments(namespace).Create(t.Context(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "cert-manager", Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccount,
					Containers:         []corev1.Container{{Name: "cert-manager-controller", Image: "quay.io/jetstack/cert-manager-controller:v1.21.2"}},
				},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
}

// bindApprover replaces the approver ClusterRole so serviceAccount can approve only resourceNames.
func bindApprover(t *testing.T, client kubernetes.Interface, namespace string, serviceAccount string, resourceNames []string) {
	t.Helper()
	ctx := t.Context()
	const name = "cert-manager-controller-approve"
	err := client.RbacV1().ClusterRoles().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		require.Contains(t, err.Error(), "not found")
	}
	if len(resourceNames) == 0 {
		return
	}
	_, err = client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules: []rbacv1.PolicyRule{{
			APIGroups:     []string{"cert-manager.io"},
			Resources:     []string{"signers"},
			Verbs:         []string{"approve"},
			ResourceNames: resourceNames,
		}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = client.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return
	}
	_, err = client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: serviceAccount, Namespace: namespace}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
}
