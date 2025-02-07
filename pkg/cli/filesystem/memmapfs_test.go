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

func TestMemMapFileSystem_MkdirTemp(t *testing.T) {
	fs := NewMemMapFileSystem()
	dir := "testdir"
	pattern := "testpattern"

	tempDir, err := fs.MkdirTemp(dir, pattern)
	require.NoError(t, err)
	require.NotNil(t, tempDir)
	require.True(t, fs.Exists(tempDir))
}

func TestMemMapFileSystem_RemoveAll(t *testing.T) {
	fs := NewMemMapFileSystem()
	dir := "testdir"
	fileName := "testfile"

	_, _ = fs.Create("testdir/testfile")

	err := fs.RemoveAll(dir)
	require.NoError(t, err)
	require.False(t, fs.Exists(fileName))
}
