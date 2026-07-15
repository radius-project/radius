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

package datamodel

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// MaxIconSizeBytes is the maximum accepted size of a resource type icon per
// NFR-002 of spec 003 (Resource Type Icons): 32 KiB.
const MaxIconSizeBytes = 32 * 1024

// ValidateIcon enforces the resource-type icon contract from FR-005 (CLI-side)
// and FR-005a (server-side) of spec 003. The rules are:
//
//   - non-empty
//   - size <= MaxIconSizeBytes
//   - well-formed XML
//   - root element is <svg>
//   - no <script> elements
//   - no on* event-handler attributes
//   - no <foreignObject> elements
//   - href / xlink:href values are either data: URLs or intra-document fragments (starting with '#')
//
// Bytes that pass validation are stored verbatim — the caller must not
// re-encode or normalize them, so that FR-010's SHA-256 iconHash content-addresses
// exactly what the author published.
func ValidateIcon(icon []byte) error {
	if len(icon) == 0 {
		return fmt.Errorf("icon is empty")
	}
	if len(icon) > MaxIconSizeBytes {
		return fmt.Errorf("icon is %d bytes, which exceeds the %d byte limit", len(icon), MaxIconSizeBytes)
	}

	decoder := xml.NewDecoder(bytes.NewReader(icon))

	sawRoot := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("icon is not well-formed XML: %w", err)
		}

		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		local := start.Name.Local
		if !sawRoot {
			if !strings.EqualFold(local, "svg") {
				return fmt.Errorf("icon root element is <%s>, expected <svg>", local)
			}
			sawRoot = true
		}

		if strings.EqualFold(local, "script") {
			return fmt.Errorf("icon contains a <script> element, which is not allowed")
		}
		if strings.EqualFold(local, "foreignObject") {
			return fmt.Errorf("icon contains a <foreignObject> element, which is not allowed")
		}

		for _, attr := range start.Attr {
			name := attr.Name.Local
			if strings.HasPrefix(strings.ToLower(name), "on") {
				return fmt.Errorf("icon <%s> element has event-handler attribute %q, which is not allowed", local, name)
			}
			if strings.EqualFold(name, "href") {
				if err := validateHrefValue(local, attr.Value); err != nil {
					return err
				}
			}
		}
	}

	if !sawRoot {
		return fmt.Errorf("icon does not contain an <svg> root element")
	}
	return nil
}

// validateHrefValue rejects href / xlink:href values that would let an SVG
// pull an external resource. Fragment references (#foo) and data: URLs are
// the only accepted forms.
func validateHrefValue(elementName, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "#") {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return nil
	}
	return fmt.Errorf("icon <%s> element references external resource %q via href; only data: URLs and intra-document fragments are allowed", elementName, value)
}
