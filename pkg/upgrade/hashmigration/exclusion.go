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
	"os"
	"sync"
	"time"
	"uuid"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	// exclusionLeaseDuration is how long another replica waits before it takes over the exclusion of a replica that
	// stopped without releasing it.
	exclusionLeaseDuration = time.Minute

	// exclusionRenewDeadline is how long the holder keeps retrying a renewal before it gives up the exclusion. It is
	// shorter than exclusionLeaseDuration, so the holder stops before another replica can take over.
	exclusionRenewDeadline = 40 * time.Second

	// exclusionRetryPeriod is how often the holder renews the exclusion and how often other replicas try to acquire it.
	exclusionRetryPeriod = 5 * time.Second
)

// Exclusion runs the migration passes of a component on one replica at a time.
type Exclusion interface {
	// Run calls fn once the replica holds the exclusion, and releases it when fn returns. The context passed to fn is
	// cancelled if the replica loses the exclusion. Run returns once fn has returned, or when ctx is done before the
	// exclusion is acquired.
	Run(ctx context.Context, fn func(ctx context.Context)) error
}

type exclusiveKey struct{}

// WithExclusive returns a context that tells steps that no other replica of the component runs the migration.
func WithExclusive(ctx context.Context) context.Context {
	return context.WithValue(ctx, exclusiveKey{}, true)
}

// IsExclusive reports whether no other replica of the component runs the migration. A step may undo a write of its own
// when a concurrent client operation removed the value it migrated, such as a delete, only when it is: otherwise the
// migration of the same value by another replica cannot be told apart from the client operation.
func IsExclusive(ctx context.Context) bool {
	exclusive, _ := ctx.Value(exclusiveKey{}).(bool)
	return exclusive
}

// LeaseExclusion is an Exclusion that holds the Lease named LeaseName(component) with Kubernetes leader election.
type LeaseExclusion struct {
	client    kubernetes.Interface
	namespace string
	name      string
	identity  string

	leaseDuration time.Duration
	renewDeadline time.Duration
	retryPeriod   time.Duration
}

var _ Exclusion = (*LeaseExclusion)(nil)

// LeaseName returns the name of the Lease that holds the exclusion of component.
func LeaseName(component string) string {
	return ConfigMapName + "-" + component
}

// NewLeaseExclusion creates a LeaseExclusion for component in namespace.
func NewLeaseExclusion(client kubernetes.Interface, namespace string, component string) *LeaseExclusion {
	// The host name identifies the pod in logs, and the UUID keeps the identity unique.
	host, _ := os.Hostname()
	return &LeaseExclusion{
		client:        client,
		namespace:     namespace,
		name:          LeaseName(component),
		identity:      host + "_" + uuid.New().String(),
		leaseDuration: exclusionLeaseDuration,
		renewDeadline: exclusionRenewDeadline,
		retryPeriod:   exclusionRetryPeriod,
	}
}

// Run implements Exclusion.
func (e *LeaseExclusion) Run(ctx context.Context, fn func(ctx context.Context)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Leader election calls OnStartedLeading on another goroutine. running lets Run wait for fn, and stopped keeps fn
	// from starting once Run returns, so fn never runs after the exclusion is released.
	var mu sync.Mutex
	var running sync.WaitGroup
	stopped := false
	elector, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock: &resourcelock.LeaseLock{
			LeaseMeta:  metav1.ObjectMeta{Name: e.name, Namespace: e.namespace},
			Client:     e.client.CoordinationV1(),
			LockConfig: resourcelock.ResourceLockConfig{Identity: e.identity},
		},
		LeaseDuration:   e.leaseDuration,
		RenewDeadline:   e.renewDeadline,
		RetryPeriod:     e.retryPeriod,
		ReleaseOnCancel: true,
		Name:            e.name,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(leaderCtx context.Context) {
				mu.Lock()
				if stopped {
					mu.Unlock()
					return
				}
				running.Add(1)
				mu.Unlock()
				defer running.Done()

				fn(leaderCtx)

				// Release the exclusion.
				cancel()
			},
			OnStoppedLeading: func() {},
		},
	})
	if err != nil {
		return err
	}

	elector.Run(ctx)

	mu.Lock()
	stopped = true
	mu.Unlock()
	running.Wait()
	return nil
}
