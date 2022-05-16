// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
