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
)

func TestMemMapFileSystem_Create(t *testing.T) {
	fs := NewMemMapFileSystem()
	file, err := fs.Create("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if file == nil {
		t.Fatalf("expected file, got nil")
	}
}

func TestMemMapFileSystem_Open(t *testing.T) {
	fs := NewMemMapFileSystem()
	_, err := fs.Create("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	file, err := fs.Open("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if file == nil {
		t.Fatalf("expected file, got nil")
	}
}

func TestMemMapFileSystem_Remove(t *testing.T) {
	fs := NewMemMapFileSystem()
	_, err := fs.Create("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	err = fs.Remove("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_, err = fs.Open("testfile")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestMemMapFileSystem_WriteFile(t *testing.T) {
	fs := NewMemMapFileSystem()
	data := []byte("hello world")
	err := fs.WriteFile("testfile", data, os.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	fileData, err := fs.ReadFile("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(fileData) != string(data) {
		t.Fatalf("expected %s, got %s", data, fileData)
	}
}

func TestMemMapFileSystem_ReadFile(t *testing.T) {
	fs := NewMemMapFileSystem()
	data := []byte("hello world")
	err := fs.WriteFile("testfile", data, os.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	fileData, err := fs.ReadFile("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(fileData) != string(data) {
		t.Fatalf("expected %s, got %s", data, fileData)
	}
}

func TestMemMapFileSystem_Stat(t *testing.T) {
	fs := NewMemMapFileSystem()
	data := []byte("hello world")
	err := fs.WriteFile("testfile", data, os.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	info, err := fs.Stat("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.Name() != "testfile" {
		t.Fatalf("expected testfile, got %s", info.Name())
	}
	if info.Size() != int64(len(data)) {
		t.Fatalf("expected %d, got %d", len(data), info.Size())
	}
}

func TestMemMapFileSystem_Exists(t *testing.T) {
	fs := NewMemMapFileSystem()
	if fs.Exists("testfile") {
		t.Fatalf("expected file to not exist")
	}
	_, err := fs.Create("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !fs.Exists("testfile") {
		t.Fatalf("expected file to exist")
	}
}
