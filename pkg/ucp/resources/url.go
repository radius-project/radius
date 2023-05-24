/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
