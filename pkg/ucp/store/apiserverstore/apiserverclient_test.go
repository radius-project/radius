// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserverstore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/store"
	ucpv1alpha1 "github.com/project-radius/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/project-radius/radius/pkg/ucp/util/etag"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	shared "github.com/project-radius/radius/test/ucp/storetest"
)

func Test_APIServer_Client(t *testing.T) {
	// The APIServer tests require installation of the Kubernetes test environment binaries.
	// Our Makefile knows how to download the the amd64 version of these on MacOS.
	rc, env, err := startEnvironment()
	require.NoError(t, err, "If this step is failing for you, run `make test` inside the repository and try again. If you are still stuck then ask for help.")
	defer func() {
		_ = env.Stop()
	}()

	ctx, cancel := testcontext.New(t)
	defer cancel()

	ns := "radius-test"
	err = ensureNamespace(ctx, rc, ns)
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

	t.Run("save_and_validate_kubernetes_object", func(t *testing.T) {
		clear(t)

		obj1 := store.Object{
			Metadata: store.Metadata{
				ID: shared.BasicResource1ID.String(),
			},
			Data: shared.Data1,
		}
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		// Now let's look at the kubernetes object.
		resourceName := resourceName(shared.BasicResource1ID)

		resource := ucpv1alpha1.Resource{}
		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expected := map[string]string{
			"ucp.dev/resource-type":        "system.resources_resourcetype1",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "group1",
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
				Name:      resourceName(shared.BasicResource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.BasicResource2ID.String(),
					ETag: etag.New(shared.Data2),
					Data: &runtime.RawExtension{Raw: shared.Data2},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		obj1 := store.Object{
			Metadata: store.Metadata{
				ID: shared.BasicResource1ID.String(),
			},
			Data: shared.Data1,
		}
		err = client.Save(ctx, &obj1)
		require.NoError(t, err)

		// Now let's look at the kubernetes object.
		resourceName := resourceName(shared.BasicResource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expectedLabels := map[string]string{
			"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "m_u_l_t_i_p_l_e",
		}
		require.Equal(t, expectedLabels, resource.Labels)

		expectedEntries := []ucpv1alpha1.ResourceEntry{
			{
				ID:   shared.BasicResource2ID.String(),
				ETag: etag.New(shared.Data2),
				Data: &runtime.RawExtension{Raw: shared.Data2},
			},
			{
				ID:   shared.BasicResource1ID.String(),
				ETag: etag.New(shared.Data1),
				Data: &runtime.RawExtension{Raw: shared.Data1},
			},
		}
		require.Equal(t, expectedEntries, resource.Entries)

		// Now we should be able to get resource 1 directly. We can't get resource 2 directly because we stored it
		// with the wrong name on purpose.
		obj, err := client.Get(ctx, shared.BasicResource1ID)
		require.NoError(t, err)
		require.Equal(t, shared.BasicResource1ID.String(), obj.ID)
		require.Equal(t, shared.Data1, obj.Data)

		// We can query it though...
		objs, err := client.Query(ctx, store.Query{RootScope: shared.RadiusScope, ScopeRecursive: true})
		require.NoError(t, err)
		expected := []store.Object{
			*obj,
			{
				Metadata: store.Metadata{
					ID:   shared.BasicResource2ID.String(),
					ETag: etag.New(shared.Data2),
				},
				Data: shared.Data2,
			},
		}
		require.ElementsMatch(t, expected, objs)
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
					ID: shared.BasicResource1ID.String(),
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
				Name:      resourceName(shared.BasicResource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.BasicResource2ID.String(),
					ETag: etag.New(shared.Data2),
					Data: &runtime.RawExtension{Raw: shared.Data2},
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
		resourceName := resourceName(shared.BasicResource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expectedLabels := map[string]string{
			"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "m_u_l_t_i_p_l_e",
		}
		require.Equal(t, expectedLabels, resource.Labels)

		expectedEntries := []ucpv1alpha1.ResourceEntry{
			{
				ID:   shared.BasicResource2ID.String(),
				ETag: etag.New(shared.Data2),
				Data: &runtime.RawExtension{Raw: shared.Data2},
			},
			{
				ID:   shared.BasicResource1ID.String(),
				ETag: etag.New(shared.Data1),
				Data: &runtime.RawExtension{Raw: shared.Data1},
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
				Name:      resourceName(shared.BasicResource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.BasicResource2ID.String(),
					ETag: etag.New(shared.Data2),
					Data: &runtime.RawExtension{Raw: shared.Data2},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		// Start an operation to "save" resource 1
		go func() {
			obj1 := store.Object{
				Metadata: store.Metadata{
					ID: shared.BasicResource1ID.String(),
				},
				Data: shared.Data1,
			}
			err = client.Save(ctx, &obj1)
			errChan <- err
		}()

		// Wait until the client is "ready" to perform a save. Now we'll cause the conflict by the Kubernetes object
		// out of back from the call to Save().
		<-readyChan

		resource.Entries[0].Data = &runtime.RawExtension{Raw: shared.Data1}
		resource.Entries[0].ETag = etag.New(shared.Data1)
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
		resourceName := resourceName(shared.BasicResource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expectedLabels := map[string]string{
			"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "m_u_l_t_i_p_l_e",
		}
		require.Equal(t, expectedLabels, resource.Labels)

		expectedEntries := []ucpv1alpha1.ResourceEntry{
			{
				ID:   shared.BasicResource2ID.String(),
				ETag: etag.New(shared.Data1),
				Data: &runtime.RawExtension{Raw: shared.Data1},
			},
			{
				ID:   shared.BasicResource1ID.String(),
				ETag: etag.New(shared.Data1),
				Data: &runtime.RawExtension{Raw: shared.Data1},
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
				Name:      resourceName(shared.BasicResource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.BasicResource1ID.String(),
					ETag: etag.New(shared.Data1),
					Data: &runtime.RawExtension{Raw: shared.Data1},
				},
				{
					ID:   shared.BasicResource2ID.String(),
					ETag: etag.New(shared.Data1),
					Data: &runtime.RawExtension{Raw: shared.Data2},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		// Start an operation to "delete" resource 1
		go func() {
			err = client.Delete(ctx, shared.BasicResource1ID)
			errChan <- err
		}()

		// Wait until the client is "ready" to perform a delete. Now we'll cause the conflict by the Kubernetes object
		// out of back from the call to Delete().
		<-readyChan

		resource.Entries[1].Data = &runtime.RawExtension{Raw: shared.Data1}
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
		resourceName := resourceName(shared.BasicResource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.NoError(t, err)

		expectedLabels := map[string]string{
			"ucp.dev/resource-type":        "system.resources_resourcetype2",
			"ucp.dev/scope-radius":         "local",
			"ucp.dev/scope-resourcegroups": "group2",
		}
		require.Equal(t, expectedLabels, resource.Labels)

		expectedEntries := []ucpv1alpha1.ResourceEntry{
			{
				ID:   shared.BasicResource2ID.String(),
				ETag: etag.New(shared.Data1),
				Data: &runtime.RawExtension{Raw: shared.Data1},
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
				Name:      resourceName(shared.BasicResource1ID),
				Namespace: ns,
			},
			Entries: []ucpv1alpha1.ResourceEntry{
				{
					ID:   shared.BasicResource1ID.String(),
					Data: &runtime.RawExtension{Raw: shared.Data1},
				},
				{
					ID:   shared.BasicResource2ID.String(),
					Data: &runtime.RawExtension{Raw: shared.Data2},
				},
			},
		}
		err := rc.Create(ctx, &resource)
		require.NoError(t, err)

		// Start an operation to "delete" resource 1
		go func() {
			err = client.Delete(ctx, shared.BasicResource1ID)
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
		resourceName := resourceName(shared.BasicResource1ID)

		err = rc.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: resourceName}, &resource)
		require.True(t, apierrors.IsNotFound(err))
	})
}

func startEnvironment() (runtimeclient.Client, *envtest.Environment, error) {
	assetDir, err := getKubeAssetsDir()
	if err != nil {
		return nil, nil, err
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "deploy", "crds", "ucpd")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: assetDir,
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ucpv1alpha1.AddToScheme(scheme))

	cfg, err := testEnv.Start()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize environment: %w", err)
	}

	client, err := runtimeclient.New(cfg, runtimeclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		_ = testEnv.Stop()
		return nil, nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return client, testEnv, nil
}

func getKubeAssetsDir() (string, error) {
	assetsDirectory := os.Getenv("KUBEBUILDER_ASSETS")
	if assetsDirectory != "" {
		return assetsDirectory, nil
	}

	// We require one or more versions of the test assets to be installed already. This
	// will use whatever's latest of the installed versions.
	cmd := exec.Command("setup-envtest", "use", "-i", "-p", "path", "--arch", "amd64")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to call setup-envtest to find path: %w", err)
	} else {
		return out.String(), err
	}
}

func ensureNamespace(ctx context.Context, client runtimeclient.Client, namespace string) error {
	nsObject := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	return client.Create(ctx, &nsObject, &runtimeclient.CreateOptions{})
}

func Test_AssignLabels_NoConflicts(t *testing.T) {
	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				ID: "ucp:/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
		},
	}

	expected := labels.Set{
		"ucp.dev/resource-type":        "applications.core_applications",
		"ucp.dev/scope-radius":         "local",
		"ucp.dev/scope-resourcegroups": "cool-group",
	}

	labels := assignLabels(&resource)
	require.Equal(t, expected, labels)
}

func Test_AssignLabels_PartialConflict(t *testing.T) {
	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				ID: "ucp:/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
			{
				ID: "ucp:/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/containers/backend",
			},
		},
	}

	expected := labels.Set{
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
				ID: "ucp:/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
			{
				ID: "ucp:/planes/azure/azurecloud/subscriptions/my-sub/resourceGroups/azure-group/providers/Applications.Core/containers/backend",
			},
		},
	}

	expected := labels.Set{
		"ucp.dev/resource-type":        "m_u_l_t_i_p_l_e",
		"ucp.dev/scope-azure":          "azurecloud",
		"ucp.dev/scope-radius":         "local",
		"ucp.dev/scope-resourcegroups": "m_u_l_t_i_p_l_e",
		"ucp.dev/scope-subscriptions":  "my-sub",
	}

	set := assignLabels(&resource)
	require.Equal(t, expected, set)
}

func Test_CreateLabelSelector(t *testing.T) {
	query := store.Query{
		RootScope:    "ucp:/planes/radius/local/resourceGroups/cool-group",
		ResourceType: "Applications.Core/containers",
	}

	selector, err := createLabelSelector(query)
	require.NoError(t, err)

	resource := ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Wrong resource type
				ID: "ucp:/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			},
		},
	}
	set := assignLabels(&resource)
	require.False(t, selector.Matches(set))

	resource = ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Different scope
				ID: "ucp:/planes/radius/local/resourceGroups/another-group/providers/Applications.Core/containers/backend",
			},
		},
	}
	set = assignLabels(&resource)
	require.False(t, selector.Matches(set))

	resource = ucpv1alpha1.Resource{
		Entries: []ucpv1alpha1.ResourceEntry{
			{
				// Match!
				ID: "ucp:/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/containers/backend",
			},
		},
	}
	set = assignLabels(&resource)
	require.True(t, selector.Matches(set))
}
