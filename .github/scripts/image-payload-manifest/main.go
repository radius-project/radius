/*
Copyright 2026 The Radius Authors.

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

// Command image-payload-manifest summarizes the Radius-owned payload inside an
// exported container image.
//
// "docker export" writes the flattened root filesystem of a container as a tar
// archive. This command reads that archive and emits a stable JSONL manifest of
// the files Radius itself places into the image, so that a production image and
// a GoReleaser-built shadow image can be compared byte for byte.
//
// Only paths under the requested payload prefixes are reported. The rest of the
// root filesystem comes from the shared base image and its package installs,
// which the two builds resolve independently, so comparing it would report
// package drift rather than a GoReleaser packaging difference.
//
// Modification times are deliberately excluded. COPY preserves the timestamp of
// the build context source, and the production build copies from a staging
// directory while the shadow build copies from the checkout, so timestamps
// differ for byte-identical content.
package main

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// entry describes a single payload file. Fields are ordered alphabetically so
// the emitted JSON keys are sorted.
type entry struct {
	GID    int    `json:"gid"`
	Mode   uint32 `json:"mode"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Target string `json:"target,omitempty"`
	Type   string `json:"type"`
	UID    int    `json:"uid"`
}

// payloadPaths collects the repeatable -payload flag values.
type payloadPaths []string

func (p *payloadPaths) String() string {
	return strings.Join(*p, ",")
}

func (p *payloadPaths) Set(value string) error {
	if value == "" {
		return errors.New("payload prefix must not be empty")
	}
	*p = append(*p, value)
	return nil
}

func main() {
	var prefixes payloadPaths
	archive := flag.String("archive", "", "exported container image tar archive")
	binary := flag.String("binary", "", "component binary name at the image root")
	manifest := flag.String("manifest", "", "JSONL manifest output path")
	hash := flag.String("hash", "", "component binary SHA-256 output path")
	flag.Var(&prefixes, "payload", "payload path prefix, repeatable")
	flag.Parse()

	if *archive == "" || *binary == "" || *manifest == "" || *hash == "" ||
		len(prefixes) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*archive, *binary, *manifest, *hash, prefixes); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(archive, binary, manifest, hash string, prefixes []string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()

	entries, binaryHash, err := collect(file, binary, prefixes)
	if err != nil {
		return err
	}
	if binaryHash == "" {
		return fmt.Errorf("/%s is missing from the image", binary)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	if err := writeManifest(manifest, entries); err != nil {
		return err
	}
	return os.WriteFile(hash, []byte(binaryHash+"\n"), 0o644)
}

func collect(reader io.Reader, binary string, prefixes []string) ([]entry, string, error) {
	archive := tar.NewReader(reader)
	entries := make([]entry, 0)
	binaryHash := ""

	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", err
		}

		path := strings.TrimSuffix(strings.TrimPrefix(header.Name, "./"), "/")
		if path == "" || !selected(path, prefixes) {
			continue
		}

		item, err := describe(archive, header, path)
		if err != nil {
			return nil, "", err
		}
		entries = append(entries, item)
		if path == binary && item.Type == "file" {
			binaryHash = item.SHA256
		}
	}

	return entries, binaryHash, nil
}

// selected reports whether an archive path belongs to the requested payload.
func selected(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func describe(archive io.Reader, header *tar.Header, path string) (entry, error) {
	item := entry{
		GID:  header.Gid,
		Mode: uint32(header.FileInfo().Mode().Perm()),
		Path: path,
		UID:  header.Uid,
	}

	switch header.Typeflag {
	case tar.TypeReg:
		digest := sha256.New()
		if _, err := io.Copy(digest, archive); err != nil {
			return entry{}, fmt.Errorf("cannot read %s: %w", header.Name, err)
		}
		item.Type = "file"
		item.SHA256 = hex.EncodeToString(digest.Sum(nil))
		item.Size = header.Size
	case tar.TypeDir:
		item.Type = "directory"
	case tar.TypeSymlink:
		item.Type = "symlink"
		item.Target = header.Linkname
	case tar.TypeLink:
		item.Type = "hardlink"
		item.Target = header.Linkname
	default:
		item.Type = "other"
	}

	return item, nil
}

func writeManifest(path string, entries []entry) (err error) {
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
	}()

	encoder := json.NewEncoder(output)
	for _, item := range entries {
		if err := encoder.Encode(item); err != nil {
			return err
		}
	}
	return nil
}
