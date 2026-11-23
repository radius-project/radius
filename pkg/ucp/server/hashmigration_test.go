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

package server

import (
	"context"
	"errors"
	"os"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type recordingReporter struct {
	statuses map[string]hashmigration.Status
}

func (r *recordingReporter) Report(_ context.Context, key string, status hashmigration.Status) error {
	r.statuses[key] = status
	return nil
}

type migratingClient struct {
	*inmemory.Client
	result hashmigration.Result
	err    error
	calls  int
	gate   hashmigration.Gate
}

func (c *migratingClient) MigrateLegacyNames(_ context.Context, gate hashmigration.Gate) (hashmigration.Result, error) {
	c.calls++
	c.gate = gate
	return c.result, c.err
}

func Test_HashMigrationService(t *testing.T) {
	t.Parallel()
	group := "/planes/radius/local/resourceGroups/test-group"
	original := resources.MustParse(group + "/providers/Applications.Test/testResources/my-resource")

	t.Run("re-keys tracked resources; the data store step does not apply to other providers", func(t *testing.T) {
		t.Parallel()
		client := inmemory.NewClient()
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: group, Data: map[string]any{"name": "test-group"}}))
		entry := datamodel.GenericResourceFromID(original, trackedresource.LegacyIDFor(original))
		entry.AsyncProvisioningState = v1.ProvisioningStateSucceeded
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: trackedresource.LegacyIDFor(original).String(), Data: entry}))

		reporter := &recordingReporter{statuses: map[string]hashmigration.Status{}}
		require.True(t, newHashMigrationService(databaseprovider.FromClient(client), reporter, nil).RunPass(t.Context()))

		tracked := reporter.statuses["ucp.trackedResources"]
		require.Equal(t, hashmigration.StateComplete, tracked.State)
		require.Equal(t, 1, tracked.Migrated)
		require.Equal(t, hashmigration.StateNotApplicable, reporter.statuses["ucp.datastore"].State)

		_, err := client.Get(t.Context(), trackedresource.IDFor(original).String())
		require.NoError(t, err)
		_, err = client.Get(t.Context(), trackedresource.LegacyIDFor(original).String())
		require.ErrorIs(t, err, &database.ErrNotFound{})
	})

	t.Run("runs the data store step for providers that derive names from hashes", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name     string
			result   hashmigration.Result
			err      error
			expected hashmigration.State
			done     bool
		}{
			{name: "complete", result: hashmigration.Result{Migrated: 2}, expected: hashmigration.StateComplete, done: true},
			{name: "remaining", result: hashmigration.Result{Remaining: 1}, expected: hashmigration.StateInProgress},
			{name: "error", err: errors.New("list failed"), expected: hashmigration.StateFailed},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				client := &migratingClient{Client: inmemory.NewClient(), result: tt.result, err: tt.err}
				reporter := &recordingReporter{statuses: map[string]hashmigration.Status{}}
				require.Equal(t, tt.done, newHashMigrationService(databaseprovider.FromClient(client), reporter, nil).RunPass(t.Context()))
				require.Equal(t, 1, client.calls)
				require.Equal(t, tt.expected, reporter.statuses["ucp.datastore"].State)
				require.Equal(t, tt.result.Migrated, reporter.statuses["ucp.datastore"].Migrated)
			})
		}
	})

	t.Run("waits for pods of other releases", func(t *testing.T) {
		t.Parallel()
		self, err := os.Hostname()
		require.NoError(t, err)
		// Each Deployment is at revision 2; pods of revision 1 belong to the earlier release.
		objects := []runtime.Object{}
		component := func(app string, pods map[string]string) {
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name: app, Namespace: hashmigration.DefaultNamespace, UID: types.UID(app),
				Annotations: map[string]string{"deployment.kubernetes.io/revision": "2"},
			}}
			objects = append(objects, deployment)
			for _, revision := range []string{"1", "2"} {
				objects = append(objects, &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
					Name: app + "-" + revision, Namespace: hashmigration.DefaultNamespace, UID: types.UID(app + "-" + revision),
					Annotations:     map[string]string{"deployment.kubernetes.io/revision": revision},
					OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(deployment, appsv1.SchemeGroupVersion.WithKind("Deployment"))},
				}})
			}
			for name, revision := range pods {
				objects = append(objects, &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: name, Namespace: hashmigration.DefaultNamespace, Labels: map[string]string{hashmigration.LabelAppName: app},
						OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: app + "-" + revision, UID: types.UID(app + "-" + revision), Controller: new(true)}},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				})
			}
		}
		component(hashmigration.AppUCP, map[string]string{self: "2"})
		component(hashmigration.AppApplicationsRP, map[string]string{"applications-rp-old": "1"})
		cluster := fake.NewClientset(objects...)

		client := &migratingClient{Client: inmemory.NewClient()}
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: group, Data: map[string]any{"name": "test-group"}}))
		entry := datamodel.GenericResourceFromID(original, trackedresource.LegacyIDFor(original))
		entry.AsyncProvisioningState = v1.ProvisioningStateSucceeded
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: trackedresource.LegacyIDFor(original).String(), Data: entry}))
		reporter := &recordingReporter{statuses: map[string]hashmigration.Status{}}
		service := newHashMigrationService(databaseprovider.FromClient(client), reporter, func() (kubernetes.Interface, error) { return cluster, nil })

		// Only UCP writes tracked resource entries, so an Applications RP pod of another release does not block them.
		// Every component writes the data store, so its step waits for that pod.
		service.RunPass(t.Context())
		require.Equal(t, hashmigration.StateComplete, reporter.statuses["ucp.trackedResources"].State)
		require.NotNil(t, client.gate)
		waiting, err := client.gate.Wait(t.Context())
		require.NoError(t, err)
		require.Contains(t, waiting, "applications-rp-old")
	})

	t.Run("reports an unavailable database as a failure", func(t *testing.T) {
		t.Parallel()
		provider := databaseprovider.FromOptions(databaseprovider.Options{Provider: "unsupported"})
		reporter := &recordingReporter{statuses: map[string]hashmigration.Status{}}
		require.False(t, newHashMigrationService(provider, reporter, nil).RunPass(t.Context()))
		require.Equal(t, hashmigration.StateFailed, reporter.statuses["ucp.trackedResources"].State)
		require.Equal(t, hashmigration.StateFailed, reporter.statuses["ucp.datastore"].State)
	})
}

func Test_NewServer_HashMigration(t *testing.T) {
	t.Parallel()
	for _, disabled := range []bool{false, true} {
		options := &ucp.Options{
			Config:           &ucp.Config{SHA256Migration: hashmigration.Config{Disabled: disabled}},
			DatabaseProvider: databaseprovider.FromMemory(),
		}
		host, err := NewServer(t.Context(), options)
		require.NoError(t, err)

		found := false
		for _, service := range host.Services {
			if _, ok := service.(*hashmigration.Service); ok {
				found = true
			}
		}
		require.Equal(t, !disabled, found)
	}
}

func Test_LoadConfig_SHA256Migration(t *testing.T) {
	t.Parallel()
	config, err := ucp.LoadConfig([]byte("sha256Migration:\n  disabled: true\n"))
	require.NoError(t, err)
	require.True(t, config.SHA256Migration.Disabled)

	config, err = ucp.LoadConfig([]byte("server:\n  port: 9443\n"))
	require.NoError(t, err)
	require.False(t, config.SHA256Migration.Disabled, "the migration must be enabled unless explicitly disabled")
}
