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

package hashutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Hex(t *testing.T) {
	// NIST SHA-256 test vectors.
	require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Hex([]byte("")))
	require.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", Hex([]byte("abc")))
}

func Test_LegacyHex(t *testing.T) {
	// NIST SHA-1 test vectors. These must remain stable so that values written by
	// older versions of Radius continue to be recognized during migration.
	require.Equal(t, "da39a3ee5e6b4b0d3255bfef95601890afd80709", LegacyHex([]byte("")))
	require.Equal(t, "a9993e364706816aba3e25717850c26c9cd0d89d", LegacyHex([]byte("abc")))
}
