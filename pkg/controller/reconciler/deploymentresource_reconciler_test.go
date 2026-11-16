/*
Copyright 2024 The Radius Authors.

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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func Test_DeploymentResourceReconciler_Passive(t *testing.T) {
	t.Parallel()
	for _, legacyToken := range []bool{false, true} {
		t.Run(fmt.Sprint(legacyToken), func(t *testing.T) {
			ctx := t.Context()
			s := runtime.NewScheme()
			require.NoError(t, radappiov1alpha3.AddToScheme(s))
			owner := &radappiov1alpha3.DeploymentTemplate{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "test", UID: "owner"}}
			child := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "tracker", Namespace: "test", Finalizers: []string{DeploymentResourceFinalizer}},
				Spec:       radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/output"},
			}
			require.NoError(t, controllerutil.SetControllerReference(owner, child, s))
			if legacyToken {
				child.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete, ResumeToken: "legacy"}
			}
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(owner, child).WithStatusSubresource(child).Build()
			require.NoError(t, c.Delete(ctx, child))
			r := &DeploymentResourceReconciler{Client: c, EventRecorder: record.NewFakeRecorder(10)}
			for range 3 {
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: k8sClient.ObjectKeyFromObject(child)})
				require.NoError(t, err)
			}
			require.NoError(t, c.Get(ctx, k8sClient.ObjectKeyFromObject(child), child))
			require.Contains(t, child.Finalizers, DeploymentResourceFinalizer)
			require.Equal(t, legacyToken, child.Status.Operation != nil)
			require.NoError(t, c.Delete(ctx, owner))
			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: k8sClient.ObjectKeyFromObject(child)})
			require.NoError(t, err)
			require.True(t, apierrors.IsNotFound(c.Get(ctx, k8sClient.ObjectKeyFromObject(child), child)))
		})
	}
}

func Test_DeploymentResourceReconciler_Provenance(t *testing.T) {
	t.Parallel()
	const outputID = "/planes/kubernetes/local/namespaces/workload/providers/core/ConfigMap/output"
	for _, tc := range []struct {
		name, recorded, inventory, spec string
		stale, missing, nonController   bool
		want                            bool
	}{
		{name: "inventory", inventory: outputID, want: true},
		{name: "recorded", recorded: outputID, want: true},
		{name: "annotation-only"},
		{name: "changed-spec", recorded: outputID, spec: "/planes/kubernetes/local/namespaces/workload/providers/core/ConfigMap/other"},
		{name: "mismatch-with-inventory", recorded: "different", inventory: outputID},
		{name: "stale-owner", recorded: outputID, stale: true},
		{name: "missing-owner", recorded: outputID, missing: true},
		{name: "noncontroller", recorded: outputID, nonController: true},
		{name: "remote", spec: "/planes/kubernetes/remote/namespaces/workload/providers/core/ConfigMap/output", inventory: "/planes/kubernetes/remote/namespaces/workload/providers/core/ConfigMap/output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := runtime.NewScheme()
			require.NoError(t, radappiov1alpha3.AddToScheme(s))
			owner := &radappiov1alpha3.DeploymentTemplate{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "test", UID: "owner"}}
			if tc.inventory != "" {
				owner.Status.OutputResources = []string{tc.inventory}
			}
			id := tc.spec
			if id == "" {
				id = outputID
			}
			child := &radappiov1alpha3.DeploymentResource{ObjectMeta: metav1.ObjectMeta{Name: "tracker", Namespace: "test", Annotations: map[string]string{"radapp.io/deployment-output-id": outputID}}, Spec: radappiov1alpha3.DeploymentResourceSpec{Id: id}, Status: radappiov1alpha3.DeploymentResourceStatus{ProvisionedResourceID: tc.recorded}}
			require.NoError(t, controllerutil.SetControllerReference(owner, child, s))
			if tc.stale {
				child.OwnerReferences[0].UID = "stale"
			}
			if tc.nonController {
				child.OwnerReferences[0].Controller = new(false)
			}
			builder := fake.NewClientBuilder().WithScheme(s).WithObjects(child)
			if !tc.missing {
				builder.WithObjects(owner)
			}
			r := &DeploymentResourceReconciler{Client: builder.Build()}
			got, err := r.deleteIsEligible(t.Context(), child, metav1.GetControllerOf(child))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func Test_resourceWithinScope(t *testing.T) {
	for _, tc := range []struct {
		id, scope string
		want      bool
	}{
		{"/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/web", "/planes/radius/local/resourceGroups/test", true},
		{"/planes/radius/local/resourceGroups/Test/providers/Applications.Core/containers/web", "/planes/radius/local/resourceGroups/test/", true},
		{"/planes/radius/local/resourceGroups/other/providers/Applications.Core/containers/web", "/planes/radius/local/resourceGroups/test", false},
	} {
		got, err := resourceWithinScope(tc.id, tc.scope)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
	_, err := resourceWithinScope("invalid", "/planes/radius/local/resourceGroups/test")
	require.Error(t, err)
}

type partialOutputDiscovery struct {
	discovery.DiscoveryInterface
	resources []*metav1.APIResourceList
	err       error
}

func (d *partialOutputDiscovery) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	return nil, d.resources, d.err
}

func Test_KubernetesOutputDiscovery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, failedGroup         string
		served, global, wantError bool
	}{
		{name: "removed"},
		{name: "served", served: true},
		{name: "unrelated", failedGroup: "metrics.k8s.io", served: true},
		{name: "target", failedGroup: "example.com", served: true, wantError: true},
		{name: "global", global: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &partialOutputDiscovery{}
			if tc.served {
				d.resources = []*metav1.APIResourceList{{GroupVersion: "example.com/v2", APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Namespaced: true}}}}
			}
			if tc.failedGroup != "" {
				d.err = &discovery.ErrGroupDiscoveryFailed{Groups: map[schema.GroupVersion]error{{Group: tc.failedGroup, Version: "v1"}: errors.New("unavailable")}}
			}
			if tc.global {
				d.err = errors.New("discovery unavailable")
			}
			r := &DeploymentResourceReconciler{Discovery: d}
			mapping, err := r.freshKubernetesMapping("example.com", "Widget")
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.served {
				require.Equal(t, "v2", mapping.GroupVersionKind.Version)
			} else {
				require.Nil(t, mapping)
			}
		})
	}
}

func Test_KubernetesOutputDeletion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, kind, namespace   string
		finalizer, ownNamespace bool
	}{
		{name: "configmap", kind: "ConfigMap", namespace: "test"},
		{name: "configmap-finalizer", kind: "ConfigMap", namespace: "test", finalizer: true},
		{name: "job", kind: "Job", namespace: "test"},
		{name: "own-namespace", kind: "Namespace", ownNamespace: true, finalizer: true},
		{name: "other-namespace", kind: "Namespace", finalizer: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			s := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(s))
			require.NoError(t, batchv1.AddToScheme(s))
			group, version := "", "v1"
			provider := "core"
			if tc.kind == "Job" {
				group, provider = "batch", "batch"
			}
			gv := schema.GroupVersion{Group: group, Version: version}
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{gv})
			scope := meta.RESTScopeNamespace
			if tc.kind == "Namespace" {
				scope = meta.RESTScopeRoot
			}
			mapper.Add(gv.WithKind(tc.kind), scope)
			output := &unstructured.Unstructured{}
			output.SetGroupVersionKind(gv.WithKind(tc.kind))
			output.SetName("output")
			output.SetNamespace(tc.namespace)
			if tc.finalizer {
				output.SetFinalizers([]string{"example.com/hold"})
			}
			c := fake.NewClientBuilder().WithScheme(s).WithRESTMapper(mapper).WithObjects(output).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c k8sClient.WithWatch, key k8sClient.ObjectKey, obj k8sClient.Object, opts ...k8sClient.GetOption) error {
					if _, ok := obj.(*unstructured.Unstructured); ok && tc.kind != "Namespace" {
						return apierrors.NewForbidden(schema.GroupResource{Resource: "outputs"}, key.Name, errors.New("no generic read grant"))
					}
					return c.Get(ctx, key, obj, opts...)
				},
				Delete: func(ctx context.Context, c k8sClient.WithWatch, obj k8sClient.Object, opts ...k8sClient.DeleteOption) error {
					options := (&k8sClient.DeleteOptions{}).ApplyOptions(opts)
					require.NotNil(t, options.PropagationPolicy)
					require.Equal(t, metav1.DeletePropagationBackground, *options.PropagationPolicy)
					return c.Delete(ctx, obj, opts...)
				},
			}).Build()
			idString := "/planes/kubernetes/local"
			if tc.namespace != "" {
				idString += "/namespaces/" + tc.namespace
			}
			idString += "/providers/" + provider + "/" + tc.kind + "/output"
			id, err := resources.ParseResource(idString)
			require.NoError(t, err)
			trackerNamespace := "test"
			if tc.ownNamespace {
				trackerNamespace = "output"
			}
			r := &DeploymentResourceReconciler{Client: c}
			done, err := r.deleteKubernetesOutput(ctx, trackerNamespace, id)
			require.NoError(t, err)
			require.Equal(t, tc.ownNamespace, done)
			if !tc.finalizer {
				done, err = r.deleteKubernetesOutput(ctx, trackerNamespace, id)
				require.NoError(t, err)
				require.True(t, done)
			}
		})
	}
}

func Test_KubernetesOutputStaleVersion(t *testing.T) {
	t.Parallel()
	for _, discoveryFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(discoveryFailure), func(t *testing.T) {
			s := runtime.NewScheme()
			oldVersion := schema.GroupVersion{Group: "example.com", Version: "v1"}
			newVersion := schema.GroupVersion{Group: "example.com", Version: "v2"}
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{oldVersion, newVersion})
			mapper.Add(oldVersion.WithKind("Widget"), meta.RESTScopeNamespace)
			mapper.Add(newVersion.WithKind("Widget"), meta.RESTScopeNamespace)
			output := &unstructured.Unstructured{}
			output.SetGroupVersionKind(newVersion.WithKind("Widget"))
			output.SetName("output")
			output.SetNamespace("test")
			output.SetFinalizers([]string{"example.com/hold"})
			c := fake.NewClientBuilder().WithScheme(s).WithRESTMapper(mapper).WithObjects(output).WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, c k8sClient.WithWatch, obj k8sClient.Object, opts ...k8sClient.DeleteOption) error {
					if obj.GetObjectKind().GroupVersionKind() == oldVersion.WithKind("Widget") {
						return apierrors.NewNotFound(oldVersion.WithResource("widgets").GroupResource(), obj.GetName())
					}
					return c.Delete(ctx, obj, opts...)
				},
			}).Build()
			d := &partialOutputDiscovery{resources: []*metav1.APIResourceList{{GroupVersion: newVersion.String(), APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Namespaced: true}}}}}
			if discoveryFailure {
				d.err = errors.New("discovery unavailable")
			}
			outputClient := dynamicfake.NewSimpleDynamicClient(s)
			outputClient.PrependReactor("delete", "widgets", func(action k8stesting.Action) (bool, runtime.Object, error) {
				obj := &unstructured.Unstructured{}
				obj.SetGroupVersionKind(action.GetResource().GroupVersion().WithKind("Widget"))
				obj.SetName(action.(k8stesting.DeleteAction).GetName())
				obj.SetNamespace(action.GetNamespace())
				return true, nil, c.Delete(t.Context(), obj)
			})
			r := &DeploymentResourceReconciler{Client: c, Discovery: d, KubernetesClient: outputClient}
			id, err := resources.ParseResource("/planes/kubernetes/local/namespaces/test/providers/example.com/Widget/output")
			require.NoError(t, err)
			done, err := r.deleteKubernetesOutput(t.Context(), "test", id)
			if discoveryFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.False(t, done)
			require.NoError(t, c.Get(t.Context(), k8sClient.ObjectKeyFromObject(output), output))
			require.NotNil(t, output.GetDeletionTimestamp())
			output.SetFinalizers(nil)
			require.NoError(t, c.Update(t.Context(), output))
			done, err = r.deleteKubernetesOutput(t.Context(), "test", id)
			require.NoError(t, err)
			require.True(t, done)
		})
	}
}

func Test_KubernetesOutputRefreshedPlural(t *testing.T) {
	t.Parallel()
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, http.MethodDelete, req.Method)
		paths = append(paths, req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path == "/apis/example.com/v1/namespaces/test/gadgets/output" {
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Success","code":200}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
	}))
	defer ts.Close()
	outputClient, err := dynamic.NewForConfig(&rest.Config{Host: ts.URL})
	require.NoError(t, err)
	gv := schema.GroupVersion{Group: "example.com", Version: "v1"}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{gv})
	mapper.AddSpecific(gv.WithKind("Widget"), gv.WithResource("widgets"), gv.WithResource("widget"), meta.RESTScopeNamespace)
	c := fake.NewClientBuilder().WithRESTMapper(mapper).Build()
	d := &partialOutputDiscovery{resources: []*metav1.APIResourceList{{
		GroupVersion: gv.String(), APIResources: []metav1.APIResource{{Name: "gadgets", Kind: "Widget", Namespaced: true}},
	}}}
	r := &DeploymentResourceReconciler{Client: c, Discovery: d, KubernetesClient: outputClient}
	id, err := resources.ParseResource("/planes/kubernetes/local/namespaces/test/providers/example.com/Widget/output")
	require.NoError(t, err)
	done, err := r.deleteKubernetesOutput(t.Context(), "test", id)
	require.NoError(t, err)
	require.False(t, done, "accepted DELETE must retain inventory until disappearance is confirmed")
	require.Equal(t, []string{
		"/apis/example.com/v1/namespaces/test/widgets/output",
		"/apis/example.com/v1/namespaces/test/gadgets/output",
	}, paths)
}

func Test_DeploymentResource_OutputIdentityStatusSubresource(t *testing.T) {
	SkipWithoutEnvironment(t)
	ctx := t.Context()
	c, err := k8sClient.New(config, k8sClient.Options{Scheme: scheme})
	require.NoError(t, err)
	namespace := "protected-output-identity"
	require.NoError(t, c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}))
	require.NoError(t, c.Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "resource-editor", Namespace: namespace},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{"radapp.io"}, Resources: []string{"deploymentresources", "deploymenttemplates"}, Verbs: []string{"get", "create", "update", "delete"}}},
	}))
	require.NoError(t, c.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "resource-editor", Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "resource-editor"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "deployment-resource-editor"}},
	}))
	callerConfig := rest.CopyConfig(config)
	callerConfig.Impersonate.UserName = "deployment-resource-editor"
	caller, err := k8sClient.New(callerConfig, k8sClient.Options{Scheme: scheme})
	require.NoError(t, err)
	child := &radappiov1alpha3.DeploymentResource{ObjectMeta: metav1.ObjectMeta{Name: "forged", Namespace: namespace}, Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "untrusted"}, Status: radappiov1alpha3.DeploymentResourceStatus{ProvisionedResourceID: "forged"}}
	require.Eventually(t, func() bool { return caller.Create(ctx, child) == nil }, time.Second*10, time.Millisecond*100)
	require.Empty(t, child.Status.ProvisionedResourceID)
	child.Status.ProvisionedResourceID = "forged-update"
	require.NoError(t, caller.Update(ctx, child))
	require.Empty(t, child.Status.ProvisionedResourceID)
	child.Status.ProvisionedResourceID = "forged-status"
	require.True(t, apierrors.IsForbidden(caller.Status().Update(ctx, child)))
	child.Status.ProvisionedResourceID = "controller-recorded"
	require.NoError(t, c.Status().Update(ctx, child))
	child.Status.ProvisionedResourceID = "forged-update"
	require.NoError(t, caller.Update(ctx, child))
	require.Equal(t, "controller-recorded", child.Status.ProvisionedResourceID)
	template := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "forged-coordinator", Namespace: namespace},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", ProviderConfig: "{}"},
		Status: radappiov1alpha3.DeploymentTemplateStatus{Coordinator: &radappiov1alpha3.DeploymentCoordinatorStatus{
			Phase: coordinatorDeleteRunning, ResourceID: "forged",
		}},
	}
	require.NoError(t, caller.Create(ctx, template))
	require.Nil(t, template.Status.Coordinator, "normal create must not persist deletion authority")
	template.Status.Coordinator = &radappiov1alpha3.DeploymentCoordinatorStatus{Phase: coordinatorDeleteRunning, ResourceID: "forged"}
	require.NoError(t, caller.Update(ctx, template))
	require.Nil(t, template.Status.Coordinator)
	template.Status.Coordinator = &radappiov1alpha3.DeploymentCoordinatorStatus{Phase: coordinatorIdle}
	require.True(t, apierrors.IsForbidden(caller.Status().Update(ctx, template)), "resource access must not confer coordinator access")
	require.NoError(t, c.Status().Update(ctx, template))
	template.Status.Coordinator.Phase = coordinatorDeleteRunning
	require.NoError(t, caller.Update(ctx, template))
	require.Equal(t, coordinatorIdle, template.Status.Coordinator.Phase)
}
