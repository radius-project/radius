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
	"testing"

	"github.com/radius-project/radius/pkg/components/database"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/ucp/resources"
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

func newMigrateClient(t *testing.T, funcs interceptor.Funcs, objects ...*ucpv1alpha1.Resource) (*APIServerClient, runtimeclient.WithWatch) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, ucpv1alpha1.AddToScheme(scheme))
	initial := make([]runtimeclient.Object, 0, len(objects))
	for _, object := range objects {
		initial = append(initial, object)
	}
	// Seeded objects bypass the interceptors so tests can intercept only the migration's own writes.
	raw := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initial...).WithInterceptorFuncs(funcs).Build()
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
			},
		},
		{
			name: "current only",
			objects: func(t *testing.T) []*ucpv1alpha1.Resource {
				return []*ucpv1alpha1.Resource{fallbackTestResource(t, migrateNamespace, current, "current", []resources.ID{migrateTarget})}
			},
			verify: func(t *testing.T, buckets map[string]*ucpv1alpha1.Resource) {
				require.Equal(t, []string{current}, bucketNames(buckets))
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
			result, err := client.MigrateLegacyNames(t.Context())
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
			buckets := listBuckets(t, raw)
			tt.verify(t, buckets)

			// A second pass is a no-op apart from values that cannot be migrated.
			again, err := client.MigrateLegacyNames(t.Context())
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

func Test_APIServerClient_MigrateLegacyNames_PromotesLegacyUpdate(t *testing.T) {
	t.Parallel()
	legacy := legacyResourceName(migrateTarget)
	updated := false
	var raw runtimeclient.WithWatch
	client, raw := newMigrateClient(t, interceptor.Funcs{
		Create: func(ctx context.Context, c runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
			if err := c.Create(ctx, obj, opts...); err != nil {
				return err
			}
			if !updated {
				// A writer that only knows legacy names updates the legacy copy after the migration copied it.
				updated = true
				bucket := &ucpv1alpha1.Resource{}
				if err := c.Get(ctx, runtimeclient.ObjectKey{Namespace: migrateNamespace, Name: legacy}, bucket); err != nil {
					return err
				}
				bucket.Entries[0].ETag = "newer-etag"
				bucket.Entries[0].Data = &runtime.RawExtension{Raw: []byte(`{"format":"newer"}`)}
				return c.Update(ctx, bucket)
			}
			return nil
		},
	}, fallbackTestResource(t, migrateNamespace, legacy, "legacy", []resources.ID{migrateTarget}))

	result, err := client.MigrateLegacyNames(t.Context())
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Migrated: 1}, result)
	buckets := listBuckets(t, raw)
	require.Equal(t, []string{resourceName(migrateTarget)}, bucketNames(buckets))
	require.Equal(t, "newer-etag", buckets[resourceName(migrateTarget)].Entries[0].ETag, "the newer legacy value must win over the stale copy")
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
		_, err := client.MigrateLegacyNames(t.Context())
		require.ErrorIs(t, err, failure)
	})

	t.Run("entry", func(t *testing.T) {
		t.Parallel()
		client, raw := newMigrateClient(t, interceptor.Funcs{
			Create: func(context.Context, runtimeclient.WithWatch, runtimeclient.Object, ...runtimeclient.CreateOption) error {
				return failure
			},
		}, fallbackTestResource(t, migrateNamespace, legacyResourceName(migrateTarget), "legacy", []resources.ID{migrateTarget}))
		result, err := client.MigrateLegacyNames(t.Context())
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
		result, err := client.MigrateLegacyNames(t.Context())
		require.NoError(t, err)
		require.Equal(t, hashmigration.Result{Remaining: 1}, result)
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
			if tt.shared {
				require.Equal(t, []string{legacyResourceName(migrateTarget)}, bucketNames(buckets))
				require.Len(t, buckets[legacyResourceName(migrateTarget)].Entries, 1)
				require.Equal(t, migrateOther.String(), buckets[legacyResourceName(migrateTarget)].Entries[0].ID)
			} else {
				require.Empty(t, buckets)
			}
		})
	}
}

func Test_APIServerClient_MigrateLegacyNames_ConcurrentDelete(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		exclusive bool

		// afterCopy deletes the resource right after the move writes the current copy, instead of right before.
		afterCopy bool

		expected hashmigration.Result

		// ghost records that the copy the move wrote remains although the resource was deleted.
		ghost bool
	}{
		{name: "before the copy", exclusive: true},
		{name: "after the copy", exclusive: true, afterCopy: true},
		{name: "after the copy without exclusion", afterCopy: true, expected: hashmigration.Result{Migrated: 1}},
		// Without exclusion, the delete cannot be told apart from another replica moving the entry, so the copy stays.
		{name: "before the copy without exclusion", expected: hashmigration.Result{Migrated: 1}, ghost: true},
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
			ctx := t.Context()
			if tt.exclusive {
				ctx = hashmigration.WithExclusive(ctx)
			}

			result, err := client.MigrateLegacyNames(ctx)
			require.NoError(t, err)
			require.True(t, deleted)
			require.Equal(t, tt.expected, result)
			_, err = client.Get(t.Context(), migrateTarget.String())
			if tt.ghost {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, &database.ErrNotFound{}, "the move must not bring back a deleted resource")
			require.Empty(t, listBuckets(t, raw))
		})
	}
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
						require.NoError(t, c.Create(ctx, fallbackTestResource(t, migrateNamespace, resourceName(migrateTarget), "legacy", []resources.ID{migrateTarget})))
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
				require.Empty(t, listBuckets(t, raw))
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
