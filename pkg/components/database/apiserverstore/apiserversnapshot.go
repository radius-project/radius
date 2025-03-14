package apiserverstore

import (
	"context"
	"encoding/json"
	"fmt"

	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// snapshotRecord represents a snapshot of a Kubernetes Resource object.
type snapshotRecord struct {
	// ResourceName is the name of the CR object.
	ResourceName string `json:"resource_name"`
	// Namespace is the namespace where the CR object is stored.
	Namespace string `json:"namespace"`
	// Records is the list of resource entries stored in the object's Spec.
	Records []ucpv1alpha1.ResourceEntry `json:"records"`
}

// Snapshot implements the database.Snapshotter interface for APIServerClient.
// It retrieves all Resource objects from the configured namespace and returns
// a JSON snapshot.
func (c *APIServerClient) Snapshot(ctx context.Context) ([]byte, error) {
	var rs ucpv1alpha1.ResourceList
	if err := c.client.List(ctx, &rs, client.InNamespace(c.namespace)); err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	var snapshots []snapshotRecord
	for _, r := range rs.Items {
		snapshots = append(snapshots, snapshotRecord{
			ResourceName: r.Name,
			Namespace:    r.Namespace,
			Records:      r.Entries,
		})
	}

	snapshotData, err := json.MarshalIndent(snapshots, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot: %w", err)
	}
	return snapshotData, nil
}

// Restore implements the database.Restorer interface for APIServerClient.
// It unmarshals the snapshot data and restores the state of Resource objects in the API server.
// For each snapshot record, if the resource exists then its entries are updated;
// otherwise, a new resource object is created.
func (c *APIServerClient) Restore(ctx context.Context, snapshot []byte) error {
	var snapshots []snapshotRecord
	if err := json.Unmarshal(snapshot, &snapshots); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot data: %w", err)
	}

	for _, rec := range snapshots {
		key := client.ObjectKey{Namespace: rec.Namespace, Name: rec.ResourceName}
		var current ucpv1alpha1.Resource
		err := c.client.Get(ctx, key, &current)
		if err == nil {
			// Update the existing resource with the snapshot records.
			current.Entries = rec.Records
			if err := c.client.Update(ctx, &current); err != nil {
				return fmt.Errorf("failed to update resource %s: %w", rec.ResourceName, err)
			}
		} else {
			// Create a new Resource object using the snapshot data.
			newResource := ucpv1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rec.ResourceName,
					Namespace: rec.Namespace,
				},
				Entries: rec.Records,
			}
			if err := c.client.Create(ctx, &newResource); err != nil {
				return fmt.Errorf("failed to create resource %s: %w", rec.ResourceName, err)
			}
		}
	}

	return nil
}
