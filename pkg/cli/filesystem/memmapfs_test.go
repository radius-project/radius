package filesystem

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewMemMapFileSystem(t *testing.T) {
	fs := NewMemMapFileSystem()
	require.NotNil(t, fs)
}

func TestMemMapFileSystem_Create(t *testing.T) {
	fs := NewMemMapFileSystem()
	fileName := "testfile"

	file, err := fs.Create(fileName)
	require.NoError(t, err)
	require.NotNil(t, file)
	require.True(t, fs.Exists(fileName))
}

func TestMemMapFileSystem_Exists(t *testing.T) {
	fs := NewMemMapFileSystem()
	fileName := "testfile"

	require.False(t, fs.Exists(fileName))

	_, _ = fs.Create(fileName)

	require.True(t, fs.Exists(fileName))
}

func TestMemMapFileSystem_Open(t *testing.T) {
	fs := NewMemMapFileSystem()
	fileName := "testfile"

	_, _ = fs.Create(fileName)

	file, err := fs.Open(fileName)
	require.NoError(t, err)
	require.NotNil(t, file)
}

func TestMemMapFileSystem_ReadFile(t *testing.T) {
	fs := NewMemMapFileSystem()
	fileName := "testfile"
	data := []byte("hello world")

	err := fs.WriteFile(fileName, data, os.ModePerm)
	require.NoError(t, err)

	readData, err := fs.ReadFile(fileName)
	require.NoError(t, err)
	require.Equal(t, data, readData)
}

func TestMemMapFileSystem_Stat(t *testing.T) {
	fs := NewMemMapFileSystem()
	fileName := "testfile"

	_, _ = fs.Create(fileName)

	info, err := fs.Stat(fileName)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, fileName, info.Name())
}

func TestMemMapFileSystem_WriteFile(t *testing.T) {
	fs := NewMemMapFileSystem()
	fileName := "testfile"
	data := []byte("hello world")

	err := fs.WriteFile(fileName, data, os.ModePerm)
	require.NoError(t, err)

	readData, err := fs.ReadFile(fileName)
	require.NoError(t, err)
	require.Equal(t, data, readData)
}
