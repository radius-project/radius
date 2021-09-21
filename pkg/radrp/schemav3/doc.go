// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schemav3

/*
Package schemav3 contains the validation logic based on the JSON schema of the types defined in Radius Resource Provider's OpenAPI spec.

We also use the JSON Schema files in this package to generate our OpenAPI spec.

The JSON schema version used is Draft 4, and the OpenAPI version used is 2.0. There are incompatiblities between the two specifications, therefore we will want to constraint our object schema to be within the intersection of the two specs.

The main types are `Validator` and `ValidationError`. All surfaced validation errors are of the type ValidationError. We strive to avoid using the error types from our dependency to make it easier to swap out the JSON validation library in the future if we want to.
*/
