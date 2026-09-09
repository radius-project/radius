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
	"os"
	"path/filepath"
	"testing"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/go-logr/logr"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

// config holds a reference to the rest config for the test environment.
var config *rest.Config

// scheme holds a reference to the scheme for the test environment.
var scheme *runtime.Scheme

// TestMain will be called before running any tests in the package.
//
// We're using this to ensure that one (and only one) copy of env-test is booted up.
func TestMain(m *testing.M) {
	// Provide controller-runtime with a logger up front. Without this, the manager's shutdown path
	// calls into the global logger before SetLogger is ever called, which triggers
	// controller-runtime's eventuallyFulfillRoot fallback: after a ~30s delay it prints a
	// "log.SetLogger(...) was never called" warning with a goroutine stack dump. That noisy, delayed
	// path showed up at the point of an intermittent package-level test failure. A discard logger is
	// safe here because it never writes to testing.T after a test completes.
	runtimelog.SetLogger(logr.Discard())

	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// We don't know how to start the envtest environment. Just go ahead and call the tests so they can skip.
		os.Exit(m.Run()) //nolint:forbidigo // this is OK inside the TestMain function.
		return
	}

	env := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deploy", "Chart", "crds", "radius"),
			filepath.Join("..", "..", "..", "test", "crd", "flux"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := env.Start()
	if err != nil {
		panic("failed to start envtest: " + err.Error())
	}

	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = radappiov1alpha3.AddToScheme(s)
	_ = sourcev1.AddToScheme(s)
	_ = apiextensionsv1.AddToScheme(s)

	config = cfg
	scheme = s

	exitCode := m.Run()

	// Note: CANNOT use defer here because os.Exit won't run defers.
	err = env.Stop()
	if err != nil {
		panic("failed to stop envtest" + err.Error())
	}

	os.Exit(exitCode) //nolint:forbidigo // this is OK inside the TestMain function.
}

func SkipWithoutEnvironment(t *testing.T) {
	if config == nil {
		t.Skip("Skipping test because envtest could not be started. Running `make test` will run tests with the correct setting.")
		return
	}
}
