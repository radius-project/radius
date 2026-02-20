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

package status

import (
	"strings"

	"github.com/radius-project/radius/pkg/cli/output"
)

// statusFormat returns the formatter options for displaying Terraform installer status.
// Note: JSONPath uses Go struct field names (capitalized), not json tags.
// Shows essential columns only. Use --output json for full details.
func statusFormat() output.FormatterOptions {
	noValue := &emptyIfNoValueTransformer{}
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:     "STATE",
				JSONPath:    "{ .State }",
				Transformer: noValue,
			},
			{
				Heading:     "VERSION",
				JSONPath:    "{ .CurrentVersion }",
				Transformer: noValue,
			},
			{
				Heading:     "LAST ERROR",
				JSONPath:    "{ .LastError }",
				Transformer: noValue,
			},
			{
				Heading:     "LAST UPDATED",
				JSONPath:    "{ .LastUpdated }",
				Transformer: noValue,
			},
		},
	}
}

type emptyIfNoValueTransformer struct{}

func (*emptyIfNoValueTransformer) Transform(input string) string {
	trimmed := strings.TrimSpace(input)
	// Handle various "no value" representations from JSONPath
	if trimmed == "<no value>" || trimmed == "<nil>" || trimmed == "" {
		return "-"
	}
	// Handle zero time values
	if trimmed == "0001-01-01T00:00:00Z" || trimmed == "\"0001-01-01T00:00:00Z\"" {
		return "-"
	}
	// Strip surrounding quotes from values (e.g., timestamps)
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		return trimmed[1 : len(trimmed)-1]
	}
	return input
}

// versionsFormat returns the formatter options for displaying Terraform versions list.
func versionsFormat() output.FormatterOptions {
	transformer := &emptyIfNoValueTransformer{}
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:     "VERSION",
				JSONPath:    "{ .Version }",
				Transformer: transformer,
			},
			{
				Heading:     "STATE",
				JSONPath:    "{ .State }",
				Transformer: transformer,
			},
			{
				Heading:     "HEALTH",
				JSONPath:    "{ .Health }",
				Transformer: transformer,
			},
			{
				Heading:     "INSTALLED AT",
				JSONPath:    "{ .InstalledAt }",
				Transformer: transformer,
			},
			{
				Heading:     "CURRENT",
				JSONPath:    "{ .IsCurrent }",
				Transformer: &currentTransformer{},
			},
		},
	}
}

type currentTransformer struct{}

func (*currentTransformer) Transform(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "true" {
		return "*"
	}
	return ""
}
