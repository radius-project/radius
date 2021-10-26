// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armexpr

const (
	KindFunction = ExpressionKind("function")
	KindString   = ExpressionKind("string")
	KindProperty = ExpressionKind("property")
)

type ExpressionKind string

type SyntaxNode interface {
	GetSpan() Span
}

type ExpressionNode interface {
	SyntaxNode
	GetKind() ExpressionKind
	Accept(Visitor) error
}

type Span struct {
	Start  int
	Length int
}

type SyntaxTree struct {
	Span       Span
	Expression ExpressionNode
}

func (node *SyntaxTree) GetSpan() Span {
	return node.Span
}

type IdentifierNode struct {
	Span Span
	Text string
}

func (node *IdentifierNode) GetSpan() Span {
	return node.Span
}

type StringLiteralNode struct {
	Span Span
	Text string
}

func (node *StringLiteralNode) GetSpan() Span {
	return node.Span
}

func (node *StringLiteralNode) GetKind() ExpressionKind {
	return KindString
}

func (node *StringLiteralNode) Accept(visitor Visitor) error {
	return visitor.VisitStringLiteral(node)
}

type FunctionCallNode struct {
	Span       Span
	Identifier IdentifierNode
	Args       []ExpressionNode
}

func (node *FunctionCallNode) GetSpan() Span {
	return node.Span
}

func (node *FunctionCallNode) GetKind() ExpressionKind {
	return KindFunction
}

func (node *FunctionCallNode) Accept(visitor Visitor) error {
	return visitor.VisitFunctionCall(node)
}

type PropertyAccessNode struct {
	Span       Span
	Base       ExpressionNode
	Identifier IdentifierNode
	String     ExpressionNode
}

func (node *PropertyAccessNode) GetSpan() Span {
	return node.Span
}

func (node *PropertyAccessNode) GetKind() ExpressionKind {
	return KindProperty
}

func (node *PropertyAccessNode) Accept(visitor Visitor) error {
	return visitor.VisitPropertyAccess(node)
}

var _ ExpressionNode = &StringLiteralNode{}
var _ ExpressionNode = &PropertyAccessNode{}
var _ ExpressionNode = &FunctionCallNode{}
