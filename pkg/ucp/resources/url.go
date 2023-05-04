// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"errors"
	"fmt"
	"strings"
)

func ExtractPlanesPrefixFromURLPath(path string) (string, string, string, error) {
	// Remove the /planes/foo/bar/ prefix from the URL with the minimum amount of
	// garbage allocated during parsing.
	splitCount := 5
	if !strings.HasPrefix(path, SegmentSeparator) {
		splitCount--
	}

	minimumSegmentCount := splitCount - 1

	segments := strings.SplitN(path, SegmentSeparator, splitCount)
	if len(segments) < minimumSegmentCount {
		return "", "", "", errors.New("URL path is not a valid UCP path")
	}

	// If we had a leading / then the first segment will be empty
	if segments[0] == "" {
		segments = segments[1:]
		minimumSegmentCount--
	}

	if !strings.EqualFold(PlanesSegment, segments[0]) {
		return "", "", "", fmt.Errorf("URL paths should contain %s as the first segment", PlanesSegment)
	}

	if segments[1] == "" || segments[2] == "" {
		return "", "", "", errors.New("URL paths should not contain empty segments")
	}

	remainder := "/"
	if len(segments) > minimumSegmentCount {
		remainder = SegmentSeparator + segments[3]
	}

	return segments[1], segments[2], remainder, nil
}

// Extract Region from  a URI like /apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/817312594854/regions/us-west-2/providers/...
func ExtractRegionFromURLPath(path string) (string, error) {
	splitCount := 12
	segments := strings.SplitN(path, SegmentSeparator, splitCount)
	if len(segments) < splitCount {
		return "", errors.New("URL path is not a valid UCP path for retrieving AWS region")
	}
	return segments[10], nil

}
