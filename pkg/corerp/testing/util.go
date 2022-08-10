// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testing

import "os"

func ReadFixture(filename string) []byte {
	raw, err := os.ReadFile("./testdata/" + filename)
	if err != nil {
		return nil
	}
	return raw
}
