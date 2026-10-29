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

package paramresolver

import (
	"testing"
)

var fuzzSeeds = []string{
	`context.resource.name`,
	`context.resource.properties.size == "S" ? "a" : context.resource.properties.size == "M" ? "b" : "c"`,
	`!(context.resource.properties.enabled && context.resource.properties.port != 5432) || null == null`,
	`context.resource.properties.missing != null ? context.resource.properties.missing : -1.5`,
	`"C:\data }}" == context.resource.properties.host`,
	`((((true))))`,
	`a ? : (`,
	`context.`,
	`"unterminated`,
}

// Fuzz_parseExpression verifies that the lexer and parser never panic and that any expression they
// accept can be evaluated without panicking.
func Fuzz_parseExpression(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}

	r := newContextResolver(testContext())
	f.Fuzz(func(t *testing.T, src string) {
		ast, err := parseExpression(src)
		if err != nil {
			return
		}
		_, _ = r.evaluate(ast)
	})
}

// Fuzz_ResolveParameterExpressions verifies that scanning and resolving arbitrary parameter strings
// never panics and that strict and best-effort resolution agree on success.
func Fuzz_ResolveParameterExpressions(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add("{{" + seed + "}}")
		f.Add("prefix-{{" + seed + "}}-{{ .Values.x }}")
	}
	f.Add("{{")
	f.Add("}}{{context.resource.name")

	f.Fuzz(func(t *testing.T, value string) {
		params := map[string]any{"p": value}
		strict, strictErr := ResolveParameterExpressions(params, testContext())
		lenient, lenientErr := ResolveParameterExpressionsBestEffort(params, testContext())

		if (strictErr == nil) != (lenientErr == nil) {
			t.Fatalf("strict error %v and best-effort error %v disagree for %q", strictErr, lenientErr, value)
		}
		if strictErr == nil && len(strict) != len(lenient) {
			t.Fatalf("strict result %v and best-effort result %v disagree for %q", strict, lenient, value)
		}
	})
}
