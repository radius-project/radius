/*
Copyright 2023 The Radius Authors.

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

package platformprovider

import "sync"

// Factory is a function that returns a new platform provider.
type Factory func() (Provider, error)

var (
	registryMu       sync.Mutex
	providerRegistry = make(map[string]Factory)
)

// Register registers a platformprovider.Factory by name.
func Register(name string, platform Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if platform == nil {
		panic("platform-provider: Register platform is nil")
	}

	if _, dup := providerRegistry[name]; dup {
		panic("platform-provider: Register called twice for platform " + name)
	}
	providerRegistry[name] = platform
}

// GetPlatform creates an instance of the named platform provider, or nil if the name is not registered.
func GetPlatform(name string) (Provider, error) {
	registryMu.Lock()
	defer registryMu.Unlock()

	platform, ok := providerRegistry[name]
	if !ok {
		return nil, nil
	}

	return platform()
}

// NewProvider creates an instance of the named platform provider, or nil if the name is not registered.
func NewProvider(name string) (Provider, error) {
	if name == "" {
		return nil, nil
	}

	return GetPlatform(name)
}
