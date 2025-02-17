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
	"time"
)

// MemMapFileSystem is an implementation of the FileSystem interface that uses an in-memory map to store file data.
type MemMapFileSystem struct {
	InternalFileSystem map[string]MemFile
}

var _ FileSystem = (*MemMapFileSystem)(nil)

func NewMemMapFileSystem() *MemMapFileSystem {
	return &MemMapFileSystem{
		InternalFileSystem: make(map[string]MemFile),
	}
}

func (m *MemMapFileSystem) Create(name string) (fs.File, error) {
	file := MemFile{
		Data: []byte{},
		Mode: fs.ModePerm,
	}
	m.InternalFileSystem[name] = file
	return &file, nil
}

func (m *MemMapFileSystem) Open(name string) (fs.File, error) {
	file, exists := m.InternalFileSystem[name]
	if !exists {
		return nil, fmt.Errorf("file %s does not exist", name)
	}
	return &file, nil
}

func (m *MemMapFileSystem) Remove(name string) error {
	_, exists := m.InternalFileSystem[name]
	if !exists {
		return fmt.Errorf("file %s does not exist", name)
	}
	delete(m.InternalFileSystem, name)
	return nil
}

func (m *MemMapFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	m.InternalFileSystem[name] = MemFile{
		Data: data,
		Mode: perm,
	}
	return nil
}

func (m *MemMapFileSystem) ReadFile(name string) ([]byte, error) {
	file, exists := m.InternalFileSystem[name]
	if !exists {
		return nil, fmt.Errorf("file %s does not exist", name)
	}
	return file.Data, nil
}

func (m *MemMapFileSystem) Stat(name string) (fs.FileInfo, error) {
	file, exists := m.InternalFileSystem[name]
	if !exists {
		return nil, fmt.Errorf("file %s does not exist", name)
	}
	return &MemFileInfo{name: name, size: int64(len(file.Data)), mode: file.Mode}, nil
}

func (m *MemMapFileSystem) Exists(name string) bool {
	_, exists := m.InternalFileSystem[name]
	return exists
}

func (m *MemMapFileSystem) MkdirTemp(dir string, pattern string) (string, error) {
	tempDir := fmt.Sprintf("%s/%s", dir, pattern)
	if _, exists := m.InternalFileSystem[tempDir]; exists {
		return "", fmt.Errorf("directory %s already exists", tempDir)
	}
	m.InternalFileSystem[tempDir] = MemFile{
		Data: nil,
		Mode: fs.ModeDir | fs.ModePerm,
	}
	return tempDir, nil
}

func (m *MemMapFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	if _, exists := m.InternalFileSystem[path]; !exists {
		m.InternalFileSystem[path] = MemFile{
			Data: nil,
			Mode: fs.ModeDir | perm,
		}
	}

	return nil
}

func (m *MemMapFileSystem) RemoveAll(path string) error {
	if _, exists := m.InternalFileSystem[path]; !exists {
		return fmt.Errorf("directory %s does not exist", path)
	}
	delete(m.InternalFileSystem, path)
	return nil
}

type MemFileInfo struct {
	name string
	size int64
	mode fs.FileMode
}

type MemFile struct {
	Data []byte
	Mode fs.FileMode
}

func (f *MemFile) Close() error {
	return nil
}

func (f *MemFile) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (f *MemFile) Stat() (fs.FileInfo, error) {
	return nil, nil
}

func (f *MemFileInfo) Name() string       { return f.name }
func (f *MemFileInfo) Size() int64        { return f.size }
func (f *MemFileInfo) Mode() fs.FileMode  { return f.mode }
func (f *MemFileInfo) ModTime() time.Time { return time.Now() }
func (f *MemFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f *MemFileInfo) Sys() interface{}   { return nil }
