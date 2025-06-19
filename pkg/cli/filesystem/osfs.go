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
	"os"
)

// OSFileSystem is an implementation of the FileSystem interface that uses the OS filesystem.
// It uses the methods from the os package to interact with the filesystem.
type OSFileSystem struct{}

var _ FileSystem = (*OSFileSystem)(nil)

func NewOSFS() *OSFileSystem {
	return &OSFileSystem{}
}

func (osfs OSFileSystem) Create(name string) (fs.File, error) {
	return os.Create(name)
}

func (osfs OSFileSystem) Exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

func (osfs OSFileSystem) Open(name string) (fs.File, error) {
	return os.Open(name)
}

func (osfs OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (osfs OSFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (osfs OSFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (osfs OSFileSystem) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

func (osfs OSFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osfs OSFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}
