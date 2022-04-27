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
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
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

func ValidateETag(armRequestContext servicecontext.ARMRequestContext, etag string) error {
	ifMatchEtag := armRequestContext.IfMatch
	ifMatchCheck := checkIfMatch(ifMatchEtag, etag)
	if ifMatchCheck != nil {
		return ifMatchCheck
	}

	ifNoneMatchEtag := armRequestContext.IfNoneMatch
	ifNoneMatchCheck := checkIfNoneMatch(ifNoneMatchEtag, etag)
	if ifNoneMatchCheck != nil {
		return ifNoneMatchCheck
	}

	return nil
}

func checkIfMatch(ifMatchEtag string, etag string) error {
	if ifMatchEtag == "" {
		return nil
	}

	if etag == "" {
		return errors.New("resource doesn't exist")
	}

	if ifMatchEtag != "*" && ifMatchEtag != etag {
		return errors.New("etags do not match")
	}

	return nil
}

func checkIfNoneMatch(ifNoneMatchEtag string, etag string) error {
	if ifNoneMatchEtag == "*" && etag != "" {
		return errors.New("resource already exists")
	}

	return nil
}
