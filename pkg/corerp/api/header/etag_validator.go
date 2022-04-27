package header

import (
	"errors"

	"github.com/project-radius/radius/pkg/corerp/servicecontext"
)

func Validate(armRequestContext servicecontext.ARMRequestContext, etag string) error {
	ifMatchETag := armRequestContext.IfMatch
	if ifMatchETag != "" {
		// wildcard
		if ifMatchETag == "*" {
			// resource doesn't exist
			if etag == "" {
				return errors.New("resource doesn't exist")
			}
			// not wildcard
		} else {
			// resource exists but doesn't match
			if etag != "" && ifMatchETag != etag {
				return errors.New("resource tags do not match")
			} else if etag == "" {
				return errors.New("resource doesn't exist")
			}
		}
	}

	ifNoneMatchETag := armRequestContext.IfNoneMatch
	if ifNoneMatchETag != "" {
		if ifNoneMatchETag == "*" && etag != "" {
			return errors.New("resource already exists")
		}
	}

	return nil
}
