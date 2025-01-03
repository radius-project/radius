package filesystem

import (
	"io/fs"
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
