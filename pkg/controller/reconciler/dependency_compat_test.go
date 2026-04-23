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
	"testing"

	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// Test_ControllerRuntime_ClientGo_Compatibility is a compile-time regression test that
// verifies sigs.k8s.io/controller-runtime is compatible with the k8s.io/client-go version
// used by this project.
//
// This test was added after a dependency bump of k8s.io/* to v0.36.0 broke the build because
// controller-runtime v0.23.3 did not implement the HasSyncedChecker method required by the
// k8s.io/client-go/tools/cache.ResourceEventHandlerRegistration interface in v0.36.0.
//
// If this test fails to compile, it means the controller-runtime version is not compatible
// with the k8s.io/client-go version. Update the controller-runtime replace directive in go.mod
// (or upgrade to a released version) to restore compatibility.
func Test_ControllerRuntime_ClientGo_Compatibility(t *testing.T) {
	// Verify that the controller-runtime cache.Informer interface is assignable to a variable
	// whose method set includes AddEventHandler returning toolscache.ResourceEventHandlerRegistration.
	// This is a compile-time check: if the interfaces are incompatible, this file will not compile.
	var _ interface {
		AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error)
	} = (cache.Informer)(nil)
}
