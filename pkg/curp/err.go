// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package curp

import (
	"errors"
)

// ErrUnsupportedWorkload indicates an unsupported workload type.
var ErrUnsupportedWorkload = errors.New("unsupported workload type")