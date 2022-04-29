// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/mitchellh/mapstructure"
)

var (
	// ContentTypeHeaderKey is the header key of Content-Type
	ContentTypeHeaderKey = http.CanonicalHeaderKey("Content-Type")
)

var (
	// ErrUnsupportedContentType represents the error of unsupported content-type.
	ErrUnsupportedContentType = errors.New("unsupported Content-Type")
)

// ReadJSONBody extracts the content from request.
func ReadJSONBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()

	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get(ContentTypeHeaderKey)))
	if i := strings.Index(contentType, ";"); i > -1 {
		contentType = contentType[0:i]
	}

	if contentType != "application/json" {
		return nil, ErrUnsupportedContentType
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}
	return data, nil
}

// DecodeMap decodes map[string]interface{} structure to the type of out.
func DecodeMap(in interface{}, out interface{}) error {
	cfg := &mapstructure.DecoderConfig{
		TagName: "json",
		Result:  out,
		Squash:  true,
	}
	decoder, _ := mapstructure.NewDecoder(cfg)
	return decoder.Decode(in)
}
