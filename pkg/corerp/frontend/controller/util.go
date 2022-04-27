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
	// ErrRequestedResourceDoesNotExist represents the error of resource that is requested not existing.
	ErrRequestedResourceDoesNotExist = errors.New("requested resource does not exist")
	// ErrEtagsDoNotMatch represents the error of the etag of the resource and the requested etag not matching.
	ErrEtagsDoNotMatch = errors.New("etags do not match")
	// ErrResourceAlreadyExists represents the error of the resource being already existent at the moment.
	ErrResourceAlreadyExists = errors.New("resource already exists")
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

// ValidateEtag receives an ARMRequestContect and gathers the values in the If-Match and/or
// If-None-Match headers and then checks to see if the etag of the resource matches what is requested.
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
		return ErrRequestedResourceDoesNotExist
	}

	if ifMatchEtag != "*" && ifMatchEtag != etag {
		return ErrEtagsDoNotMatch
	}

	return nil
}

func checkIfNoneMatch(ifNoneMatchEtag string, etag string) error {
	if ifNoneMatchEtag == "*" && etag != "" {
		return ErrResourceAlreadyExists
	}

	return nil
}
