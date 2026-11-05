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

package hashmigration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// newTestExclusion returns a LeaseExclusion with short durations, so tests do not wait for the production ones.
func newTestExclusion(client *fake.Clientset) *LeaseExclusion {
	exclusion := NewLeaseExclusion(client, DefaultNamespace, "test")
	exclusion.leaseDuration = 2 * time.Second
	exclusion.renewDeadline = time.Second
	exclusion.retryPeriod = 100 * time.Millisecond
	return exclusion
}

func exclusionLease(t *testing.T, client *fake.Clientset) *coordinationv1.Lease {
	t.Helper()
	lease, err := client.CoordinationV1().Leases(DefaultNamespace).Get(t.Context(), LeaseName("test"), metav1.GetOptions{})
	require.NoError(t, err)
	return lease
}

func Test_LeaseExclusion(t *testing.T) {
	t.Parallel()

	t.Run("runs and releases", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientset()
		exclusion := newTestExclusion(client)

		ran := false
		require.NoError(t, exclusion.Run(t.Context(), func(ctx context.Context) {
			ran = true
			require.NoError(t, ctx.Err())
			lease := exclusionLease(t, client)
			require.Equal(t, exclusion.identity, *lease.Spec.HolderIdentity)
		}))
		require.True(t, ran)
		require.Equal(t, "radius-sha256-migration-test", LeaseName("test"))

		// The exclusion is released once fn returns, so another replica acquires it without waiting for it to expire.
		lease := exclusionLease(t, client)
		require.True(t, lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "", "the exclusion must be released")
	})

	t.Run("one holder at a time", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientset()
		first, second := newTestExclusion(client), newTestExclusion(client)

		release := make(chan struct{})
		firstStarted := make(chan struct{})
		firstDone := make(chan error, 1)
		go func() {
			firstDone <- first.Run(t.Context(), func(context.Context) {
				close(firstStarted)
				<-release
			})
		}()
		<-firstStarted

		var secondRan atomic.Bool
		secondDone := make(chan error, 1)
		go func() {
			secondDone <- second.Run(t.Context(), func(context.Context) { secondRan.Store(true) })
		}()

		// The second replica waits while the first holds the exclusion.
		time.Sleep(500 * time.Millisecond)
		require.False(t, secondRan.Load(), "two replicas must not hold the exclusion at once")

		close(release)
		require.NoError(t, <-firstDone)
		require.NoError(t, <-secondDone)
		require.True(t, secondRan.Load())
	})

	t.Run("cancelled while waiting", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientset()
		holder := "another-replica"
		renewed := metav1.NewMicroTime(time.Now().Add(time.Hour))
		duration := int32(3600)
		_, err := client.CoordinationV1().Leases(DefaultNamespace).Create(t.Context(), &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: LeaseName("test"), Namespace: DefaultNamespace},
			Spec:       coordinationv1.LeaseSpec{HolderIdentity: &holder, RenewTime: &renewed, AcquireTime: &renewed, LeaseDurationSeconds: &duration},
		}, metav1.CreateOptions{})
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
		defer cancel()
		ran := false
		require.NoError(t, newTestExclusion(client).Run(ctx, func(context.Context) { ran = true }))
		require.False(t, ran, "fn must not run without the exclusion")
		require.Equal(t, holder, *exclusionLease(t, client).Spec.HolderIdentity)
	})

	t.Run("lost exclusion cancels fn", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientset()
		exclusion := newTestExclusion(client)

		// The fake clientset does not check resource versions. Once another replica takes over the Lease, reject the
		// renewals of this replica, as the API server does for an update of a version that was replaced.
		var takenOver atomic.Bool
		client.PrependReactor("update", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			lease := action.(k8stesting.UpdateAction).GetObject().(*coordinationv1.Lease)
			if takenOver.Load() && lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == exclusion.identity {
				return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"}, lease.Name, errors.New("the object has been modified"))
			}
			return false, nil, nil
		})

		var cancelled atomic.Bool
		require.NoError(t, exclusion.Run(t.Context(), func(ctx context.Context) {
			// Another replica takes over the Lease, for example after this one stalled past its expiry.
			lease := exclusionLease(t, client)
			other := "another-replica"
			renewed := metav1.NewMicroTime(time.Now().Add(time.Hour))
			lease.Spec.HolderIdentity = &other
			lease.Spec.RenewTime = &renewed
			_, err := client.CoordinationV1().Leases(DefaultNamespace).Update(t.Context(), lease, metav1.UpdateOptions{})
			require.NoError(t, err)
			takenOver.Store(true)

			select {
			case <-ctx.Done():
				cancelled.Store(true)
			case <-time.After(10 * time.Second):
			}
		}))
		require.True(t, cancelled.Load(), "fn must stop once the replica loses the exclusion")
	})
}
