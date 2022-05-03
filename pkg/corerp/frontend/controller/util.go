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
	// ErrETagsDoNotMatch represents the error of the eTag of the resource and the requested etag not matching.
	ErrETagsDoNotMatch = errors.New("etags do not match")
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

// ValidateETag receives an ARMRequestContect and gathers the values in the If-Match and/or
// If-None-Match headers and then checks to see if the etag of the resource matches what is requested.
func ValidateETag(armRequestContext servicecontext.ARMRequestContext, etag string) error {
	ifMatchETag := armRequestContext.IfMatch
	ifMatchCheck := checkIfMatchHeader(ifMatchETag, etag)
	if ifMatchCheck != nil {
		return ifMatchCheck
	}

	ifNoneMatchETag := armRequestContext.IfNoneMatch
	ifNoneMatchCheck := checkIfNoneMatchHeader(ifNoneMatchETag, etag)
	if ifNoneMatchCheck != nil {
		return ifNoneMatchCheck
	}

	return nil
}

func checkIfMatchHeader(ifMatchETag string, etag string) error {
	if ifMatchETag == "" {
		return nil
	}

	if etag == "" {
		return ErrRequestedResourceDoesNotExist
	}

	if ifMatchETag != "*" && ifMatchETag != etag {
		return ErrETagsDoNotMatch
	}

	return nil
}

func checkIfNoneMatchHeader(ifNoneMatchETag string, etag string) error {
	if ifNoneMatchETag == "*" && etag != "" {
		return ErrResourceAlreadyExists
	}

	return nil
}
