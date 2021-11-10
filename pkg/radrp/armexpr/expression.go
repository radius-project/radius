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

type IntLiteralNode struct {
	Span  Span
	Value int
}

func (node *IntLiteralNode) GetSpan() Span {
	return node.Span
}

func (node *IntLiteralNode) GetKind() ExpressionKind {
	return KindString
}

func (node *IntLiteralNode) Accept(visitor Visitor) error {
	return visitor.VisitIntLiteral(node)
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

type IndexingNode struct {
	Span       Span
	Base       ExpressionNode
	Identifier IdentifierNode
	IndexExpr  ExpressionNode
}

func (node *IndexingNode) GetSpan() Span {
	return node.Span
}

func (node *IndexingNode) GetKind() ExpressionKind {
	return KindProperty
}

func (node *IndexingNode) Accept(visitor Visitor) error {
	return visitor.VisitIndexingNode(node)
}

var _ ExpressionNode = &StringLiteralNode{}
var _ ExpressionNode = &IndexingNode{}
var _ ExpressionNode = &FunctionCallNode{}
