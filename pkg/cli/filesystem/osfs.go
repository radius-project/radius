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
	return err != nil
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
