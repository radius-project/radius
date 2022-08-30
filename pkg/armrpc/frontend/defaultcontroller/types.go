// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import "github.com/project-radius/radius/pkg/armrpc/api/conv"

// ToVersionedModel is the function to convert data model to version agnostic model.
type ToVersionedModel[T conv.DataModelInterface] func(model *T, version string) (conv.VersionedModelInterface, error)
