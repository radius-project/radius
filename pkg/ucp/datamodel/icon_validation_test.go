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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateIcon(t *testing.T) {
	tests := []struct {
		name    string
		icon    string
		wantErr string
	}{
		{
			name: "valid minimal svg",
			icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40" fill="red"/></svg>`,
		},
		{
			name: "valid svg with xml declaration",
			icon: `<?xml version="1.0" encoding="UTF-8"?><svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`,
		},
		{
			name: "valid svg with fragment href",
			icon: `<svg xmlns="http://www.w3.org/2000/svg"><use href="#glyph"/></svg>`,
		},
		{
			name: "valid svg with data uri href",
			icon: `<svg xmlns="http://www.w3.org/2000/svg"><image href="data:image/png;base64,iVBORw0KGgo="/></svg>`,
		},
		{
			name:    "empty",
			icon:    "",
			wantErr: "icon is empty",
		},
		{
			name:    "not xml",
			icon:    "not an svg",
			wantErr: "does not contain an <svg>",
		},
		{
			name:    "malformed xml",
			icon:    "<svg><rect>",
			wantErr: "well-formed XML",
		},
		{
			name:    "wrong root element",
			icon:    `<html><body/></html>`,
			wantErr: "root element is <html>",
		},
		{
			name:    "script element",
			icon:    `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
			wantErr: "<script>",
		},
		{
			name:    "onclick attribute",
			icon:    `<svg xmlns="http://www.w3.org/2000/svg"><rect onclick="alert(1)"/></svg>`,
			wantErr: "event-handler attribute",
		},
		{
			name:    "onload attribute on root",
			icon:    `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`,
			wantErr: "event-handler attribute",
		},
		{
			name:    "foreignObject element",
			icon:    `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><div/></foreignObject></svg>`,
			wantErr: "<foreignObject>",
		},
		{
			name:    "external href",
			icon:    `<svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.com/x.png"/></svg>`,
			wantErr: "references external resource",
		},
		{
			name:    "external xlink href",
			icon:    `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><image xlink:href="https://example.com/x.png"/></svg>`,
			wantErr: "references external resource",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateIcon([]byte(tc.icon))
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateIcon_SizeLimit(t *testing.T) {
	// Body is padded with comment content so the SVG remains well-formed while
	// pushing the total byte count past MaxIconSizeBytes.
	prefix := `<svg xmlns="http://www.w3.org/2000/svg"><!--`
	suffix := `--></svg>`
	padding := strings.Repeat("x", MaxIconSizeBytes-len(prefix)-len(suffix)+1)
	oversized := prefix + padding + suffix
	require.Greater(t, len(oversized), MaxIconSizeBytes)

	err := ValidateIcon([]byte(oversized))
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds")
}
