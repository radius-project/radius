// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armexpr

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Parse_InvalidSyntaxTree(t *testing.T) {
	inputs := []string{
		"[[[reference()]",
		"[[reference()]]",
		"reference()]",
		"reference()",
		"[reference()]foo",
		"",
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse SyntaxTree: %s", i), func(t *testing.T) {
			parsed, err := Parse(i)
			require.Error(t, err, "parsing should not have succeeded: %+v", parsed)
		})
	}
}

func Test_Parse_SyntaxTree_Valid(t *testing.T) {
	inputs := []input{
		{
			// "integration test" - includes all features
			Text: "[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'app', 'backend')).bindings.web]",
			Expected: &SyntaxTree{
				Span: Span{
					Start:  0,
					Length: 135,
				},
				Expression: &PropertyAccessNode{
					Span: Span{
						Start:  1,
						Length: 133,
					},
					Identifier: IdentifierNode{
						Span: Span{
							Start:  131,
							Length: 3,
						},
						Text: "web",
					},
					Base: &PropertyAccessNode{
						Span: Span{
							Start:  1,
							Length: 129,
						},
						Identifier: IdentifierNode{
							Span: Span{
								Start:  122,
								Length: 8,
							},
							Text: "bindings",
						},
						Base: &FunctionCallNode{
							Span: Span{
								Start:  1,
								Length: 120,
							},
							Identifier: IdentifierNode{
								Span: Span{
									Start:  1,
									Length: 9,
								},
								Text: "reference",
							},
							Args: []ExpressionNode{
								&FunctionCallNode{
									Span: Span{
										Start:  11,
										Length: 109,
									},
									Identifier: IdentifierNode{
										Span: Span{
											Start:  11,
											Length: 10,
										},
										Text: "resourceId",
									},
									Args: []ExpressionNode{
										&StringLiteralNode{
											Span: Span{
												Start:  22,
												Length: 69,
											},
											Text: "'Microsoft.CustomProviders/resourceProviders/Applications/Components'",
										},
										&StringLiteralNode{
											Span: Span{
												Start:  93,
												Length: 8,
											},
											Text: "'radius'",
										},
										&StringLiteralNode{
											Span: Span{
												Start:  103,
												Length: 5,
											},
											Text: "'app'",
										},
										&StringLiteralNode{
											Span: Span{
												Start:  110,
												Length: 9,
											},
											Text: "'backend'",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			// Empty string
			Text: "['']",
			Expected: &SyntaxTree{
				Span: Span{
					Start:  0,
					Length: 4,
				},
				Expression: &StringLiteralNode{
					Span: Span{
						Start:  1,
						Length: 2,
					},
					Text: "''",
				},
			},
		},
		{
			// Function call
			Text: "[reference()]",
			Expected: &SyntaxTree{
				Span: Span{
					Start:  0,
					Length: 13,
				},
				Expression: &FunctionCallNode{
					Span: Span{
						Start:  1,
						Length: 11,
					},
					Identifier: IdentifierNode{
						Span: Span{
							Start:  1,
							Length: 9,
						},
						Text: "reference",
					},
					Args: []ExpressionNode{},
				},
			},
		},
		{
			// [[ form
			Text: "[['']",
			Expected: &SyntaxTree{
				Span: Span{
					Start:  0,
					Length: 5,
				},
				Expression: &StringLiteralNode{
					Span: Span{
						Start:  2,
						Length: 2,
					},
					Text: "''",
				},
			},
		},
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse SyntaxTree: %s", i.Text), func(t *testing.T) {
			parsed, err := Parse(i.Text)
			require.NoErrorf(t, err, "failed to parse")

			require.Equal(t, i.Expected, parsed)
		})
	}
}

func Test_Parse_PropertyAccess_Invalid(t *testing.T) {
	inputs := []string{
		"foo",
		".",
		"reference()]",
		"reference()",
		"[reference()]foo",
		"",
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse PropertyAccess: %s", i), func(t *testing.T) {
			base := FunctionCallNode{
				Span: Span{
					Start:  3,
					Length: 10,
				},
			}
			tokenizer := tokenizer{
				Text: i,
			}

			parsed, err := ParsePropertyAccess(&tokenizer, &base)
			require.Error(t, err, "parsing should not have succeeded: %+v", parsed)
		})
	}
}

func Test_Parse_FunctionCall_Valid(t *testing.T) {
	identifier := IdentifierNode{
		Span: Span{
			Start:  0,
			Length: 4,
		},
		Text: "test",
	}

	inputs := []input{
		{
			Text: "test ()",
			Expected: &FunctionCallNode{
				Span: Span{
					Start:  0,
					Length: 7,
				},
				Identifier: identifier,
				Args:       []ExpressionNode{},
			},
		},
		{
			Text: "test ( )",
			Expected: &FunctionCallNode{
				Span: Span{
					Start:  0,
					Length: 8,
				},
				Identifier: identifier,
				Args:       []ExpressionNode{},
			},
		},
		{
			Text: "test ('foo')",
			Expected: &FunctionCallNode{
				Span: Span{
					Start:  0,
					Length: 12,
				},
				Identifier: identifier,
				Args: []ExpressionNode{
					&StringLiteralNode{
						Span: Span{
							Start:  6,
							Length: 5,
						},
						Text: "'foo'",
					},
				},
			},
		},
		{
			Text: "test ('foo','bar')",
			Expected: &FunctionCallNode{
				Span: Span{
					Start:  0,
					Length: 18,
				},
				Identifier: identifier,
				Args: []ExpressionNode{
					&StringLiteralNode{
						Span: Span{
							Start:  6,
							Length: 5,
						},
						Text: "'foo'",
					},
					&StringLiteralNode{
						Span: Span{
							Start:  12,
							Length: 5,
						},
						Text: "'bar'",
					},
				},
			},
		},
		{
			Text: "test ('foo' ,   'bar')",
			Expected: &FunctionCallNode{
				Span: Span{
					Start:  0,
					Length: 22,
				},
				Identifier: identifier,
				Args: []ExpressionNode{
					&StringLiteralNode{
						Span: Span{
							Start:  6,
							Length: 5,
						},
						Text: "'foo'",
					},
					&StringLiteralNode{
						Span: Span{
							Start:  16,
							Length: 5,
						},
						Text: "'bar'",
					},
				},
			},
		},
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse FunctionCall: %s", i), func(t *testing.T) {
			tokenizer := tokenizer{
				Text:    i.Text,
				Current: 5, // Set cursor after 'test '
			}

			parsed, err := ParseFunctionCall(&tokenizer, identifier)
			require.NoErrorf(t, err, "failed to parse")

			require.Equal(t, i.Expected, parsed)
		})
	}
}

func Test_Parse_FunctionCall_Invalid(t *testing.T) {
	inputs := []string{
		"test (",
		"test )",
		"test (()",
		"test ('foo'",
		"test ('foo',",
		"test ('foo' ,'bar'",
		"test ('foo' ,'bar',)",
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse FunctionCall: %s", i), func(t *testing.T) {
			identifier := IdentifierNode{
				Span: Span{
					Start:  0,
					Length: 4,
				},
				Text: "test",
			}
			tokenizer := tokenizer{
				Text:    i,
				Current: 5, // Set cursor after 'test '
			}

			parsed, err := ParseFunctionCall(&tokenizer, identifier)
			require.Error(t, err, "parsing should not have succeeded: %+v", parsed)
		})
	}
}

func Test_Parse_PropertyAccess_Valid(t *testing.T) {
	base := FunctionCallNode{
		Span: Span{
			Start:  0,
			Length: 6,
		},
		Identifier: IdentifierNode{
			Span: Span{
				Start:  0,
				Length: 4,
			},
			Text: "test",
		},
		Args: []ExpressionNode{},
	}

	inputs := []input{
		{
			Text: "test() .foo",
			Expected: &PropertyAccessNode{
				Span: Span{
					Start:  0,
					Length: 11,
				},
				Identifier: IdentifierNode{
					Span: Span{
						Start:  8,
						Length: 3,
					},
					Text: "foo",
				},
				Base: &base,
			},
		},
		{
			Text: "test() .  foo  ",
			Expected: &PropertyAccessNode{
				Span: Span{
					Start:  0,
					Length: 13,
				},
				Identifier: IdentifierNode{
					Span: Span{
						Start:  10,
						Length: 3,
					},
					Text: "foo",
				},
				Base: &base,
			},
		},
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse PropertyAccess: %s", i), func(t *testing.T) {
			tokenizer := tokenizer{
				Text:    i.Text,
				Current: 7, // Set cursor after 'test() '
			}

			parsed, err := ParsePropertyAccess(&tokenizer, &base)
			require.NoErrorf(t, err, "failed to parse")

			require.Equal(t, i.Expected, parsed)
		})
	}
}

func Test_Parse_Identifier_Invalid(t *testing.T) {
	inputs := []string{
		"&foo",
		"3bar",
		"    ",
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse Identifier: %s", i), func(t *testing.T) {
			tokenizer := tokenizer{
				Text: i,
			}

			parsed, err := ParseIdentifier(&tokenizer)
			require.Error(t, err, "parsing should not have succeeded: %+v", parsed)
		})
	}
}

func Test_Parse_Identifier_Valid(t *testing.T) {
	inputs := []input{
		{
			Text: "foo",
			Expected: &IdentifierNode{
				Span: Span{
					Start:  0,
					Length: 3,
				},
				Text: "foo",
			},
		},
		{
			Text: "  foo  ",
			Expected: &IdentifierNode{
				Span: Span{
					Start:  2,
					Length: 3,
				},
				Text: "foo",
			},
		},
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse Identifier: %s", i), func(t *testing.T) {
			tokenizer := tokenizer{
				Text: i.Text,
			}

			parsed, err := ParseIdentifier(&tokenizer)
			require.NoErrorf(t, err, "failed to parse")

			require.Equal(t, i.Expected, parsed)
		})
	}
}

func Test_Parse_StringLiteral_Valid(t *testing.T) {
	inputs := []input{
		{
			Text: `'foo'`,
			Expected: &StringLiteralNode{
				Span: Span{
					Start:  0,
					Length: 5,
				},
				Text: `'foo'`,
			},
		},
		{
			Text: `''`,
			Expected: &StringLiteralNode{
				Span: Span{
					Start:  0,
					Length: 2,
				},
				Text: `''`,
			},
		},
		{
			Text: `''`,
			Expected: &StringLiteralNode{
				Span: Span{
					Start:  0,
					Length: 2,
				},
				Text: `''`,
			},
		},
		{
			Text: `'\''`,
			Expected: &StringLiteralNode{
				Span: Span{
					Start:  0,
					Length: 4,
				},
				Text: `'\''`,
			},
		},
		{
			Text: `'\\'`,
			Expected: &StringLiteralNode{
				Span: Span{
					Start:  0,
					Length: 4,
				},
				Text: `'\\'`,
			},
		},
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse StringLiteral: %s", i), func(t *testing.T) {
			tokenizer := tokenizer{
				Text: i.Text,
			}

			parsed, err := ParseString(&tokenizer)
			require.NoErrorf(t, err, "failed to parse")

			require.Equal(t, i.Expected, parsed)
		})
	}
}

func Test_Parse_StringLiteral_Invalid(t *testing.T) {
	inputs := []string{
		`foo'`,
		`'foo`,
		`'`,
		`'\'`,
		`'\`,
		`'\\\'`,
	}

	for _, i := range inputs {
		t.Run(fmt.Sprintf("Parse StringLiteral: %s", i), func(t *testing.T) {
			tokenizer := tokenizer{
				Text: i,
			}

			parsed, err := ParseString(&tokenizer)
			require.Error(t, err, "parsing should not have succeeded: %+v", parsed)
		})
	}
}

type input struct {
	Text     string
	Expected SyntaxNode
}
