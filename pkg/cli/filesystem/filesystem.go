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
}
