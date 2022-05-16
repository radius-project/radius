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
	if strings.HasPrefix(path, UCPPrefix) {
		return "", "", "", fmt.Errorf("URL paths should not start with %s", UCPPrefix)
	}

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
