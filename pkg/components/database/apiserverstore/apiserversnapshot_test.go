package apiserverstore

import (
	"context"
	"testing"

	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	// Register your CRD type. Adjust this if your package exposes a SchemeBuilder.
	_ = ucpv1alpha1.AddToScheme(s)
	return s
}

func TestAPIServerClient_Restore_RevertsChanges(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	// --- Set up initial snapshot state ---
	// Create two resources representing the initial (snapshot) state.
	resource1 := &ucpv1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "resource1",
			Namespace: "test-ns",
		},
		Entries: []ucpv1alpha1.ResourceEntry{
			{ID: "1", ETag: "etag1", Data: nil},
		},
	}
	resource2 := &ucpv1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "resource2",
			Namespace: "test-ns",
		},
		Entries: []ucpv1alpha1.ResourceEntry{
			{ID: "2", ETag: "etag2", Data: nil},
		},
	}
	// Build a fake client with the initial resources.
	cl := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(resource1, resource2).Build()
	client := &APIServerClient{
		client:    cl,
		namespace: "test-ns",
	}

	// Take a snapshot of the initial state.
	snapshot, err := client.Snapshot(ctx)
	assert.NoError(t, err)

	// --- Modify the state: update resource1 and add an extra resource3 ---
	// Update resource1's entry (simulate a change).
	resource1Modified := resource1.DeepCopy()
	resource1Modified.Entries[0].ETag = "changed"
	err = cl.Update(ctx, resource1Modified)
	assert.NoError(t, err)

	// Add a new resource that was not part of the snapshot.
	resource3 := &ucpv1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "resource3",
			Namespace: "test-ns",
		},
		Entries: []ucpv1alpha1.ResourceEntry{
			{ID: "3", ETag: "etag3", Data: nil},
		},
	}
	err = cl.Create(ctx, resource3)
	assert.NoError(t, err)

	// --- Call Restore using the original snapshot data ---
	err = client.Restore(ctx, snapshot)
	assert.NoError(t, err)

	// --- Verify that the resources from the snapshot are restored ---
	// Resource1 should revert to its original state from the snapshot.
	var res1 ucpv1alpha1.Resource
	err = cl.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "resource1"}, &res1)
	assert.NoError(t, err)
	assert.Len(t, res1.Entries, 1)
	assert.Equal(t, "etag1", res1.Entries[0].ETag, "resource1 should be restored to 'etag1'")

	// Resource2 should remain unchanged.
	var res2 ucpv1alpha1.Resource
	err = cl.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "resource2"}, &res2)
	assert.NoError(t, err)
	assert.Len(t, res2.Entries, 1)
	assert.Equal(t, "etag2", res2.Entries[0].ETag, "resource2 should remain as 'etag2'")

	// Extra resource (resource3) is not in the snapshot. Our current Restore implementation doesn't delete extra resources,
	// so resource3 is still present.
	var res3 ucpv1alpha1.Resource
	err = cl.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "resource3"}, &res3)
	assert.NoError(t, err)
	assert.Len(t, res3.Entries, 1)
	assert.Equal(t, "etag3", res3.Entries[0].ETag, "resource3 remains unaffected")
}
