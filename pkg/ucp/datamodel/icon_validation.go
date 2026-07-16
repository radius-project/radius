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

// MaxIconSizeBytes is the maximum accepted size of a resource type icon: 32 KiB.
const MaxIconSizeBytes = 32 * 1024

// ValidateIcon enforces the resource-type icon contract on both the CLI and
// the server. The rules are:
//
//   - non-empty
//   - size <= MaxIconSizeBytes
//   - well-formed XML
//   - root element is <svg>
//   - no <script> elements
//   - no <style> elements (CSS can carry @import, url(), and legacy expression())
//   - no on* event-handler attributes
//   - no style attributes (same CSS-based exfiltration surface as <style>)
//   - no <foreignObject> elements
//   - href / xlink:href values are either data: URLs or intra-document fragments (starting with '#')
//   - fill / stroke / filter / mask / clip-path values may reference paint
//     servers only via intra-document fragments (url(#foo)); external
//     url(...) targets are rejected
//
// The <style>/style= rejection is intentionally strict: authors that need
// styling should inline it via presentation attributes (fill, stroke, etc.)
// on the shapes themselves. This mirrors what a lightweight sanitizer would
// strip and keeps the surface small enough to reason about without a full
// CSS parser.
//
// The paint-server rule closes the same beacon/tracker/SSRF surface as the
// href rule for the SVG "paint server" family of attributes: gradients,
// patterns, filters, masks, and clip paths must all be defined inside the
// same <svg>. An icon that passes validation is therefore a closed
// document — rendering it never contacts the network. See
// docs/architecture/application-graph.md ("Client-side rendering and
// sanitization boundary") for the threat model.
//
// Bytes that pass validation are stored verbatim.
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
		if strings.EqualFold(local, "style") {
			return fmt.Errorf("icon contains a <style> element, which is not allowed")
		}
		if strings.EqualFold(local, "foreignObject") {
			return fmt.Errorf("icon contains a <foreignObject> element, which is not allowed")
		}

		for _, attr := range start.Attr {
			name := attr.Name.Local
			if strings.HasPrefix(strings.ToLower(name), "on") {
				return fmt.Errorf("icon <%s> element has event-handler attribute %q, which is not allowed", local, name)
			}
			if strings.EqualFold(name, "style") {
				return fmt.Errorf("icon <%s> element has style attribute, which is not allowed", local)
			}
			if strings.EqualFold(name, "href") {
				if err := validateHrefValue(local, attr.Value); err != nil {
					return err
				}
			}
			if isPaintServerAttr(name) {
				if err := validatePaintServerValue(local, name, attr.Value); err != nil {
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

// isPaintServerAttr reports whether the given SVG attribute name accepts a
// paint-server url(...) reference. SVG defines this narrow family — fill,
// stroke, filter, mask, and clip-path — as the set of presentation
// attributes whose value can point at another element (a <linearGradient>,
// <pattern>, <filter>, <mask>, or <clipPath>) via url(...). All other
// SVG attributes are either colors, path data, dimensions, or opaque
// literals that do not carry URLs, so scanning them for url(...) would
// produce false positives.
func isPaintServerAttr(name string) bool {
	switch strings.ToLower(name) {
	case "fill", "stroke", "filter", "mask", "clip-path":
		return true
	}
	return false
}

// validatePaintServerValue rejects url(...) values on paint-server
// attributes that reference anything other than an intra-document fragment
// (#foo). Non-url values (colors, "none", "currentColor", CSS basic shapes
// like inset()/circle()/polygon() on clip-path) pass through unchanged.
//
// The rule mirrors validateHrefValue for the paint-server family: an icon
// that passes validation is a closed document whose gradients, patterns,
// filters, masks, and clip paths are all defined inside the same <svg>.
// A rendering client therefore never triggers a network fetch on the icon
// author's behalf, even when it inlines the bytes into the DOM outside
// the icon endpoint's CSP.
//
// The scan is deliberately conservative: we walk every url(...) occurrence
// in the value (fill supports a "url(#g) red" fallback form, filter
// supports space-separated filter lists) and require each target to start
// with '#'. Malformed url(...) values with no closing paren are rejected
// rather than accepted-by-omission.
func validatePaintServerValue(elementName, attrName, value string) error {
	remaining := value
	for {
		lower := strings.ToLower(remaining)
		i := strings.Index(lower, "url(")
		if i < 0 {
			return nil
		}
		after := remaining[i+len("url("):]
		j := strings.Index(after, ")")
		if j < 0 {
			return fmt.Errorf("icon <%s> element attribute %q has malformed url(...) value %q", elementName, attrName, value)
		}
		target := strings.TrimSpace(after[:j])
		if len(target) >= 2 {
			first, last := target[0], target[len(target)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				target = target[1 : len(target)-1]
			}
		}
		if !strings.HasPrefix(target, "#") {
			return fmt.Errorf("icon <%s> element attribute %q references external resource %q via url(); only intra-document fragments (url(#...)) are allowed", elementName, attrName, value)
		}
		remaining = after[j+1:]
	}
}
