// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserverstore

import (
	"path/filepath"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/store"
	ucpv1alpha1 "github.com/project-radius/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/project-radius/radius/pkg/ucp/util/etag"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/test/ucp/kubeenv"
	shared "github.com/project-radius/radius/test/ucp/storetest"
)

//
func Test_APIServer_Client(t *testing.T) {
	// The APIServer tests require installation of the Kubernetes test environment binaries.
	// Our Makefile knows how to download the the amd64 version of these on MacOS.
	rc, env, err := kubeenv.StartEnvironment([]string{filepath.Join("..", "..", "..", "..", "deploy", "Chart", "crds", "ucpd")})

	require.NoError(t, err, "If this step is failing for you, run `make test` inside the repository and try again. If you are still stuck then ask for help.")
	defer func() {
		_ = env.Stop()
	}()

	ctx, cancel := testcontext.New(t)
	defer cancel()

	ns := "radius-test"
	err = kubeenv.EnsureNamespace(ctx, rc, ns)
	require.NoError(t, err)

	client := NewAPIServerClient(rc, ns)
	require.NotNil(t, client)

	clear := func(t *testing.T) {
		err := client.client.DeleteAllOf(ctx, &ucpv1alpha1.Resource{}, runtimeclient.InNamespace(ns))
		require.NoError(t, err)
	}

	// The actual test logic lives in a shared package, we're just doing the setup here.
	shared.RunTest(t, client, clear)

	// The APIServer implementation is complex enough that we have some of our tests in addition
	// to the standard suite.

	t.Run("save_resource_and_validate_kubernetes_object", func(t *testing.T) {
		clear(t)

		obj1 := store.Object{
			Metadata: store.Metadata{
				ID: shared.Resource1ID.String(),
			},
			Data: shared.Data1,
		}
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		// Now let's look at the kubernetes object.
		resourceName := resourceName(shared.Resource1ID)

		resource := ucpv1alpha1.Resource{}
		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expected := map[string]string{
			"ucp.dev/kind":                 "resource",
			"ucp.dev/resource-type":        "system.resources_resourcetype1",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "group1",
		}

		require.Equal(t, expected, resource.Labels)
	})

	t.Run("save_resource_and_validate_kubernetes_object_uppercase_name", func(t *testing.T) {
		clear(t)

		obj1 := store.Object{
			Metadata: store.Metadata{
				ID: shared.Resource3ID.String(),
			},
			Data: shared.Data1,
		}
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		// Now let's look at the kubernetes object.
		resourceName := resourceName(shared.Resource3ID)

		resource := ucpv1alpha1.Resource{}
		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expected := map[string]string{
			"ucp.dev/kind":                 "resource",
			"ucp.dev/resource-type":        "system.resources_resourcetype2",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "group2",
		}

		require.Equal(t, expected, resource.Labels)
	})

	t.Run("save_scope_and_validate_kubernetes_object", func(t *testing.T) {
		clear(t)

		obj1 := store.Object{
			Metadata: store.Metadata{
				ID: shared.ResourceGroup1ID.String(),
			},
			Data: shared.ResourceGroup1Data,
		}
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		// Now let's look at the kubernetes object.
		resourceName := resourceName(shared.ResourceGroup1ID)

		resource := ucpv1alpha1.Resource{}
		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expected := map[string]string{
			"ucp.dev/kind":          "scope",
			"ucp.dev/resource-type": "resourcegroups",
			"ucp.dev/scope-radius":  "local",
		}

		require.Equal(t, expected, resource.Labels)
	})

	t.Run("save_and_validate_kubernetes_object_with_collision", func(t *testing.T) {
		clear(t)

		// In this test we're going to **similuate** a hash collision and verify that it is saved correctly.
		//
		// Let's PRETEND that shared.BasicResource1ID and shared.BasicResource2ID result in the same
		// resource name. That's obviously not the case, but it's good enough for tests.
		resource := ucpv1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName(shared.Resource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.Resource2ID.String(),
					ETag: etag.New(shared.MarshalOrPanic(shared.Data2)),
					Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data2)},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		obj1 := store.Object{
			Metadata: store.Metadata{
				ID: shared.Resource1ID.String(),
			},
			Data: shared.Data1,
		}
		err = client.Save(ctx, &obj1)
		require.NoError(t, err)

		// Now let's look at the kubernetes object.
		resourceName := resourceName(shared.Resource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expectedLabels := map[string]string{
			"ucp.dev/kind":                 "resource",
			"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "m_u_l_t_i_p_l_e",
		}
		require.Equal(t, expectedLabels, resource.Labels)

		expectedEntries := []ucpv1alpha1.ResourceEntry{
			{
				ID:   shared.Resource2ID.String(),
				ETag: etag.New(shared.MarshalOrPanic(shared.Data2)),
				Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data2)},
			},
			{
				ID:   shared.Resource1ID.String(),
				ETag: etag.New(shared.MarshalOrPanic(shared.Data1)),
				Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)},
			},
		}
		require.Equal(t, expectedEntries, resource.Entries)

		// Now we should be able to get resource 1 directly. We can't get resource 2 directly because we stored it
		// with the wrong name on purpose.
		obj, err := client.Get(ctx, shared.Resource1ID.String())
		require.NoError(t, err)
		require.Equal(t, shared.Resource1ID.String(), obj.ID)
		require.Equal(t, shared.Data1, obj.Data)

		// We can query it though...
		objs, err := client.Query(ctx, store.Query{RootScope: shared.RadiusScope, ScopeRecursive: true})
		require.NoError(t, err)
		expected := []store.Object{
			*obj,
			{
				Metadata: store.Metadata{
					ID:   shared.Resource2ID.String(),
					ETag: etag.New(shared.MarshalOrPanic(shared.Data2)),
				},
				Data: shared.Data2,
			},
		}
		shared.CompareObjectLists(t, expected, objs.Items)
	})

	t.Run("save_with_create_conflict", func(t *testing.T) {
		clear(t)

		// Setup to control the client precisely.
		readyChan := make(chan struct{})
		waitChan := make(chan struct{})

		errChan := make(chan error)

		client := NewAPIServerClient(rc, ns)
		client.readyChan = readyChan
		client.waitChan = waitChan

		// In this test we're going to simulate a conflict caused by concurrent creation of a resource.
		//
		// We'll also pretend that we've encountered a hash collision to make this possible.

		// Start an operation to "save" resource 1
		go func() {
			obj1 := store.Object{
				Metadata: store.Metadata{
					ID: shared.Resource1ID.String(),
				},
				Data: shared.Data1,
			}
			err = client.Save(ctx, &obj1)
			errChan <- err
		}()

		// Wait until the client is "ready" to perform a save. Now we'll cause the conflict by the Kubernetes object
		// out of back from the call to Save().
		<-readyChan

		resource := ucpv1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName(shared.Resource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.Resource2ID.String(),
					ETag: etag.New(shared.MarshalOrPanic(shared.Data2)),
					Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data2)},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		// Now we've created the object, so we can let the "save" proceed and find out if it was successful.
		waitChan <- struct{}{}

		// NOTE: we need to cycle readyChan and waitChan again because of the retry logic.
		<-readyChan
		waitChan <- struct{}{}

		err = <-errChan
		require.NoError(t, err, "concurrent save of resource1 failed")

		// Now let's look at the kubernetes object to make sure it wasn't corrupted.
		resourceName := resourceName(shared.Resource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expectedLabels := map[string]string{
			"ucp.dev/kind":                 "resource",
			"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "m_u_l_t_i_p_l_e",
		}
		require.Equal(t, expectedLabels, resource.Labels)

		expectedEntries := []ucpv1alpha1.ResourceEntry{
			{
				ID:   shared.Resource2ID.String(),
				ETag: etag.New(shared.MarshalOrPanic(shared.Data2)),
				Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data2)},
			},
			{
				ID:   shared.Resource1ID.String(),
				ETag: etag.New(shared.MarshalOrPanic(shared.Data1)),
				Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)},
			},
		}
		require.Equal(t, expectedEntries, resource.Entries)
	})

	t.Run("save_with_update_conflict", func(t *testing.T) {
		clear(t)

		// Setup to control the client precisely.
		readyChan := make(chan struct{})
		waitChan := make(chan struct{})

		errChan := make(chan error)

		client := NewAPIServerClient(rc, ns)
		client.readyChan = readyChan
		client.waitChan = waitChan

		// In this test we're going to simulate a conflict caused by concurrent update of a resource.
		//
		// We'll also pretend that we've encountered a hash collision to make this possible.

		// First we create the resource
		resource := ucpv1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName(shared.Resource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.Resource2ID.String(),
					ETag: etag.New(shared.MarshalOrPanic(shared.Data2)),
					Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data2)},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		// Start an operation to "save" resource 1
		go func() {
			obj1 := store.Object{
				Metadata: store.Metadata{
					ID: shared.Resource1ID.String(),
				},
				Data: shared.Data1,
			}
			err = client.Save(ctx, &obj1)
			errChan <- err
		}()

		// Wait until the client is "ready" to perform a save. Now we'll cause the conflict by the Kubernetes object
		// out of back from the call to Save().
		<-readyChan

		resource.Entries[0].Data = &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)}
		resource.Entries[0].ETag = etag.New(shared.MarshalOrPanic(shared.Data1))
		err = rc.Update(ctx, &resource)
		require.NoError(t, err)

		// Now we've updated the object, so we can let the "save" proceed and find out if it was successful.
		waitChan <- struct{}{}

		// NOTE: we need to cycle readyChan and waitChan again because of the retry logic.
		<-readyChan
		waitChan <- struct{}{}

		err = <-errChan
		require.NoError(t, err, "concurrent save of resource1 failed")

		// Now let's look at the kubernetes object to make sure it wasn't corrupted.
		resourceName := resourceName(shared.Resource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expectedLabels := map[string]string{
			"ucp.dev/kind":                 "resource",
			"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "m_u_l_t_i_p_l_e",
		}
		require.Equal(t, expectedLabels, resource.Labels)

		expectedEntries := []ucpv1alpha1.ResourceEntry{
			{
				ID:   shared.Resource2ID.String(),
				ETag: etag.New(shared.MarshalOrPanic(shared.Data1)),
				Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)},
			},
			{
				ID:   shared.Resource1ID.String(),
				ETag: etag.New(shared.MarshalOrPanic(shared.Data1)),
				Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)},
			},
		}
		require.Equal(t, expectedEntries, resource.Entries)
	})

	t.Run("delete_with_update_conflict", func(t *testing.T) {
		clear(t)

		// Setup to control the client precisely.
		readyChan := make(chan struct{})
		waitChan := make(chan struct{})

		errChan := make(chan error)

		client := NewAPIServerClient(rc, ns)
		client.readyChan = readyChan
		client.waitChan = waitChan

		// In this test we're going to simulate a conflict caused by concurrent update of a resource.
		//
		// We'll also pretend that we've encountered a hash collision to make this possible.

		// First we create the resource
		resource := ucpv1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName(shared.Resource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.Resource1ID.String(),
					ETag: etag.New(shared.MarshalOrPanic(shared.Data1)),
					Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)},
				},
				{
					ID:   shared.Resource2ID.String(),
					ETag: etag.New(shared.MarshalOrPanic(shared.Data1)),
					Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data2)},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		// Start an operation to "delete" resource 1
		go func() {
			err = client.Delete(ctx, shared.Resource1ID.String())
			errChan <- err
		}()

		// Wait until the client is "ready" to perform a delete. Now we'll cause the conflict by the Kubernetes object
		// out of back from the call to Delete().
		<-readyChan

		resource.Entries[1].Data = &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)}
		err = rc.Update(ctx, &resource)
		require.NoError(t, err)

		// Now we've created the object, so we can let the "delete" proceed and find out if it was successful.
		waitChan <- struct{}{}

		// NOTE: we need to cycle readyChan and waitChan again because of the retry logic.
		<-readyChan
		waitChan <- struct{}{}

		err = <-errChan
		require.NoError(t, err, "concurrent delete of resource1 failed")

		// Now let's look at the kubernetes object to make sure it wasn't corrupted.
		resourceName := resourceName(shared.Resource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expectedLabels := map[string]string{
			"ucp.dev/kind":                 "resource",
			"ucp.dev/resource-type":        "system.resources_resourcetype2",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "group2",
		}
		require.Equal(t, expectedLabels, resource.Labels)

		expectedEntries := []ucpv1alpha1.ResourceEntry{
			{
				ID:   shared.Resource2ID.String(),
				ETag: etag.New(shared.MarshalOrPanic(shared.Data1)),
				Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)},
			},
		}
		require.Equal(t, expectedEntries, resource.Entries)
	})

	t.Run("delete_with_delete_conflict", func(t *testing.T) {
		clear(t)

		// Setup to control the client precisely.
		readyChan := make(chan struct{})
		waitChan := make(chan struct{})

		errChan := make(chan error)

		client := NewAPIServerClient(rc, ns)
		client.readyChan = readyChan
		client.waitChan = waitChan

		// In this test we're going to simulate a conflict caused by concurrent delete of a resource.
		//
		// We'll also pretend that we've encountered a hash collision to make this possible.

		// First we create the resource
		resource := ucpv1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName(shared.Resource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.Resource1ID.String(),
					Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data1)},
				},
				{
					ID:   shared.Resource2ID.String(),
					Data: &runtime.RawExtension{Raw: shared.MarshalOrPanic(shared.Data2)},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		// Start an operation to "delete" resource 1
		go func() {
			err = client.Delete(ctx, shared.Resource1ID.String())
			errChan <- err
		}()

		// Wait until the client is "ready" to perform a delete. Now we'll cause the conflict by the Kubernetes object
		// out of back from the call to Delete().
		<-readyChan

		resource.Entries = resource.Entries[:1]
		err = rc.Update(ctx, &resource)
		require.NoError(t, err)

		// Now we've updated the object, so we can let the "delete" proceed and find out if it was successful.
		waitChan <- struct{}{}

		// NOTE: we need to cycle readyChan and waitChan again because of the retry logic.
		<-readyChan
		waitChan <- struct{}{}

		err = <-errChan
		require.NoError(t, err, "concurrent delete of resource1 failed")

		// Now let's look at the kubernetes object to make sure it was deleted.
		resourceName := resourceName(shared.Resource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.True(t, apierrors.IsNotFound(err))
	})
}

func Test_AssignLabels_Resource_NoConflicts(t *testing.T) {
	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
		},
	}

	expected := labels.Set{
		"ucp.dev/kind":                 "resource",
		"ucp.dev/resource-type":        "applications.core_applications",
		"ucp.dev/scope-radius":         "local",
		"ucp.dev/scope-resourcegroups": "cool-group",
	}

	labels := assignLabels(&resource)
	require.Equal(t, expected, labels)
}

func Test_AssignLabels_Scope_NoConflicts(t *testing.T) {
	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				ID: "/planes/radius/local/resourceGroups/cool-group",
			},
		},
	}

	expected := labels.Set{
		"ucp.dev/kind":          "scope",
		"ucp.dev/resource-type": "resourcegroups",
		"ucp.dev/scope-radius":  "local",
	}

	labels := assignLabels(&resource)
	require.Equal(t, expected, labels)
}

func Test_AssignLabels_PartialConflict(t *testing.T) {
	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
			{
				ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/containers/backend",
			},
		},
	}

	expected := labels.Set{
		"ucp.dev/kind":                 "resource",
		"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
		"ucp.dev/scope-radius":         "local",
		"ucp.dev/scope-resourcegroups": "cool-group",
	}

	labels := assignLabels(&resource)
	require.Equal(t, expected, labels)
}

func Test_AssignLabels_AllConflict(t *testing.T) {
	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
			{
				ID: "/planes/azure/azurecloud/subscriptions/my-sub/resourceGroups/azure-group/providers/Applications.Core/containers/backend",
			},
		},
	}

	expected := labels.Set{
		"ucp.dev/kind":                 "resource",
		"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
		"ucp.dev/scope-azure":          "azurecloud",
		"ucp.dev/scope-radius":         "local",
		"ucp.dev/scope-resourcegroups": "m_u_l_t_i_p_l_e",
		"ucp.dev/scope-subscriptions":  "my-sub",
	}

	set := assignLabels(&resource)
	require.Equal(t, expected, set)
}

func Test_CreateLabelSelector_UCPID(t *testing.T) {
	query := store.Query{
		RootScope:    "/planes/radius/local/resourceGroups/cool-group",
		ResourceType: "Applications.Core/containers",
	}

	selector, err := createLabelSelector(query)
	require.NoError(t, err)

	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Wrong resource type
				ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
		},
	}
	set := assignLabels(&resource)
	require.False(t, selector.Matches(set))

	resource = ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Different scope
				ID: "/planes/radius/local/resourceGroups/another-group/providers/Applications.Core/containers/backend",
			},
		},
	}
	set = assignLabels(&resource)
	require.False(t, selector.Matches(set))

	resource = ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Match!
				ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/containers/backend",
			},
		},
	}
	set = assignLabels(&resource)
	require.True(t, selector.Matches(set))
}

func Test_CreateLabelSelector_ResourceQuery(t *testing.T) {
	query := store.Query{
		RootScope:    "/planes/radius/local/resourceGroups/cool-group",
		ResourceType: "Applications.Core/containers",
	}

	selector, err := createLabelSelector(query)
	require.NoError(t, err)

	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Wrong resource type
				ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
		},
	}
	set := assignLabels(&resource)
	require.False(t, selector.Matches(set))

	resource = ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Different scope
				ID: "/planes/radius/local/resourceGroups/another-group/providers/Applications.Core/containers/backend",
			},
		},
	}
	set = assignLabels(&resource)
	require.False(t, selector.Matches(set))

	resource = ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Match!
				ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/containers/backend",
			},
		},
	}
	set = assignLabels(&resource)
	require.True(t, selector.Matches(set))
}

func Test_CreateLabelSelector_ScopeQuery(t *testing.T) {
	query := store.Query{
		RootScope:    "/planes/radius/local",
		ResourceType: "resourceGroups",
		IsScopeQuery: true,
	}

	selector, err := createLabelSelector(query)
	require.NoError(t, err)

	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Wrong resource type
				ID: "/planes/radius/local/subscriptions/cool-subscription",
			},
		},
	}
	set := assignLabels(&resource)
	require.False(t, selector.Matches(set))

	resource = ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Different scope
				ID: "/planes/radius/local/resourceGroups/another-group/anotherScope/cool-name",
			},
		},
	}
	set = assignLabels(&resource)
	require.False(t, selector.Matches(set))

	resource = ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Match!
				ID: "/planes/radius/local/resourceGroups/cool-group",
			},
		},
	}
	set = assignLabels(&resource)
	require.True(t, selector.Matches(set))
}
