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
	"fmt"
	"io/fs"
	"math/rand"
	"testing/fstest"
)

// MemMapFileSystem is an implementation of the FileSystem interface that uses an in-memory map to store files.
// It uses the methods from the fstest package to interact with the in-memory map.
type MemMapFileSystem struct {
	InternalFileSystem fstest.MapFS
}

var _ FileSystem = (*MemMapFileSystem)(nil)

func NewMemMapFileSystem() *MemMapFileSystem {
	return &MemMapFileSystem{
		InternalFileSystem: fstest.MapFS{},
	}
}

func (mmfs MemMapFileSystem) Create(name string) (fs.File, error) {
	mmfs.InternalFileSystem[name] = &fstest.MapFile{}

	return mmfs.InternalFileSystem.Open(name)
}

func (mmfs MemMapFileSystem) Exists(name string) bool {
	_, ok := mmfs.InternalFileSystem[name]
	return ok
}

func (mmfs MemMapFileSystem) Open(name string) (fs.File, error) {
	return mmfs.InternalFileSystem.Open(name)
}

func (mmfs MemMapFileSystem) ReadFile(name string) ([]byte, error) {
	return mmfs.InternalFileSystem.ReadFile(name)
}

func (mmfs MemMapFileSystem) Stat(name string) (fs.FileInfo, error) {
	return mmfs.InternalFileSystem.Stat(name)
}

func (mmfs MemMapFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	mmfs.InternalFileSystem[name] = &fstest.MapFile{
		Data: data,
		Mode: perm,
	}

	return nil
}

func (mmfs MemMapFileSystem) MkdirTemp(dir, pattern string) (string, error) {
	tempDir := fmt.Sprintf("%s%s%d", dir, pattern, rand.Intn(100000))
	mmfs.InternalFileSystem[tempDir] = &fstest.MapFile{
		Mode: fs.ModeDir,
	}

	return tempDir, nil
}

func (mmfs MemMapFileSystem) RemoveAll(path string) error {
	delete(mmfs.InternalFileSystem, path)

	return nil
}
