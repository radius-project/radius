// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import "github.com/project-radius/radius/pkg/armrpc/api/conv"

// OutputConverter is the function to convert data model to version agnostic model.
type OutputConverter[T conv.DataModelInterface] func(model *T, version string) (conv.VersionedModelInterface, error)
