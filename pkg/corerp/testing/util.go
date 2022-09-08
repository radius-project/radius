// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testing

import (
	"os"

	"github.com/project-radius/radius/pkg/armrpc/rest"
)

func ReadFixture(filename string) []byte {
	raw, err := os.ReadFile("./testdata/" + filename)
	if err != nil {
		return nil
	}
	return raw
}

func ResponseFromError(res rest.Response, err error) (rest.Response, error) {
	if res, ok := err.(rest.Response); ok {
		return res, nil
	}

	return res, err
}
