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

package list

import (
	"strings"

	"github.com/radius-project/radius/pkg/cli/output"
)

// versionsFormat returns the formatter options for displaying Terraform versions list.
func versionsFormat() output.FormatterOptions {
	transformer := &versionTransformer{}
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

type versionTransformer struct{}

func (*versionTransformer) Transform(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "<no value>" || trimmed == "<nil>" || trimmed == "" {
		return "-"
	}
	if trimmed == "0001-01-01T00:00:00Z" || trimmed == "\"0001-01-01T00:00:00Z\"" {
		return "-"
	}
	// Strip surrounding quotes
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		return trimmed[1 : len(trimmed)-1]
	}
	return input
}

type currentTransformer struct{}

func (*currentTransformer) Transform(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "true" {
		return "*"
	}
	return ""
}
