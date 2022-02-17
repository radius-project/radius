// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"bytes"

	"github.com/marstr/randname"
)

// GenerateRandomName generates a string with the specified prefix and a random 5-character suffix.
func GenerateRandomName(prefix string, affixes ...string) string {
	b := bytes.NewBufferString(prefix)
	b.WriteRune('-')
	for _, affix := range affixes {
		b.WriteString(affix)
		b.WriteRune('-')
	}
	return randname.GenerateWithPrefix(b.String(), 5)
}
