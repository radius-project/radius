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
	"encoding/json"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/components/database"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func Test_APIServerClient_HashFallback(t *testing.T) {
	t.Parallel()

	id := resources.MustParse("/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/target")
	other := resources.MustParse("/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/other")
	tests := []struct {
		name    string
		current []resources.ID
		legacy  []resources.ID
		stored  string
	}{
		{name: "current only", current: []resources.ID{id}, stored: "current"},
		{name: "legacy only", legacy: []resources.ID{id}, stored: "legacy"},
		{name: "current wins", current: []resources.ID{id}, legacy: []resources.ID{id}, stored: "current"},
		{name: "neither exists"},
		{name: "current collision with legacy target", current: []resources.ID{other}, legacy: []resources.ID{id, other}, stored: "legacy"},
		{name: "empty current with legacy target", current: []resources.ID{}, legacy: []resources.ID{id}, stored: "legacy"},
		{name: "current collision without target", current: []resources.ID{other}},
		{name: "legacy collision without target", legacy: []resources.ID{other}},
		{name: "both collisions without target", current: []resources.ID{other}, legacy: []resources.ID{other}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, operation := range []string{"get", "save", "conditional save", "delete", "conditional delete", "stale save", "stale delete"} {
				t.Run(operation, func(t *testing.T) {
					t.Parallel()
					const namespace = "fallback-test"
					scheme := runtime.NewScheme()
					require.NoError(t, ucpv1alpha1.AddToScheme(scheme))
					reads := []string{}
					raw := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, client runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
							reads = append(reads, key.Name)
							return client.Get(ctx, key, obj, opts...)
						},
					}).Build()
					before := map[string]*ucpv1alpha1.Resource{}
					for format, ids := range map[string][]resources.ID{"current": tt.current, "legacy": tt.legacy} {
						if ids == nil {
							continue
						}
						name := resourceName(id)
						if format == "legacy" {
							name = legacyResourceName(id)
						}
						bucket := fallbackTestResource(t, namespace, name, format, ids)
						require.NoError(t, raw.Create(t.Context(), bucket))
						before[name] = bucket.DeepCopy()
					}
					client := NewAPIServerClient(raw, namespace)
					selected := resourceName(id)
					if tt.stored == "legacy" {
						selected = legacyResourceName(id)
					}
					expectedETag := tt.stored + "-etag"
					obj := &database.Object{ID: id.String(), Data: map[string]any{"format": "updated"}}
					var err error
					var wantErr error
					switch operation {
					case "get":
						var got *database.Object
						got, err = client.Get(t.Context(), id.String())
						if tt.stored == "" {
							wantErr = &database.ErrNotFound{}
						} else if err == nil {
							require.Equal(t, expectedETag, got.ETag)
							require.Equal(t, map[string]any{"format": tt.stored}, got.Data)
						}
					case "save":
						err = client.Save(t.Context(), obj)
					case "conditional save":
						err = client.Save(t.Context(), obj, database.WithETag(expectedETag))
						if tt.stored == "" {
							wantErr = &database.ErrConcurrency{}
						}
					case "delete":
						err = client.Delete(t.Context(), id.String())
						if tt.stored == "" {
							wantErr = &database.ErrNotFound{}
						}
					case "conditional delete":
						err = client.Delete(t.Context(), id.String(), database.WithETag(expectedETag))
						if tt.stored == "" {
							wantErr = &database.ErrNotFound{}
							if tt.current == nil && tt.legacy == nil {
								wantErr = &database.ErrConcurrency{}
							}
						}
					case "stale save":
						err = client.Save(t.Context(), obj, database.WithETag("stale"))
						wantErr = &database.ErrConcurrency{}
					case "stale delete":
						err = client.Delete(t.Context(), id.String(), database.WithETag("stale"))
						wantErr = &database.ErrConcurrency{}
						if tt.stored == "" && (tt.current != nil || tt.legacy != nil) {
							wantErr = &database.ErrNotFound{}
						}
					}
					if wantErr != nil {
						require.ErrorIs(t, err, wantErr)
					} else {
						require.NoError(t, err)
					}
					expectedReads := []string{resourceName(id)}
					if tt.stored != "current" {
						expectedReads = append(expectedReads, legacyResourceName(id))
					}
					require.Equal(t, expectedReads, reads)

					all := &ucpv1alpha1.ResourceList{}
					require.NoError(t, raw.List(t.Context(), all, runtimeclient.InNamespace(namespace)))
					after := map[string]*ucpv1alpha1.Resource{}
					for i := range all.Items {
						after[all.Items[i].Name] = &all.Items[i]
					}
					if wantErr != nil || operation == "get" {
						require.Equal(t, before, after, "reads and rejected mutations must leave all buckets intact")
						return
					}

					for name, bucket := range before {
						if name != selected {
							require.Equal(t, bucket, after[name], "the unselected bucket must not change")
						}
					}
					wantCount := len(before)
					previous := before[selected]
					if operation == "delete" || operation == "conditional delete" {
						if len(previous.Entries) == 1 {
							require.NotContains(t, after, selected)
							wantCount--
						} else {
							require.Nil(t, findIndex(after[selected], id))
							require.Equal(t, previous.Entries[1:], after[selected].Entries)
							require.Equal(t, map[string]string(assignLabels(after[selected])), after[selected].Labels)
						}
					} else {
						bucket := after[selected]
						require.NotNil(t, bucket)
						index := findIndex(bucket, id)
						require.NotNil(t, index)
						require.Equal(t, obj.ETag, bucket.Entries[*index].ETag)
						require.JSONEq(t, `{"format":"updated"}`, string(bucket.Entries[*index].Data.Raw))
						require.Equal(t, map[string]string(assignLabels(bucket)), bucket.Labels)
						if previous == nil {
							wantCount++
							require.Len(t, bucket.Entries, 1)
						} else {
							wantEntries := len(previous.Entries)
							if findIndex(previous, id) == nil {
								wantEntries++
							}
							require.Len(t, bucket.Entries, wantEntries)
							for _, entry := range previous.Entries {
								if entry.ID != id.String() {
									require.Contains(t, bucket.Entries, entry)
								}
							}
						}
					}
					require.Len(t, after, wantCount, "do not create an alternate hash bucket")
				})
			}
		})
	}
}

func Test_APIServerClient_HashFallbackErrors(t *testing.T) {
	t.Parallel()

	id := resources.MustParse("/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/target")
	other := resources.MustParse("/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/other")
	for _, currentExists := range []bool{false, true} {
		for _, failCurrent := range []bool{false, true} {
			for _, operation := range []string{"get", "save", "delete"} {
				name := operation
				if currentExists {
					name += " current collision"
				}
				if failCurrent {
					name += " current error"
				} else {
					name += " legacy error"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					scheme := runtime.NewScheme()
					require.NoError(t, ucpv1alpha1.AddToScheme(scheme))
					failure := apierrors.NewForbidden(schema.GroupResource{Resource: "resources"}, "target", errors.New("denied"))
					reads := []string{}
					raw := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, client runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
							reads = append(reads, key.Name)
							if failCurrent || key.Name == legacyResourceName(id) {
								return failure
							}
							return client.Get(ctx, key, obj, opts...)
						},
					}).Build()
					if currentExists {
						require.NoError(t, raw.Create(t.Context(), fallbackTestResource(t, "fallback-test", resourceName(id), "current", []resources.ID{other})))
					}
					client := NewAPIServerClient(raw, "fallback-test")
					var err error
					switch operation {
					case "get":
						_, err = client.Get(t.Context(), id.String())
					case "save":
						err = client.Save(t.Context(), &database.Object{ID: id.String(), Data: map[string]any{}})
					case "delete":
						err = client.Delete(t.Context(), id.String())
					}
					require.ErrorIs(t, err, failure)
					expectedReads := []string{resourceName(id)}
					if !failCurrent {
						expectedReads = append(expectedReads, legacyResourceName(id))
					}
					require.Equal(t, expectedReads, reads)
					all := &ucpv1alpha1.ResourceList{}
					require.NoError(t, raw.List(t.Context(), all))
					if currentExists {
						require.Len(t, all.Items, 1)
						require.Nil(t, findIndex(&all.Items[0], id))
					} else {
						require.Empty(t, all.Items)
					}
				})
			}
		}
	}
}

func fallbackTestResource(t *testing.T, namespace, name, format string, ids []resources.ID) *ucpv1alpha1.Resource {
	t.Helper()
	data, err := json.Marshal(map[string]any{"format": format})
	require.NoError(t, err)
	bucket := &ucpv1alpha1.Resource{Name: name, Namespace: namespace}
	for _, id := range ids {
		bucket.Entries = append(bucket.Entries, ucpv1alpha1.ResourceEntry{
			ID: id.String(), ETag: format + "-etag", Data: &runtime.RawExtension{Raw: data},
		})
	}
	if len(bucket.Entries) > 0 {
		bucket.Labels = assignLabels(bucket)
	}
	return bucket
}
