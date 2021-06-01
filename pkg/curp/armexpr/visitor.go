// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armexpr

type Visitor interface {
	VisitFunctionCall(*FunctionCallNode) error
	VisitStringLiteral(*StringLiteralNode) error
	VisitPropertyAccess(*PropertyAccessNode) error
}
