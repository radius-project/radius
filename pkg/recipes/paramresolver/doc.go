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

/*
Package paramresolver resolves {{context.*}} expressions in the parameters of direct-module
recipes (Bicep or Terraform modules that do not declare a recipe context parameter).

# Expressions

An expression is the text between "{{" and "}}". Only expressions that reference a context path
outside a double-quoted or backtick-quoted string are evaluated; any other {{...}} text, such as a
Helm or Go template, is passed through unchanged. A reference counts unless "context." is part of a
longer name, that is, preceded by a letter, digit, "_", ".", or "$". So `{{ .Values.context.x }}`
and `{{ $context.x }}` pass through, while a malformed expression such as `{{[context.x]}}` is an
error.

The expression language is closed and side-effect free: there are no function calls, arithmetic,
indexing, or identifiers other than context paths and the keywords below. From lowest to highest
precedence:

	expr     := ternary
	ternary  := or ( "?" ternary ":" ternary )?        right-associative
	or       := and ( "||" and )*
	and      := equality ( "&&" equality )*
	equality := unary ( ( "==" | "!=" ) unary )*
	unary    := "!" unary | primary
	primary  := STRING | NUMBER | "true" | "false" | "null" | PATH | "(" expr ")"

	PATH     := "context" ( "." SEGMENT )+             SEGMENT is one or more characters other
	                                                    than whitespace, ".", '"', "`", brackets,
	                                                    and !=&|?:<>+*%,
	STRING   := text between double quotes, taken verbatim; there are no escape sequences
	NUMBER   := optional "-", digits, optional "." digits
	            integers must be between -2^63 and 2^64-1; decimals are 64-bit floats

For example, to map a developer-facing size enum onto a module SKU and a typed storage size:

	skuName:       '{{context.resource.properties.size == "S" ? "Standard_B1ms" : context.resource.properties.size == "M" ? "Standard_D2ds_v5" : "Standard_D4ds_v5"}}'
	storageSizeGB: '{{context.resource.properties.size == "L" ? 512 : 128}}'

# Context paths

The supported paths are context.resource.{name,id,type}, context.application.{name,id},
context.environment.{name,id}, context.runtime.kubernetes.{namespace,environmentNamespace},
context.azure.resourceGroup.{name,id}, context.azure.subscription.{subscriptionId,id},
context.azure.resourceNameHash, context.aws.{region,account},
context.resource.properties.<path>, and
context.resource.connections.<name>.{id,name,type,properties.<path>}, where <path> is a property
name optionally followed by nested object keys, such as properties.config.tier. A top-level property
whose name is exactly <path>, such as one named "service.name", takes precedence over nested keys.

Resource and connection properties that are not set, including nested keys under a missing or null
object, evaluate to null, so optional properties can be tested with "== null", "!= null", or as a
condition. Reading a nested key from a value that is not an object, such as a string or an array, is
an error. Any other path that is not available, such as an Azure path when no Azure provider is
configured, is an error.

# Semantics

Evaluation is lazy: only the selected ternary arm is evaluated, and && and || short-circuit. The
whole expression is always parsed first, so syntax errors are reported in every arm.

Numbers compare numerically, booleans compare as booleans, and null equals only null. Other scalar
combinations compare by their string form, so `context.resource.properties.port == "5432"` is true
when port is the number 5432. Objects and arrays cannot be compared.

In a condition and as operands of &&, ||, and !, null, false, "", 0, and empty objects or arrays are
false; every other value is true. The logical operators produce booleans.

# Results

When a parameter value is exactly one expression (surrounding whitespace aside), the typed result is
used, so a conditional can produce a number, boolean, object, or array. A string result keeps the
surrounding whitespace, except for a bare resource or connection property path, which is replaced by
the property value alone. When an expression is embedded in other text, its result is formatted as
a string.

An expression that is malformed, references an unknown path, or evaluates to null fails resolution
with an error that names the parameter and the expression.
*/
package paramresolver
