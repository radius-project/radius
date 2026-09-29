package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type archiveFile struct {
	header  tar.Header
	content string
}

func makeArchive(t *testing.T, files []archiveFile) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, file := range files {
		file.header.Size = int64(len(file.content))
		require.NoError(t, writer.WriteHeader(&file.header))
		_, err := io.WriteString(writer, file.content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func TestSelected(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		path string
		want bool
	}{
		{"ucpd", true},
		{"manifest", true},
		{"manifest/provider/config.yaml", true},
		{"ucpd-backup", false},
		{"manifest-old/config.yaml", false},
		{"other/manifest/config.yaml", false},
		{"etc/os-release", false},
	} {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, selected(test.path, []string{"ucpd", "manifest"}))
		})
	}
}

func TestCollect(t *testing.T) {
	t.Parallel()
	archive := makeArchive(t, []archiveFile{
		{header: tar.Header{Name: "./ucpd", Typeflag: tar.TypeReg, Mode: 0o755, Uid: 65532, Gid: 65532}, content: "server"},
		{header: tar.Header{Name: "./manifest/", Typeflag: tar.TypeDir, Mode: 0o755}},
		{header: tar.Header{Name: "manifest/config", Typeflag: tar.TypeReg, Mode: 0o644}, content: "config"},
		{header: tar.Header{Name: "manifest/link", Typeflag: tar.TypeSymlink, Mode: 0o777, Linkname: "config"}},
		{header: tar.Header{Name: "manifest/hardlink", Typeflag: tar.TypeLink, Mode: 0o644, Linkname: "manifest/config"}},
		{header: tar.Header{Name: "manifest/pipe", Typeflag: tar.TypeFifo, Mode: 0o600}},
		{header: tar.Header{Name: "etc/os-release", Typeflag: tar.TypeReg}, content: "excluded"},
	})
	entries, binaryHash, err := collect(bytes.NewReader(archive), "ucpd", []string{"ucpd", "manifest"})
	require.NoError(t, err)
	serverHash := fmt.Sprintf("%x", sha256.Sum256([]byte("server")))
	configHash := fmt.Sprintf("%x", sha256.Sum256([]byte("config")))
	require.Equal(t, serverHash, binaryHash)
	require.Equal(t, []entry{
		{Path: "ucpd", Type: "file", Mode: 0o755, UID: 65532, GID: 65532, Size: 6, SHA256: serverHash},
		{Path: "manifest", Type: "directory", Mode: 0o755},
		{Path: "manifest/config", Type: "file", Mode: 0o644, Size: 6, SHA256: configHash},
		{Path: "manifest/link", Type: "symlink", Mode: 0o777, Target: "config"},
		{Path: "manifest/hardlink", Type: "hardlink", Mode: 0o644, Target: "manifest/config"},
		{Path: "manifest/pipe", Type: "other", Mode: 0o600},
	}, entries)
}

func TestRunDeterministicOutput(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	archivePath := filepath.Join(directory, "image.tar")
	manifestPath := filepath.Join(directory, "manifest.jsonl")
	hashPath := filepath.Join(directory, "binary.sha256")
	files := []archiveFile{
		{header: tar.Header{Name: "ucpd", Typeflag: tar.TypeReg, Mode: 0o755}, content: "server"},
		{header: tar.Header{Name: "manifest/config", Typeflag: tar.TypeReg, Mode: 0o644}, content: "config"},
	}
	var firstManifest []byte
	for attempt := range 2 {
		files[0], files[1] = files[1], files[0]
		for index := range files {
			files[index].header.ModTime = time.Unix(int64(attempt+1), 0)
		}
		require.NoError(t, os.WriteFile(archivePath, makeArchive(t, files), 0o600))
		require.NoError(t, run(archivePath, "ucpd", manifestPath, hashPath, []string{"ucpd", "manifest"}))
		manifest, err := os.ReadFile(manifestPath)
		require.NoError(t, err)
		if attempt == 0 {
			firstManifest = manifest
		} else {
			require.Equal(t, firstManifest, manifest)
		}
		hash, err := os.ReadFile(hashPath)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%x\n", sha256.Sum256([]byte("server"))), string(hash))
	}
	decoder := json.NewDecoder(bytes.NewReader(firstManifest))
	var item entry
	require.NoError(t, decoder.Decode(&item))
	require.Equal(t, "manifest/config", item.Path)
	require.NoError(t, decoder.Decode(&item))
	require.Equal(t, "ucpd", item.Path)
	require.ErrorIs(t, decoder.Decode(&item), io.EOF)
}

func TestRunRejectsInvalidArchives(t *testing.T) {
	t.Parallel()
	valid := makeArchive(t, []archiveFile{
		{header: tar.Header{Name: "ucpd", Typeflag: tar.TypeReg}, content: "server"},
	})
	for _, test := range []struct {
		name    string
		archive []byte
		message string
	}{
		{"missing binary", makeArchive(t, nil), "/ucpd is missing from the image"},
		{"truncated header", []byte("bad tar"), "unexpected EOF"},
		{"truncated content", valid[:512+3], "cannot read ucpd"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			archivePath := filepath.Join(directory, "image.tar")
			manifestPath := filepath.Join(directory, "manifest.jsonl")
			hashPath := filepath.Join(directory, "binary.sha256")
			require.NoError(t, os.WriteFile(archivePath, test.archive, 0o600))
			require.ErrorContains(t, run(archivePath, "ucpd", manifestPath, hashPath, []string{"ucpd"}), test.message)
			require.NoFileExists(t, manifestPath)
			require.NoFileExists(t, hashPath)
		})
	}
}

func TestRunFileErrors(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"archive", "manifest", "hash"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			archivePath := filepath.Join(directory, "image.tar")
			manifestPath := filepath.Join(directory, "manifest.jsonl")
			hashPath := filepath.Join(directory, "binary.sha256")
			require.NoError(t, os.WriteFile(archivePath, makeArchive(t, []archiveFile{
				{header: tar.Header{Name: "ucpd", Typeflag: tar.TypeReg}, content: "server"},
			}), 0o600))
			switch stage {
			case "archive":
				archivePath = filepath.Join(directory, "missing.tar")
			case "manifest":
				manifestPath = directory
			case "hash":
				hashPath = directory
			}
			err := run(archivePath, "ucpd", manifestPath, hashPath, []string{"ucpd"})
			var pathError *os.PathError
			require.ErrorAs(t, err, &pathError)
		})
	}
}
