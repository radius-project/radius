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

package apiserverstore

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/radius-project/radius/pkg/components/database"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/util/etag"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const migrateNamespace = "migrate-test"

var (
	migrateTarget = resources.MustParse("/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/target")
	migrateOther  = resources.MustParse("/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/other")
	migrateGroup  = resources.MustParse("/planes/radius/local/resourceGroups/test")
)

// tombstone is the migration record a delete leaves for an entry that had a legacy copy.
var tombstone = migrationRecord{State: recordDeleted}

// migratedFrom returns the migration record of an entry whose current value was copied from the legacy value with etag.
func migratedFrom(etag string) migrationRecord {
	return migrationRecord{State: recordMigrated, ETag: etag}
}

// recordKey returns the key of the migration record of id.
func recordKey(id resources.ID) string {
	return strings.ToLower(id.String())
}

func newMigrateClient(t *testing.T, funcs interceptor.Funcs, objects ...*ucpv1alpha1.Resource) (*APIServerClient, runtimeclient.WithWatch) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, ucpv1alpha1.AddToScheme(scheme))
	initial := make([]runtimeclient.Object, 0, len(objects))
	for _, object := range objects {
		initial = append(initial, object)
	}
	// Seeded objects bypass the interceptors so tests can intercept only the migration's own writes. Resource versions
	// are unique across objects, as they are in a cluster, so a write conditional on an object that was deleted and
	// created again fails.
	raw := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initial...).WithInterceptorFuncs(funcs).WithGlobalResourceVersionCounter().Build()
	return NewAPIServerClient(raw, migrateNamespace), raw
}

func listBuckets(t *testing.T, raw runtimeclient.Client) map[string]*ucpv1alpha1.Resource {
	t.Helper()
	all := &ucpv1alpha1.ResourceList{}
	require.NoError(t, raw.List(t.Context(), all, runtimeclient.InNamespace(migrateNamespace)))
	result := map[string]*ucpv1alpha1.Resource{}
	for i := range all.Items {
		result[all.Items[i].Name] = &all.Items[i]
	}
	return result
}

// recordsOf returns the migration records of bucket.
func recordsOf(t *testing.T, bucket *ucpv1alpha1.Resource) map[string]migrationRecord {
	t.Helper()
	records, err := migrationRecords(bucket)
	require.NoError(t, err)
	return records
}

// withRecord adds the migration record of migrateTarget to bucket and returns it.
func withRecord(t *testing.T, bucket *ucpv1alpha1.Resource, record migrationRecord) *ucpv1alpha1.Resource {
	t.Helper()
	require.NoError(t, setMigrationRecord(bucket, migrateTarget, record))
	return bucket
}

// putEntry writes the entry for migrateTarget with format to the object named name, creating the object if needed, and
// records record unless it is empty.
func putEntry(t *testing.T, ctx context.Context, c runtimeclient.Client, name string, format string, record migrationRecord) {
	t.Helper()
	entry := fallbackTestResource(t, migrateNamespace, name, format, []resources.ID{migrateTarget}).Entries[0]
	bucket := &ucpv1alpha1.Resource{}
	err := c.Get(ctx, runtimeclient.ObjectKey{Namespace: migrateNamespace, Name: name}, bucket)
	exists := err == nil
	if apierrors.IsNotFound(err) {
		bucket = fallbackTestResource(t, migrateNamespace, name, format, nil)
	} else {
		require.NoError(t, err)
	}

	if index := findIndex(bucket, migrateTarget); index != nil {
		bucket.Entries[*index] = entry
	} else {
		bucket.Entries = append(bucket.Entries, entry)
	}
	bucket.Labels = assignLabels(bucket)
	if record.State != "" {
		require.NoError(t, setMigrationRecord(bucket, migrateTarget, record))
	}

	if exists {
		require.NoError(t, c.Update(ctx, bucket))
	} else {
		require.NoError(t, c.Create(ctx, bucket))
	}
}

// requireDeleted requires that migrateTarget is deleted, and that the only object left is its current object, which
// holds the tombstone of the entry and the entries of other.
func requireDeleted(t *testing.T, client *APIServerClient, raw runtimeclient.Client, other ...resources.ID) {
	t.Helper()
	_, err := client.Get(t.Context(), migrateTarget.String())
	require.ErrorIs(t, err, &database.ErrNotFound{}, "a deleted resource must stay deleted")
	buckets := listBuckets(t, raw)
	current := resourceName(migrateTarget)
	require.Equal(t, []string{current}, bucketNames(buckets))
	ids := []string{}
	for _, entry := range buckets[current].Entries {
		ids = append(ids, entry.ID)
	}
	expected := []string{}
	for _, id := range other {
		expected = append(expected, id.String())
	}
	require.Equal(t, expected, ids)
	require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): tombstone}, recordsOf(t, buckets[current]))
}

// gateFunc adapts a function to hashmigration.Gate.
type gateFunc func() (string, error)

func (f gateFunc) Wait(context.Context) (string, error) {
	return f()
}

// pausePoint pauses an operation once, at a point a test chooses, until the test resumes it.
type pausePoint struct {
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func newPausePoint() *pausePoint {
	return &pausePoint{reached: make(chan struct{}), resume: make(chan struct{})}
}

// pause signals that the operation reached the point, and waits until the test resumes it. Only the first call pauses.
func (p *pausePoint) pause() {
	p.once.Do(func() {
		close(p.reached)
		<-p.resume
	})
}

// valueOf returns a value of migrateTarget with format.
func valueOf(format string) *database.Object {
	return &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": format}}
}

// legacyValue returns the legacy object of migrateTarget holding the value with format, with the ETag that a save of
// the value computes.
func legacyValue(t *testing.T, format string) *ucpv1alpha1.Resource {
	t.Helper()
	bucket := fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), format, []resources.ID{migrateTarget})
	bucket.Entries[0].ETag = etag.New(bucket.Entries[0].Data.Raw)
	return bucket
}

// savePauses are the points at which a save of migrateTarget that writes its legacy object pauses once: before it
// writes the legacy object (write), after it wrote it, before it reads the current object again (wrote), and after it
// read the legacy object again (reread). A test resumes the points it does not use up front.
type savePauses struct {
	write, wrote, reread *pausePoint
}

// skip makes points return without pausing.
func skip(points ...*pausePoint) {
	for _, point := range points {
		close(point.resume)
	}
}

// pausingSaver returns a client whose saves pause at savePauses.
func pausingSaver(raw runtimeclient.WithWatch) (*APIServerClient, savePauses) {
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	points := savePauses{write: newPausePoint(), wrote: newPausePoint(), reread: newPausePoint()}
	written := false
	client := NewAPIServerClient(interceptor.NewClient(raw, interceptor.Funcs{
		Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			if written && key.Name == current {
				points.wrote.pause()
			}
			err := c.Get(ctx, key, obj, opts...)
			if written && key.Name == legacy {
				points.reread.pause()
			}
			return err
		},
		Update: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			if obj.GetName() == legacy {
				points.write.pause()
			}
			err := c.Update(ctx, obj, opts...)
			written = written || (err == nil && obj.GetName() == legacy)
			return err
		},
	}), migrateNamespace)
	return client, points
}

func Test_APIServerClient_MigrateLegacyNames(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	for _, tt := range []struct {
		name     string
		objects  func(t *testing.T) []*ucpv1alpha1.Resource
		expected hashmigration.Result
		verify   func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource)
	}{
		{
			name: "legacy only",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				return []*ucpv1alpha1.Resource{fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget})}
			},
			expected: hashmigration.Result{Migrated: 1},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.NotContains(t, buckets, legacy)
				require.Len(t, buckets[current].Entries, 1)
				require.Equal(t, "legacy-etag", buckets[current].Entries[0].ETag, "the ETag must be preserved")
				require.JSONEq(t, `{"format":"legacy"}`, string(buckets[current].Entries[0].Data.Raw))
				require.Equal(t, map[string]string(assignLabels(buckets[current])), buckets[current].Labels)
				require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): migratedFrom("legacy-etag")}, recordsOf(t, buckets[current]))
			},
		},
		{
			name: "legacy scope",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				return []*ucpv1alpha1.Resource{fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateGroup), "legacy", []resources.ID{migrateGroup})}
			},
			expected: hashmigration.Result{Migrated: 1},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.Equal(t, []string{resourceName(migrateGroup)}, bucketNames(buckets))
				require.Equal(t, database.UCPScopePrefix, buckets[resourceName(migrateGroup)].Labels[LabelKind])
				require.Equal(t, map[string]migrationRecord{recordKey(migrateGroup): migratedFrom("legacy-etag")}, recordsOf(t, buckets[resourceName(migrateGroup)]))
			},
		},
		{
			// The unrelated entry is synthetic: its own name does not match the bucket, so it is reported as skipped.
			name: "current collision bucket",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				return []*ucpv1alpha1.Resource{
					fallbackTestResource(t, migrateNamespace, current, "current", []resources.ID{migrateOther}),
					fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}),
				}
			},
			expected: hashmigration.Result{Migrated: 1, Skipped: 1},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.Equal(t, []string{current}, bucketNames(buckets))
				require.Len(t, buckets[current].Entries, 2)
				require.Equal(t, migrateOther.String(), buckets[current].Entries[0].ID, "unrelated entries are preserved")
				require.Equal(t, "legacy-etag", buckets[current].Entries[1].ETag)
				require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): migratedFrom("legacy-etag")}, recordsOf(t, buckets[current]))
			},
		},
		{
			name: "legacy bucket with unrecognized entry",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				return []*ucpv1alpha1.Resource{fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget, migrateOther})}
			},
			expected: hashmigration.Result{Migrated: 1, Skipped: 1},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.Len(t, buckets[current].Entries, 1)
				require.Len(t, buckets[legacy].Entries, 1, "an entry whose name does not match its ID is left in place")
				require.Equal(t, migrateOther.String(), buckets[legacy].Entries[0].ID)
				require.Equal(t, map[string]string(assignLabels(buckets[legacy])), buckets[legacy].Labels)
				require.Empty(t, recordsOf(t, buckets[legacy]))
			},
		},
		{
			name: "identical duplicate",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				return []*ucpv1alpha1.Resource{
					fallbackTestResource(t, migrateNamespace, current, "same", []resources.ID{migrateTarget}),
					fallbackTestResource(t, migrateNamespace, legacy, "same", []resources.ID{migrateTarget}),
				}
			},
			expected: hashmigration.Result{Deduplicated: 1},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.Equal(t, []string{current}, bucketNames(buckets))
				require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): migratedFrom("same-etag")}, recordsOf(t, buckets[current]), "a later delete must leave a tombstone")
			},
		},
		{
			name: "identical data with different ETag formats",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				currentBucket := fallbackTestResource(t, migrateNamespace, current, "same", []resources.ID{migrateTarget})
				legacyBucket := fallbackTestResource(t, migrateNamespace, legacy, "same", []resources.ID{migrateTarget})
				legacyBucket.Entries[0].ETag = "20-legacy"
				return []*ucpv1alpha1.Resource{currentBucket, legacyBucket}
			},
			expected: hashmigration.Result{Deduplicated: 1},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.Equal(t, []string{current}, bucketNames(buckets))
				require.Equal(t, "same-etag", buckets[current].Entries[0].ETag)
			},
		},
		{
			name: "conflicting duplicate",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				return []*ucpv1alpha1.Resource{
					fallbackTestResource(t, migrateNamespace, current, "current", []resources.ID{migrateTarget}),
					fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}),
				}
			},
			expected: hashmigration.Result{Conflicts: 1},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.ElementsMatch(t, []string{current, legacy}, bucketNames(buckets), "differing copies are never deleted")
				require.Empty(t, recordsOf(t, buckets[current]))
			},
		},
		{
			name: "current only",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				return []*ucpv1alpha1.Resource{fallbackTestResource(t, migrateNamespace, current, "current", []resources.ID{migrateTarget})}
			},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.Equal(t, []string{current}, bucketNames(buckets))
				require.Empty(t, buckets[current].Annotations, "an entry without a legacy copy needs no record")
			},
		},
		{
			name: "invalid entry ID",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				bucket := fallbackTestResource(t, migrateNamespace, "resource.invalid.0000", "invalid", []resources.ID{migrateTarget})
				bucket.Entries[0].ID = "not a resource id"
				bucket.Labels = nil
				return []*ucpv1alpha1.Resource{bucket}
			},
			expected: hashmigration.Result{Skipped: 1},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.Equal(t, []string{"resource.invalid.0000"}, bucketNames(buckets))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, raw := newMigrateClient(t, interceptor.Funcs{}, tt.objects(t)...)
			result, err := client.MigrateLegacyNames(t.Context(), nil)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
			buckets := listBuckets(t, raw)
			tt.verify(t, buckets)

			// A second pass is a no-op apart from values that cannot be migrated.
			again, err := client.MigrateLegacyNames(t.Context(), nil)
			require.NoError(t, err)
			require.Equal(t, hashmigration.Result{Conflicts: tt.expected.Conflicts, Skipped: tt.expected.Skipped}, again)
			require.Equal(t, buckets, listBuckets(t, raw))

			if tt.name != "legacy scope" && tt.name != "invalid entry ID" {
				got, err := client.Get(t.Context(), migrateTarget.String())
				require.NoError(t, err)
				require.NotNil(t, got)
			}
		})
	}
}

// Writers of a release before this one read and write an entry under its legacy name until it has a current copy, and
// do not make a write visible in a copy written meanwhile. With a copy, two conditional writers that read the same
// ETag could both succeed, one in the copy and one, hidden, in the legacy object, so no entry is copied while their
// pods run.
func Test_APIServerClient_MigrateLegacyNames_WaitsForEarlierReleases(t *testing.T) {
	t.Parallel()
	legacy := legacyResourceName(migrateTarget)
	client, raw := newMigrateClient(t, interceptor.Funcs{}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

	// A writer of the earlier release read the legacy object and pauses before it writes the object back, conditional
	// on the version it read.
	paused := &ucpv1alpha1.Resource{}
	require.NoError(t, raw.Get(t.Context(), runtimeclient.ObjectKey{Namespace: migrateNamespace, Name: legacy}, paused))

	const reason = "waiting for pods that run another release of Radius to stop"
	closed := true
	gate := gateFunc(func() (string, error) {
		if closed {
			return reason, nil
		}
		return "", nil
	})
	result, err := client.MigrateLegacyNames(t.Context(), gate)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Remaining: 1, Message: reason}, result)
	require.Equal(t, []string{legacy}, bucketNames(listBuckets(t, raw)), "no entry is copied while the gate is closed")

	// A writer of this release saves with the ETag both writers read. Without a copy it writes to the legacy object,
	// so the paused write fails, as it does without the migration.
	saved := &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": "saved"}}
	require.NoError(t, client.Save(t.Context(), saved, database.WithETag("legacy-etag")))
	paused.Entries = fallbackTestResource(t, migrateNamespace, legacy, "paused", []resources.ID{migrateTarget}).Entries
	require.True(t, apierrors.IsConflict(raw.Update(t.Context(), paused)), "the delayed write of the earlier release must fail")

	closed = false
	result, err = client.MigrateLegacyNames(t.Context(), gate)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Migrated: 1}, result)
	obj, err := client.Get(t.Context(), migrateTarget.String())
	require.NoError(t, err)
	require.Equal(t, saved.ETag, obj.ETag)
}

func Test_APIServerClient_MigrateLegacyNames_Gate(t *testing.T) {
	t.Parallel()
	objects := func(t *testing.T) []*ucpv1alpha1.Resource {
		return []*ucpv1alpha1.Resource{
			fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget}),
			fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateGroup), "legacy", []resources.ID{migrateGroup}),
		}
	}

	t.Run("checked before each entry", func(t *testing.T) {
		t.Parallel()
		checks := 0
		client, raw := newMigrateClient(t, interceptor.Funcs{}, objects(t)...)
		result, err := client.MigrateLegacyNames(t.Context(), gateFunc(func() (string, error) {
			checks++
			if checks > 1 {
				return "closed", nil
			}
			return "", nil
		}))
		require.NoError(t, err)
		require.Equal(t, hashmigration.Result{Migrated: 1, Remaining: 1, Message: "closed"}, result)
		require.Equal(t, 2, checks)
		require.Len(t, listBuckets(t, raw), 2)
	})

	t.Run("fails", func(t *testing.T) {
		t.Parallel()
		failure := errors.New("pods unavailable")
		client, raw := newMigrateClient(t, interceptor.Funcs{}, objects(t)...)
		_, err := client.MigrateLegacyNames(t.Context(), gateFunc(func() (string, error) { return "", failure }))
		require.ErrorIs(t, err, failure)
		require.ElementsMatch(t, []string{legacyResourceName(migrateTarget), legacyResourceName(migrateGroup)}, bucketNames(listBuckets(t, raw)))
	})
}

// A legacy value that changed after the current object received a copy of it is newer than the copy, unless the copy
// was written since. A save of this release that read the legacy copy before the copy existed writes such a value if
// it stops before it makes the write visible.
func Test_APIServerClient_MigrateLegacyNames_KeepsNewerLegacyValue(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)

	t.Run("written during the move", func(t *testing.T) {
		t.Parallel()
		updated := false
		client, raw := newMigrateClient(t, interceptor.Funcs{
			Create: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				if err := c.Create(ctx, obj, opts...); err != nil {
					return err
				}
				if !updated {
					updated = true
					putEntry(t, ctx, c, legacy, "newer", migrationRecord{})
				}
				return nil
			},
		}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

		result, err := client.MigrateLegacyNames(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, hashmigration.Result{Migrated: 1}, result)
		buckets := listBuckets(t, raw)
		require.Equal(t, []string{current}, bucketNames(buckets))
		require.Equal(t, "newer-etag", buckets[current].Entries[0].ETag, "the newer legacy value must win over the stale copy")
		require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): migratedFrom("newer-etag")}, recordsOf(t, buckets[current]))
	})

	for _, tt := range []struct {
		name     string
		copy     string
		expected hashmigration.Result
		stored   string
	}{
		{name: "after an interrupted move", copy: "legacy", expected: hashmigration.Result{Migrated: 1}, stored: "newer"},
		{name: "copy saved since", copy: "current", expected: hashmigration.Result{Conflicts: 1}, stored: "current"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// The current object received a copy of the legacy value "legacy", and the legacy value changed since.
			client, raw := newMigrateClient(t, interceptor.Funcs{},
				withRecord(t, fallbackTestResource(t, migrateNamespace, current, tt.copy, []resources.ID{migrateTarget}), migratedFrom("legacy-etag")),
				fallbackTestResource(t, migrateNamespace, legacy, "newer", []resources.ID{migrateTarget}),
			)

			result, err := client.MigrateLegacyNames(t.Context(), nil)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
			obj, err := client.Get(t.Context(), migrateTarget.String())
			require.NoError(t, err)
			require.Equal(t, map[string]any{"format": tt.stored}, obj.Data)
			if tt.expected.Conflicts > 0 {
				require.Len(t, listBuckets(t, raw), 2)
			} else {
				require.Equal(t, []string{current}, bucketNames(listBuckets(t, raw)))
			}
		})
	}
}

// A migrator reads the current object before the legacy object, so a legacy value it read is never older than a copy
// it finds. A migrator that pauses after its first read must not replace a newer value that another migrator copied
// meanwhile with an older value.
func Test_APIServerClient_MigrateLegacyNames_PausedMigrator(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	_, raw := newMigrateClient(t, interceptor.Funcs{}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

	read, resume, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	paused := NewAPIServerClient(interceptor.NewClient(raw, interceptor.Funcs{
		Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			err := c.Get(ctx, key, obj, opts...)
			if key.Name == current || key.Name == legacy {
				once.Do(func() {
					close(read)
					<-resume
				})
			}
			return err
		},
	}), migrateNamespace)
	go func() {
		_, err := paused.MigrateLegacyNames(context.WithoutCancel(t.Context()), nil)
		done <- err
	}()
	<-read

	// A save writes a newer legacy value, and another migrator moves it.
	putEntry(t, t.Context(), raw, legacy, "newer", migrationRecord{})
	client := NewAPIServerClient(raw, migrateNamespace)
	result, err := client.MigrateLegacyNames(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Migrated: 1}, result)

	close(resume)
	require.NoError(t, <-done)
	obj, err := client.Get(t.Context(), migrateTarget.String())
	require.NoError(t, err)
	require.Equal(t, "newer-etag", obj.ETag, "the paused migrator must not write the older value")
	require.Equal(t, []string{current}, bucketNames(listBuckets(t, raw)))
}

// A delete leaves a tombstone in the current object of an entry that had a legacy copy, so a legacy copy that exists
// with the tombstone was written after the delete, by a release that knows only legacy names. It is left in place.
func Test_APIServerClient_MigrateLegacyNames_LegacyCopyWrittenAfterDelete(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	client, raw := newMigrateClient(t, interceptor.Funcs{},
		withRecord(t, fallbackTestResource(t, migrateNamespace, current, "", nil), tombstone),
		fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}),
	)

	result, err := client.MigrateLegacyNames(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Conflicts: 1}, result)
	require.ElementsMatch(t, []string{current, legacy}, bucketNames(listBuckets(t, raw)))
	obj, err := client.Get(t.Context(), migrateTarget.String())
	require.NoError(t, err)
	require.Equal(t, "legacy-etag", obj.ETag)
}

func Test_APIServerClient_MigrateLegacyNames_Errors(t *testing.T) {
	t.Parallel()
	failure := apierrors.NewForbidden(schema.GroupResource{Resource: "resources"}, "x", errors.New("denied"))

	t.Run("list", func(t *testing.T) {
		t.Parallel()
		client, _ := newMigrateClient(t, interceptor.Funcs{
			List: func(context.Context, runtimeclient.WithWatch, runtimeclient.ObjectList, ...runtimeclient.ListOption) error {
				return failure
			},
		})
		_, err := client.MigrateLegacyNames(t.Context(), nil)
		require.ErrorIs(t, err, failure)
	})

	t.Run("entry", func(t *testing.T) {
		t.Parallel()
		client, raw := newMigrateClient(t, interceptor.Funcs{
			Create: func(context.Context, runtimeclient.WithWatch, runtimeclient.Object, ...runtimeclient.CreateOption) error {
				return failure
			},
		}, fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget}))
		result, err := client.MigrateLegacyNames(t.Context(), nil)
		require.NoError(t, err, "a single entry failure must not stop the pass")
		require.Equal(t, 1, result.Remaining)
		require.Contains(t, result.Message, "denied")
		require.Equal(t, []string{legacyResourceName(migrateTarget)}, bucketNames(listBuckets(t, raw)))
	})

	t.Run("busy", func(t *testing.T) {
		t.Parallel()
		client, _ := newMigrateClient(t, interceptor.Funcs{
			Create: func(context.Context, runtimeclient.WithWatch, runtimeclient.Object, ...runtimeclient.CreateOption) error {
				return apierrors.NewAlreadyExists(schema.GroupResource{Resource: "resources"}, "x")
			},
		}, fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget}))
		result, err := client.MigrateLegacyNames(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, hashmigration.Result{Remaining: 1}, result)
	})

	t.Run("invalid record", func(t *testing.T) {
		t.Parallel()
		currentBucket := fallbackTestResource(t, migrateNamespace, resourceName(migrateTarget), "current", []resources.ID{migrateOther})
		currentBucket.Annotations = map[string]string{annotationMigration: "{"}
		client, _ := newMigrateClient(t, interceptor.Funcs{}, currentBucket,
			fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget}))
		result, err := client.MigrateLegacyNames(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, 1, result.Remaining)
		require.Contains(t, result.Message, annotationMigration)
	})
}

func Test_APIServerClient_Query_PrefersCurrentCopy(t *testing.T) {
	t.Parallel()
	for _, order := range []string{"legacy first", "current first"} {
		t.Run(order, func(t *testing.T) {
			t.Parallel()
			currentBucket := fallbackTestResource(t, migrateNamespace, resourceName(migrateTarget), "current", []resources.ID{migrateTarget})
			legacyBucket := fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget, migrateOther})
			objects := []*ucpv1alpha1.Resource{legacyBucket, currentBucket}
			if order == "current first" {
				objects = []*ucpv1alpha1.Resource{currentBucket, legacyBucket}
			}
			client, _ := newMigrateClient(t, interceptor.Funcs{}, objects...)

			result, err := client.Query(t.Context(), database.Query{RootScope: "/planes/radius/local", ScopeRecursive: true, ResourceType: migrateTarget.Type()})
			require.NoError(t, err)
			require.Len(t, result.Items, 2)
			byID := map[string]database.Object{}
			for _, item := range result.Items {
				byID[item.ID] = item
			}
			require.Equal(t, "current-etag", byID[migrateTarget.String()].ETag)
			require.Equal(t, "legacy-etag", byID[migrateOther.String()].ETag)
		})
	}
}

func Test_APIServerClient_Query_FiltersCopyReadsReturn(t *testing.T) {
	t.Parallel()
	client, _ := newMigrateClient(t, interceptor.Funcs{},
		fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget, migrateOther}),
		fallbackTestResource(t, migrateNamespace, resourceName(migrateTarget), "current", []resources.ID{migrateTarget}),
	)
	query := func(format string) []string {
		result, err := client.Query(t.Context(), database.Query{
			RootScope:      "/planes/radius/local",
			ScopeRecursive: true,
			ResourceType:   migrateTarget.Type(),
			Filters:        []database.QueryFilter{{Field: "format", Value: format}},
		})
		require.NoError(t, err)
		matches := []string{}
		for _, item := range result.Items {
			matches = append(matches, item.ID+"="+item.ETag)
		}
		return matches
	}

	// The stale legacy copy of the target must not match a filter that the copy reads return does not match.
	require.Equal(t, []string{migrateOther.String() + "=legacy-etag"}, query("legacy"))
	require.Equal(t, []string{migrateTarget.String() + "=current-etag"}, query("current"))
}

func Test_APIServerClient_Delete_RemovesEveryCopy(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string

		// legacy is the format of the legacy copy, and shared adds another entry to its object.
		legacy string
		shared bool

		etag    string
		wantErr error
	}{
		{name: "interrupted move", legacy: "current"},
		{name: "conflicting copies", legacy: "legacy"},
		{name: "legacy object shared with another entry", legacy: "legacy", shared: true},
		{name: "ETag of the copy reads return", legacy: "legacy", etag: "current-etag"},
		{name: "ETag of the legacy copy", legacy: "legacy", etag: "legacy-etag", wantErr: &database.ErrConcurrency{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			legacyIDs := []resources.ID{migrateTarget}
			if tt.shared {
				legacyIDs = append(legacyIDs, migrateOther)
			}
			client, raw := newMigrateClient(t, interceptor.Funcs{},
				fallbackTestResource(t, migrateNamespace, resourceName(migrateTarget), "current", []resources.ID{migrateTarget}),
				fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), tt.legacy, legacyIDs),
			)
			options := []database.DeleteOptions{}
			if tt.etag != "" {
				options = append(options, database.WithETag(tt.etag))
			}

			err := client.Delete(t.Context(), migrateTarget.String(), options...)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Len(t, listBuckets(t, raw), 2, "a rejected delete must not remove any copy")
				return
			}

			require.NoError(t, err)
			_, err = client.Get(t.Context(), migrateTarget.String())
			require.ErrorIs(t, err, &database.ErrNotFound{}, "a deleted resource must not be read from a copy the delete left behind")
			buckets := listBuckets(t, raw)
			current := buckets[resourceName(migrateTarget)]
			require.NotNil(t, current, "the current object keeps the tombstone")
			require.Empty(t, current.Entries)
			require.Empty(t, current.Labels)
			require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): tombstone}, recordsOf(t, current))
			if tt.shared {
				require.Len(t, buckets, 2)
				require.Len(t, buckets[legacyResourceName(migrateTarget)].Entries, 1)
				require.Equal(t, migrateOther.String(), buckets[legacyResourceName(migrateTarget)].Entries[0].ID)
			} else {
				require.Len(t, buckets, 1)
			}

			// The entry can be created again, under its current name.
			require.NoError(t, client.Save(t.Context(), &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": "recreated"}}, database.WithCreateOnly()))
			obj, err := client.Get(t.Context(), migrateTarget.String())
			require.NoError(t, err)
			require.Equal(t, map[string]any{"format": "recreated"}, obj.Data)
			require.Len(t, listBuckets(t, raw)[resourceName(migrateTarget)].Entries, 1)
		})
	}
}

// A delete of an entry that had a legacy copy leaves a tombstone. A delete of an entry that never had one removes the
// current object, as it did before the migration.
func Test_APIServerClient_Delete_Tombstones(t *testing.T) {
	t.Parallel()
	current := resourceName(migrateTarget)

	t.Run("moved entry", func(t *testing.T) {
		t.Parallel()
		client, raw := newMigrateClient(t, interceptor.Funcs{},
			withRecord(t, fallbackTestResource(t, migrateNamespace, current, "legacy", []resources.ID{migrateTarget}), migratedFrom("legacy-etag")))
		require.NoError(t, client.Delete(t.Context(), migrateTarget.String(), database.WithETag("legacy-etag")))
		requireDeleted(t, client, raw)

		// A delete of the entry created again keeps the tombstone.
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": "recreated"}}))
		require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): tombstone}, recordsOf(t, listBuckets(t, raw)[current]))
		require.NoError(t, client.Delete(t.Context(), migrateTarget.String()))
		requireDeleted(t, client, raw)
	})

	t.Run("entry without a legacy copy", func(t *testing.T) {
		t.Parallel()
		client, raw := newMigrateClient(t, interceptor.Funcs{}, fallbackTestResource(t, migrateNamespace, current, "current", []resources.ID{migrateTarget}))
		require.NoError(t, client.Delete(t.Context(), migrateTarget.String()))
		require.Empty(t, listBuckets(t, raw))
	})

	t.Run("conditional delete of a deleted entry", func(t *testing.T) {
		t.Parallel()
		client, _ := newMigrateClient(t, interceptor.Funcs{},
			withRecord(t, fallbackTestResource(t, migrateNamespace, current, "", nil), tombstone))
		require.ErrorIs(t, client.Delete(t.Context(), migrateTarget.String(), database.WithETag("legacy-etag")), &database.ErrConcurrency{}, "an object that holds only a tombstone does not count as stored")
		require.ErrorIs(t, client.Delete(t.Context(), migrateTarget.String()), &database.ErrNotFound{})
	})
}

func Test_APIServerClient_MigrateLegacyNames_ConcurrentDelete(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string

		// afterCopy deletes the resource right after the move writes the current copy, instead of right before.
		afterCopy bool
	}{
		{name: "before the copy"},
		{name: "after the copy", afterCopy: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var client *APIServerClient
			deleted := false
			deleteTarget := func(ctx context.Context) {
				deleted = true
				require.NoError(t, client.Delete(ctx, migrateTarget.String()))
			}
			client, raw := newMigrateClient(t, interceptor.Funcs{
				Create: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
					// The move read the legacy copy and now writes the current copy.
					if !deleted && !tt.afterCopy {
						deleteTarget(ctx)
					}
					if err := c.Create(ctx, obj, opts...); err != nil {
						return err
					}
					if !deleted && tt.afterCopy {
						deleteTarget(ctx)
					}
					return nil
				},
			}, fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget}))

			result, err := client.MigrateLegacyNames(t.Context(), nil)
			require.NoError(t, err)
			require.True(t, deleted)
			require.Equal(t, hashmigration.Result{}, result)
			requireDeleted(t, client, raw)
		})
	}
}

// A delete writes the tombstone of an entry that had a legacy copy before it removes any copy, conditional on the
// current object it read. A copy written after the delete read the current object therefore makes the delete read
// again, and remove it. A value saved after the delete removed the entry is kept.
func Test_APIServerClient_Delete_DuringWrites(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)

	t.Run("copy written after the delete read the current object", func(t *testing.T) {
		t.Parallel()
		written := false
		client, raw := newMigrateClient(t, interceptor.Funcs{
			Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if key.Name == legacy && !written {
					written = true
					putEntry(t, ctx, c, current, "legacy", migratedFrom("legacy-etag"))
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

		require.NoError(t, client.Delete(t.Context(), migrateTarget.String(), database.WithETag("legacy-etag")))
		require.True(t, written)
		requireDeleted(t, client, raw)
	})

	for _, tt := range []struct {
		name string

		// moved stores the entry in its current object, as a move leaves it, instead of its legacy object.
		moved bool
	}{
		{name: "legacy entry"},
		{name: "moved entry", moved: true},
	} {
		t.Run("value saved again after the delete of a "+tt.name, func(t *testing.T) {
			t.Parallel()
			var client *APIServerClient
			saved := false
			save := func(ctx context.Context) {
				// The same value has the same ETag as the one the delete removed.
				saved = true
				require.NoError(t, client.Save(ctx, &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": "legacy"}}, database.WithCreateOnly()))
			}
			valueETag := etag.New([]byte(`{"format":"legacy"}`))
			stored := fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget})
			if tt.moved {
				stored = withRecord(t, fallbackTestResource(t, migrateNamespace, current, "legacy", []resources.ID{migrateTarget}), migratedFrom(valueETag))
			}
			stored.Entries[0].ETag = valueETag
			objects := []*ucpv1alpha1.Resource{stored}
			var raw runtimeclient.WithWatch
			client, raw = newMigrateClient(t, interceptor.Funcs{
				Update: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
					if err := c.Update(ctx, obj, opts...); err != nil {
						return err
					}
					if !saved && obj.GetName() == current && len(obj.(*ucpv1alpha1.Resource).Entries) == 0 {
						save(ctx)
					}
					return nil
				},
				Delete: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
					if err := c.Delete(ctx, obj, opts...); err != nil {
						return err
					}
					if !saved && obj.GetName() == legacy {
						save(ctx)
					}
					return nil
				},
			}, objects...)

			require.NoError(t, client.Delete(t.Context(), migrateTarget.String(), database.WithETag(valueETag)))
			require.True(t, saved)
			obj, err := client.Get(t.Context(), migrateTarget.String())
			require.NoError(t, err, "a value saved after the delete removed the entry must be kept")
			require.Equal(t, valueETag, obj.ETag)
			require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): tombstone}, recordsOf(t, listBuckets(t, raw)[current]), "a migrator that read the legacy copy before the delete must still find the tombstone")
		})
	}

	t.Run("retried removal of the current copy", func(t *testing.T) {
		t.Parallel()
		conflicted := false
		client, raw := newMigrateClient(t, interceptor.Funcs{
			Update: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				if obj.GetName() == current && !conflicted {
					conflicted = true
					return apierrors.NewConflict(schema.GroupResource{Resource: "resources"}, obj.GetName(), errors.New("changed"))
				}
				return c.Update(ctx, obj, opts...)
			},
		},
			withRecord(t, fallbackTestResource(t, migrateNamespace, current, "legacy", []resources.ID{migrateTarget}), migratedFrom("legacy-etag")),
			fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}),
		)

		require.NoError(t, client.Delete(t.Context(), migrateTarget.String(), database.WithETag("legacy-etag")))
		require.True(t, conflicted)
		requireDeleted(t, client, raw)
	})
}

// A save that writes to the legacy object while a migrator copies the entry changes the current object, so the
// migrator cannot publish the value the save replaced, which would hide the saved value.
func Test_APIServerClient_Save_FencesMigrator(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	_, raw := newMigrateClient(t, interceptor.Funcs{}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

	// The migrator pauses after it read both objects.
	read, resume, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	paused := NewAPIServerClient(interceptor.NewClient(raw, interceptor.Funcs{
		Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			err := c.Get(ctx, key, obj, opts...)
			if key.Name == legacy {
				once.Do(func() {
					close(read)
					<-resume
				})
			}
			return err
		},
	}), migrateNamespace)
	var result hashmigration.Result
	go func() {
		var err error
		result, err = paused.MigrateLegacyNames(context.WithoutCancel(t.Context()), nil)
		done <- err
	}()
	<-read

	client := NewAPIServerClient(raw, migrateNamespace)
	saved := &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": "saved"}}
	require.NoError(t, client.Save(t.Context(), saved, database.WithETag("legacy-etag")))
	require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): {State: recordLegacy, Fence: 1}}, recordsOf(t, listBuckets(t, raw)[current]))

	close(resume)
	require.NoError(t, <-done)
	require.Equal(t, hashmigration.Result{Migrated: 1}, result)
	obj, err := client.Get(t.Context(), migrateTarget.String())
	require.NoError(t, err)
	require.Equal(t, saved.ETag, obj.ETag, "the migrator must copy the saved value")
	require.Equal(t, []string{current}, bucketNames(listBuckets(t, raw)))
}

// The current object received a copy of a legacy value, and the legacy value changed after the copy was taken. A save
// to the current object since, even of the copied value, makes the current value newer than the legacy value.
func Test_APIServerClient_MigrateLegacyNames_KeepsLaterCurrentSaves(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	client, raw := newMigrateClient(t, interceptor.Funcs{},
		withRecord(t, fallbackTestResource(t, migrateNamespace, current, "legacy", []resources.ID{migrateTarget}), migratedFrom("legacy-etag")),
		fallbackTestResource(t, migrateNamespace, legacy, "newer", []resources.ID{migrateTarget}),
	)

	for _, format := range []string{"other", "legacy"} {
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": format}}))
	}
	require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): {State: recordMigrated}}, recordsOf(t, listBuckets(t, raw)[current]))

	result, err := client.MigrateLegacyNames(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Conflicts: 1}, result)
	obj, err := client.Get(t.Context(), migrateTarget.String())
	require.NoError(t, err)
	require.Equal(t, map[string]any{"format": "legacy"}, obj.Data, "the last saved value must be kept")
}

// Migrators, for example in two UCP replicas, read the legacy copy of an entry before a delete removes it, and write
// their copies only after the delete returned. Neither copy may bring back the deleted resource.
func Test_APIServerClient_Delete_DuringMigrators(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	for _, tt := range []struct {
		name string

		// currentObject makes the current object exist before the delete, holding another entry.
		currentObject bool

		// movedFirst lets another migrator move the entry before the delete, instead of pausing a second migrator.
		movedFirst bool
	}{
		{name: "current object absent"},
		{name: "current object exists", currentObject: true},
		{name: "entry moved by another migrator first", movedFirst: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			objects := []*ucpv1alpha1.Resource{fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget})}
			if tt.currentObject {
				objects = append(objects, fallbackTestResource(t, migrateNamespace, current, "other", []resources.ID{migrateOther}))
			}
			deleter, raw := newMigrateClient(t, interceptor.Funcs{}, objects...)

			type outcome struct {
				result hashmigration.Result
				err    error
			}

			// pause starts a migrator that pauses once it read the legacy object, and returns a function that resumes it
			// and waits for its result.
			pause := func() func() outcome {
				read, resume, done := make(chan struct{}), make(chan struct{}), make(chan outcome, 1)
				var once sync.Once
				migrator := NewAPIServerClient(interceptor.NewClient(raw, interceptor.Funcs{
					Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
						err := c.Get(ctx, key, obj, opts...)
						if key.Name == legacy {
							once.Do(func() {
								close(read)
								<-resume
							})
						}
						return err
					},
				}), migrateNamespace)
				go func() {
					result, err := migrator.MigrateLegacyNames(context.WithoutCancel(t.Context()), nil)
					done <- outcome{result, err}
				}()
				<-read
				return func() outcome {
					close(resume)
					return <-done
				}
			}

			resumes := []func() outcome{pause()}
			if tt.movedFirst {
				result, err := NewAPIServerClient(raw, migrateNamespace).MigrateLegacyNames(t.Context(), nil)
				require.NoError(t, err)
				require.Equal(t, hashmigration.Result{Migrated: 1}, result)
			} else {
				resumes = append(resumes, pause())
			}

			require.NoError(t, deleter.Delete(t.Context(), migrateTarget.String()))
			for _, resume := range resumes {
				finished := resume()
				require.NoError(t, finished.err)
				finished.result.Skipped = 0 // The synthetic entry of the current object does not match its name.
				require.Equal(t, hashmigration.Result{}, finished.result, "the copy of the deleted entry must not be written")
			}

			if tt.currentObject {
				requireDeleted(t, deleter, raw, migrateOther)
			} else {
				requireDeleted(t, deleter, raw)
			}
		})
	}
}

// A save that read the entry from its legacy object writes to that object. If the migration copied the entry to its
// current object before that write, reads no longer return the legacy object, so the save makes its write visible
// or undoes it.
func Test_APIServerClient_Save_ReconcilesLegacyWrite(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	for _, tt := range []struct {
		name string

		// copied is the value of the current copy when the save writes to the legacy object: a copy of the legacy
		// entry, which the migration records, or a value another writer saved to the copy.
		copied string

		// cleared makes another writer save copied to the copy the migration made, which clears the copied ETag.
		cleared bool

		// later writes another value to the legacy object after the save, as an older writer that knows only legacy
		// names would.
		later bool

		conditional bool
		wantErr     error

		// stored is the value reads return after the save, objects the objects that remain, and migrated the result of
		// the migration that runs after the save.
		stored   string
		objects  []string
		migrated hashmigration.Result
	}{
		{name: "copy of the replaced value", copied: "legacy", conditional: true, stored: "updated", objects: []string{current, legacy}, migrated: hashmigration.Result{Deduplicated: 1}},
		{name: "copy saved by another writer", copied: "other", conditional: true, wantErr: &database.ErrConcurrency{}, stored: "other", objects: []string{current}},
		{name: "copy saved by another writer, unconditional save", copied: "other", stored: "other", objects: []string{current}},
		{name: "copy saved by another writer after the migration made it", copied: "other", cleared: true, conditional: true, wantErr: &database.ErrConcurrency{}, stored: "other", objects: []string{current}},
		{name: "copy saved by another writer with the written value", copied: "updated", conditional: true, stored: "updated", objects: []string{current, legacy}, migrated: hashmigration.Result{Deduplicated: 1}},
		{name: "legacy write replaced by a later write", copied: "other", later: true, conditional: true, stored: "other", objects: []string{current, legacy}, migrated: hashmigration.Result{Conflicts: 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			saved := false
			client, raw := newMigrateClient(t, interceptor.Funcs{
				Update: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
					if obj.GetName() != legacy || saved {
						return c.Update(ctx, obj, opts...)
					}

					// The migration copies the entry after the save read it from the legacy object, and another writer
					// may save a new value to that copy.
					saved = true
					record := migrationRecord{}
					if tt.copied == "legacy" {
						record = migratedFrom("legacy-etag")
					} else if tt.cleared {
						record = migrationRecord{State: recordMigrated}
					}
					putEntry(t, ctx, c, current, tt.copied, record)
					if err := c.Update(ctx, obj, opts...); err != nil {
						return err
					}

					if tt.later {
						putEntry(t, ctx, c, legacy, "later", migrationRecord{})
					}
					return nil
				},
			}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

			options := []database.SaveOptions{}
			if tt.conditional {
				options = append(options, database.WithETag("legacy-etag"))
			}
			obj := &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": "updated"}}
			err := client.Save(t.Context(), obj, options...)
			require.True(t, saved)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tt.copied == "legacy" {
				// The copy is again a copy of the legacy value, so a later change of the legacy value replaces it.
				require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): {State: recordMigrated, ETag: obj.ETag, Fence: 1}}, recordsOf(t, listBuckets(t, raw)[current]))
			}

			stored, err := client.Get(t.Context(), migrateTarget.String())
			require.NoError(t, err)
			require.Equal(t, map[string]any{"format": tt.stored}, stored.Data)
			objects := slices.Clone(tt.objects)
			slices.Sort(objects)
			require.Equal(t, objects, bucketNames(listBuckets(t, raw)))

			result, err := client.MigrateLegacyNames(t.Context(), nil)
			require.NoError(t, err)
			require.Equal(t, tt.migrated, result)
			stored, err = client.Get(t.Context(), migrateTarget.String())
			require.NoError(t, err)
			require.Equal(t, map[string]any{"format": tt.stored}, stored.Data, "the migration must keep the value reads returned")
		})
	}
}

// A save writes to the legacy object while a delete removes the entry, after the delete wrote its tombstone, and a
// migrator reads both objects before the delete removes the saved value. The save must keep the tombstone, so the
// migrator cannot bring back the entry after the delete returned.
func Test_APIServerClient_Save_KeepsTombstone(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	_, raw := newMigrateClient(t, interceptor.Funcs{}, legacyValue(t, "legacy"))
	background := context.WithoutCancel(t.Context())
	saver, savePoints := pausingSaver(raw)
	skip(savePoints.write, savePoints.reread)
	saveWrote := savePoints.wrote

	// The save writes the legacy object after the delete wrote its tombstone. The delete then fails to remove the value
	// the save replaced, reads both objects again, and pauses.
	saved, deleteRead := make(chan error, 1), newPausePoint()
	started, legacyReads := false, 0
	deleter := NewAPIServerClient(interceptor.NewClient(raw, interceptor.Funcs{
		Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			err := c.Get(ctx, key, obj, opts...)
			if key.Name == legacy {
				legacyReads++
				if legacyReads == 2 {
					deleteRead.pause()
				}
			}
			return err
		},
		Delete: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			if obj.GetName() == legacy && !started {
				started = true
				go func() {
					saved <- saver.Save(background, valueOf("saved"), database.WithETag(etag.New([]byte(`{"format":"legacy"}`))))
				}()
				<-saveWrote.reached
			}
			return c.Delete(ctx, obj, opts...)
		},
	}), migrateNamespace)
	deleted := make(chan error, 1)
	go func() {
		deleted <- deleter.Delete(background, migrateTarget.String())
	}()
	<-deleteRead.reached

	close(saveWrote.resume)
	require.NoError(t, <-saved)
	require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): tombstone}, recordsOf(t, listBuckets(t, raw)[current]), "the save must keep the tombstone")

	// A migrator reads both objects before the delete removes the saved value, and writes after the delete returned.
	migratorRead := newPausePoint()
	migrator := NewAPIServerClient(interceptor.NewClient(raw, interceptor.Funcs{
		Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			err := c.Get(ctx, key, obj, opts...)
			if key.Name == legacy {
				migratorRead.pause()
			}
			return err
		},
	}), migrateNamespace)
	migrated := make(chan error, 1)
	go func() {
		_, err := migrator.MigrateLegacyNames(background, nil)
		migrated <- err
	}()
	<-migratorRead.reached

	close(deleteRead.resume)
	require.NoError(t, <-deleted)
	close(migratorRead.resume)
	require.NoError(t, <-migrated)
	requireDeleted(t, NewAPIServerClient(raw, migrateNamespace), raw)
}

// A save that wrote the legacy object reconciles only after later saves replaced its value and the migration moved the
// entry. Its write is older than the later saves, so it must not replace their value, even if that value equals the
// value the save replaced.
func Test_APIServerClient_Save_DelayedReconcile(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string

		// toCurrent saves the later value to the moved entry, rather than to the legacy object before the move.
		toCurrent bool
	}{
		{name: "later value saved to the legacy object"},
		{name: "later value saved to the current object", toCurrent: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, raw := newMigrateClient(t, interceptor.Funcs{}, legacyValue(t, "legacy"))
			delayed, points := pausingSaver(raw)
			skip(points.write, points.reread)
			wrote := points.wrote
			saved := make(chan error, 1)
			go func() {
				saved <- delayed.Save(context.WithoutCancel(t.Context()), valueOf("delayed"), database.WithETag(etag.New([]byte(`{"format":"legacy"}`))))
			}()
			<-wrote.reached

			client := NewAPIServerClient(raw, migrateNamespace)
			obj, err := client.Get(t.Context(), migrateTarget.String())
			require.NoError(t, err)
			later := valueOf("legacy")
			if tt.toCurrent {
				require.NoError(t, client.Save(t.Context(), valueOf("other"), database.WithETag(obj.ETag)))
			} else {
				require.NoError(t, client.Save(t.Context(), later, database.WithETag(obj.ETag)))
			}
			result, err := client.MigrateLegacyNames(t.Context(), nil)
			require.NoError(t, err)
			require.Equal(t, hashmigration.Result{Migrated: 1}, result)
			if tt.toCurrent {
				require.NoError(t, client.Save(t.Context(), later))
			}

			close(wrote.resume)
			require.NoError(t, <-saved)
			obj, err = client.Get(t.Context(), migrateTarget.String())
			require.NoError(t, err)
			require.Equal(t, later.Data, obj.Data, "the later value must be kept")
			require.Equal(t, later.ETag, obj.ETag)
			require.Equal(t, []string{resourceName(migrateTarget)}, bucketNames(listBuckets(t, raw)))
		})
	}
}

// Two saves write the legacy object, one after the other, after a migrator read it, and reconcile after the migrator
// copied the value they replaced. The earlier save reads the legacy object before the later save writes it, and writes
// the copy only after the later save reconciled, or after the migrator removed the legacy object. The later value must
// be kept, even if it equals the copy.
func Test_APIServerClient_Save_ConcurrentReconciles(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	for _, tt := range []struct {
		name string

		// migratorFirst lets the migrator remove the legacy object before the earlier save writes the copy, and the
		// later save reconcile last. Otherwise both saves return before the migrator finishes.
		migratorFirst bool
	}{
		{name: "later save reconciles first"},
		{name: "migrator removes the legacy object first", migratorFirst: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, raw := newMigrateClient(t, interceptor.Funcs{}, legacyValue(t, "legacy"))
			background := context.WithoutCancel(t.Context())

			// The migrator pauses after it read both objects, and after it wrote its copy.
			migratorRead, migratorWrote := newPausePoint(), newPausePoint()
			migrator := NewAPIServerClient(interceptor.NewClient(raw, interceptor.Funcs{
				Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					err := c.Get(ctx, key, obj, opts...)
					if key.Name == legacy {
						migratorRead.pause()
					}
					return err
				},
				Create: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
					err := c.Create(ctx, obj, opts...)
					if err == nil && obj.GetName() == current {
						migratorWrote.pause()
					}
					return err
				},
			}), migrateNamespace)
			type outcome struct {
				result hashmigration.Result
				err    error
			}
			migrated := make(chan outcome, 1)
			go func() {
				result, err := migrator.MigrateLegacyNames(background, nil)
				migrated <- outcome{result, err}
			}()
			<-migratorRead.reached

			earlier, earlierPoints := pausingSaver(raw)
			skip(earlierPoints.write)
			earlierSaved := make(chan error, 1)
			go func() {
				earlierSaved <- earlier.Save(background, valueOf("earlier"), database.WithETag(etag.New([]byte(`{"format":"legacy"}`))))
			}()
			<-earlierPoints.wrote.reached

			// The later save reads both objects before the migrator writes its copy, and writes the value it copies.
			laterClient, laterPoints := pausingSaver(raw)
			skip(laterPoints.reread)
			later, laterSaved := valueOf("legacy"), make(chan error, 1)
			go func() {
				laterSaved <- laterClient.Save(background, later, database.WithETag(etag.New([]byte(`{"format":"earlier"}`))))
			}()
			<-laterPoints.write.reached

			close(migratorRead.resume)
			<-migratorWrote.reached
			close(earlierPoints.wrote.resume)
			<-earlierPoints.reread.reached
			close(laterPoints.write.resume)
			<-laterPoints.wrote.reached

			finishMigrator := func() {
				close(migratorWrote.resume)
				finished := <-migrated
				require.NoError(t, finished.err)
				require.Equal(t, hashmigration.Result{Migrated: 1}, finished.result)
			}
			requireLater := func() {
				obj, err := NewAPIServerClient(raw, migrateNamespace).Get(t.Context(), migrateTarget.String())
				require.NoError(t, err)
				require.Equal(t, later.Data, obj.Data, "the later value must be kept")
				require.Equal(t, later.ETag, obj.ETag)
			}
			if tt.migratorFirst {
				finishMigrator()
				close(earlierPoints.reread.resume)
				require.NoError(t, <-earlierSaved)
				close(laterPoints.wrote.resume)
				require.NoError(t, <-laterSaved)
			} else {
				// Both saves return before the migrator removes the legacy object, and reads must already return the
				// later value.
				close(laterPoints.wrote.resume)
				require.NoError(t, <-laterSaved)
				close(earlierPoints.reread.resume)
				require.NoError(t, <-earlierSaved)
				requireLater()
				finishMigrator()
			}

			requireLater()
			require.Equal(t, []string{current}, bucketNames(listBuckets(t, raw)))
		})
	}
}

// A save that read the entry from its legacy object retries when the migration moved the entry and deleted that object
// before the save wrote it.
func Test_APIServerClient_Save_AfterMoveDeletedLegacyObject(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	moved := false
	client, raw := newMigrateClient(t, interceptor.Funcs{
		Update: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			if obj.GetName() == legacy && !moved {
				moved = true
				putEntry(t, ctx, c, current, "legacy", migratedFrom("legacy-etag"))
				stale := &ucpv1alpha1.Resource{}
				require.NoError(t, c.Get(ctx, runtimeclient.ObjectKey{Namespace: migrateNamespace, Name: legacy}, stale))
				require.NoError(t, c.Delete(ctx, stale))
			}
			return c.Update(ctx, obj, opts...)
		},
	}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

	saved := &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": "saved"}}
	require.NoError(t, client.Save(t.Context(), saved, database.WithETag("legacy-etag")))
	require.True(t, moved)
	obj, err := client.Get(t.Context(), migrateTarget.String())
	require.NoError(t, err)
	require.Equal(t, saved.ETag, obj.ETag)
	require.Equal(t, []string{current}, bucketNames(listBuckets(t, raw)))
}

// A pass that stops right after it wrote a copy leaves the entry in both objects. Reads return the copy, a later pass
// finishes the move, and a delete removes both copies.
func Test_APIServerClient_MigrateLegacyNames_Interrupted(t *testing.T) {
	t.Parallel()
	current, legacy := resourceName(migrateTarget), legacyResourceName(migrateTarget)
	for _, then := range []string{"pass", "delete"} {
		t.Run(then, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			client, raw := newMigrateClient(t, interceptor.Funcs{
				Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					return c.Get(ctx, key, obj, opts...)
				},
				Update: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					return c.Update(ctx, obj, opts...)
				},
				Delete: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					return c.Delete(ctx, obj, opts...)
				},
				Create: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					if err := c.Create(ctx, obj, opts...); err != nil {
						return err
					}

					// The component shuts down right after the copy was written.
					cancel()
					return nil
				},
			}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

			result, err := client.MigrateLegacyNames(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, hashmigration.Result{Remaining: 1, Message: context.Canceled.Error()}, result)
			require.ElementsMatch(t, []string{current, legacy}, bucketNames(listBuckets(t, raw)))
			obj, err := client.Get(t.Context(), migrateTarget.String())
			require.NoError(t, err)
			require.Equal(t, "legacy-etag", obj.ETag)

			if then == "delete" {
				require.NoError(t, client.Delete(t.Context(), migrateTarget.String(), database.WithETag("legacy-etag")))
				requireDeleted(t, client, raw)
				result, err = client.MigrateLegacyNames(t.Context(), nil)
				require.NoError(t, err)
				require.Equal(t, hashmigration.Result{}, result)
				return
			}

			result, err = client.MigrateLegacyNames(t.Context(), nil)
			require.NoError(t, err)
			require.Equal(t, hashmigration.Result{Deduplicated: 1}, result)
			buckets := listBuckets(t, raw)
			require.Equal(t, []string{current}, bucketNames(buckets))
			// The deduplication changed the record of the copy before it removed the legacy copy.
			require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): {State: recordMigrated, ETag: "legacy-etag", Fence: 1}}, recordsOf(t, buckets[current]))
		})
	}
}

func Test_APIServerClient_MigrateLegacyNames_ConcurrentMigrators(t *testing.T) {
	t.Parallel()
	var other hashmigration.Result
	ran := false
	client, raw := newMigrateClient(t, interceptor.Funcs{
		Create: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
			if err := c.Create(ctx, obj, opts...); err != nil {
				return err
			}
			if ran {
				return nil
			}

			// Another migrator, such as one in a second UCP replica, finds both copies right after the move wrote the
			// current copy, and removes the legacy copy.
			ran = true
			var err error
			other, err = NewAPIServerClient(c, migrateNamespace).MigrateLegacyNames(ctx, nil)
			return err
		},
	}, fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget}))

	// The first migrator then finds the legacy copy gone, and must not lose the entry.
	result, err := client.MigrateLegacyNames(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, ran)
	require.Equal(t, hashmigration.Result{Deduplicated: 1}, other)
	require.Equal(t, hashmigration.Result{}, result)

	obj, err := client.Get(t.Context(), migrateTarget.String())
	require.NoError(t, err, "the entry must not be lost")
	require.Equal(t, "legacy-etag", obj.ETag)
	buckets := listBuckets(t, raw)
	require.Equal(t, []string{resourceName(migrateTarget)}, bucketNames(buckets))
	// The other migrator changed the record of the copy before it removed the legacy copy.
	require.Equal(t, map[string]migrationRecord{recordKey(migrateTarget): {State: recordMigrated, ETag: "legacy-etag", Fence: 1}}, recordsOf(t, buckets[resourceName(migrateTarget)]))
}

func Test_APIServerClient_OperationsDuringMove(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"get", "save", "delete"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			legacy := legacyResourceName(migrateTarget)
			moved := false
			client, raw := newMigrateClient(t, interceptor.Funcs{
				Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					if key.Name == legacy && !moved {
						// The migration moves the entry after the current object was read and before the legacy object
						// is read, so the entry is in neither object when it is read.
						moved = true
						putEntry(t, ctx, c, resourceName(migrateTarget), "legacy", migratedFrom("legacy-etag"))
						stale := &ucpv1alpha1.Resource{}
						require.NoError(t, c.Get(ctx, key, stale))
						require.NoError(t, c.Delete(ctx, stale))
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

			switch operation {
			case "get":
				obj, err := client.Get(t.Context(), migrateTarget.String())
				require.NoError(t, err, "a moved entry must not be reported as missing")
				require.Equal(t, "legacy-etag", obj.ETag)
			case "save":
				require.NoError(t, client.Save(t.Context(), &database.Object{ID: migrateTarget.String(), Data: map[string]any{"format": "updated"}}, database.WithETag("legacy-etag")))
				buckets := listBuckets(t, raw)
				require.Equal(t, []string{resourceName(migrateTarget)}, bucketNames(buckets))
				require.Len(t, buckets[resourceName(migrateTarget)].Entries, 1)
				require.JSONEq(t, `{"format":"updated"}`, string(buckets[resourceName(migrateTarget)].Entries[0].Data.Raw))
			case "delete":
				require.NoError(t, client.Delete(t.Context(), migrateTarget.String(), database.WithETag("legacy-etag")))
				requireDeleted(t, client, raw)
			}
			require.True(t, moved)
		})
	}
}

func Test_APIServerClient_Enumerate(t *testing.T) {
	t.Parallel()
	client, _ := newMigrateClient(t, interceptor.Funcs{},
		fallbackTestResource(t, migrateNamespace, resourceName(migrateTarget), "current", []resources.ID{migrateTarget, migrateOther}),
		fallbackTestResource(t, migrateNamespace, resourceName(migrateGroup), "group", []resources.ID{migrateGroup}),
	)

	ids := []string{}
	require.NoError(t, client.Enumerate(t.Context(), func(obj *database.Object) error {
		ids = append(ids, obj.ID)
		require.Equal(t, map[string]any{"format": obj.ETag[:len(obj.ETag)-len("-etag")]}, obj.Data)
		return nil
	}))
	require.ElementsMatch(t, []string{migrateTarget.String(), migrateOther.String(), migrateGroup.String()}, ids)
}

func bucketNames(buckets map[string]*ucpv1alpha1.Resource) []string {
	names := []string{}
	for name := range buckets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
