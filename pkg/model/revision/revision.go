// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package revision

import (
	"bytes"
	"crypto/sha1"
	"fmt"

	"github.com/gibson042/canonicaljson-go"
)

// Revision represents a computed revision number for a resource.
type Revision string

// Marshaler defines a contract for custom marshaling of an object for computing revisions.
type Marshaler interface {
	Marshal() interface{}
}

// Marshal returns an object suitable for computing revisions.
func Marshal(obj interface{}) interface{} {
	if marshaler, ok := obj.(Marshaler); ok {
		obj = marshaler.Marshal()
	}
	return obj
}

// Compute computes the revision for an object.
func Compute(obj interface{}, parent Revision, related []Revision) (Revision, error) {
	obj = Marshal(obj)
	buf, err := canonicaljson.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to convert %T to bytes: %w", obj, err)
	}

	hash := sha1.New()
	_, err = hash.Write(buf)
	if err != nil {
		return "", fmt.Errorf("failed to hash %T: %w", obj, err)
	}

	_, err = hash.Write([]byte(parent))
	if err != nil {
		return "", fmt.Errorf("failed to hash %T: %w", obj, err)
	}

	for _, item := range related {
		_, err = hash.Write([]byte(item))
		if err != nil {
			return "", fmt.Errorf("failed to hash %T: %w", obj, err)
		}
	}

	result := hash.Sum(nil)
	return Revision(fmt.Sprintf("%x", result)), nil
}

// Equals computes the semantic equality of two values.
func Equals(x interface{}, y interface{}) (bool, error) {
	xbytes, err := canonicaljson.Marshal(Marshal(x))
	if err != nil {
		return false, fmt.Errorf("failed to convert %T to bytes: %w", x, err)
	}

	ybytes, err := canonicaljson.Marshal(Marshal(y))
	if err != nil {
		return false, fmt.Errorf("failed to convert %T to bytes: %w", y, err)
	}

	result := bytes.Equal(xbytes, ybytes)
	return result, nil
}
