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

package kubeenv

import (
	"errors"
	"sync"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// Environment adds native process cleanup to envtest. Its lifecycle methods must
// not be called concurrently.
type Environment struct {
	*envtest.Environment
	stop func() error
}

// NewEnvironment wraps an envtest configuration without starting the control plane.
func NewEnvironment(environment *envtest.Environment) *Environment {
	return &Environment{Environment: environment}
}

// Start starts the control plane and cleans up even when initialization fails.
func (e *Environment) Start() (*rest.Config, error) {
	if e.stop != nil {
		return nil, errors.New("test environment has already been started")
	}
	cfg, stop, err := startEnvironment(e.Environment)
	e.stop = sync.OnceValue(stop)
	if err != nil {
		return nil, errors.Join(err, e.Stop())
	}
	return cfg, nil
}

// Stop stops only this environment's processes and releases its test resources.
// Repeated calls return the original cleanup result.
func (e *Environment) Stop() error {
	if e.stop == nil {
		return nil
	}
	return e.stop()
}
