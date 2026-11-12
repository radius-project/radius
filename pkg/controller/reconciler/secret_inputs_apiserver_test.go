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

package reconciler

import (
	"fmt"
	"testing"
	"time"

	radapp "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestSecretInputsAPIServerAuthorization(t *testing.T) {
	SkipWithoutEnvironment(t)
	ctx := t.Context()
	admin, err := client.New(config, client.Options{Scheme: scheme})
	require.NoError(t, err)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "secret-inputs-"}}
	require.NoError(t, admin.Create(ctx, namespace))
	deployment := referenceDeployment(t)
	deployment.Namespace = namespace.Name
	username := "controller-" + namespace.Name
	user, err := secretInputTestEnvironment.AddUser(envtest.User{Name: username}, config)
	require.NoError(t, err)
	resolver, err := NewSecretInputResolver(user.Config())
	require.NoError(t, err)
	controller, err := client.New(user.Config(), client.Options{Scheme: scheme})
	require.NoError(t, err)
	account := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: secretReaderAccount, Namespace: namespace.Name}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: namespace.Name}, Data: map[string][]byte{"password": []byte("resolved-secret-marker")}}
	require.NoError(t, admin.Create(ctx, secret))
	// Grant only account existence checks and impersonation, never controller Secret access.
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "controller-inputs", Namespace: namespace.Name},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""}, Resources: []string{"serviceaccounts"},
			ResourceNames: []string{secretReaderAccount}, Verbs: []string{"get", "impersonate"},
		}},
	}
	require.NoError(t, admin.Create(ctx, role))
	require.NoError(t, admin.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: role.Name, Namespace: namespace.Name},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: username, APIGroup: rbacv1.GroupName}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: role.Name, APIGroup: rbacv1.GroupName},
	}))
	_, _, err = resolver.Resolve(ctx, deployment)
	require.ErrorContains(t, err, "SecretInputUnavailable")
	require.NoError(t, admin.Create(ctx, account))
	_, _, err = resolver.Resolve(ctx, deployment)
	require.ErrorContains(t, err, "SecretInputUnavailable")
	require.NotContains(t, err.Error(), "resolved-secret-marker")

	readerRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "approved-inputs", Namespace: namespace.Name},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""}, Resources: []string{"secrets"},
			ResourceNames: []string{"credentials"}, Verbs: []string{"get"},
		}},
	}
	require.NoError(t, admin.Create(ctx, readerRole))
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: readerRole.Name, Namespace: namespace.Name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: secretReaderAccount, Namespace: namespace.Name}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: readerRole.Name, APIGroup: rbacv1.GroupName},
	}
	require.NoError(t, admin.Create(ctx, binding))
	var revision string
	require.Eventually(t, func() bool {
		values, resolvedRevision, resolveErr := resolver.Resolve(ctx, deployment)
		if resolveErr != nil {
			return false
		}
		require.Equal(t, "resolved-secret-marker", values["password"]["value"])
		revision = resolvedRevision
		return true
	}, 10*time.Second, 100*time.Millisecond)
	require.True(t, apierrors.IsForbidden(controller.Get(ctx, client.ObjectKeyFromObject(secret), &corev1.Secret{})))
	// Even the approved identity cannot list or watch Secrets, or get unapproved objects.
	impersonated := *user.Config()
	impersonated.Impersonate.UserName = fmt.Sprintf("system:serviceaccount:%s:%s", namespace.Name, secretReaderAccount)
	reader, err := kubernetes.NewForConfig(&impersonated)
	require.NoError(t, err)
	_, err = reader.CoreV1().Secrets(namespace.Name).List(ctx, metav1.ListOptions{})
	require.True(t, apierrors.IsForbidden(err))
	_, err = reader.CoreV1().Secrets(namespace.Name).Watch(ctx, metav1.ListOptions{})
	require.True(t, apierrors.IsForbidden(err))
	_, err = reader.CoreV1().Secrets(namespace.Name).Get(ctx, "unapproved", metav1.GetOptions{})
	require.True(t, apierrors.IsForbidden(err))
	// Rotation, including recreation with a different UID, changes only metadata revision.
	require.NoError(t, admin.Delete(ctx, secret))
	secret.ResourceVersion = ""
	secret.UID = ""
	require.NoError(t, admin.Create(ctx, secret))
	_, recreatedRevision, err := resolver.Resolve(ctx, deployment)
	require.NoError(t, err)
	require.NotEqual(t, revision, recreatedRevision)
	require.NoError(t, admin.Delete(ctx, binding))
	require.Eventually(t, func() bool {
		_, _, err := resolver.Resolve(ctx, deployment)
		return err != nil
	}, 10*time.Second, 100*time.Millisecond)
	binding.ResourceVersion = ""
	binding.UID = ""
	require.NoError(t, admin.Create(ctx, binding))
	require.Eventually(t, func() bool {
		_, _, err := resolver.Resolve(ctx, deployment)
		return err == nil
	}, 10*time.Second, 100*time.Millisecond)
	role.Rules[0].Verbs = []string{"get"}
	require.NoError(t, admin.Update(ctx, role))
	require.Eventually(t, func() bool {
		_, _, err := resolver.Resolve(ctx, deployment)
		return err != nil
	}, 10*time.Second, 100*time.Millisecond)

	// Forbidden namespace fields must not be silently pruned by the API server.
	deployment.Name = "invalid-reference"
	for _, otherNamespace := range []string{"other", ""} {
		deployment.Spec.ParameterReferences["password"] = radapp.ParameterReference{
			SecretKeyRef: radapp.SecretKeyReference{Name: "credentials", Key: "password", Namespace: &otherNamespace},
		}
		err = admin.Create(ctx, deployment)
		require.True(t, apierrors.IsInvalid(err), "expected schema validation error, got %v", err)
	}
}
