/*
Copyright 2026 The Radius Authors.

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

package helm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_QuoteFlagValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  string
	}{
		{
			value: "contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes}",
			want:  `"contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes}"`,
		},
		{
			value: "envoy.service.annotations.example=it's here",
			want:  `"envoy.service.annotations.example=it's here"`,
		},
		{value: "key=$HOME", want: `'key=$HOME'`},
		{value: "key=`date`", want: "'key=`date`'"},
		{value: `key="quoted"`, want: `'key="quoted"'`},
		{value: "key=hello!", want: `'key=hello!'`},
		// bash keeps \, inside double quotes but turns \\ into \, so escaped --set values need single quotes.
		{value: `contour.extraArgs={--a=x\,y,--b=\\}`, want: `'contour.extraArgs={--a=x\,y,--b=\\}'`},
		{value: `key=it's $1`, want: `'key=it'\''s $1'`},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()

			quoted := QuoteFlagValue(tt.value)
			require.Equal(t, tt.want, quoted)
			require.Equal(t, tt.value, unquoteFlagValue(t, quoted), "a shell must read the quoted value back unchanged")
		})
	}
}

// unquoteFlagValue reads a value that QuoteFlagValue quoted the way bash does.
func unquoteFlagValue(t *testing.T, quoted string) string {
	t.Helper()

	if len(quoted) >= 2 && quoted[0] == '"' && quoted[len(quoted)-1] == '"' {
		inner := quoted[1 : len(quoted)-1]
		// QuoteFlagValue uses double quotes only for values in which bash expands nothing.
		require.NotContains(t, inner, "$")
		require.NotContains(t, inner, "`")
		require.NotContains(t, inner, `\`)
		require.NotContains(t, inner, "!")
		require.NotContains(t, inner, `"`)
		return inner
	}

	var value []byte
	quotedPart := false
	for i := 0; i < len(quoted); i++ {
		switch {
		case quoted[i] == '\'':
			quotedPart = !quotedPart
		case !quotedPart && quoted[i] == '\\' && i+1 < len(quoted):
			i++
			value = append(value, quoted[i])
		case !quotedPart:
			t.Fatalf("%q has an unquoted character that a shell could change", quoted)
		default:
			value = append(value, quoted[i])
		}
	}
	require.False(t, quotedPart, "%q has an unterminated quote", quoted)

	return string(value)
}
