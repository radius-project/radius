package filesystem

import (
	"io"
	"io/fs"
	"testing"
	"time"
)

func TestNewMemMapFileSystem(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	if mmfs == nil {
		t.Fatal("expected non-nil MemMapFileSystem")
	}
}

func TestMemMapFileSystem_Create(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	file, err := mmfs.Create("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if file == nil {
		t.Fatal("expected non-nil file")
	}
}

func TestMemMapFileSystem_Open(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	_, err := mmfs.Create("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	file, err := mmfs.Open("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if file == nil {
		t.Fatal("expected non-nil file")
	}
	file, err = mmfs.Open("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if file != nil {
		t.Fatal("expected nil file for nonexistent file")
	}
}

func TestMemMapFileSystem_Remove(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	_, err := mmfs.Create("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	err = mmfs.Remove("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	err = mmfs.Remove("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestMemMapFileSystem_WriteFile(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	err := mmfs.WriteFile("testfile", []byte("data"), fs.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestMemMapFileSystem_ReadFile(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	err := mmfs.WriteFile("testfile", []byte("data"), fs.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	data, err := mmfs.ReadFile("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("expected 'data', got %s", string(data))
	}
	_, err = mmfs.ReadFile("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestMemMapFileSystem_Stat(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	err := mmfs.WriteFile("testfile", []byte("data"), fs.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	info, err := mmfs.Stat("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.Name() != "testfile" {
		t.Fatalf("expected 'testfile', got %s", info.Name())
	}
	info, err = mmfs.Stat("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if info != nil {
		t.Fatal("expected nil info for nonexistent file")
	}
}

func TestMemMapFileSystem_Exists(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	err := mmfs.WriteFile("testfile", []byte("data"), fs.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !mmfs.Exists("testfile") {
		t.Fatal("expected file to exist")
	}
	if mmfs.Exists("nonexistent") {
		t.Fatal("expected file to not exist")
	}
}

func TestMemMapFileSystem_MkdirTemp(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	dir, err := mmfs.MkdirTemp("/tmp", "testdir")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if dir != "/tmp/testdir" {
		t.Fatalf("expected '/tmp/testdir', got %s", dir)
	}
	_, err = mmfs.MkdirTemp("/tmp", "testdir")
	if err == nil {
		t.Fatal("expected error for existing directory")
	}
}

func TestMemMapFileSystem_MkdirAll(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	err := mmfs.MkdirAll("/tmp/testdir", fs.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !mmfs.Exists("/tmp/testdir") {
		t.Fatal("expected directory to exist")
	}
}

func TestMemMapFileSystem_RemoveAll(t *testing.T) {
	mmfs := NewMemMapFileSystem()
	err := mmfs.MkdirAll("/tmp/testdir", fs.ModePerm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	err = mmfs.RemoveAll("/tmp/testdir")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mmfs.Exists("/tmp/testdir") {
		t.Fatal("expected directory to not exist")
	}
	err = mmfs.RemoveAll("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestMemFile_Read(t *testing.T) {
	file := &MemFile{
		Data: []byte("data"),
	}
	buf := make([]byte, 4)
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 bytes read, got %d", n)
	}
	if string(buf) != "data" {
		t.Fatalf("expected 'data', got %s", string(buf))
	}
	n, err = file.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}
}

func TestMemFile_Close(t *testing.T) {
	file := &MemFile{}
	err := file.Close()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestMemFile_Stat(t *testing.T) {
	file := &MemFile{
		Name: "testfile",
		Data: []byte("data"),
		Mode: fs.ModePerm,
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.Name() != "testfile" {
		t.Fatalf("expected 'testfile', got %s", info.Name())
	}
	if info.Size() != 4 {
		t.Fatalf("expected size 4, got %d", info.Size())
	}
	if info.Mode() != fs.ModePerm {
		t.Fatalf("expected mode %v, got %v", fs.ModePerm, info.Mode())
	}
}

func TestMemFileInfo_ModTime(t *testing.T) {
	info := &MemFileInfo{}
	modTime := info.ModTime()
	if modTime.After(time.Now()) {
		t.Fatalf("expected modTime to be before or equal to current time, got %v", modTime)
	}
}

func TestMemFileInfo_IsDir(t *testing.T) {
	info := &MemFileInfo{mode: fs.ModeDir}
	if !info.IsDir() {
		t.Fatal("expected IsDir to return true for directory mode")
	}
	info = &MemFileInfo{mode: fs.ModePerm}
	if info.IsDir() {
		t.Fatal("expected IsDir to return false for non-directory mode")
	}
}

func TestMemFileInfo_Sys(t *testing.T) {
	info := &MemFileInfo{}
	if info.Sys() != nil {
		t.Fatal("expected Sys to return nil")
	}
}
