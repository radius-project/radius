/*
Copyright 2024 The Radius Authors.

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

package filesystem

import (
	"io/fs"
)

// FileSystem is an interface that defines the methods needed to interact with a file system.
type FileSystem interface {
	Create(name string) (fs.File, error)
	Exists(name string) bool
	Open(name string) (fs.File, error)
	ReadFile(name string) ([]byte, error)
	Stat(name string) (fs.FileInfo, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	MkdirTemp(dir string, pattern string) (string, error)
	RemoveAll(path string) error
}
