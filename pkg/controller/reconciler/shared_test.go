/*
Copyright 2023.

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

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// managerShutdownTimeout bounds how long test cleanup waits for a controller-runtime manager to
	// exit after its context is cancelled. It is comfortably above controller-runtime's default 30s
	// graceful shutdown timeout, so a clean shutdown always finishes first. If it is exceeded, cleanup
	// reports a failure instead of hanging until the global `go test` timeout, which would obscure the
	// root cause.
	managerShutdownTimeout = time.Second * 60
)

// startManager starts the controller-runtime manager in a background goroutine and registers a
// t.Cleanup that cancels the manager's context and then blocks until the manager goroutine has
// fully exited (or managerShutdownTimeout elapses).
//
// Awaiting shutdown matters: cancelling the context only signals the manager to stop, it does not
// wait for mgr.Start to return. Without this wait, manager goroutines outlive the test that owns
// them and race with subsequent tests and package teardown (env.Stop in TestMain), which made this
// suite intermittently fail at the package level with no named failing test.
func startManager(t *testing.T, mgr manager.Manager, ctx context.Context, cancel context.CancelFunc) {
	t.Helper()

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		// Don't use require/assert here: they can call t.FailNow/t.Fail, which must run on the test
		// goroutine, not this background one.
		if err := mgr.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			panic(fmt.Sprintf("manager exited with error: %v", err))
		}
	}()

	t.Cleanup(func() {
		cancel()
		waitForManagerShutdown(t, stopped)
	})
}

// waitForManagerShutdown blocks until the manager goroutine signals it has exited by closing
// stopped, or fails the test if that does not happen within managerShutdownTimeout.
func waitForManagerShutdown(t *testing.T, stopped <-chan struct{}) {
	t.Helper()

	select {
	case <-stopped:
	case <-time.After(managerShutdownTimeout):
		t.Errorf("timed out after %s waiting for the controller-runtime manager to shut down", managerShutdownTimeout)
	}
}

func makeDeploymentTemplate(name types.NamespacedName, template, providerConfig string, parameters map[string]string) *radappiov1alpha3.DeploymentTemplate {
	return &radappiov1alpha3.DeploymentTemplate{
		Namespace: name.Namespace,
		Name:      name.Name,
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			ProviderConfig: providerConfig,
			Parameters:     parameters,
		},
	}
}

func makeDeploymentResource(name types.NamespacedName, id string) *radappiov1alpha3.DeploymentResource {
	return &radappiov1alpha3.DeploymentResource{
		Namespace: name.Namespace,
		Name:      name.Name,
		Spec: radappiov1alpha3.DeploymentResourceSpec{
			Id: id,
		},
	}
}
