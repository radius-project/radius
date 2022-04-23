// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testing

import "io/ioutil"

func ReadFixture(filename string) []byte {
	raw, err := ioutil.ReadFile("./testdata/" + filename)
	if err != nil {
		return nil
	}
	return raw
}
