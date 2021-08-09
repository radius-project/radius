// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

/*
Package schema contains the validation logic based on the JSON schema
of the types defined in Radius Resource Provider's OpenAPI spec.

The JSON schema version used is Draft 4, and the OpenAPI version used
is 2.0. There are incompatiblities between the two specifications,
therefore we will want to constraint our object schema to be within
the intersection of the two specs.

Since our OpenAPI spec should be top-level documents, we want to keep
them in the "/schemas" directory of the repository. However, we _also_
want to embed the files into our Go package using go:embed, which does
not support reference to parent directory or symbolic links.  As a
result, we keep the real file in this package, and create a symlink in
the /schemas directory.

The main types are `Validator` and `ValidationError`. All surfaced
validation errors are of the type ValidationError. We strive to avoid
using the error types from our dependency to make it easier to swap
out the JSON validation library in the future if we want to.
*/
