/*
Copyright 2023 The Radius Authors.

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

package etag

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

func New(data []byte) string {
	hash := sha1.Sum(data)
	return fmt.Sprintf("%d-%x", int(len(hash)), hash)
}

func NewFromRevision(revision int64) string {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(revision))
	return hex.EncodeToString(b)
}

func ParseRevision(etag string) (int64, error) {
	b, err := hex.DecodeString(etag)
	if err != nil {
		return 0, errors.New("etag value is invalid")
	}

	return int64(binary.LittleEndian.Uint64(b)), nil
}
